package auth

import (
	"context"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/luma/flightrecorder/services"
)

const SessionCookieName = "flightrecorder_session"

// SecureCookie returns true when cookies should have the Secure flag set.
// In production (any non-localhost base URL), cookies must only travel over TLS.
func SecureCookie(baseURL string) bool {
	return baseURL != "" &&
		!strings.HasPrefix(baseURL, "http://localhost") &&
		!strings.HasPrefix(baseURL, "http://127.0.0.1")
}

// RequireAuth enforces Bearer token authentication. The raw token is SHA-256
// hashed before the database lookup — only hashes are stored, never plaintext
// tokens. On success the project ID is stored in context via
// services.ContextWithProjectID for downstream handlers to use.
func RequireAuth(authSvc services.Auth, log *slog.Logger) app.HandlerFunc {
	return requireAuth(authSvc, log)
}

func requireAuth(authSvc services.Auth, log *slog.Logger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		header := string(c.GetHeader("Authorization"))
		if header != "" && strings.HasPrefix(header, "Bearer ") {
			token := strings.TrimPrefix(header, "Bearer ")
			if token != "" && strings.HasPrefix(token, services.TelemetryTokenPrefix) {
				hash := HashToken(token)
				projectID, err := authSvc.ValidateAccessToken(ctx, hash)
				if err == nil {
					ctx = services.ContextWithProjectID(ctx, projectID)
					c.Next(ctx)
					return
				}

				log.DebugContext(
					ctx, "bearer auth failed",
					slog.String("api_key_err", err.Error()),
				)
			}
		}

		c.JSON(consts.StatusUnauthorized, map[string]string{"error": "authentication required"})
		c.Abort()
	}
}

// RequireMCPAgent enforces bearer authentication for remote MCP agents.
func RequireMCPAgent(agentAuth services.AgentAuth, log *slog.Logger, resourceMetadataURL string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		header := string(c.GetHeader("Authorization"))
		if header != "" && strings.HasPrefix(header, "Bearer ") {
			token := strings.TrimPrefix(header, "Bearer ")
			if token != "" && strings.HasPrefix(token, services.AgentTokenPrefix) {
				session, err := agentAuth.ValidateAgentToken(ctx, token)
				if err == nil {
					ctx = services.ContextWithAgentSession(ctx, session)
					c.Next(ctx)
					return
				}
				log.DebugContext(ctx, "agent bearer auth failed", slog.String("agent_auth_err", err.Error()))
			}
		}

		if resourceMetadataURL != "" {
			c.Header("WWW-Authenticate", `Bearer resource_metadata="`+resourceMetadataURL+`"`)
		}
		c.JSON(consts.StatusUnauthorized, map[string]string{"error": "agent authentication required"})
		c.Abort()
	}
}

// HashToken produces a SHA-256 hex digest of the raw token.
func HashToken(token string) string {
	return services.HashToken(token)
}
