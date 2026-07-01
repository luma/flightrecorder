package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/db/dbq"
	"github.com/luma/flightrecorder/env"
)

var ErrAdminAuthFailed = errors.New("admin authentication failed")

const (
	AdminOAuthStatusAuthenticated = "authenticated"
	AdminOAuthStatusPendingInvite = "pending_invite"
	AdminOAuthStatusDisabled      = "disabled"
	AdminOAuthStatusDomainDenied  = "domain_denied"

	adminSessionProviderDev    = "dev"
	adminSessionProviderGoogle = "google"
)

// AdminSession is serialized as a custom HMAC-SHA256 signed token (not a JWT).
// Wire format: base64url(json_payload) + "." + base64url(hmac_sha256_over_encoded_payload).
type AdminSession struct {
	Email     string    `json:"email"`
	Name      string    `json:"name,omitempty"`
	Picture   string    `json:"picture,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type OAuthIdentity struct {
	Provider string
	Subject  string
	Email    string
	Name     string
	Picture  string
}

type AdminOAuthResult struct {
	Status       string
	Token        string
	PendingToken string
	Session      AdminSession
}

type adminStateToken struct {
	ReturnPath string    `json:"return_path,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type pendingAdminSession struct {
	OAuthIdentity
	ExpiresAt time.Time `json:"expires_at"`
}

type adminSessionContextKey struct{}

func ContextWithAdminSession(ctx context.Context, session AdminSession) context.Context {
	return context.WithValue(ctx, adminSessionContextKey{}, session)
}

func AdminSessionFromContext(ctx context.Context) (AdminSession, bool) {
	session, ok := ctx.Value(adminSessionContextKey{}).(AdminSession)
	return session, ok
}

type AdminAuth interface {
	IssueDevSession(ctx context.Context, email string) (string, AdminSession, error)
	IssueOAuthSession(ctx context.Context, identity OAuthIdentity) (AdminOAuthResult, error)
	AcceptInvitation(ctx context.Context, pendingToken string, inviteToken string) (string, AdminSession, error)
	ValidateSession(ctx context.Context, token string) (AdminSession, error)
	IssueOAuthState(returnPath string) (string, error)
	ValidateOAuthState(token string) (string, error)
	DevLoginEnabled() bool
	GoogleLoginEnabled() bool
	SessionMaxAgeSeconds() int
}

type AdminUserStore interface {
	CountUsers(ctx context.Context) (int64, error)
	FindUserByEmail(ctx context.Context, email string) (AdminUserRecord, bool, error)
	FindUserBySubject(ctx context.Context, subject string) (AdminUserRecord, bool, error)
	CreateUser(ctx context.Context, identity OAuthIdentity) (AdminUserRecord, error)
	RefreshUserLogin(ctx context.Context, userID uuid.UUID, identity OAuthIdentity) (AdminUserRecord, error)
	AcceptInvitationAndCreateUser(ctx context.Context, identity OAuthIdentity, tokenHash string) (AdminUserRecord, error)
}

