package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/luma/flightrecorder/services"
)

const SessionCookieName = "pithy_session"

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
			if token != "" {
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

// HashToken produces a SHA-256 hex digest of the raw token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
