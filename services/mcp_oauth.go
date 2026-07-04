package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/db/dbq"
)

const (
	MCPReadScope  = "mcp:read"
	MCPWriteScope = "mcp:write"
)

type MCPOAuth interface {
	ServerMetadata() map[string]any
	ProtectedResourceMetadata() map[string]any
	RegisterClient(ctx context.Context, req MCPClientRegistrationRequest) (MCPClientRegistrationResponse, error)
	PrepareAuthorization(ctx context.Context, req MCPAuthorizationRequest) (MCPPreparedAuthorization, error)
	ConsentDetails(ctx context.Context, requestToken string) (MCPConsentDetails, error)
	ConfirmConsent(ctx context.Context, requestToken string, adminSession AdminSession, req MCPConsentRequest) (string, error)
	ExchangeCode(ctx context.Context, req MCPTokenRequest) (MCPTokenResponse, error)
}

type MCPClientRegistrationRequest struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	ClientURI    string   `json:"client_uri,omitempty"`
	LogoURI      string   `json:"logo_uri,omitempty"`
}

type MCPClientRegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type MCPAuthorizationRequest struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	State               string
	Resource            string
	Scope               string
}

type MCPPreparedAuthorization struct {
	RequestToken string
	ClientName   string
}

type MCPConsentDetails struct {
	ClientName string           `json:"client_name"`
	Projects   []ProjectSummary `json:"projects"`
}

type MCPConsentRequest struct {
	AllProjects bool     `json:"all_projects"`
	ProjectKeys []string `json:"project_keys"`
}

type MCPTokenRequest struct {
	GrantType    string
	ClientID     string
	Code         string
	RedirectURI  string
	CodeVerifier string
	Resource     string
}

type MCPTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
}

type mcpOAuthService struct {
	db      db.Pool
	queries *dbq.Queries
	admin   Admin
	baseURL string
	secret  []byte
}

type MCPOAuthOptions struct {
	DB            db.Pool
	Admin         Admin
	BaseURL       string
	SessionSecret string
}

func NewMCPOAuthService(opts MCPOAuthOptions) MCPOAuth {
	return &mcpOAuthService{
		db:      opts.DB,
		queries: dbq.New(opts.DB),
		admin:   opts.Admin,
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		secret:  []byte(opts.SessionSecret),
	}
}

func (s *mcpOAuthService) ServerMetadata() map[string]any {
	return map[string]any{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/api/mcp/oauth/authorize",
		"token_endpoint":                        s.baseURL + "/api/mcp/oauth/token",
		"registration_endpoint":                 s.baseURL + "/api/mcp/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{MCPReadScope, MCPWriteScope},
	}
}

func (s *mcpOAuthService) ProtectedResourceMetadata() map[string]any {
	return map[string]any{
		"resource":              s.baseURL + "/mcp",
		"authorization_servers": []string{s.baseURL},
		"scopes_supported":      []string{MCPReadScope, MCPWriteScope},
		"bearer_methods_supported": []string{
			"header",
		},
	}
}

func (s *mcpOAuthService) RegisterClient(ctx context.Context, req MCPClientRegistrationRequest) (MCPClientRegistrationResponse, error) {
	name := strings.TrimSpace(req.ClientName)
	if name == "" {
		name = "MCP Agent"
	}
	if len(req.RedirectURIs) == 0 {
		return MCPClientRegistrationResponse{}, fmt.Errorf("%w: redirect_uris is required", ErrBadRequest)
	}
	redirectURIs := normalizeRedirectURIs(req.RedirectURIs)
	if len(redirectURIs) == 0 {
		return MCPClientRegistrationResponse{}, fmt.Errorf("%w: redirect_uris must contain a valid URI", ErrBadRequest)
	}
	random, err := randomURLToken()
	if err != nil {
		return MCPClientRegistrationResponse{}, err
	}
	clientID := "fr_mcp_client_" + random
	row, err := s.queries.MCPUpsertOAuthClient(ctx, dbq.MCPUpsertOAuthClientParams{
		ClientID:     clientID,
		ClientName:   name,
		RedirectUris: redirectURIs,
		ClientUri:    strings.TrimSpace(req.ClientURI),
		LogoUri:      strings.TrimSpace(req.LogoURI),
	})
	if err != nil {
		return MCPClientRegistrationResponse{}, err
	}
	return clientRegistrationResponse(row.ClientID, row.ClientName, row.RedirectUris), nil
}