type AdminUserRecord struct {
	ID           uuid.UUID
	Email        string
	OAuthSubject string
	Role         string
	Enabled      bool
	Name         string
	Picture      string
	Provider     string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type AdminAuthOptions struct {
	// SessionSecret is the HMAC signing key. Must be a strong random value in
	// production — the default "dev-admin-session-secret-change-me" must not
	// be used outside of local development.
	SessionSecret string
	// AllowedDomains is a comma-separated list of email domains permitted to
	// access the admin UI.
	AllowedDomains string
	// BootstrapEmail can create the first enabled admin user without an invite.
	// It must still match AllowedDomains and is ignored once any user exists.
	BootstrapEmail string
	// DevLogin enables password-less local login. Must be false in production.
	DevLogin        bool
	SessionDuration time.Duration
	UserStore       AdminUserStore
	GoogleEnabled   bool
}

type adminAuthService struct {
	secret          []byte
	allowedDomains  map[string]struct{}
	bootstrapEmail  string
	devLogin        bool
	googleEnabled   bool
	sessionDuration time.Duration
	userStore       AdminUserStore
}

func NewAdminAuthService(opts AdminAuthOptions) AdminAuth {
	duration := opts.SessionDuration
	if duration <= 0 {
		duration = 12 * time.Hour
	}
	return &adminAuthService{
		secret:          []byte(opts.SessionSecret),
		allowedDomains:  parseAllowedDomains(opts.AllowedDomains),
		bootstrapEmail:  normalizeEmail(opts.BootstrapEmail),
		devLogin:        opts.DevLogin,
		googleEnabled:   opts.GoogleEnabled,
		sessionDuration: duration,
		userStore:       opts.UserStore,
	}
}

func (s *adminAuthService) DevLoginEnabled() bool {
	return s.devLogin
}

func (s *adminAuthService) GoogleLoginEnabled() bool {
	return s.googleEnabled
}

func (s *adminAuthService) SessionMaxAgeSeconds() int {
	return int(s.sessionDuration.Seconds())
}

func (s *adminAuthService) IssueDevSession(ctx context.Context, email string) (string, AdminSession, error) {
	if !s.devLogin {
		return "", AdminSession{}, ErrAdminAuthFailed
	}
	identity := normalizeOAuthIdentity(OAuthIdentity{
		Provider: adminSessionProviderDev,
		Email:    email,
		Name:     normalizeEmail(email),
	})
	if identity.Email == "" {
		return "", AdminSession{}, ErrAdminAuthFailed
	}
	if len(s.allowedDomains) > 0 && !s.emailDomainAllowed(identity.Email) {
		return "", AdminSession{}, ErrAdminAuthFailed
	}
	if s.userStore != nil {
		user, ok, err := s.userStore.FindUserByEmail(ctx, identity.Email)
		if err != nil {
			return "", AdminSession{}, err
		}
		if ok {
			if !user.Enabled {
				return "", AdminSession{}, ErrAdminAuthFailed
			}
			user, err = s.userStore.RefreshUserLogin(ctx, user.ID, identity)
			if err != nil {
				return "", AdminSession{}, err
			}
			return s.issueSession(user)
		}
		user, err = s.userStore.CreateUser(ctx, identity)
		if err != nil {
			return "", AdminSession{}, err
		}
		return s.issueSession(user)
	}
	session := s.sessionFromIdentity(identity)
	token, err := s.signSession(session)
	if err != nil {
		return "", AdminSession{}, err
	}
	return token, session, nil
}

func (s *adminAuthService) IssueOAuthSession(ctx context.Context, identity OAuthIdentity) (AdminOAuthResult, error) {
	identity = normalizeOAuthIdentity(identity)
	if identity.Provider == "" {
		identity.Provider = adminSessionProviderGoogle
	}
	if identity.Email == "" || identity.Subject == "" {
		return AdminOAuthResult{}, ErrAdminAuthFailed
	}
	if !s.emailDomainAllowed(identity.Email) {
		return AdminOAuthResult{Status: AdminOAuthStatusDomainDenied}, nil
	}
	if s.userStore == nil {
		return AdminOAuthResult{}, fmt.Errorf("admin user store is required")
	}

	user, ok, err := s.findExistingUser(ctx, identity)
	if err != nil {
		return AdminOAuthResult{}, err
	}
	if ok {
		if !user.Enabled {
			return AdminOAuthResult{Status: AdminOAuthStatusDisabled}, nil
		}
		user, err = s.userStore.RefreshUserLogin(ctx, user.ID, identity)
		if err != nil {
			return AdminOAuthResult{}, err
		}
		token, session, err := s.issueSession(user)
		if err != nil {
			return AdminOAuthResult{}, err
		}
		return AdminOAuthResult{Status: AdminOAuthStatusAuthenticated, Token: token, Session: session}, nil
	}

	count, err := s.userStore.CountUsers(ctx)
	if err != nil {
		return AdminOAuthResult{}, err
	}
	if count == 0 && identity.Email == s.bootstrapEmail {
		user, err := s.userStore.CreateUser(ctx, identity)
		if err != nil {
			return AdminOAuthResult{}, err
		}
		token, session, err := s.issueSession(user)
		if err != nil {
			return AdminOAuthResult{}, err
		}
		return AdminOAuthResult{Status: AdminOAuthStatusAuthenticated, Token: token, Session: session}, nil
	}

	pendingToken, err := s.signPendingSession(pendingAdminSession{
		OAuthIdentity: identity,
		ExpiresAt:     time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		return AdminOAuthResult{}, err
	}
	return AdminOAuthResult{Status: AdminOAuthStatusPendingInvite, PendingToken: pendingToken}, nil
}

func (s *adminAuthService) AcceptInvitation(ctx context.Context, pendingToken string, inviteToken string) (string, AdminSession, error) {
	if s.userStore == nil {
		return "", AdminSession{}, fmt.Errorf("admin user store is required")
	}
	pending, err := s.validatePendingSession(pendingToken)
	if err != nil {
		return "", AdminSession{}, err
	}
	identity := normalizeOAuthIdentity(pending.OAuthIdentity)
	if !s.emailDomainAllowed(identity.Email) {
		return "", AdminSession{}, ErrAdminAuthFailed
	}
	user, ok, err := s.findExistingUser(ctx, identity)
	if err != nil {
		return "", AdminSession{}, err
	}
	if ok {
		if !user.Enabled {
			return "", AdminSession{}, ErrAdminAuthFailed
		}
		user, err = s.userStore.RefreshUserLogin(ctx, user.ID, identity)
		if err != nil {
			return "", AdminSession{}, err
		}
		return s.issueSession(user)
	}
	user, err = s.userStore.AcceptInvitationAndCreateUser(ctx, identity, HashAdminToken(inviteToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", AdminSession{}, ErrAdminAuthFailed
		}
		return "", AdminSession{}, err
	}
	return s.issueSession(user)
}

func (s *adminAuthService) ValidateSession(ctx context.Context, token string) (AdminSession, error) {
	session, err := s.decodeSession(token)
	if err != nil {
		return AdminSession{}, err
	}
	if !s.emailDomainAllowed(session.Email) {
		return AdminSession{}, ErrAdminAuthFailed
	}
	if s.userStore != nil {
		user, ok, err := s.userStore.FindUserByEmail(ctx, session.Email)
		if err != nil {
			return AdminSession{}, err
		}
		if !ok || !user.Enabled {
			return AdminSession{}, ErrAdminAuthFailed
		}
		session.Name = user.Name
		session.Picture = user.Picture
		session.Provider = user.Provider
		session.Subject = user.OAuthSubject
	}
	return session, nil
}

func (s *adminAuthService) IssueOAuthState(returnPath string) (string, error) {
	return s.signJSON(adminStateToken{
		ReturnPath: returnPath,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
}

func (s *adminAuthService) ValidateOAuthState(token string) (string, error) {
	var state adminStateToken
	if err := s.decodeJSON(token, &state); err != nil {
		return "", err
	}
	if time.Now().UTC().After(state.ExpiresAt) {
		return "", ErrAdminAuthFailed
	}
	return state.ReturnPath, nil
}

func (s *adminAuthService) findExistingUser(ctx context.Context, identity OAuthIdentity) (AdminUserRecord, bool, error) {
	if identity.Subject != "" {
		user, ok, err := s.userStore.FindUserBySubject(ctx, identity.Subject)
		if err != nil {
			return AdminUserRecord{}, false, err
		}
		if ok {
			return user, true, nil
		}
	}
	return s.userStore.FindUserByEmail(ctx, identity.Email)
}

func (s *adminAuthService) issueSession(user AdminUserRecord) (string, AdminSession, error) {
	session := AdminSession{
		Email:     user.Email,
		Name:      user.Name,
		Picture:   user.Picture,
		Provider:  user.Provider,
		Subject:   user.OAuthSubject,
		ExpiresAt: time.Now().UTC().Add(s.sessionDuration),
	}
	token, err := s.signSession(session)
	if err != nil {
		return "", AdminSession{}, err
	}
	return token, session, nil
}

func (s *adminAuthService) sessionFromIdentity(identity OAuthIdentity) AdminSession {
	return AdminSession{
		Email:     identity.Email,
		Name:      identity.Name,
		Picture:   identity.Picture,
		Provider:  identity.Provider,
		Subject:   identity.Subject,
		ExpiresAt: time.Now().UTC().Add(s.sessionDuration),
	}
}

func (s *adminAuthService) signSession(session AdminSession) (string, error) {
	return s.signJSON(session)
}

func (s *adminAuthService) decodeSession(token string) (AdminSession, error) {
	var session AdminSession
	if err := s.decodeJSON(token, &session); err != nil {
		return AdminSession{}, err
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return AdminSession{}, ErrAdminAuthFailed
	}
	return session, nil
}

func (s *adminAuthService) signPendingSession(session pendingAdminSession) (string, error) {
	return s.signJSON(session)
}

func (s *adminAuthService) validatePendingSession(token string) (pendingAdminSession, error) {
	var session pendingAdminSession
	if err := s.decodeJSON(token, &session); err != nil {
		return pendingAdminSession{}, err
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return pendingAdminSession{}, ErrAdminAuthFailed
	}
	return session, nil
}

func (s *adminAuthService) signJSON(value any) (string, error) {
	if len(s.secret) == 0 {
		return "", fmt.Errorf("admin session secret is required")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + s.signature(encoded), nil
}

func (s *adminAuthService) decodeJSON(token string, out any) error {
	if strings.TrimSpace(token) == "" {
		return ErrAdminAuthFailed
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return ErrAdminAuthFailed
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ErrAdminAuthFailed
	}
	expected := s.signature(parts[0])
	// hmac.Equal performs a constant-time comparison — do not replace with == or
	// bytes.Equal, which leak timing information and enable signature forgery.
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return ErrAdminAuthFailed
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return ErrAdminAuthFailed
	}
	return nil
}

func (s *adminAuthService) signature(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *adminAuthService) emailDomainAllowed(email string) bool {
	if len(s.allowedDomains) == 0 {
		return true
	}
	domain := emailDomain(email)
	if domain == "" {
		return false
	}
	_, ok := s.allowedDomains[domain]
	return ok
}

type postgresAdminUserStore struct {
	db      db.Pool
	queries *dbq.Queries
}

func NewPostgresAdminUserStore(pool db.Pool) AdminUserStore {
	return &postgresAdminUserStore{
		db:      pool,
		queries: dbq.New(pool),
	}
}

func (s *postgresAdminUserStore) CountUsers(ctx context.Context) (int64, error) {
	return s.queries.AdminCountUsers(ctx)
}

func (s *postgresAdminUserStore) FindUserByEmail(ctx context.Context, email string) (AdminUserRecord, bool, error) {
	row, err := s.queries.AdminGetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminUserRecord{}, false, nil
		}
		return AdminUserRecord{}, false, err
	}
	return adminUserRecordFromEmailRow(row), true, nil
}

func (s *postgresAdminUserStore) FindUserBySubject(ctx context.Context, subject string) (AdminUserRecord, bool, error) {
	row, err := s.queries.AdminGetUserBySubject(ctx, textParam(subject))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminUserRecord{}, false, nil
		}
		return AdminUserRecord{}, false, err
	}
	return adminUserRecordFromSubjectRow(row), true, nil
}

