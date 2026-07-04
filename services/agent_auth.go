package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/db/dbq"
)

var ErrAgentAuthFailed = errors.New("agent authentication failed")

type AgentAuth interface {
	ValidateAgentToken(ctx context.Context, token string) (AgentSession, error)
}

type AgentSession struct {
	AuthorizationID uuid.UUID
	AdminUserID     *uuid.UUID
	ClientID        string
	ClientName      string
	AllProjects     bool
	ProjectIDs      map[uuid.UUID]bool
	ProjectKeys     map[string]uuid.UUID
	Scopes          map[string]bool
}

type agentSessionContextKey struct{}

func ContextWithAgentSession(ctx context.Context, session AgentSession) context.Context {
	return context.WithValue(ctx, agentSessionContextKey{}, session)
}

func AgentSessionFromContext(ctx context.Context) (AgentSession, bool) {
	session, ok := ctx.Value(agentSessionContextKey{}).(AgentSession)
	return session, ok
}

func (s AgentSession) CanAccessProject(projectKey string) bool {
	if s.AllProjects {
		return true
	}
	_, ok := s.ProjectKeys[strings.TrimSpace(projectKey)]
	return ok
}

func (s AgentSession) HasScope(scope string) bool {
	return s.Scopes[scope]
}

type agentAuthService struct {
	queries *dbq.Queries
}

func NewAgentAuthService(pool db.Pool) AgentAuth {
	return &agentAuthService{queries: dbq.New(pool)}
}

func (s *agentAuthService) ValidateAgentToken(ctx context.Context, token string) (AgentSession, error) {
	if !strings.HasPrefix(token, AgentTokenPrefix) {
		return AgentSession{}, ErrAgentAuthFailed
	}
	hash := HashToken(token)
	row, err := s.queries.MCPValidateAgentToken(ctx, pgtype.Text{String: hash, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AgentSession{}, ErrAgentAuthFailed
		}
		return AgentSession{}, err
	}
	session := AgentSession{
		AuthorizationID: row.ID,
		ClientID:        row.ClientID,
		ClientName:      row.ClientName,
		AllProjects:     row.AllProjects,
		ProjectIDs:      map[uuid.UUID]bool{},
		ProjectKeys:     map[string]uuid.UUID{},
		Scopes:          map[string]bool{},
	}
	if row.CreatedByAdminUserID.Valid {
		id := uuid.UUID(row.CreatedByAdminUserID.Bytes)
		session.AdminUserID = &id
	}
	for _, scope := range row.Scopes {
		session.Scopes[scope] = true
	}
	if !row.AllProjects {
		projects, err := s.queries.MCPListAgentAuthorizationProjects(ctx, row.ID)
		if err != nil {
			return AgentSession{}, err
		}
		for _, project := range projects {
			session.ProjectIDs[project.ID] = true
			session.ProjectKeys[project.ProjectKey] = project.ID
		}
	}
	return session, nil
}

func NewAgentToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := AgentTokenPrefix + base64.RawURLEncoding.EncodeToString(raw[:])
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