func (s *mcpOAuthService) PrepareAuthorization(ctx context.Context, req MCPAuthorizationRequest) (MCPPreparedAuthorization, error) {
	_ = s.queries.MCPCleanupExpiredOAuthState(ctx)
	_ = s.queries.MCPCleanupExpiredOAuthCodes(ctx)
	if req.ResponseType != "code" {
		return MCPPreparedAuthorization{}, fmt.Errorf("%w: unsupported response_type", ErrBadRequest)
	}
	if strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.RedirectURI) == "" {
		return MCPPreparedAuthorization{}, fmt.Errorf("%w: client_id and redirect_uri are required", ErrBadRequest)
	}
	if strings.TrimSpace(req.CodeChallenge) == "" || req.CodeChallengeMethod != "S256" {
		return MCPPreparedAuthorization{}, fmt.Errorf("%w: S256 PKCE is required", ErrBadRequest)
	}
	if normalizeResource(req.Resource, s.baseURL) != s.baseURL+"/mcp" {
		return MCPPreparedAuthorization{}, fmt.Errorf("%w: invalid resource", ErrBadRequest)
	}
	client, err := s.queries.MCPGetOAuthClient(ctx, req.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			client, err = s.fetchAndStoreClientMetadata(ctx, req.ClientID)
			if err != nil {
				return MCPPreparedAuthorization{}, err
			}
		} else {
			return MCPPreparedAuthorization{}, err
		}
	}
	if !stringInSlice(req.RedirectURI, client.RedirectUris) {
		return MCPPreparedAuthorization{}, fmt.Errorf("%w: redirect_uri is not registered for this client", ErrBadRequest)
	}
	scopes, err := normalizeScopes(req.Scope)
	if err != nil {
		return MCPPreparedAuthorization{}, err
	}
	pending := pendingMCPAuthorization{
		ClientID:            client.ClientID,
		ClientName:          client.ClientName,
		RedirectURI:         req.RedirectURI,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		State:               req.State,
		Resource:            s.baseURL + "/mcp",
		Scopes:              scopes,
		ExpiresAt:           time.Now().UTC().Add(10 * time.Minute),
	}
	token, err := s.signPending(pending)
	if err != nil {
		return MCPPreparedAuthorization{}, err
	}
	return MCPPreparedAuthorization{RequestToken: token, ClientName: client.ClientName}, nil
}

func (s *mcpOAuthService) ConsentDetails(ctx context.Context, requestToken string) (MCPConsentDetails, error) {
	pending, err := s.validatePending(requestToken)
	if err != nil {
		return MCPConsentDetails{}, err
	}
	projects, err := s.admin.ListProjects(ctx)
	if err != nil {
		return MCPConsentDetails{}, err
	}
	return MCPConsentDetails{ClientName: pending.ClientName, Projects: projects}, nil
}

