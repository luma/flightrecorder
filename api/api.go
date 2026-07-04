package api

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/cors"
	"github.com/hertz-contrib/pprof"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luma/flightrecorder/api/auth"
	"github.com/luma/flightrecorder/api/requestid"
	"github.com/luma/flightrecorder/api/spa"
	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/services"
)

// Options contains the options for the API server.
type Options struct {
	APIPort            int
	APIBasePath        string
	APIExitGracePeriod time.Duration
	APIBaseURL         string // Base URL for OAuth callbacks
	WebBaseURL         string // Frontend URL for redirects

	EnablePprof bool

	DBPool  db.Pool
	PgxPool *pgxpool.Pool // For River background jobs
	Log     *slog.Logger
	Ctx     context.Context // Parent context for graceful shutdown

	// Services
	AuthService   services.Auth
	IngestService services.Ingest
	AdminAuth     services.AdminAuth
	AdminService  services.Admin
	GoogleOAuth   services.GoogleOAuth
	AgentAuth     services.AgentAuth
	MCPOAuth      services.MCPOAuth
}

func (o *Options) Validate() error {
	var errs []string

	if o.DBPool == nil {
		errs = append(errs, "DBPool is required")
	}
	if o.Log == nil {
		errs = append(errs, "Log is required")
	}

	if o.AuthService == nil {
		errs = append(errs, "AuthService is required")
	}
	if o.IngestService == nil {
		errs = append(errs, "IngestService is required")
	}
	if o.AdminAuth == nil {
		errs = append(errs, "AdminAuth is required")
	}
	if o.AdminService == nil {
		errs = append(errs, "AdminService is required")
	}
	if o.GoogleOAuth == nil {
		errs = append(errs, "GoogleOAuth is required")
	}
	if o.AgentAuth == nil {
		errs = append(errs, "AgentAuth is required")
	}
	if o.MCPOAuth == nil {
		errs = append(errs, "MCPOAuth is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// Run starts the API server.
func Run(opts Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	log := opts.Log

	// Use provided context or background if none given
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	h := server.New(
		server.WithHostPorts(fmt.Sprintf(":%d", opts.APIPort)),
		server.WithBasePath(opts.APIBasePath),
		server.WithExitWaitTime(opts.APIExitGracePeriod),
		server.WithMaxRequestBodySize(100*1024*1024),
	)

	// Basic panic handling.
	h.Use(recovery.Recovery())

	// Request ID middleware - must come early for log correlation.
	h.Use(requestid.Middleware(log))

	// CORS middleware - allow frontend to make requests
	allowedOrigins := []string{"http://localhost:3000"}
	if opts.WebBaseURL != "" && opts.WebBaseURL != "http://localhost:3000" {
		allowedOrigins = append(allowedOrigins, opts.WebBaseURL)
	}
	h.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Timezone"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	log.Info("CORS enabled", "origins", allowedOrigins)

	// Register pprof endpoints if enabled
	if opts.EnablePprof {
		log.Warn("pprof endpoints enabled - this should not be enabled in production",
			"prefix", "/debug/pprof")
		pprof.Register(h)
	}

	// v1 API routes
	h.GET("/healthz", handleReady)

	v1 := h.Group("/v1")
	v1.GET("/ready", handleReady)
	v1.GET("/health", makeHandleHealth(opts.DBPool))

	// Protected routes accept Bearer API keys
	protected := v1.Group("", auth.RequireAuth(opts.AuthService, log))
	protected.POST("/events", makeHandleEvents(opts.IngestService))
	protected.POST("/bug-reports", makeHandleBugReports(opts.IngestService))

	admin := h.Group("/api/admin/v1")
	registerAdminRoutes(admin, opts.AdminAuth, opts.AdminService, opts.GoogleOAuth, log, auth.SecureCookie(opts.APIBaseURL))

	registerMCPRoutes(h, opts.AdminAuth, opts.AdminService, opts.AgentAuth, opts.MCPOAuth, log, auth.SecureCookie(opts.APIBaseURL), opts.APIBaseURL)

	// Serve embedded SPA assets. Static files (JS, CSS) are served directly;
	// all other paths fall back to index.html for client-side routing.
	indexHTML, err := fs.ReadFile(spa.Assets, "index.html")
	if err != nil {
		return fmt.Errorf("embedded SPA missing index.html: %w", err)
	}

	// MIME types for embedded SPA files.
	mimeTypes := map[string]string{
		".html":        "text/html; charset=utf-8",
		".js":          "application/javascript",
		".css":         "text/css",
		".json":        "application/json",
		".svg":         "image/svg+xml",
		".png":         "image/png",
		".ico":         "image/x-icon",
		".woff2":       "font/woff2",
		".woff":        "font/woff",
		".ttf":         "font/ttf",
		".webp":        "image/webp",
		".webmanifest": "application/manifest+json",
	}

	// Files that should never be served from the embedded FS.
	blockedFiles := map[string]bool{
		"embed.go": true,
		".gitkeep": true,
	}

	h.NoRoute(func(ctx context.Context, c *app.RequestContext) {
		trimmed := strings.TrimPrefix(string(c.URI().Path()), "/")

		if blockedFiles[trimmed] {
			c.SetStatusCode(consts.StatusNotFound)
			return
		}

		// Try serving any embedded file (assets/, registerSW.js, sw.js, etc.)
		if trimmed != "" {
			data, err := fs.ReadFile(spa.Assets, trimmed)
			if err == nil {
				ext := filepath.Ext(trimmed)
				ct, ok := mimeTypes[ext]
				if !ok {
					ct = "text/plain; charset=utf-8"
				}
				c.SetContentType(ct)

				// Hashed assets are immutable; everything else must revalidate.
				if strings.HasPrefix(trimmed, "assets/") {
					c.Response.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					c.Response.Header.Set("Cache-Control", "no-cache")
				}
				c.SetStatusCode(consts.StatusOK)
				c.Response.SetBody(data)
				return
			}

			// Requests with a file extension that didn't match an embedded file
			// are genuinely missing, not SPA routes. 404 instead of serving index.html.
			if filepath.Ext(trimmed) != "" {
				c.SetStatusCode(consts.StatusNotFound)
				return
			}
		}

		// SPA fallback: serve index.html for client-side routing.
		c.SetContentType("text/html; charset=utf-8")
		c.Response.Header.Set("Cache-Control", "no-cache")
		c.SetStatusCode(consts.StatusOK)
		c.Response.SetBody(indexHTML)
	})

	log.Info("starting API server", "port", opts.APIPort)

	// Handle context cancellation for graceful shutdown
	go func() {
		<-ctx.Done()
		log.Info("context cancelled, initiating graceful shutdown")

		// Then stop the HTTP server
		shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.APIExitGracePeriod)
		defer cancel()
		if err := h.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown HTTP server", "error", err)
		}
	}()

	h.Spin()

	return nil
}

func handleReady(ctx context.Context, c *app.RequestContext) {
	c.String(consts.StatusOK, "ready")
}

// makeHandleHealth returns a handler that always reports "healthy".
// The pool parameter is not used — this endpoint does not perform a live DB
// connectivity check. Do not rely on it for health-gated deployments without
// adding an actual pool.Ping call.
func makeHandleHealth(pool db.Pool) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		health := map[string]any{
			"status":    "healthy",
			"timestamp": time.Now().UTC(),
		}

		c.JSON(consts.StatusOK, health)
	}
}
