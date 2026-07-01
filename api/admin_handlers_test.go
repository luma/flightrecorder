package api

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/luma/flightrecorder/services"
)

// fakeAdminAuth is a configurable stand-in for services.AdminAuth used to
// exercise the OAuth handler control flow without a real signer or database.
type fakeAdminAuth struct {
	devEnabled    bool
	googleEnabled bool
	maxAge        int

	stateToken      string
	stateReturnPath string
	stateErr        error

	oauthResult services.AdminOAuthResult
	oauthErr    error

	acceptToken   string
	acceptSession services.AdminSession
	acceptErr     error

	devToken   string
	devSession services.AdminSession
	devErr     error

	validateSession services.AdminSession
	validateErr     error
}

func (f *fakeAdminAuth) IssueDevSession(context.Context, string) (string, services.AdminSession, error) {
	return f.devToken, f.devSession, f.devErr
}

func (f *fakeAdminAuth) IssueOAuthSession(context.Context, services.OAuthIdentity) (services.AdminOAuthResult, error) {
	return f.oauthResult, f.oauthErr
}

func (f *fakeAdminAuth) AcceptInvitation(context.Context, string, string) (string, services.AdminSession, error) {
	return f.acceptToken, f.acceptSession, f.acceptErr
}

func (f *fakeAdminAuth) ValidateSession(context.Context, string) (services.AdminSession, error) {
	return f.validateSession, f.validateErr
}

func (f *fakeAdminAuth) IssueOAuthState(returnPath string) (string, error) {
	return f.stateToken, f.stateErr
}

func (f *fakeAdminAuth) ValidateOAuthState(string) (string, error) {
	return f.stateReturnPath, f.stateErr
}

func (f *fakeAdminAuth) DevLoginEnabled() bool     { return f.devEnabled }
func (f *fakeAdminAuth) GoogleLoginEnabled() bool  { return f.googleEnabled }
func (f *fakeAdminAuth) SessionMaxAgeSeconds() int { return f.maxAge }

type fakeGoogleOAuth struct {
	enabled     bool
	authURL     string
	identity    services.OAuthIdentity
	exchangeErr error
}

func (f *fakeGoogleOAuth) AuthCodeURL(string) string { return f.authURL }
func (f *fakeGoogleOAuth) Exchange(context.Context, string) (services.OAuthIdentity, error) {
	return f.identity, f.exchangeErr
}
func (f *fakeGoogleOAuth) Enabled() bool { return f.enabled }

func newRequestContext(uri string) *app.RequestContext {
	ctx := &app.RequestContext{}
	if uri != "" {
		ctx.Request.SetRequestURI(uri)
	}
	return ctx
}

func responseLocation(ctx *app.RequestContext) string {
	return string(ctx.Response.Header.PeekLocation())
}

func responseCookie(ctx *app.RequestContext, name string) (*protocol.Cookie, bool) {
	cookie := protocol.AcquireCookie()
	cookie.SetKey(name)
	ok := ctx.Response.Header.Cookie(cookie)
	return cookie, ok
}

func TestAdminGoogleStartRedirectsAndSetsStateCookie(t *testing.T) {
	auth := &fakeAdminAuth{googleEnabled: true, stateToken: "state-token"}
	google := &fakeGoogleOAuth{enabled: true, authURL: "https://accounts.google.com/o/oauth2/auth?x=1"}
	handler := makeAdminGoogleStart(auth, google, false)

	ctx := newRequestContext("/api/admin/v1/auth/google/start")
	handler(context.Background(), ctx)

	if got := ctx.Response.StatusCode(); got != consts.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", got)
	}
	if loc := responseLocation(ctx); loc != google.authURL {
		t.Fatalf("expected redirect to auth URL, got %q", loc)
	}
	cookie, ok := responseCookie(ctx, oauthStateCookieName)
	if !ok || string(cookie.Value()) != "state-token" {
		t.Fatalf("expected state cookie to be set, got ok=%v value=%q", ok, cookie.Value())
	}
}