func (s *postgresAdminUserStore) CreateUser(ctx context.Context, identity OAuthIdentity) (AdminUserRecord, error) {
	row, err := s.queries.AdminCreateUser(ctx, dbq.AdminCreateUserParams{
		Email:        identity.Email,
		OauthSubject: textParam(identity.Subject),
		Name:         identity.Name,
		PictureUrl:   identity.Picture,
		Provider:     identity.Provider,
	})
	if err != nil {
		return AdminUserRecord{}, err
	}
	return adminUserRecordFromCreateRow(row), nil
}

func (s *postgresAdminUserStore) RefreshUserLogin(ctx context.Context, userID uuid.UUID, identity OAuthIdentity) (AdminUserRecord, error) {
	row, err := s.queries.AdminRefreshUserLogin(ctx, dbq.AdminRefreshUserLoginParams{
		ID:           userID,
		OauthSubject: textParam(identity.Subject),
		Name:         identity.Name,
		PictureUrl:   identity.Picture,
		Provider:     identity.Provider,
	})
	if err != nil {
		return AdminUserRecord{}, err
	}
	return adminUserRecordFromRefreshRow(row), nil
}

func (s *postgresAdminUserStore) AcceptInvitationAndCreateUser(ctx context.Context, identity OAuthIdentity, tokenHash string) (AdminUserRecord, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AdminUserRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)
	userRow, err := qtx.AdminCreateUser(ctx, dbq.AdminCreateUserParams{
		Email:        identity.Email,
		OauthSubject: textParam(identity.Subject),
		Name:         identity.Name,
		PictureUrl:   identity.Picture,
		Provider:     identity.Provider,
	})
	if err != nil {
		return AdminUserRecord{}, err
	}
	user := adminUserRecordFromCreateRow(userRow)
	if _, err := qtx.AdminAcceptInvitation(ctx, dbq.AdminAcceptInvitationParams{
		Email:                 identity.Email,
		TokenHash:             tokenHash,
		AcceptedByAdminUserID: uuidParam(user.ID),
	}); err != nil {
		return AdminUserRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AdminUserRecord{}, err
	}
	return user, nil
}