func (s *mcpOAuthService) ConfirmConsent(ctx context.Context, requestToken string, adminSession AdminSession, req MCPConsentRequest) (string, error) {
	pending, err := s.validatePending(requestToken)
	if err != nil {
		return "", err
	}
	if !req.AllProjects && len(req.ProjectKeys) == 0 {
		return "", fmt.Errorf("%w: select at least one project", ErrBadRequest)
	}
	adminUser, err := s.queries.AdminGetUserByEmail(ctx, adminSession.Email)
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().UTC().Add(90 * 24 * time.Hour)
	randomCode, err := randomURLToken()
	if err != nil {
		return "", err
	}
	code := "fr_mcp_code_" + randomCode
	codeHash := HashToken(code)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	authRow, err := qtx.MCPCreateAgentAuthorization(ctx, dbq.MCPCreateAgentAuthorizationParams{
		ClientID:             pending.ClientID,
		ClientName:           pending.ClientName,
		CreatedByAdminUserID: uuidParam(adminUser.ID),
		AllProjects:          req.AllProjects,
		Scopes:               pending.Scopes,
		ExpiresAt:            expiresAt,
	})
	if err != nil {
		return "", err
	}
	if !req.AllProjects {
		seen := map[string]bool{}
		projectCount := 0
		for _, projectKey := range req.ProjectKeys {
			projectKey = strings.TrimSpace(projectKey)
			if projectKey == "" || seen[projectKey] {
				continue
			}
			seen[projectKey] = true
			project, err := qtx.GetProjectByKey(ctx, projectKey)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", fmt.Errorf("%w: unknown project_key %q", ErrBadRequest, projectKey)
				}
				return "", err
			}
			if err := qtx.MCPCreateAgentAuthorizationProject(ctx, dbq.MCPCreateAgentAuthorizationProjectParams{
				AgentAuthorizationID: authRow.ID,
				ProjectID:            project.ID,
			}); err != nil {
				return "", err
			}
			projectCount++
		}
		if projectCount == 0 {
			return "", fmt.Errorf("%w: select at least one project", ErrBadRequest)
		}
	}
	if err := qtx.MCPCreateOAuthCode(ctx, dbq.MCPCreateOAuthCodeParams{
		CodeHash:             codeHash,
		ClientID:             pending.ClientID,
		RedirectUri:          pending.RedirectURI,
		CodeChallenge:        pending.CodeChallenge,
		CodeChallengeMethod:  pending.CodeChallengeMethod,
		Resource:             pending.Resource,
		Scopes:               pending.Scopes,
		AdminUserID:          adminUser.ID,
		AgentAuthorizationID: authRow.ID,
		ExpiresAt:            time.Now().UTC().Add(5 * time.Minute),
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	redirectURL, err := url.Parse(pending.RedirectURI)
	if err != nil {
		return "", err
	}
	values := redirectURL.Query()
	values.Set("code", code)
	if pending.State != "" {
		values.Set("state", pending.State)
	}
	redirectURL.RawQuery = values.Encode()
	return redirectURL.String(), nil
}

func (s *mcpOAuthService) ExchangeCode(ctx context.Context, req MCPTokenRequest) (MCPTokenResponse, error) {
	_ = s.queries.MCPCleanupExpiredOAuthState(ctx)
	_ = s.queries.MCPCleanupExpiredOAuthCodes(ctx)
	if req.GrantType != "authorization_code" {
		return MCPTokenResponse{}, fmt.Errorf("%w: unsupported grant_type", ErrBadRequest)
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.RedirectURI) == "" {
		return MCPTokenResponse{}, fmt.Errorf("%w: code, client_id, and redirect_uri are required", ErrBadRequest)
	}
	resource := normalizeResource(req.Resource, s.baseURL)
	if resource != s.baseURL+"/mcp" {
		return MCPTokenResponse{}, fmt.Errorf("%w: invalid resource", ErrBadRequest)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MCPTokenResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	codeRow, err := qtx.MCPConsumeOAuthCode(ctx, dbq.MCPConsumeOAuthCodeParams{
		CodeHash:    HashToken(req.Code),
		ClientID:    req.ClientID,
		RedirectUri: req.RedirectURI,
		Resource:    resource,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MCPTokenResponse{}, ErrAgentAuthFailed
		}
		return MCPTokenResponse{}, err
	}
	if !verifyPKCES256(req.CodeVerifier, codeRow.CodeChallenge) {
		return MCPTokenResponse{}, ErrAgentAuthFailed
	}
	token, tokenHash, err := NewAgentToken()
	if err != nil {
		return MCPTokenResponse{}, err
	}
	if _, err := qtx.MCPActivateAgentAuthorization(ctx, dbq.MCPActivateAgentAuthorizationParams{
		ID:        codeRow.AgentAuthorizationID,
		TokenHash: textParam(tokenHash),
	}); err != nil {
		return MCPTokenResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MCPTokenResponse{}, err
	}
	return MCPTokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64((90 * 24 * time.Hour).Seconds()),
		Scope:       strings.Join(codeRow.Scopes, " "),
	}, nil
}

type pendingMCPAuthorization struct {
	ClientID            string    `json:"client_id"`
	ClientName          string    `json:"client_name"`
	RedirectURI         string    `json:"redirect_uri"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	State               string    `json:"state,omitempty"`
	Resource            string    `json:"resource"`
	Scopes              []string  `json:"scopes"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type mcpClientMetadataDocument struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	ClientURI    string   `json:"client_uri"`
	LogoURI      string   `json:"logo_uri"`
}

func (s *mcpOAuthService) fetchAndStoreClientMetadata(ctx context.Context, clientID string) (dbq.McpOauthClient, error) {
	metadataURL, err := url.Parse(clientID)
	if err != nil || metadataURL.Scheme != "https" || metadataURL.Host == "" {
		return dbq.McpOauthClient{}, fmt.Errorf("%w: unknown OAuth client", ErrBadRequest)
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: ssrfSafeTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return errors.New("metadata redirects must stay on https")
			}
			return nil
		},
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL.String(), nil)
	if err != nil {
		return dbq.McpOauthClient{}, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return dbq.McpOauthClient{}, fmt.Errorf("%w: could not fetch OAuth client metadata", ErrBadRequest)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return dbq.McpOauthClient{}, fmt.Errorf("%w: OAuth client metadata returned %d", ErrBadRequest, resp.StatusCode)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "application/json") {
		return dbq.McpOauthClient{}, fmt.Errorf("%w: OAuth client metadata must be JSON", ErrBadRequest)
	}
	var doc mcpClientMetadataDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&doc); err != nil {
		return dbq.McpOauthClient{}, fmt.Errorf("%w: could not decode OAuth client metadata", ErrBadRequest)
	}
	redirectURIs := normalizeRedirectURIs(doc.RedirectURIs)
	if len(redirectURIs) == 0 {
		return dbq.McpOauthClient{}, fmt.Errorf("%w: OAuth client metadata has no valid redirect_uris", ErrBadRequest)
	}
	name := strings.TrimSpace(doc.ClientName)
	if name == "" {
		name = "MCP Agent"
	}
	return s.queries.MCPUpsertOAuthClient(ctx, dbq.MCPUpsertOAuthClientParams{
		ClientID:     clientID,
		ClientName:   name,
		RedirectUris: redirectURIs,
		ClientUri:    strings.TrimSpace(doc.ClientURI),
		LogoUri:      strings.TrimSpace(doc.LogoURI),
	})
}

func ssrfSafeTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, candidate := range ips {
			if forbiddenMetadataIP(candidate.IP) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		}
		return nil, fmt.Errorf("metadata host resolved only to private or local addresses")
	}
	return transport
}

func forbiddenMetadataIP(ip net.IP) bool {
	return ip == nil ||
		ip.IsUnspecified() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast()
}

func (s *mcpOAuthService) signPending(pending pendingMCPAuthorization) (string, error) {
	payload, err := json.Marshal(pending)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + sig, nil
}

func (s *mcpOAuthService) validatePending(token string) (pendingMCPAuthorization, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return pendingMCPAuthorization{}, ErrAgentAuthFailed
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, actual) {
		return pendingMCPAuthorization{}, ErrAgentAuthFailed
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return pendingMCPAuthorization{}, err
	}
	var pending pendingMCPAuthorization
	if err := json.Unmarshal(payload, &pending); err != nil {
		return pendingMCPAuthorization{}, err
	}
	if time.Now().UTC().After(pending.ExpiresAt) {
		return pendingMCPAuthorization{}, ErrAgentAuthFailed
	}
	return pending, nil
}

func normalizeRedirectURIs(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed, err := url.Parse(value); err == nil && validMCPRedirectURI(parsed) {
			out = append(out, value)
		}
	}
	return out
}

func validMCPRedirectURI(parsed *url.URL) bool {
	if parsed.Scheme == "https" && parsed.Host != "" {
		return true
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	return false
}

func normalizeScopes(scope string) ([]string, error) {
	if strings.TrimSpace(scope) == "" {
		return []string{MCPReadScope, MCPWriteScope}, nil
	}
	allowed := map[string]bool{MCPReadScope: true, MCPWriteScope: true}
	out := []string{}
	for _, item := range strings.Fields(scope) {
		if !allowed[item] {
			return nil, fmt.Errorf("%w: invalid_scope %q", ErrBadRequest, item)
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return []string{MCPReadScope, MCPWriteScope}, nil
	}
	return out, nil
}

func normalizeResource(resource string, baseURL string) string {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return strings.TrimRight(baseURL, "/") + "/mcp"
	}
	return strings.TrimRight(resource, "/")
}

func verifyPKCES256(verifier string, challenge string) bool {
	sum := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	return hmac.Equal([]byte(expected), []byte(challenge))
}

func clientRegistrationResponse(clientID string, clientName string, redirectURIs []string) MCPClientRegistrationResponse {
	return MCPClientRegistrationResponse{
		ClientID:                clientID,
		ClientName:              clientName,
		RedirectURIs:            append([]string(nil), redirectURIs...),
		GrantTypes:              []string{"authorization_code"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
	}
}

func randomURLToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func stringInSlice(value string, values []string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