func TestAdminGoogleCallbackRejectsMissingState(t *testing.T) {
	auth := &fakeAdminAuth{googleEnabled: true}
	google := &fakeGoogleOAuth{enabled: true}
	handler := makeAdminGoogleCallback(auth, google, false)

	ctx := newRequestContext("/api/admin/v1/auth/google/callback")
	handler(context.Background(), ctx)

	if loc := responseLocation(ctx); !strings.Contains(loc, "/login-error?reason=invalid_state") {
		t.Fatalf("expected invalid_state redirect, got %q", loc)
	}
}

func TestAdminGoogleCallbackRejectsMismatchedState(t *testing.T) {
	auth := &fakeAdminAuth{googleEnabled: true, stateReturnPath: ""}
	google := &fakeGoogleOAuth{enabled: true}
	handler := makeAdminGoogleCallback(auth, google, false)

	ctx := newRequestContext("/api/admin/v1/auth/google/callback?state=param-state")
	ctx.Request.Header.SetCookie(oauthStateCookieName, "different-cookie-state")
	handler(context.Background(), ctx)

	if loc := responseLocation(ctx); !strings.Contains(loc, "reason=invalid_state") {
		t.Fatalf("expected invalid_state redirect, got %q", loc)
	}
}

func TestAdminGoogleCallbackSetsAdminCookieForAuthenticatedUser(t *testing.T) {
	auth := &fakeAdminAuth{
		googleEnabled: true,
		maxAge:        3600,
		oauthResult: services.AdminOAuthResult{
			Status: services.AdminOAuthStatusAuthenticated,
			Token:  "admin-token",
		},
	}
	google := &fakeGoogleOAuth{enabled: true, identity: services.OAuthIdentity{Email: "user@example.com"}}
	handler := makeAdminGoogleCallback(auth, google, false)

	ctx := newRequestContext("/api/admin/v1/auth/google/callback?state=matching&code=abc")
	ctx.Request.Header.SetCookie(oauthStateCookieName, "matching")
	handler(context.Background(), ctx)

	if loc := responseLocation(ctx); loc != "/" {
		t.Fatalf("expected redirect to /, got %q", loc)
	}
	cookie, ok := responseCookie(ctx, "flightrecorder_admin")
	if !ok || string(cookie.Value()) != "admin-token" {
		t.Fatalf("expected admin cookie, got ok=%v value=%q", ok, cookie.Value())
	}
	if cookie.MaxAge() != 3600 {
		t.Fatalf("expected admin cookie max-age 3600, got %d", cookie.MaxAge())
	}
}

func TestAdminGoogleCallbackSetsPendingCookieForUnknownUser(t *testing.T) {
	auth := &fakeAdminAuth{
		googleEnabled: true,
		oauthResult: services.AdminOAuthResult{
			Status:       services.AdminOAuthStatusPendingInvite,
			PendingToken: "pending-token",
		},
	}
	google := &fakeGoogleOAuth{enabled: true}
	handler := makeAdminGoogleCallback(auth, google, false)

	ctx := newRequestContext("/api/admin/v1/auth/google/callback?state=matching&code=abc")
	ctx.Request.Header.SetCookie(oauthStateCookieName, "matching")
	handler(context.Background(), ctx)

	if loc := responseLocation(ctx); loc != "/accept-invite" {
		t.Fatalf("expected redirect to /accept-invite, got %q", loc)
	}
	cookie, ok := responseCookie(ctx, pendingAdminCookieName)
	if !ok || string(cookie.Value()) != "pending-token" {
		t.Fatalf("expected pending cookie, got ok=%v value=%q", ok, cookie.Value())
	}
}

