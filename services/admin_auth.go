package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrAdminAuthFailed = errors.New("admin authentication failed")

// AdminSession is serialized as a custom HMAC-SHA256 signed token (not a JWT).
// Wire format: base64url(json_payload) + "." + base64url(hmac_sha256_over_encoded_payload).
type AdminSession struct {
	Email     string    `json:"email"`
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
	IssueDevSession(email string) (string, AdminSession, error)
	ValidateSession(token string) (AdminSession, error)
	DevLoginEnabled() bool
}

type AdminAuthOptions struct {
	// SessionSecret is the HMAC signing key. Must be a strong random value in
	// production — the default "dev-admin-session-secret-change-me" must not
	// be used outside of local development.
	SessionSecret string
	// AllowedEmails is a comma-separated list of email addresses permitted to
	// access the admin UI.
	AllowedEmails string
	// DevLogin enables password-less login for any email in AllowedEmails.
	// Must be false in production — when true, anyone who knows an allowed
	// email address can authenticate without any credential.
	DevLogin        bool
	SessionDuration time.Duration
}

type adminAuthService struct {
	secret          []byte
	allowedEmails   map[string]struct{}
	devLogin        bool
	sessionDuration time.Duration
}

func NewAdminAuthService(opts AdminAuthOptions) AdminAuth {
	duration := opts.SessionDuration
	if duration <= 0 {
		duration = 12 * time.Hour
	}
	return &adminAuthService{
		secret:          []byte(opts.SessionSecret),
		allowedEmails:   parseAllowedEmails(opts.AllowedEmails),
		devLogin:        opts.DevLogin,
		sessionDuration: duration,
	}
}

func (s *adminAuthService) DevLoginEnabled() bool {
	return s.devLogin
}

func (s *adminAuthService) IssueDevSession(email string) (string, AdminSession, error) {
	if !s.devLogin {
		return "", AdminSession{}, ErrAdminAuthFailed
	}
	if !s.emailAllowed(email) {
		return "", AdminSession{}, ErrAdminAuthFailed
	}
	session := AdminSession{
		Email:     normalizeEmail(email),
		ExpiresAt: time.Now().UTC().Add(s.sessionDuration),
	}
	token, err := s.signSession(session)
	if err != nil {
		return "", AdminSession{}, err
	}
	return token, session, nil
}

func (s *adminAuthService) ValidateSession(token string) (AdminSession, error) {
	if strings.TrimSpace(token) == "" {
		return AdminSession{}, ErrAdminAuthFailed
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return AdminSession{}, ErrAdminAuthFailed
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return AdminSession{}, ErrAdminAuthFailed
	}
	expected := s.signature(parts[0])
	// hmac.Equal performs a constant-time comparison — do not replace with == or
	// bytes.Equal, which leak timing information and enable signature forgery.
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return AdminSession{}, ErrAdminAuthFailed
	}
	var session AdminSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return AdminSession{}, ErrAdminAuthFailed
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return AdminSession{}, ErrAdminAuthFailed
	}
	if !s.emailAllowed(session.Email) {
		return AdminSession{}, ErrAdminAuthFailed
	}
	return session, nil
}

func (s *adminAuthService) signSession(session AdminSession) (string, error) {
	if len(s.secret) == 0 {
		return "", fmt.Errorf("admin session secret is required")
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + s.signature(encoded), nil
}

func (s *adminAuthService) signature(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *adminAuthService) emailAllowed(email string) bool {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false
	}
	_, ok := s.allowedEmails[normalized]
	return ok
}

func parseAllowedEmails(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, email := range strings.Split(raw, ",") {
		normalized := normalizeEmail(email)
		if normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