func normalizeOAuthIdentity(identity OAuthIdentity) OAuthIdentity {
	identity.Email = normalizeEmail(identity.Email)
	identity.Subject = strings.TrimSpace(identity.Subject)
	identity.Name = strings.TrimSpace(identity.Name)
	identity.Picture = strings.TrimSpace(identity.Picture)
	identity.Provider = strings.TrimSpace(identity.Provider)
	if identity.Provider == "" {
		identity.Provider = adminSessionProviderGoogle
	}
	return identity
}

func parseAllowedDomains(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, domain := range strings.Split(raw, ",") {
		normalized := normalizeDomain(domain)
		if normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func normalizeEmail(email string) string {
	return env.NormalizeEmail(email)
}

func normalizeDomain(domain string) string {
	return env.NormalizeDomain(domain)
}

func emailDomain(email string) string {
	return env.EmailDomain(email)
}

func HashAdminToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func NewAdminInviteToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := "fr_invite_" + base64.RawURLEncoding.EncodeToString(raw)
	return token, HashAdminToken(token), nil
}

func textParam(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func uuidParam(value uuid.UUID) pgtype.UUID {
	if value == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: value, Valid: true}
}

func pgTextValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func adminUserRecordFromEmailRow(row dbq.AdminGetUserByEmailRow) AdminUserRecord {
	return AdminUserRecord{
		ID:           row.ID,
		Email:        row.Email,
		OAuthSubject: pgTextValue(row.OauthSubject),
		Role:         row.Role,
		Enabled:      row.Enabled,
		Name:         row.Name,
		Picture:      row.PictureUrl,
		Provider:     row.Provider,
		LastLoginAt:  timePtr(row.LastLoginAt),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func adminUserRecordFromSubjectRow(row dbq.AdminGetUserBySubjectRow) AdminUserRecord {
	return AdminUserRecord{
		ID:           row.ID,
		Email:        row.Email,
		OAuthSubject: pgTextValue(row.OauthSubject),
		Role:         row.Role,
		Enabled:      row.Enabled,
		Name:         row.Name,
		Picture:      row.PictureUrl,
		Provider:     row.Provider,
		LastLoginAt:  timePtr(row.LastLoginAt),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func adminUserRecordFromCreateRow(row dbq.AdminCreateUserRow) AdminUserRecord {
	return AdminUserRecord{
		ID:           row.ID,
		Email:        row.Email,
		OAuthSubject: pgTextValue(row.OauthSubject),
		Role:         row.Role,
		Enabled:      row.Enabled,
		Name:         row.Name,
		Picture:      row.PictureUrl,
		Provider:     row.Provider,
		LastLoginAt:  timePtr(row.LastLoginAt),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func adminUserRecordFromRefreshRow(row dbq.AdminRefreshUserLoginRow) AdminUserRecord {
	return AdminUserRecord{
		ID:           row.ID,
		Email:        row.Email,
		OAuthSubject: pgTextValue(row.OauthSubject),
		Role:         row.Role,
		Enabled:      row.Enabled,
		Name:         row.Name,
		Picture:      row.PictureUrl,
		Provider:     row.Provider,
		LastLoginAt:  timePtr(row.LastLoginAt),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
