// Package services implements the core business logic for flightrecorder.
// It is organized into four domains: Auth (API key validation), Ingest (event
// and bug report ingestion), Admin (read/write access for the admin UI), and
// AdminAuth (session management for the admin UI itself). All services accept
// a db.Pool and return domain types; HTTP concerns live in the api package.
package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/db/dbq"
)

var ErrInvalidAccessToken = errors.New("invalid access token")

type Auth interface {
	// ValidateAccessToken returns the project ID for a valid token hash.
	ValidateAccessToken(ctx context.Context, tokenHash string) (uuid.UUID, error)
}

type authService struct {
	queries *dbq.Queries
}

func NewAuthService(pool db.Pool) Auth {
	return &authService{queries: dbq.New(pool)}
}

func (s *authService) ValidateAccessToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	projectID, err := s.queries.GetProjectIDByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrInvalidAccessToken
		}
		return uuid.Nil, err
	}
	return projectID, nil
}

type projectContextKey struct{}

// ContextWithProjectID stores the user's accessible project ID in the context.
// The auth middleware calls this after looking up the project id from a API key.
func ContextWithProjectID(ctx context.Context, projectID uuid.UUID) context.Context {
	return context.WithValue(ctx, projectContextKey{}, projectID)
}

// ProjectIDFromContext returns the accessible project ID stored in context.
// Returns nil if none are set.
func ProjectIDFromContext(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(projectContextKey{}).(uuid.UUID)
	return id
}
