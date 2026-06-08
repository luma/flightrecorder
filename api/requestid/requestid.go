// Package requestid provides middleware for request ID correlation across
// components. Request IDs allow tracing a request from agent through API
// for debugging and log correlation.
package requestid

import (
	"context"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
)

const (
	// Header is the HTTP header name for request ID.
	Header = "X-Request-ID"
)

type contextKey struct{}

// Middleware creates a Hertz middleware that extracts or generates a request ID,
// stores it in context, and adds it to the response headers.
func Middleware(logger *slog.Logger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		requestID := string(c.GetHeader(Header))

		if requestID == "" {
			requestID = uuid.New().String()
		}

		ctx = WithRequestID(ctx, requestID)

		c.Response.Header.Set(Header, requestID)

		logger := logger.With("request_id", requestID)
		ctx = WithLogger(ctx, logger)

		c.Next(ctx)
	}
}

// WithRequestID stores a request ID in the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, contextKey{}, requestID)
}

// FromContext retrieves the request ID from context. Returns empty string if not present.
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(contextKey{}).(string); ok {
		return id
	}
	return ""
}

type loggerKey struct{}

// WithLogger stores a logger in the context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// LoggerFromContext retrieves the logger from context. Returns nil if not present.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return nil
}

// MustLoggerFromContext retrieves the logger from context or panics.
func MustLoggerFromContext(ctx context.Context) *slog.Logger {
	l := LoggerFromContext(ctx)
	if l == nil {
		panic("logger not found in context")
	}
	return l
}