func TestAdminGoogleCallbackRedirectsDisabledUser(t *testing.T) {
	auth := &fakeAdminAuth{
		googleEnabled: true,
		oauthResult:   services.AdminOAuthResult{Status: services.AdminOAuthStatusDisabled},
	}
	google := &fakeGoogleOAuth{enabled: true}
	handler := makeAdminGoogleCallback(auth, google, false)

	ctx := newRequestContext("/api/admin/v1/auth/google/callback?state=matching&code=abc")
	ctx.Request.Header.SetCookie(oauthStateCookieName, "matching")
	handler(context.Background(), ctx)

	if loc := responseLocation(ctx); !strings.Contains(loc, "reason=disabled") {
		t.Fatalf("expected disabled redirect, got %q", loc)
	}
}

func TestAdminDevLoginUsesConfiguredMaxAge(t *testing.T) {
	auth := &fakeAdminAuth{
		devEnabled: true,
		maxAge:     7200,
		devToken:   "dev-token",
		devSession: services.AdminSession{Email: "admin@example.com"},
	}
	handler := makeAdminDevLogin(auth, false)

	ctx := newRequestContext("/api/admin/v1/auth/dev-login")
	ctx.Request.Header.SetMethod(consts.MethodPost)
	ctx.Request.SetBodyString(`{"email":"admin@example.com"}`)
	handler(context.Background(), ctx)

	if got := ctx.Response.StatusCode(); got != consts.StatusOK {
		t.Fatalf("expected 200, got %d", got)
	}
	cookie, ok := responseCookie(ctx, "flightrecorder_admin")
	if !ok || cookie.MaxAge() != 7200 {
		t.Fatalf("expected dev-login cookie max-age 7200, got ok=%v maxAge=%d", ok, cookie.MaxAge())
	}
}

func TestAdminMeReturnsProfile(t *testing.T) {
	auth := &fakeAdminAuth{
		validateSession: services.AdminSession{
			Email:   "admin@example.com",
			Name:    "Admin User",
			Picture: "https://pic.example/a.png",
		},
	}
	handler := makeAdminMe(auth)

	ctx := newRequestContext("/api/admin/v1/auth/me")
	handler(context.Background(), ctx)

	if got := ctx.Response.StatusCode(); got != consts.StatusOK {
		t.Fatalf("expected 200, got %d", got)
	}
	body := string(ctx.Response.Body())
	for _, want := range []string{"admin@example.com", "Admin User", "https://pic.example/a.png"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got %s", want, body)
		}
	}
}

func TestAdminAcceptInviteCodeSetsCookieOnSuccess(t *testing.T) {
	auth := &fakeAdminAuth{
		maxAge:        3600,
		acceptToken:   "admin-token",
		acceptSession: services.AdminSession{Email: "invitee@example.com"},
	}
	handler := makeAdminAcceptInviteCode(auth, false)

	ctx := newRequestContext("/api/admin/v1/auth/invite-code")
	ctx.Request.Header.SetMethod(consts.MethodPost)
	ctx.Request.Header.SetCookie(pendingAdminCookieName, "pending-token")
	ctx.Request.SetBodyString(`{"code":"fr_invite_ok"}`)
	handler(context.Background(), ctx)

	if got := ctx.Response.StatusCode(); got != consts.StatusOK {
		t.Fatalf("expected 200, got %d", got)
	}
	cookie, ok := responseCookie(ctx, "flightrecorder_admin")
	if !ok || string(cookie.Value()) != "admin-token" {
		t.Fatalf("expected admin cookie, got ok=%v value=%q", ok, cookie.Value())
	}
}

func TestAdminAcceptInviteCodeRejectsInvalidCode(t *testing.T) {
	auth := &fakeAdminAuth{acceptErr: services.ErrAdminAuthFailed}
	handler := makeAdminAcceptInviteCode(auth, false)

	ctx := newRequestContext("/api/admin/v1/auth/invite-code")
	ctx.Request.Header.SetMethod(consts.MethodPost)
	ctx.Request.Header.SetCookie(pendingAdminCookieName, "pending-token")
	ctx.Request.SetBodyString(`{"code":"wrong"}`)
	handler(context.Background(), ctx)

	if got := ctx.Response.StatusCode(); got != consts.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", got)
	}
}
