package auth

import (
	"context"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/luma/flightrecorder/services"
)

const AdminSessionCookieName = "flightrecorder_admin"

func RequireAdmin(adminAuth services.AdminAuth, log *slog.Logger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		token := string(c.Cookie(AdminSessionCookieName))
		session, err := adminAuth.ValidateSession(token)
		if err != nil {
			log.DebugContext(ctx, "admin session auth failed", slog.String("err", err.Error()))
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "admin authentication required"})
			c.Abort()
			return
		}
		c.Next(services.ContextWithAdminSession(ctx, session))
	}
}
