package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/db/dbq"
)

type Admin interface {
	ListProjects(ctx context.Context) ([]ProjectSummary, error)
	CreateProject(ctx context.Context, req CreateProjectRequest) (ProjectSettings, error)
	UpdateProject(ctx context.Context, req CreateProjectRequest) (ProjectSettings, error)
	Summary(ctx context.Context, filter TimeProjectFilter) (SummaryResponse, error)
	ListEvents(ctx context.Context, filter EventListFilter) ([]EventSummary, error)
	PlayerTrace(ctx context.Context, projectKey string, playerID string, limit int32) ([]TraceEvent, error)
	RegionHeatmap(ctx context.Context, filter HeatmapFilter) ([]RegionHeatmapCell, error)
	ZoneHeatmap(ctx context.Context, filter ZoneHeatmapFilter) ([]ZoneHeatmapCell, error)
	Funnels(ctx context.Context, filter TimeProjectFilter) (FunnelsResponse, error)
	ListReports(ctx context.Context, filter ReportListFilter) ([]ReportSummary, error)
	GetReport(ctx context.Context, projectKey string, reportID string) (ReportDetail, error)
	ReportScreenshot(ctx context.Context, projectKey string, reportID string) (ScreenshotReadResult, error)
	UpdateReport(ctx context.Context, projectKey string, reportID string, req UpdateReportRequest) (ReportUpdateResponse, error)
	EventTypes(ctx context.Context, projectKey string) ([]EventTypeSummary, error)
	RejectedEvents(ctx context.Context, projectKey string) (RejectedEventsResponse, error)
	RejectedEventCount(ctx context.Context, projectKey string) (int64, error)
	AcknowledgeRejectedEvents(ctx context.Context, projectKey string) error
	Settings(ctx context.Context, projectKey string) (SettingsResponse, error)
	CreateIngestToken(ctx context.Context, projectKey string, req CreateIngestTokenRequest) (CreateIngestTokenResponse, error)
	SetIngestTokenEnabled(ctx context.Context, projectKey string, tokenID string, enabled bool) (IngestTokenSummary, error)
	ListAdminUsers(ctx context.Context) ([]AdminUserSummary, error)
	SetAdminUserEnabled(ctx context.Context, userID string, enabled bool) (AdminUserSummary, error)
	ListAdminInvitations(ctx context.Context) ([]AdminInvitationSummary, error)
	CreateAdminInvitation(ctx context.Context, req CreateAdminInvitationRequest) (CreateAdminInvitationResponse, error)
	DeleteAdminInvitation(ctx context.Context, invitationID string) (AdminInvitationSummary, error)
	ListAgentAuthorizations(ctx context.Context) ([]AgentAuthorizationSummary, error)
	SetAgentAuthorizationEnabled(ctx context.Context, authorizationID string, enabled bool) (AgentAuthorizationSummary, error)
}

const (
	TelemetryTokenPrefix = "fr_tel_"
	AgentTokenPrefix     = "fr_agnt_"
)

type TimeProjectFilter struct {
	ProjectID string
	From      time.Time
	To        time.Time
}

type EventListFilter struct {
	TimeProjectFilter
	EventType    *string
	RegionID     *string
	ZoneID       *string
	PlayerID     *string
	GameVersion  *string
	BuildChannel *string
	FieldKey     *string
	FieldValue   *string
	Limit        int32
	Offset       int32
}

type HeatmapFilter struct {
	TimeProjectFilter
	EventType    *string
	GameVersion  *string
	BuildChannel *string
	FieldKey     *string
	FieldValue   *string
}

type ZoneHeatmapFilter struct {
	HeatmapFilter
	RegionID string
	ZoneID   *string
	CellM    float64
}

type ReportListFilter struct {
	ProjectID string
	Status    *string
	Label     *string
	Limit     int32
	Offset    int32
}

type SummaryResponse struct {
	EventCount   int64  `json:"event_count"`
	PlayerCount  int64  `json:"player_count"`
	SessionCount int64  `json:"session_count"`
	DeathCount   int64  `json:"death_count"`
	ReportCount  int64  `json:"report_count"`
	OptInCount   int64  `json:"opt_in_count"` // reserved; AdminSummary query does not return this — always 0
	From         string `json:"from"`
	To           string `json:"to"`
}

type EventSummary struct {
	ID               string          `json:"id"`
	PlayerID         string          `json:"player_id"`
	EventType        string          `json:"event_type"`
	RealTS           string          `json:"real_ts"`
	GameTime         int64           `json:"game_time"`
	RegionID         string          `json:"region_id"`
	ZoneID           string          `json:"zone_id"`
	Coordinates      []float64       `json:"coordinates"`
	GameVersion      string          `json:"game_version"`
	BuildChannel     string          `json:"build_channel"`
	CommitSHA        string          `json:"commit_sha"`
	Platform         string          `json:"platform"`
	Context          json.RawMessage `json:"context"`
	Metrics          json.RawMessage `json:"metrics"`
	Dimensions       json.RawMessage `json:"dimensions"`
	Payload          json.RawMessage `json:"payload"`
	EventJSON        json.RawMessage `json:"event_json"`
	Fields           json.RawMessage `json:"fields"`
	ValidationErrors json.RawMessage `json:"validation_errors"`
}

type TraceEvent struct {
	ID          string          `json:"id"`
	EventType   string          `json:"event_type"`
	RealTS      string          `json:"real_ts"`
	GameTime    int64           `json:"game_time"`
	RegionID    string          `json:"region_id"`
	ZoneID      string          `json:"zone_id"`
	Coordinates []float64       `json:"coordinates,omitempty"`
	Context     json.RawMessage `json:"context"`
	Metrics     json.RawMessage `json:"metrics"`
	Dimensions  json.RawMessage `json:"dimensions"`
	Fields      json.RawMessage `json:"fields"`
	Payload     json.RawMessage `json:"payload"`
}

type RegionHeatmapCell struct {
	RegionID   string `json:"region_id"`
	EventType  string `json:"event_type"`
	EventCount int64  `json:"event_count"`
}

type ZoneHeatmapCell struct {
	RegionID   string `json:"region_id"`
	ZoneID     string `json:"zone_id"`
	GridX      int64  `json:"grid_x"`
	GridZ      int64  `json:"grid_z"`
	EventType  string `json:"event_type"`
	EventCount int64  `json:"event_count"`
}

type FunnelSummary struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Entity      string              `json:"entity"`
	Started     int64               `json:"started"`
	Completed   int64               `json:"completed"`
	Rate        float64             `json:"rate"`
	Dropoff     string              `json:"dropoff"`
	Steps       []FunnelStepSummary `json:"steps"`
}

type FunnelStepSummary struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Count int64   `json:"count"`
	Rate  float64 `json:"rate"`
}

type FunnelDefinition struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Entity      string             `json:"entity"`
	Mode        string             `json:"mode,omitempty"`
	Enabled     *bool              `json:"enabled,omitempty"`
	Steps       []FunnelStepConfig `json:"steps"`
}

type FunnelStepConfig struct {
	ID            string             `json:"id"`
	Label         string             `json:"label"`
	Match         FunnelEventMatcher `json:"match"`
	After         string             `json:"after,omitempty"`
	WithinSeconds *int64             `json:"within_seconds,omitempty"`
}

type FunnelEventMatcher struct {
	EventType  string          `json:"event_type,omitempty"`
	EventTypes []string        `json:"event_types,omitempty"`
	FieldKey   string          `json:"field_key,omitempty"`
	FieldValue json.RawMessage `json:"field_value,omitempty"`
	RegionID   string          `json:"region_id,omitempty"`
	ZoneID     string          `json:"zone_id,omitempty"`
}

type FunnelsResponse struct {
	Funnels []FunnelSummary `json:"funnels"`
}

type ReportSummary struct {
	ReportID            string          `json:"report_id"`
	Status              string          `json:"status"`
	Labels              []string        `json:"labels"`
	Mood                int32           `json:"mood"`
	MoodLabel           string          `json:"mood_label"`
	NotesPreview        string          `json:"notes_preview"`
	ScreenshotObjectKey string          `json:"screenshot_object_key,omitempty"`
	CreatedAt           string          `json:"created_at"`
	PlayerID            string          `json:"player_id"`
	RealTS              string          `json:"real_ts"`
	GameTime            int64           `json:"game_time"`
	RegionID            string          `json:"region_id"`
	ZoneID              string          `json:"zone_id"`
	Context             json.RawMessage `json:"context"`
	Metrics             json.RawMessage `json:"metrics"`
	Dimensions          json.RawMessage `json:"dimensions"`
	Payload             json.RawMessage `json:"payload"`
}

type ReportDetail struct {
	ReportSummary
	ScreenshotStorageError string       `json:"screenshot_storage_error,omitempty"`
	Coordinates            []float64    `json:"coordinates"`
	Trace                  []TraceEvent `json:"trace"`
	Notes                  []ReportNote `json:"notes"`
}

type ReportNote struct {
	ID        string `json:"id"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

type UpdateReportRequest struct {
	Status string   `json:"status"`
	Labels []string `json:"labels"`
	Note   string   `json:"note"`
}

type ReportUpdateResponse struct {
	ReportID  string   `json:"report_id"`
	Status    string   `json:"status"`
	Labels    []string `json:"labels"`
	UpdatedAt string   `json:"updated_at"`
}

type EventTypeSummary struct {
	EventType     string          `json:"event_type"`
	EventCount    int64           `json:"event_count"`
	LastSeenAt    string          `json:"last_seen_at"`
	SamplePayload json.RawMessage `json:"sample_payload"`
}

// RejectedEventsResponse powers the Data Quality view: rejection groups plus the
// count of groups active in the last 24h that are newer than the last
// acknowledgement (the nav badge value).
type RejectedEventsResponse struct {
	Groups           []RejectedEventGroup `json:"groups"`
	ActiveGroupCount int64                `json:"active_group_count"`
}

type RejectedEventGroup struct {
	EventType     string          `json:"event_type"`
	ReasonCode    string          `json:"reason_code"`
	ReasonMessage string          `json:"reason_message"`
	GameVersion   string          `json:"game_version"`
	BuildChannel  string          `json:"build_channel"`
	EventCount    int64           `json:"event_count"`
	FirstSeenAt   string          `json:"first_seen_at"`
	LastSeenAt    string          `json:"last_seen_at"`
	SampleEvent   json.RawMessage `json:"sample_event"`
}

type SettingsResponse struct {
	Project ProjectSettings      `json:"project"`
	Tokens  []IngestTokenSummary `json:"tokens"`
}

type ProjectSummary struct {
	ProjectID      string `json:"project_id"`
	DisplayName    string `json:"display_name"`
	ValidationMode string `json:"validation_mode"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type ProjectSettings struct {
	ProjectID       string                 `json:"project_id"`
	DisplayName     string                 `json:"display_name"`
	ValidationMode  string                 `json:"validation_mode"`
	IngestConfig    ProjectIngestConfig    `json:"ingest_config"`
	RetentionConfig ProjectRetentionConfig `json:"retention_config"`
	MapConfig       ProjectMapConfig       `json:"map_config"`
	ReportConfig    ProjectReportConfig    `json:"report_config"`
	EventGroups     map[string][]string    `json:"event_groups"`
	QueryFields     []QueryFieldDefinition `json:"query_fields"`
	Funnels         []FunnelDefinition     `json:"funnels"`
}

type CreateProjectRequest struct {
	ProjectID       string                      `json:"project_id"`
	DisplayName     string                      `json:"display_name"`
	ValidationMode  string                      `json:"validation_mode"`
	IngestConfig    ProjectIngestConfigInput    `json:"ingest_config"`
	RetentionConfig ProjectRetentionConfigInput `json:"retention_config"`
	MapConfig       ProjectMapConfigInput       `json:"map_config"`
	ReportConfig    ProjectReportConfigInput    `json:"report_config"`
	EventGroups     map[string][]string         `json:"event_groups"`
	QueryFields     []QueryFieldDefinition      `json:"query_fields"`
	Funnels         []FunnelDefinition          `json:"funnels"`
}

type ProjectIngestConfigInput struct {
	MaxEventsPerBatch       *int  `json:"max_events_per_batch,omitempty"`
	AcceptGzip              *bool `json:"accept_gzip,omitempty"`
	AllowUnknownEventTypes  *bool `json:"allow_unknown_event_types,omitempty"`
	AllowScreenshotFailures *bool `json:"allow_screenshot_failures,omitempty"`
}

type ProjectIngestConfig struct {
	MaxEventsPerBatch       int  `json:"max_events_per_batch"`
	AcceptGzip              bool `json:"accept_gzip"`
	AllowUnknownEventTypes  bool `json:"allow_unknown_event_types"`
	AllowScreenshotFailures bool `json:"allow_screenshot_failures"`
}

type ProjectRetentionConfigInput struct {
	EventDays     *int `json:"event_days,omitempty"`
	ReportDays    *int `json:"report_days,omitempty"`
	AccessLogDays *int `json:"access_log_days,omitempty"`
}

type ProjectRetentionConfig struct {
	EventDays     int `json:"event_days"`
	ReportDays    int `json:"report_days"`
	AccessLogDays int `json:"access_log_days"`
}

type ProjectMapConfigInput struct {
	SpatialEnabled   *bool `json:"spatial_enabled,omitempty"`
	ZoneExtentM      *int  `json:"zone_extent_m,omitempty"`
	ZoneHeatmapCellM *int  `json:"zone_heatmap_cell_m,omitempty"`
}

type ProjectMapConfig struct {
	SpatialEnabled   bool `json:"spatial_enabled"`
	ZoneExtentM      int  `json:"zone_extent_m"`
	ZoneHeatmapCellM int  `json:"zone_heatmap_cell_m"`
}

type ProjectReportConfigInput struct {
	Statuses         []string `json:"statuses,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	RateLimitSeconds *int     `json:"rate_limit_seconds,omitempty"`
}

type ProjectReportConfig struct {
	Statuses         []string `json:"statuses"`
	Labels           []string `json:"labels"`
	RateLimitSeconds int      `json:"rate_limit_seconds"`
}

type IngestTokenSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type CreateIngestTokenRequest struct {
	Name string `json:"name"`
}

// CreateIngestTokenResponse is returned exactly once when a token is created.
// Token is the raw plaintext value — only the SHA-256 hash is stored in the
// database. If the caller loses this value, a new token must be issued.
type CreateIngestTokenResponse struct {
	Token   string             `json:"token"`
	Summary IngestTokenSummary `json:"summary"`
}

type AdminUserSummary struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Picture     string `json:"picture"`
	Role        string `json:"role"`
	Enabled     bool   `json:"enabled"`
	Provider    string `json:"provider"`
	LastLoginAt string `json:"last_login_at,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type AdminInvitationSummary struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	ExpiresAt      string `json:"expires_at"`
	CreatedAt      string `json:"created_at"`
	CreatedByEmail string `json:"created_by_email,omitempty"`
}

type CreateAdminInvitationRequest struct {
	Email string `json:"email"`
}

type CreateAdminInvitationResponse struct {
	Invitation AdminInvitationSummary `json:"invitation"`
	Token      string                 `json:"token"`
}

type AgentAuthorizationSummary struct {
	ID                   string   `json:"id"`
	ClientID             string   `json:"client_id"`
	ClientName           string   `json:"client_name"`
	CreatedByAdminUserID string   `json:"created_by_admin_user_id,omitempty"`
	CreatedByEmail       string   `json:"created_by_email,omitempty"`
	AllProjects          bool     `json:"all_projects"`
	ProjectKeys          []string `json:"project_keys"`
	Scopes               []string `json:"scopes"`
	Enabled              bool     `json:"enabled"`
	ExpiresAt            string   `json:"expires_at"`
	ActivatedAt          string   `json:"activated_at,omitempty"`
	LastUsedAt           string   `json:"last_used_at,omitempty"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

type adminService struct {
	queries         *dbq.Queries
	pool            db.Pool
	screenshotStore ScreenshotStore
	allowedDomains  map[string]struct{}
}

func NewAdminService(pool db.Pool, screenshotStore ScreenshotStore, allowedDomains ...string) Admin {
	rawAllowedDomains := ""
	if len(allowedDomains) > 0 {
		rawAllowedDomains = allowedDomains[0]
	}
	return &adminService{
		queries:         dbq.New(pool),
		pool:            pool,
		screenshotStore: screenshotStore,
		allowedDomains:  parseAllowedDomains(rawAllowedDomains),
	}
}

func (s *adminService) ListProjects(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := s.queries.AdminListProjects(ctx)
	if err != nil {
		return nil, err
	}
	projects := make([]ProjectSummary, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, ProjectSummary{
			ProjectID:      row.ProjectKey,
			DisplayName:    row.DisplayName,
			ValidationMode: row.ValidationMode,
			CreatedAt:      formatTime(row.CreatedAt),
			UpdatedAt:      formatTime(row.UpdatedAt),
		})
	}
	return projects, nil
}

func (s *adminService) CreateProject(ctx context.Context, req CreateProjectRequest) (ProjectSettings, error) {
	return s.upsertProject(ctx, req)
}

func (s *adminService) UpdateProject(ctx context.Context, req CreateProjectRequest) (ProjectSettings, error) {
	projectKey := strings.TrimSpace(req.ProjectID)
	if projectKey == "" {
		return ProjectSettings{}, fmt.Errorf("%w: project_id is required", ErrBadRequest)
	}
	if _, err := s.loadProject(ctx, projectKey); err != nil {
		return ProjectSettings{}, err
	}
	return s.upsertProject(ctx, req)
}

func (s *adminService) upsertProject(ctx context.Context, req CreateProjectRequest) (ProjectSettings, error) {
	params, err := createProjectParams(req)
	if err != nil {
		return ProjectSettings{}, err
	}
	row, err := s.queries.AdminUpsertProject(ctx, params)
	if err != nil {
		return ProjectSettings{}, err
	}
	return projectSettingsFromRaw(rawProjectSettingsFromUpsertRow(row)), nil
}

func (s *adminService) Summary(ctx context.Context, filter TimeProjectFilter) (SummaryResponse, error) {
	project, err := s.loadProject(ctx, filter.ProjectID)
	if err != nil {
		return SummaryResponse{}, err
	}
	row, err := s.queries.AdminSummary(ctx, dbq.AdminSummaryParams{
		ProjectID:   project.ID,
		CreatedAt:   filter.From,
		CreatedAt_2: filter.To,
	})
	if err != nil {
		return SummaryResponse{}, err
	}
	return SummaryResponse{
		EventCount:   row.EventCount,
		PlayerCount:  row.PlayerCount,
		SessionCount: row.SessionCount,
		DeathCount:   row.DeathCount,
		ReportCount:  row.ReportCount,
		From:         formatTime(filter.From),
		To:           formatTime(filter.To),
	}, nil
}

func (s *adminService) ListEvents(ctx context.Context, filter EventListFilter) ([]EventSummary, error) {
	project, err := s.loadProject(ctx, filter.ProjectID)
	if err != nil {
		return nil, err
	}
	playerID, err := optionalUUIDPtr(filter.PlayerID)
	if err != nil {
		return nil, err
	}
	fieldFilter, hasFieldFilter, err := projectFieldFilter(project.QueryFields, filter.FieldKey, filter.FieldValue)
	if err != nil {
		return nil, err
	}
	if hasFieldFilter {
		rows, err := s.queries.AdminListEventsByField(ctx, dbq.AdminListEventsByFieldParams{
			ProjectID:        project.ID,
			RealTs:           filter.From,
			RealTs_2:         filter.To,
			Limit:            clampLimit(filter.Limit, 100),
			Offset:           maxInt32(filter.Offset, 0),
			FieldKey:         fieldFilter.Key,
			FieldValueType:   fieldFilter.ValueType,
			HasFieldValue:    fieldFilter.HasValue,
			FieldStringValue: fieldFilter.StringValue,
			FieldNumberValue: fieldFilter.NumberValue,
			FieldBoolValue:   fieldFilter.BoolValue,
			EventType:        optionalTextPtr(filter.EventType),
			RegionID:         optionalTextPtr(filter.RegionID),
			ZoneID:           optionalTextPtr(filter.ZoneID),
			PlayerID:         playerID,
			GameVersion:      optionalTextPtr(filter.GameVersion),
			BuildChannel:     optionalTextPtr(filter.BuildChannel),
		})
		if err != nil {
			return nil, err
		}
		out := make([]EventSummary, 0, len(rows))
		for _, row := range rows {
			out = append(out, EventSummary{
				ID:               row.ID.String(),
				PlayerID:         row.PlayerID.String(),
				EventType:        row.EventType,
				RealTS:           formatTime(row.RealTs),
				GameTime:         row.GameTime,
				RegionID:         row.RegionID,
				ZoneID:           row.ZoneID,
				Coordinates:      []float64{row.CoordX, row.CoordY, row.CoordZ},
				GameVersion:      row.GameVersion,
				BuildChannel:     row.BuildChannel,
				CommitSHA:        row.CommitSHA,
				Platform:         row.Platform,
				Context:          row.Context,
				Metrics:          row.Metrics,
				Dimensions:       row.Dimensions,
				Payload:          row.Payload,
				EventJSON:        row.EventJson,
				Fields:           row.Fields,
				ValidationErrors: row.ValidationErrors,
			})
		}
		return out, nil
	}
	rows, err := s.queries.AdminListEvents(ctx, dbq.AdminListEventsParams{
		ProjectID:    project.ID,
		RealTs:       filter.From,
		RealTs_2:     filter.To,
		Limit:        clampLimit(filter.Limit, 100),
		Offset:       maxInt32(filter.Offset, 0),
		EventType:    optionalTextPtr(filter.EventType),
		RegionID:     optionalTextPtr(filter.RegionID),
		ZoneID:       optionalTextPtr(filter.ZoneID),
		PlayerID:     playerID,
		GameVersion:  optionalTextPtr(filter.GameVersion),
		BuildChannel: optionalTextPtr(filter.BuildChannel),
	})
	if err != nil {
		return nil, err
	}
	out := make([]EventSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, EventSummary{
			ID:               row.ID.String(),
			PlayerID:         row.PlayerID.String(),
			EventType:        row.EventType,
			RealTS:           formatTime(row.RealTs),
			GameTime:         row.GameTime,
			RegionID:         row.RegionID,
			ZoneID:           row.ZoneID,
			Coordinates:      []float64{row.CoordX, row.CoordY, row.CoordZ},
			GameVersion:      row.GameVersion,
			BuildChannel:     row.BuildChannel,
			CommitSHA:        row.CommitSHA,
			Platform:         row.Platform,
			Context:          row.Context,
			Metrics:          row.Metrics,
			Dimensions:       row.Dimensions,
			Payload:          row.Payload,
			EventJSON:        row.EventJson,
			Fields:           row.Fields,
			ValidationErrors: row.ValidationErrors,
		})
	}
	return out, nil
}

func (s *adminService) PlayerTrace(ctx context.Context, projectKey string, playerID string, limit int32) ([]TraceEvent, error) {
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return nil, err
	}
	parsedPlayerID, err := uuid.Parse(playerID)
	if err != nil {
		return nil, fmt.Errorf("%w: player_id must be a UUID", ErrBadRequest)
	}
	rows, err := s.queries.AdminPlayerTrace(ctx, dbq.AdminPlayerTraceParams{
		ProjectID: project.ID,
		PlayerID:  parsedPlayerID,
		Limit:     clampLimit(limit, 500),
	})
	if err != nil {
		return nil, err
	}
	out := make([]TraceEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, TraceEvent{
			ID:          row.ID.String(),
			EventType:   row.EventType,
			RealTS:      formatTime(row.RealTs),
			GameTime:    row.GameTime,
			RegionID:    row.RegionID,
			ZoneID:      row.ZoneID,
			Coordinates: []float64{row.CoordX, row.CoordY, row.CoordZ},
			Context:     row.Context,
			Metrics:     row.Metrics,
			Dimensions:  row.Dimensions,
			Fields:      row.Fields,
			Payload:     row.Payload,
		})
	}
	return out, nil
}

func (s *adminService) RegionHeatmap(ctx context.Context, filter HeatmapFilter) ([]RegionHeatmapCell, error) {
	project, err := s.loadProject(ctx, filter.ProjectID)
	if err != nil {
		return nil, err
	}
	fieldFilter, hasFieldFilter, err := projectFieldFilter(project.QueryFields, filter.FieldKey, filter.FieldValue)
	if err != nil {
		return nil, err
	}
	if hasFieldFilter {
		rows, err := s.queries.AdminRegionHeatmapByField(ctx, dbq.AdminRegionHeatmapByFieldParams{
			ProjectID:        project.ID,
			RealTs:           filter.From,
			RealTs_2:         filter.To,
			FieldKey:         fieldFilter.Key,
			FieldValueType:   fieldFilter.ValueType,
			HasFieldValue:    fieldFilter.HasValue,
			FieldStringValue: fieldFilter.StringValue,
			FieldNumberValue: fieldFilter.NumberValue,
			FieldBoolValue:   fieldFilter.BoolValue,
			EventType:        optionalTextPtr(filter.EventType),
			GameVersion:      optionalTextPtr(filter.GameVersion),
			BuildChannel:     optionalTextPtr(filter.BuildChannel),
		})
		if err != nil {
			return nil, err
		}
		out := make([]RegionHeatmapCell, 0, len(rows))
		for _, row := range rows {
			out = append(out, RegionHeatmapCell{
				RegionID:   row.RegionID,
				EventType:  row.EventType,
				EventCount: row.EventCount,
			})
		}
		return out, nil
	}
	rows, err := s.queries.AdminRegionHeatmap(ctx, dbq.AdminRegionHeatmapParams{
		ProjectID:    project.ID,
		RealTs:       filter.From,
		RealTs_2:     filter.To,
		EventType:    optionalTextPtr(filter.EventType),
		GameVersion:  optionalTextPtr(filter.GameVersion),
		BuildChannel: optionalTextPtr(filter.BuildChannel),
	})
	if err != nil {
		return nil, err
	}
	out := make([]RegionHeatmapCell, 0, len(rows))
	for _, row := range rows {
		out = append(out, RegionHeatmapCell{
			RegionID:   row.RegionID,
			EventType:  row.EventType,
			EventCount: row.EventCount,
		})
	}
	return out, nil
}

func (s *adminService) ZoneHeatmap(ctx context.Context, filter ZoneHeatmapFilter) ([]ZoneHeatmapCell, error) {
	if filter.RegionID == "" {
		return nil, fmt.Errorf("%w: region_id is required", ErrBadRequest)
	}
	project, err := s.loadProject(ctx, filter.ProjectID)
	if err != nil {
		return nil, err
	}
	cellM := filter.CellM
	if cellM <= 0 {
		cellM = 300
	}
	fieldFilter, hasFieldFilter, err := projectFieldFilter(project.QueryFields, filter.FieldKey, filter.FieldValue)
	if err != nil {
		return nil, err
	}
	if hasFieldFilter {
		rows, err := s.queries.AdminZoneHeatmapByField(ctx, dbq.AdminZoneHeatmapByFieldParams{
			ProjectID:        project.ID,
			RealTs:           filter.From,
			RealTs_2:         filter.To,
			CoordX:           cellM,
			RegionID:         filter.RegionID,
			FieldKey:         fieldFilter.Key,
			FieldValueType:   fieldFilter.ValueType,
			HasFieldValue:    fieldFilter.HasValue,
			FieldStringValue: fieldFilter.StringValue,
			FieldNumberValue: fieldFilter.NumberValue,
			FieldBoolValue:   fieldFilter.BoolValue,
			ZoneID:           optionalTextPtr(filter.ZoneID),
			EventType:        optionalTextPtr(filter.EventType),
			GameVersion:      optionalTextPtr(filter.GameVersion),
			BuildChannel:     optionalTextPtr(filter.BuildChannel),
		})
		if err != nil {
			return nil, err
		}
		out := make([]ZoneHeatmapCell, 0, len(rows))
		for _, row := range rows {
			out = append(out, ZoneHeatmapCell{
				RegionID:   row.RegionID,
				ZoneID:     row.ZoneID,
				GridX:      row.GridX,
				GridZ:      row.GridZ,
				EventType:  row.EventType,
				EventCount: row.EventCount,
			})
		}
		return out, nil
	}
	rows, err := s.queries.AdminZoneHeatmap(ctx, dbq.AdminZoneHeatmapParams{
		ProjectID:    project.ID,
		RealTs:       filter.From,
		RealTs_2:     filter.To,
		CoordX:       cellM,
		RegionID:     filter.RegionID,
		ZoneID:       optionalTextPtr(filter.ZoneID),
		EventType:    optionalTextPtr(filter.EventType),
		GameVersion:  optionalTextPtr(filter.GameVersion),
		BuildChannel: optionalTextPtr(filter.BuildChannel),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ZoneHeatmapCell, 0, len(rows))
	for _, row := range rows {
		out = append(out, ZoneHeatmapCell{
			RegionID:   row.RegionID,
			ZoneID:     row.ZoneID,
			GridX:      row.GridX,
			GridZ:      row.GridZ,
			EventType:  row.EventType,
			EventCount: row.EventCount,
		})
	}
	return out, nil
}

func (s *adminService) Funnels(ctx context.Context, filter TimeProjectFilter) (FunnelsResponse, error) {
	project, err := s.loadProject(ctx, filter.ProjectID)
	if err != nil {
		return FunnelsResponse{}, err
	}
	defs, err := projectFunnels(project.Funnels)
	if err != nil {
		return FunnelsResponse{}, err
	}
	fieldList, err := projectQueryFields(project.QueryFields)
	if err != nil {
		return FunnelsResponse{}, err
	}
	if err := validateProjectFunnels(fieldList, defs); err != nil {
		return FunnelsResponse{}, err
	}
	fieldDefs, err := queryFieldMap(fieldList)
	if err != nil {
		return FunnelsResponse{}, err
	}
	out := make([]FunnelSummary, 0, len(defs))
	for _, def := range defs {
		if def.Enabled != nil && !*def.Enabled {
			continue
		}
		summary, err := s.evaluateFunnel(ctx, project.ID, filter.From, filter.To, def, fieldDefs)
		if err != nil {
			return FunnelsResponse{}, err
		}
		out = append(out, summary)
	}
	return FunnelsResponse{Funnels: out}, nil
}

func (s *adminService) ListReports(ctx context.Context, filter ReportListFilter) ([]ReportSummary, error) {
	project, err := s.loadProject(ctx, filter.ProjectID)
	if err != nil {
		return nil, err
	}
	if filter.Label != nil && strings.TrimSpace(*filter.Label) != "" {
		rows, err := s.queries.AdminListReportsByLabel(ctx, dbq.AdminListReportsByLabelParams{
			ProjectID: project.ID,
			Limit:     clampLimit(filter.Limit, 100),
			Offset:    maxInt32(filter.Offset, 0),
			Label:     strings.TrimSpace(*filter.Label),
			Status:    optionalTextPtr(filter.Status),
		})
		if err != nil {
			return nil, err
		}
		out := make([]ReportSummary, 0, len(rows))
		for _, row := range rows {
			out = append(out, ReportSummary{
				ReportID:            row.ReportID,
				Status:              row.Status,
				Labels:              row.Labels,
				Mood:                row.Mood,
				MoodLabel:           row.MoodLabel,
				NotesPreview:        row.NotesPreview,
				ScreenshotObjectKey: textValue(row.ScreenshotObjectKey),
				CreatedAt:           formatTime(row.CreatedAt),
				PlayerID:            row.PlayerID.String(),
				RealTS:              formatTime(row.RealTs),
				GameTime:            row.GameTime,
				RegionID:            row.RegionID,
				ZoneID:              row.ZoneID,
				Context:             row.Context,
				Metrics:             row.Metrics,
				Dimensions:          row.Dimensions,
				Payload:             row.Payload,
			})
		}
		return out, nil
	}
	rows, err := s.queries.AdminListReports(ctx, dbq.AdminListReportsParams{
		ProjectID: project.ID,
		Limit:     clampLimit(filter.Limit, 100),
		Offset:    maxInt32(filter.Offset, 0),
		Status:    optionalTextPtr(filter.Status),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ReportSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReportSummary{
			ReportID:            row.ReportID,
			Status:              row.Status,
			Labels:              row.Labels,
			Mood:                row.Mood,
			MoodLabel:           row.MoodLabel,
			NotesPreview:        row.NotesPreview,
			ScreenshotObjectKey: textValue(row.ScreenshotObjectKey),
			CreatedAt:           formatTime(row.CreatedAt),
			PlayerID:            row.PlayerID.String(),
			RealTS:              formatTime(row.RealTs),
			GameTime:            row.GameTime,
			RegionID:            row.RegionID,
			ZoneID:              row.ZoneID,
			Context:             row.Context,
			Metrics:             row.Metrics,
			Dimensions:          row.Dimensions,
			Payload:             row.Payload,
		})
	}
	return out, nil
}

func (s *adminService) GetReport(ctx context.Context, projectKey string, reportID string) (ReportDetail, error) {
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return ReportDetail{}, err
	}
	row, err := s.queries.AdminGetReport(ctx, dbq.AdminGetReportParams{
		ProjectID: project.ID,
		ReportID:  reportID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReportDetail{}, fmt.Errorf("%w: report not found", ErrBadRequest)
		}
		return ReportDetail{}, err
	}
	traceRows, err := s.queries.AdminReportTrace(ctx, dbq.AdminReportTraceParams{
		ProjectID: project.ID,
		ReportID:  reportID,
		GameTime:  600, // look back 600 in-game seconds before the report event
	})
	if err != nil {
		return ReportDetail{}, err
	}
	trace := make([]TraceEvent, 0, len(traceRows))
	for _, traceRow := range traceRows {
		trace = append(trace, TraceEvent{
			ID:         traceRow.ID.String(),
			EventType:  traceRow.EventType,
			RealTS:     formatTime(traceRow.RealTs),
			GameTime:   traceRow.GameTime,
			RegionID:   traceRow.RegionID,
			ZoneID:     traceRow.ZoneID,
			Context:    traceRow.Context,
			Metrics:    traceRow.Metrics,
			Dimensions: traceRow.Dimensions,
			Payload:    traceRow.Payload,
		})
	}
	noteRows, err := s.queries.AdminListReportNotes(ctx, row.ID)
	if err != nil {
		return ReportDetail{}, err
	}
	notes := make([]ReportNote, 0, len(noteRows))
	for _, noteRow := range noteRows {
		notes = append(notes, ReportNote{
			ID:        noteRow.ID.String(),
			Note:      noteRow.Note,
			CreatedAt: formatTime(noteRow.CreatedAt),
		})
	}
	return ReportDetail{
		ReportSummary: ReportSummary{
			ReportID:            row.ReportID,
			Status:              row.Status,
			Labels:              row.Labels,
			Mood:                row.Mood,
			MoodLabel:           row.MoodLabel,
			NotesPreview:        row.NotesPreview,
			ScreenshotObjectKey: textValue(row.ScreenshotObjectKey),
			CreatedAt:           formatTime(row.CreatedAt),
			PlayerID:            row.PlayerID.String(),
			RealTS:              formatTime(row.RealTs),
			GameTime:            row.GameTime,
			RegionID:            row.RegionID,
			ZoneID:              row.ZoneID,
			Context:             row.Context,
			Metrics:             row.Metrics,
			Dimensions:          row.Dimensions,
			Payload:             row.Payload,
		},
		ScreenshotStorageError: textValue(row.ScreenshotStorageError),
		Coordinates:            []float64{row.CoordX, row.CoordY, row.CoordZ},
		Trace:                  trace,
		Notes:                  notes,
	}, nil
}

func (s *adminService) ReportScreenshot(ctx context.Context, projectKey string, reportID string) (ScreenshotReadResult, error) {
	if s.screenshotStore == nil {
		return ScreenshotReadResult{}, fmt.Errorf("%w: screenshot storage is not configured", ErrBadRequest)
	}
	report, err := s.GetReport(ctx, projectKey, reportID)
	if err != nil {
		return ScreenshotReadResult{}, err
	}
	if strings.TrimSpace(report.ScreenshotObjectKey) == "" {
		return ScreenshotReadResult{}, fmt.Errorf("%w: report has no screenshot", ErrBadRequest)
	}
	return s.screenshotStore.ReadPNG(ctx, report.ScreenshotObjectKey)
}

func (s *adminService) UpdateReport(ctx context.Context, projectKey string, reportID string, req UpdateReportRequest) (ReportUpdateResponse, error) {
	if strings.TrimSpace(req.Status) == "" {
		return ReportUpdateResponse{}, fmt.Errorf("%w: status is required", ErrBadRequest)
	}
	if !validReportStatus(req.Status) {
		return ReportUpdateResponse{}, fmt.Errorf("%w: unsupported report status", ErrBadRequest)
	}
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return ReportUpdateResponse{}, err
	}
	labels := req.Labels
	if labels == nil {
		labels = []string{}
	}
	row, err := s.queries.AdminUpdateReport(ctx, dbq.AdminUpdateReportParams{
		ProjectID: project.ID,
		ReportID:  reportID,
		Status:    req.Status,
		Labels:    labels,
	})
	if err != nil {
		return ReportUpdateResponse{}, err
	}
	if strings.TrimSpace(req.Note) != "" {
		if _, err := s.queries.AdminCreateReportNote(ctx, dbq.AdminCreateReportNoteParams{
			ReportID: row.ID,
			Note:     req.Note,
		}); err != nil {
			return ReportUpdateResponse{}, err
		}
	}
	return ReportUpdateResponse{
		ReportID:  row.ReportID,
		Status:    row.Status,
		Labels:    row.Labels,
		UpdatedAt: formatTime(row.UpdatedAt),
	}, nil
}

func (s *adminService) EventTypes(ctx context.Context, projectKey string) ([]EventTypeSummary, error) {
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.AdminEventTypes(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	out := make([]EventTypeSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, EventTypeSummary{
			EventType:     row.EventType,
			EventCount:    row.EventCount,
			LastSeenAt:    formatTime(row.LastSeenAt),
			SamplePayload: row.SamplePayload,
		})
	}
	return out, nil
}

func (s *adminService) RejectedEvents(ctx context.Context, projectKey string) (RejectedEventsResponse, error) {
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return RejectedEventsResponse{}, err
	}
	rows, err := s.queries.AdminRejectedEventGroups(ctx, project.ID)
	if err != nil {
		return RejectedEventsResponse{}, err
	}
	count, err := s.queries.AdminCountActiveRejectionGroups(ctx, project.ID)
	if err != nil {
		return RejectedEventsResponse{}, err
	}
	groups := make([]RejectedEventGroup, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, RejectedEventGroup{
			EventType:     row.EventType,
			ReasonCode:    row.ReasonCode,
			ReasonMessage: row.ReasonMessage,
			GameVersion:   row.GameVersion,
			BuildChannel:  row.BuildChannel,
			EventCount:    row.EventCount,
			FirstSeenAt:   formatTime(row.FirstSeenAt),
			LastSeenAt:    formatTime(row.LastSeenAt),
			SampleEvent:   row.SampleEvent,
		})
	}
	return RejectedEventsResponse{Groups: groups, ActiveGroupCount: count}, nil
}

func (s *adminService) RejectedEventCount(ctx context.Context, projectKey string) (int64, error) {
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return 0, err
	}
	return s.queries.AdminCountActiveRejectionGroups(ctx, project.ID)
}

func (s *adminService) AcknowledgeRejectedEvents(ctx context.Context, projectKey string) error {
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return err
	}
	return s.queries.AdminAcknowledgeRejectedEvents(ctx, project.ID)
}

func (s *adminService) Settings(ctx context.Context, projectKey string) (SettingsResponse, error) {
	settings, err := s.queries.AdminProjectSettings(ctx, requiredProjectKey(projectKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SettingsResponse{}, fmt.Errorf("%w: unknown project_id", ErrBadRequest)
		}
		return SettingsResponse{}, err
	}
	tokenRows, err := s.queries.AdminListIngestTokens(ctx, settings.ID)
	if err != nil {
		return SettingsResponse{}, err
	}
	tokens := make([]IngestTokenSummary, 0, len(tokenRows))
	for _, row := range tokenRows {
		tokens = append(tokens, ingestTokenSummary(row.ID, row.Name, row.Enabled, row.ExpiresAt, row.LastUsedAt, row.CreatedAt))
	}
	return SettingsResponse{
		Project: projectSettingsFromRaw(rawProjectSettingsFromSettingsRow(settings)),
		Tokens:  tokens,
	}, nil
}

func (s *adminService) CreateIngestToken(ctx context.Context, projectKey string, req CreateIngestTokenRequest) (CreateIngestTokenResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return CreateIngestTokenResponse{}, fmt.Errorf("%w: token name is required", ErrBadRequest)
	}
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return CreateIngestTokenResponse{}, err
	}
	token, tokenHash, err := newIngestToken()
	if err != nil {
		return CreateIngestTokenResponse{}, err
	}
	row, err := s.queries.AdminCreateIngestToken(ctx, dbq.AdminCreateIngestTokenParams{
		ProjectID: project.ID,
		Name:      name,
		TokenHash: tokenHash,
	})
	if err != nil {
		return CreateIngestTokenResponse{}, err
	}
	return CreateIngestTokenResponse{
		Token:   token,
		Summary: ingestTokenSummary(row.ID, row.Name, row.Enabled, row.ExpiresAt, row.LastUsedAt, row.CreatedAt),
	}, nil
}

func (s *adminService) SetIngestTokenEnabled(ctx context.Context, projectKey string, tokenID string, enabled bool) (IngestTokenSummary, error) {
	project, err := s.loadProject(ctx, projectKey)
	if err != nil {
		return IngestTokenSummary{}, err
	}
	parsedTokenID, err := uuid.Parse(tokenID)
	if err != nil {
		return IngestTokenSummary{}, fmt.Errorf("%w: token_id must be a UUID", ErrBadRequest)
	}
	row, err := s.queries.AdminSetIngestTokenEnabled(ctx, dbq.AdminSetIngestTokenEnabledParams{
		ProjectID: project.ID,
		ID:        parsedTokenID,
		Enabled:   enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IngestTokenSummary{}, fmt.Errorf("%w: ingest token not found", ErrBadRequest)
		}
		return IngestTokenSummary{}, err
	}
	return ingestTokenSummary(row.ID, row.Name, row.Enabled, row.ExpiresAt, row.LastUsedAt, row.CreatedAt), nil
}

func (s *adminService) ListAdminUsers(ctx context.Context) ([]AdminUserSummary, error) {
	rows, err := s.queries.AdminListUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]AdminUserSummary, 0, len(rows))
	for _, row := range rows {
		users = append(users, adminUserSummaryFromListRow(row))
	}
	return users, nil
}

func (s *adminService) SetAdminUserEnabled(ctx context.Context, userID string, enabled bool) (AdminUserSummary, error) {
	parsedID, err := uuid.Parse(userID)
	if err != nil {
		return AdminUserSummary{}, fmt.Errorf("%w: user_id must be a UUID", ErrBadRequest)
	}
	if !enabled {
		if session, ok := AdminSessionFromContext(ctx); ok {
			currentUser, err := s.queries.AdminGetUserByEmail(ctx, session.Email)
			if err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					return AdminUserSummary{}, err
				}
			} else if currentUser.ID == parsedID {
				return AdminUserSummary{}, fmt.Errorf("%w: cannot disable your own admin user", ErrBadRequest)
			}
		}
		enabledCount, err := s.queries.AdminCountEnabledUsers(ctx)
		if err != nil {
			return AdminUserSummary{}, err
		}
		if enabledCount <= 1 {
			return AdminUserSummary{}, fmt.Errorf("%w: cannot disable the last enabled admin user", ErrBadRequest)
		}
	}
	row, err := s.queries.AdminSetUserEnabled(ctx, dbq.AdminSetUserEnabledParams{
		ID:      parsedID,
		Enabled: enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminUserSummary{}, fmt.Errorf("%w: admin user not found", ErrBadRequest)
		}
		return AdminUserSummary{}, err
	}
	return adminUserSummaryFromSetEnabledRow(row), nil
}

func (s *adminService) ListAdminInvitations(ctx context.Context) ([]AdminInvitationSummary, error) {
	rows, err := s.queries.AdminListActiveInvitations(ctx)
	if err != nil {
		return nil, err
	}
	invitations := make([]AdminInvitationSummary, 0, len(rows))
	for _, row := range rows {
		invitations = append(invitations, AdminInvitationSummary{
			ID:             row.ID.String(),
			Email:          row.Email,
			ExpiresAt:      formatTime(row.ExpiresAt),
			CreatedAt:      formatTime(row.CreatedAt),
			CreatedByEmail: textValue(row.CreatedByEmail),
		})
	}
	return invitations, nil
}

func (s *adminService) CreateAdminInvitation(ctx context.Context, req CreateAdminInvitationRequest) (CreateAdminInvitationResponse, error) {
	email := normalizeEmail(req.Email)
	if email == "" || emailDomain(email) == "" {
		return CreateAdminInvitationResponse{}, fmt.Errorf("%w: valid email is required", ErrBadRequest)
	}
	if len(s.allowedDomains) > 0 {
		if _, ok := s.allowedDomains[emailDomain(email)]; !ok {
			return CreateAdminInvitationResponse{}, fmt.Errorf("%w: email domain is not allowed", ErrBadRequest)
		}
	}
	if _, err := s.queries.AdminGetUserByEmail(ctx, email); err == nil {
		return CreateAdminInvitationResponse{}, fmt.Errorf("%w: admin user already exists", ErrBadRequest)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return CreateAdminInvitationResponse{}, err
	}
	token, tokenHash, err := NewAdminInviteToken()
	if err != nil {
		return CreateAdminInvitationResponse{}, err
	}
	var createdBy pgtype.UUID
	if session, ok := AdminSessionFromContext(ctx); ok {
		if user, err := s.queries.AdminGetUserByEmail(ctx, session.Email); err == nil {
			createdBy = uuidParam(user.ID)
		}
	}
	row, err := s.queries.AdminCreateInvitation(ctx, dbq.AdminCreateInvitationParams{
		Email:                email,
		TokenHash:            tokenHash,
		CreatedByAdminUserID: createdBy,
	})
	if err != nil {
		return CreateAdminInvitationResponse{}, err
	}
	return CreateAdminInvitationResponse{
		Invitation: AdminInvitationSummary{
			ID:        row.ID.String(),
			Email:     row.Email,
			ExpiresAt: formatTime(row.ExpiresAt),
			CreatedAt: formatTime(row.CreatedAt),
		},
		Token: token,
	}, nil
}

func (s *adminService) DeleteAdminInvitation(ctx context.Context, invitationID string) (AdminInvitationSummary, error) {
	parsedID, err := uuid.Parse(invitationID)
	if err != nil {
		return AdminInvitationSummary{}, fmt.Errorf("%w: invitation_id must be a UUID", ErrBadRequest)
	}
	row, err := s.queries.AdminDeleteInvitation(ctx, parsedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminInvitationSummary{}, fmt.Errorf("%w: invitation not found", ErrBadRequest)
		}
		return AdminInvitationSummary{}, err
	}
	return AdminInvitationSummary{
		ID:        row.ID.String(),
		Email:     row.Email,
		ExpiresAt: formatTime(row.ExpiresAt),
		CreatedAt: formatTime(row.CreatedAt),
	}, nil
}

func (s *adminService) ListAgentAuthorizations(ctx context.Context) ([]AgentAuthorizationSummary, error) {
	rows, err := s.queries.AdminListAgentAuthorizations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AgentAuthorizationSummary, 0, len(rows))
	for _, row := range rows {
		summary, err := agentAuthorizationSummaryFromListRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}

func (s *adminService) SetAgentAuthorizationEnabled(ctx context.Context, authorizationID string, enabled bool) (AgentAuthorizationSummary, error) {
	parsedID, err := uuid.Parse(authorizationID)
	if err != nil {
		return AgentAuthorizationSummary{}, fmt.Errorf("%w: authorization_id must be a UUID", ErrBadRequest)
	}
	row, err := s.queries.AdminSetAgentAuthorizationEnabled(ctx, dbq.AdminSetAgentAuthorizationEnabledParams{
		ID:      parsedID,
		Enabled: enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AgentAuthorizationSummary{}, fmt.Errorf("%w: agent authorization not found", ErrBadRequest)
		}
		return AgentAuthorizationSummary{}, err
	}
	return agentAuthorizationSummaryFromSetEnabledRow(row), nil
}

func (s *adminService) loadProject(ctx context.Context, projectKey string) (dbq.GetProjectByKeyRow, error) {
	project, err := s.queries.GetProjectByKey(ctx, requiredProjectKey(projectKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbq.GetProjectByKeyRow{}, fmt.Errorf("%w: unknown project_id", ErrBadRequest)
		}
		return dbq.GetProjectByKeyRow{}, err
	}
	return project, nil
}

func DefaultTimeProjectFilter(projectKey string) TimeProjectFilter {
	to := time.Now().UTC()
	return TimeProjectFilter{
		ProjectID: projectKey,
		From:      to.Add(-30 * 24 * time.Hour),
		To:        to,
	}
}

func OptionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func requiredProjectKey(projectKey string) string {
	return strings.TrimSpace(projectKey)
}

type normalizedProjectConfig struct {
	IngestConfig    ProjectIngestConfig
	RetentionConfig ProjectRetentionConfig
	MapConfig       ProjectMapConfig
	ReportConfig    ProjectReportConfig
	EventGroups     map[string][]string
	QueryFields     []QueryFieldDefinition
	Funnels         []FunnelDefinition
}

type rawProjectSettings struct {
	ProjectKey      string
	DisplayName     string
	ValidationMode  string
	IngestConfig    json.RawMessage
	RetentionConfig json.RawMessage
	MapConfig       json.RawMessage
	ReportConfig    json.RawMessage
	EventGroups     json.RawMessage
	QueryFields     json.RawMessage
	Funnels         json.RawMessage
}

func rawProjectSettingsFromUpsertRow(row dbq.AdminUpsertProjectRow) rawProjectSettings {
	return rawProjectSettings{
		ProjectKey:      row.ProjectKey,
		DisplayName:     row.DisplayName,
		ValidationMode:  row.ValidationMode,
		IngestConfig:    row.IngestConfig,
		RetentionConfig: row.RetentionConfig,
		MapConfig:       row.MapConfig,
		ReportConfig:    row.ReportConfig,
		EventGroups:     row.EventGroups,
		QueryFields:     row.QueryFields,
		Funnels:         row.Funnels,
	}
}

func rawProjectSettingsFromSettingsRow(row dbq.AdminProjectSettingsRow) rawProjectSettings {
	return rawProjectSettings{
		ProjectKey:      row.ProjectKey,
		DisplayName:     row.DisplayName,
		ValidationMode:  row.ValidationMode,
		IngestConfig:    row.IngestConfig,
		RetentionConfig: row.RetentionConfig,
		MapConfig:       row.MapConfig,
		ReportConfig:    row.ReportConfig,
		EventGroups:     row.EventGroups,
		QueryFields:     row.QueryFields,
		Funnels:         row.Funnels,
	}
}

func projectSettingsFromRaw(row rawProjectSettings) ProjectSettings {
	return ProjectSettings{
		ProjectID:       row.ProjectKey,
		DisplayName:     row.DisplayName,
		ValidationMode:  row.ValidationMode,
		IngestConfig:    readIngestConfig(row.IngestConfig),
		RetentionConfig: readRetentionConfig(row.RetentionConfig),
		MapConfig:       readMapConfig(row.MapConfig),
		ReportConfig:    readReportConfig(row.ReportConfig),
		EventGroups:     readEventGroups(row.EventGroups),
		QueryFields:     readQueryFields(row.QueryFields),
		Funnels:         readFunnels(row.Funnels),
	}
}

func createProjectParams(req CreateProjectRequest) (dbq.AdminUpsertProjectParams, error) {
	projectKey := strings.TrimSpace(req.ProjectID)
	if projectKey == "" {
		return dbq.AdminUpsertProjectParams{}, fmt.Errorf("%w: project_id is required", ErrBadRequest)
	}
	if !validProjectKey(projectKey) {
		return dbq.AdminUpsertProjectParams{}, fmt.Errorf("%w: project_id must use lowercase letters, numbers, hyphens, or underscores", ErrBadRequest)
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		return dbq.AdminUpsertProjectParams{}, fmt.Errorf("%w: display_name is required", ErrBadRequest)
	}
	validationMode := strings.TrimSpace(req.ValidationMode)
	if validationMode == "" {
		validationMode = "warn"
	}
	if validationMode != "warn" && validationMode != "strict" {
		return dbq.AdminUpsertProjectParams{}, fmt.Errorf("%w: validation_mode must be warn or strict", ErrBadRequest)
	}

	config, err := normalizeProjectConfig(req)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	ingestConfig, err := marshalProjectJSON("ingest_config", config.IngestConfig)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	retentionConfig, err := marshalProjectJSON("retention_config", config.RetentionConfig)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	mapConfig, err := marshalProjectJSON("map_config", config.MapConfig)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	reportConfig, err := marshalProjectJSON("report_config", config.ReportConfig)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	eventGroups, err := marshalProjectJSON("event_groups", config.EventGroups)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	queryFields, err := marshalProjectJSON("query_fields", config.QueryFields)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	funnels, err := marshalProjectJSON("funnels", config.Funnels)
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}

	return dbq.AdminUpsertProjectParams{
		ProjectKey:      projectKey,
		DisplayName:     displayName,
		ValidationMode:  validationMode,
		IngestConfig:    ingestConfig,
		RetentionConfig: retentionConfig,
		MapConfig:       mapConfig,
		ReportConfig:    reportConfig,
		EventGroups:     eventGroups,
		QueryFields:     queryFields,
		Funnels:         funnels,
	}, nil
}

func validProjectKey(value string) bool {
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func normalizeProjectConfig(req CreateProjectRequest) (normalizedProjectConfig, error) {
	ingestConfig, err := normalizeIngestConfig(req.IngestConfig)
	if err != nil {
		return normalizedProjectConfig{}, err
	}
	retentionConfig, err := normalizeRetentionConfig(req.RetentionConfig)
	if err != nil {
		return normalizedProjectConfig{}, err
	}
	mapConfig, err := normalizeMapConfig(req.MapConfig)
	if err != nil {
		return normalizedProjectConfig{}, err
	}
	reportConfig, err := normalizeReportConfig(req.ReportConfig)
	if err != nil {
		return normalizedProjectConfig{}, err
	}
	eventGroups, err := normalizeEventGroups(req.EventGroups)
	if err != nil {
		return normalizedProjectConfig{}, err
	}
	queryFields, err := normalizeQueryFields(req.QueryFields)
	if err != nil {
		return normalizedProjectConfig{}, err
	}
	funnels := req.Funnels
	if funnels == nil {
		funnels = []FunnelDefinition{}
	}
	if err := validateProjectFunnels(queryFields, funnels); err != nil {
		return normalizedProjectConfig{}, err
	}
	return normalizedProjectConfig{
		IngestConfig:    ingestConfig,
		RetentionConfig: retentionConfig,
		MapConfig:       mapConfig,
		ReportConfig:    reportConfig,
		EventGroups:     eventGroups,
		QueryFields:     queryFields,
		Funnels:         funnels,
	}, nil
}

func marshalProjectJSON[T any](name string, value T) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be valid JSON", ErrBadRequest, name)
	}
	return json.RawMessage(raw), nil
}

func readIngestConfig(raw json.RawMessage) ProjectIngestConfig {
	var input ProjectIngestConfigInput
	if err := unmarshalProjectConfig(raw, &input); err != nil {
		return defaultIngestConfig
	}
	config, err := normalizeIngestConfig(input)
	if err != nil {
		return defaultIngestConfig
	}
	return config
}

func readRetentionConfig(raw json.RawMessage) ProjectRetentionConfig {
	var input ProjectRetentionConfigInput
	if err := unmarshalProjectConfig(raw, &input); err != nil {
		return defaultRetentionConfig
	}
	config, err := normalizeRetentionConfig(input)
	if err != nil {
		return defaultRetentionConfig
	}
	return config
}

func readMapConfig(raw json.RawMessage) ProjectMapConfig {
	var input ProjectMapConfigInput
	if err := unmarshalProjectConfig(raw, &input); err != nil {
		return defaultMapConfig
	}
	config, err := normalizeMapConfig(input)
	if err != nil {
		return defaultMapConfig
	}
	return config
}

func readReportConfig(raw json.RawMessage) ProjectReportConfig {
	var input ProjectReportConfigInput
	if err := unmarshalProjectConfig(raw, &input); err != nil {
		return defaultReportConfig
	}
	config, err := normalizeReportConfig(input)
	if err != nil {
		return defaultReportConfig
	}
	return config
}

func readEventGroups(raw json.RawMessage) map[string][]string {
	var input map[string][]string
	if err := unmarshalProjectConfig(raw, &input); err != nil {
		return map[string][]string{}
	}
	groups, err := normalizeEventGroups(input)
	if err != nil {
		return map[string][]string{}
	}
	return groups
}

func readQueryFields(raw json.RawMessage) []QueryFieldDefinition {
	var input []QueryFieldDefinition
	if err := unmarshalProjectConfig(raw, &input); err != nil {
		return []QueryFieldDefinition{}
	}
	fields, err := normalizeQueryFields(input)
	if err != nil {
		return []QueryFieldDefinition{}
	}
	return fields
}

func readFunnels(raw json.RawMessage) []FunnelDefinition {
	var funnels []FunnelDefinition
	if err := unmarshalProjectConfig(raw, &funnels); err != nil || funnels == nil {
		return []FunnelDefinition{}
	}
	return funnels
}

func unmarshalProjectConfig(raw json.RawMessage, out any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func normalizeIngestConfig(input ProjectIngestConfigInput) (ProjectIngestConfig, error) {
	config := defaultIngestConfig
	if input.MaxEventsPerBatch != nil {
		config.MaxEventsPerBatch = *input.MaxEventsPerBatch
	}
	if input.AcceptGzip != nil {
		config.AcceptGzip = *input.AcceptGzip
	}
	if input.AllowUnknownEventTypes != nil {
		config.AllowUnknownEventTypes = *input.AllowUnknownEventTypes
	}
	if input.AllowScreenshotFailures != nil {
		config.AllowScreenshotFailures = *input.AllowScreenshotFailures
	}
	if config.MaxEventsPerBatch <= 0 {
		return ProjectIngestConfig{}, fmt.Errorf("%w: max_events_per_batch must be positive", ErrBadRequest)
	}
	return config, nil
}

func normalizeRetentionConfig(input ProjectRetentionConfigInput) (ProjectRetentionConfig, error) {
	config := defaultRetentionConfig
	if input.EventDays != nil {
		config.EventDays = *input.EventDays
	}
	if input.ReportDays != nil {
		config.ReportDays = *input.ReportDays
	}
	if input.AccessLogDays != nil {
		config.AccessLogDays = *input.AccessLogDays
	}
	if config.EventDays < 0 || config.ReportDays < 0 || config.AccessLogDays < 0 {
		return ProjectRetentionConfig{}, fmt.Errorf("%w: retention days must be non-negative", ErrBadRequest)
	}
	return config, nil
}

func normalizeMapConfig(input ProjectMapConfigInput) (ProjectMapConfig, error) {
	config := defaultMapConfig
	if input.SpatialEnabled != nil {
		config.SpatialEnabled = *input.SpatialEnabled
	}
	if input.ZoneExtentM != nil {
		config.ZoneExtentM = *input.ZoneExtentM
	}
	if input.ZoneHeatmapCellM != nil {
		config.ZoneHeatmapCellM = *input.ZoneHeatmapCellM
	}
	if config.ZoneExtentM < 0 || config.ZoneHeatmapCellM < 0 {
		return ProjectMapConfig{}, fmt.Errorf("%w: map dimensions must be non-negative", ErrBadRequest)
	}
	return config, nil
}

func normalizeReportConfig(input ProjectReportConfigInput) (ProjectReportConfig, error) {
	config := defaultReportConfig
	if input.Statuses != nil {
		statuses, err := normalizeUniqueStrings("report statuses", input.Statuses, false)
		if err != nil {
			return ProjectReportConfig{}, err
		}
		config.Statuses = statuses
	}
	if input.Labels != nil {
		labels, err := normalizeUniqueStrings("report labels", input.Labels, true)
		if err != nil {
			return ProjectReportConfig{}, err
		}
		config.Labels = labels
	}
	if input.RateLimitSeconds != nil {
		config.RateLimitSeconds = *input.RateLimitSeconds
	}
	if config.RateLimitSeconds < 0 {
		return ProjectReportConfig{}, fmt.Errorf("%w: rate_limit_seconds must be non-negative", ErrBadRequest)
	}
	return config, nil
}

func normalizeEventGroups(input map[string][]string) (map[string][]string, error) {
	if input == nil {
		return map[string][]string{}, nil
	}
	out := make(map[string][]string, len(input))
	seenGroups := map[string]bool{}
	for group, events := range input {
		group = strings.TrimSpace(group)
		if group == "" {
			return nil, fmt.Errorf("%w: event_groups require non-empty group names", ErrBadRequest)
		}
		if seenGroups[group] {
			return nil, fmt.Errorf("%w: duplicate event group name", ErrBadRequest)
		}
		seenGroups[group] = true
		seen := map[string]bool{}
		normalizedEvents := make([]string, 0, len(events))
		for _, eventType := range events {
			eventType = strings.TrimSpace(eventType)
			if eventType == "" {
				return nil, fmt.Errorf("%w: event_groups require non-empty event types", ErrBadRequest)
			}
			if seen[eventType] {
				return nil, fmt.Errorf("%w: duplicate event type in event group", ErrBadRequest)
			}
			seen[eventType] = true
			normalizedEvents = append(normalizedEvents, eventType)
		}
		out[group] = normalizedEvents
	}
	return out, nil
}

func normalizeQueryFields(fields []QueryFieldDefinition) ([]QueryFieldDefinition, error) {
	if fields == nil {
		fields = []QueryFieldDefinition{}
	}
	out := make([]QueryFieldDefinition, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		field.Key = strings.TrimSpace(field.Key)
		field.Source = strings.TrimSpace(field.Source)
		field.Type = strings.TrimSpace(field.Type)
		field.Label = strings.TrimSpace(field.Label)
		if field.Key == "" || field.Source == "" {
			return nil, fmt.Errorf("%w: query_fields require key and source", ErrBadRequest)
		}
		if !validQueryFieldKey(field.Key) {
			return nil, fmt.Errorf("%w: query_fields key may use non-empty dotted segments without whitespace", ErrBadRequest)
		}
		if seen[field.Key] {
			return nil, fmt.Errorf("%w: duplicate query_fields key", ErrBadRequest)
		}
		seen[field.Key] = true
		if !validQueryFieldSource(field.Source) {
			return nil, fmt.Errorf("%w: query_fields source must use context, metrics, dimensions, or payload plus a dotted path", ErrBadRequest)
		}
		switch field.Type {
		case "string", "number", "bool":
		default:
			return nil, fmt.Errorf("%w: query_fields type must be string, number, or bool", ErrBadRequest)
		}
		if field.Label == "" {
			field.Label = field.Key
		}
		field.Aggregations = trimmedStrings(field.Aggregations)
		out = append(out, field)
	}
	return out, nil
}

func projectQueryFields(value json.RawMessage) ([]QueryFieldDefinition, error) {
	var fields []QueryFieldDefinition
	if err := json.Unmarshal(value, &fields); err != nil {
		return nil, fmt.Errorf("%w: query_fields must match the project field schema", ErrBadRequest)
	}
	return fields, nil
}

func projectFunnels(value json.RawMessage) ([]FunnelDefinition, error) {
	var funnels []FunnelDefinition
	if err := json.Unmarshal(value, &funnels); err != nil {
		return nil, fmt.Errorf("%w: funnels must match the project funnel schema", ErrBadRequest)
	}
	return funnels, nil
}

func validateProjectFunnels(queryFields []QueryFieldDefinition, funnels []FunnelDefinition) error {
	fields, err := queryFieldMap(queryFields)
	if err != nil {
		return err
	}
	seenFunnels := map[string]bool{}
	for _, funnel := range funnels {
		id := strings.TrimSpace(funnel.ID)
		if id == "" || !validProjectKey(id) {
			return fmt.Errorf("%w: funnel id must use lowercase letters, numbers, hyphens, or underscores", ErrBadRequest)
		}
		if seenFunnels[id] {
			return fmt.Errorf("%w: duplicate funnel id", ErrBadRequest)
		}
		seenFunnels[id] = true
		if strings.TrimSpace(funnel.Name) == "" {
			return fmt.Errorf("%w: funnel name is required", ErrBadRequest)
		}
		if strings.TrimSpace(funnel.Entity) != "player" {
			return fmt.Errorf("%w: funnel entity must be player", ErrBadRequest)
		}
		mode := funnelMode(funnel)
		if mode != "ordered" && mode != "unordered_presence" {
			return fmt.Errorf("%w: funnel mode must be ordered or unordered_presence", ErrBadRequest)
		}
		if len(funnel.Steps) == 0 {
			return fmt.Errorf("%w: funnels require at least one step", ErrBadRequest)
		}
		seenSteps := map[string]bool{}
		for stepIndex, step := range funnel.Steps {
			stepID := strings.TrimSpace(step.ID)
			if stepID == "" || !validProjectKey(stepID) {
				return fmt.Errorf("%w: funnel step id must use lowercase letters, numbers, hyphens, or underscores", ErrBadRequest)
			}
			if seenSteps[stepID] {
				return fmt.Errorf("%w: duplicate funnel step id", ErrBadRequest)
			}
			if strings.TrimSpace(step.Label) == "" {
				return fmt.Errorf("%w: funnel step label is required", ErrBadRequest)
			}
			if err := validateFunnelMatcher(step.Match, fields); err != nil {
				return err
			}
			if strings.TrimSpace(step.After) != "" {
				if mode != "ordered" {
					return fmt.Errorf("%w: after may only be used with ordered funnels", ErrBadRequest)
				}
				if !seenSteps[strings.TrimSpace(step.After)] {
					return fmt.Errorf("%w: funnel step after must reference an earlier step", ErrBadRequest)
				}
			}
			if step.WithinSeconds != nil {
				if mode != "ordered" {
					return fmt.Errorf("%w: within_seconds may only be used with ordered funnels", ErrBadRequest)
				}
				if *step.WithinSeconds <= 0 {
					return fmt.Errorf("%w: within_seconds must be positive", ErrBadRequest)
				}
				if stepIndex == 0 {
					return fmt.Errorf("%w: first funnel step cannot use within_seconds", ErrBadRequest)
				}
			}
			seenSteps[stepID] = true
		}
	}
	return nil
}

func normalizeUniqueStrings(name string, values []string, allowEmpty bool) ([]string, error) {
	if allowEmpty && len(values) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%w: %s require non-empty values", ErrBadRequest, name)
		}
		if seen[value] {
			return nil, fmt.Errorf("%w: duplicate %s value", ErrBadRequest, name)
		}
		seen[value] = true
		out = append(out, value)
	}
	if !allowEmpty && len(out) == 0 {
		return nil, fmt.Errorf("%w: %s require at least one value", ErrBadRequest, name)
	}
	return out, nil
}

func validQueryFieldKey(value string) bool {
	if value == "" || hasWhitespace(value) {
		return false
	}
	for _, segment := range strings.Split(value, ".") {
		if segment == "" {
			return false
		}
	}
	return true
}

func validQueryFieldSource(value string) bool {
	root, path, ok := strings.Cut(value, ".")
	if !ok || path == "" {
		return false
	}
	switch root {
	case "context", "metrics", "dimensions", "payload":
	default:
		return false
	}
	return validQueryFieldKey(path)
}

func hasWhitespace(value string) bool {
	for _, r := range value {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}

func validateFunnelMatcher(match FunnelEventMatcher, fields map[string]QueryFieldDefinition) error {
	matcherCount := 0
	if strings.TrimSpace(match.EventType) != "" {
		matcherCount++
	}
	eventTypes := trimmedStrings(match.EventTypes)
	if len(eventTypes) > 0 {
		matcherCount++
	}
	if strings.TrimSpace(match.EventType) != "" && len(eventTypes) > 0 {
		return fmt.Errorf("%w: event_type and event_types are mutually exclusive", ErrBadRequest)
	}
	fieldKey := strings.TrimSpace(match.FieldKey)
	if fieldKey != "" {
		matcherCount++
		field, ok := fields[fieldKey]
		if !ok {
			return fmt.Errorf("%w: unknown funnel field_key", ErrBadRequest)
		}
		if !field.Filterable {
			return fmt.Errorf("%w: funnel field_key must be filterable", ErrBadRequest)
		}
		if hasRawJSONValue(match.FieldValue) {
			if _, _, err := funnelFieldValue(field.Type, match.FieldValue); err != nil {
				return err
			}
		}
	}
	if strings.TrimSpace(match.RegionID) != "" {
		matcherCount++
	}
	if strings.TrimSpace(match.ZoneID) != "" {
		matcherCount++
	}
	if matcherCount == 0 {
		return fmt.Errorf("%w: funnel step requires at least one matcher", ErrBadRequest)
	}
	return nil
}

func queryFieldMap(fields []QueryFieldDefinition) (map[string]QueryFieldDefinition, error) {
	out := make(map[string]QueryFieldDefinition, len(fields))
	for _, field := range fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}
		field.Key = key
		field.Type = strings.TrimSpace(field.Type)
		out[key] = field
	}
	return out, nil
}

func (s *adminService) evaluateFunnel(ctx context.Context, projectID uuid.UUID, from time.Time, to time.Time, def FunnelDefinition, fields map[string]QueryFieldDefinition) (FunnelSummary, error) {
	sql, args, err := buildFunnelCountsQuery(projectID, from, to, def, fields)
	if err != nil {
		return FunnelSummary{}, err
	}
	counts := make([]int64, len(def.Steps))
	scanTargets := make([]any, len(counts))
	for i := range counts {
		scanTargets[i] = &counts[i]
	}
	if err := s.pool.QueryRow(ctx, sql, args...).Scan(scanTargets...); err != nil {
		return FunnelSummary{}, err
	}
	return funnelSummary(def, counts), nil
}

func buildFunnelCountsQuery(projectID uuid.UUID, from time.Time, to time.Time, def FunnelDefinition, fields map[string]QueryFieldDefinition) (string, []any, error) {
	builder := funnelQueryBuilder{args: []any{projectID, from, to}}
	stepIndexByID := make(map[string]int, len(def.Steps))
	ctes := make([]string, 0, len(def.Steps))
	for i, step := range def.Steps {
		stepName := funnelStepCTEName(i)
		stepIndexByID[strings.TrimSpace(step.ID)] = i
		stepSQL, err := builder.funnelStepCTE(stepName, i, step, def, stepIndexByID, fields)
		if err != nil {
			return "", nil, err
		}
		ctes = append(ctes, stepSQL)
	}
	var sql strings.Builder
	sql.WriteString("WITH ")
	sql.WriteString(strings.Join(ctes, ", "))
	sql.WriteString(" SELECT ")
	for i := range def.Steps {
		if i > 0 {
			sql.WriteString(", ")
		}
		sql.WriteString("(SELECT count(*)::bigint FROM ")
		sql.WriteString(funnelStepCTEName(i))
		sql.WriteString(") AS step_")
		sql.WriteString(strconv.Itoa(i + 1))
		sql.WriteString("_count")
	}
	return sql.String(), builder.args, nil
}

type funnelQueryBuilder struct {
	args []any
}

func (b *funnelQueryBuilder) addArg(value any) string {
	b.args = append(b.args, value)
	return "$" + strconv.Itoa(len(b.args))
}

func (b *funnelQueryBuilder) funnelStepCTE(name string, index int, step FunnelStepConfig, def FunnelDefinition, stepIndexByID map[string]int, fields map[string]QueryFieldDefinition) (string, error) {
	join, where, err := b.funnelMatcherSQL(step.Match, fields)
	if err != nil {
		return "", err
	}
	if funnelMode(def) == "unordered_presence" {
		return b.unorderedStepCTE(name, index, join, where), nil
	}
	dependency := index - 1
	if after := strings.TrimSpace(step.After); after != "" {
		dependency = stepIndexByID[after]
	}
	return b.orderedStepCTE(name, index, dependency, step.WithinSeconds, join, where), nil
}

func (b *funnelQueryBuilder) unorderedStepCTE(name string, index int, join string, where string) string {
	var sql strings.Builder
	sql.WriteString(name)
	sql.WriteString(" AS (SELECT DISTINCT e.player_id FROM events e")
	if index > 0 {
		sql.WriteString(" JOIN ")
		sql.WriteString(funnelStepCTEName(index - 1))
		sql.WriteString(" prev ON prev.player_id = e.player_id")
	}
	sql.WriteString(join)
	sql.WriteString(where)
	sql.WriteString(")")
	return sql.String()
}

func (b *funnelQueryBuilder) orderedStepCTE(name string, index int, dependency int, withinSeconds *int64, join string, where string) string {
	var sql strings.Builder
	sql.WriteString(name)
	sql.WriteString(" AS (SELECT DISTINCT ON (e.player_id) e.player_id, e.game_time AS first_game_time, e.real_ts AS first_real_ts FROM events e")
	if index > 0 {
		sql.WriteString(" JOIN ")
		sql.WriteString(funnelStepCTEName(dependency))
		sql.WriteString(" prev ON prev.player_id = e.player_id")
	}
	sql.WriteString(join)
	sql.WriteString(where)
	if index > 0 {
		sql.WriteString(" AND e.game_time >= prev.first_game_time")
		if withinSeconds != nil {
			sql.WriteString(" AND e.real_ts >= prev.first_real_ts")
			sql.WriteString(" AND e.real_ts <= prev.first_real_ts + (")
			sql.WriteString(b.addArg(*withinSeconds))
			sql.WriteString("::bigint * interval '1 second')")
		}
	}
	sql.WriteString(" ORDER BY e.player_id, e.game_time ASC, e.real_ts ASC)")
	return sql.String()
}

func (b *funnelQueryBuilder) funnelMatcherSQL(match FunnelEventMatcher, fields map[string]QueryFieldDefinition) (string, string, error) {
	var join strings.Builder
	var where strings.Builder
	where.WriteString(" WHERE e.project_id = $1 AND e.real_ts >= $2 AND e.real_ts <= $3")

	if eventType := strings.TrimSpace(match.EventType); eventType != "" {
		where.WriteString(" AND e.event_type = ")
		where.WriteString(b.addArg(eventType))
	}
	if eventTypes := trimmedStrings(match.EventTypes); len(eventTypes) > 0 {
		where.WriteString(" AND e.event_type = ANY(")
		where.WriteString(b.addArg(eventTypes))
		where.WriteString("::text[])")
	}
	if regionID := strings.TrimSpace(match.RegionID); regionID != "" {
		where.WriteString(" AND e.region_id = ")
		where.WriteString(b.addArg(regionID))
	}
	if zoneID := strings.TrimSpace(match.ZoneID); zoneID != "" {
		where.WriteString(" AND e.zone_id = ")
		where.WriteString(b.addArg(zoneID))
	}
	if fieldKey := strings.TrimSpace(match.FieldKey); fieldKey != "" {
		field, ok := fields[fieldKey]
		if !ok {
			return "", "", fmt.Errorf("%w: unknown funnel field_key", ErrBadRequest)
		}
		if !field.Filterable {
			return "", "", fmt.Errorf("%w: funnel field_key must be filterable", ErrBadRequest)
		}
		join.WriteString(" JOIN event_fields ef ON ef.event_id = e.id AND ef.project_id = e.project_id")
		join.WriteString(" AND ef.field_key = ")
		join.WriteString(b.addArg(fieldKey))
		join.WriteString(" AND ef.value_type = ")
		join.WriteString(b.addArg(field.Type))
		fieldValue, hasFieldValue, err := funnelFieldValue(field.Type, match.FieldValue)
		if err != nil {
			return "", "", err
		}
		if hasFieldValue {
			switch field.Type {
			case "string":
				join.WriteString(" AND ef.string_value = ")
			case "number":
				join.WriteString(" AND ef.number_value = ")
			case "bool":
				join.WriteString(" AND ef.bool_value = ")
			default:
				return "", "", fmt.Errorf("%w: unsupported funnel field type", ErrBadRequest)
			}
			join.WriteString(b.addArg(fieldValue))
		}
	}
	return join.String(), where.String(), nil
}

func funnelStepCTEName(index int) string {
	return "step_" + strconv.Itoa(index+1)
}

func funnelSummary(def FunnelDefinition, counts []int64) FunnelSummary {
	var started int64
	var completed int64
	if len(counts) > 0 {
		started = counts[0]
		completed = counts[len(counts)-1]
	}
	rate := 0.0
	if started > 0 {
		rate = float64(completed) / float64(started)
	}
	dropoff := "none"
	if started > completed {
		dropoff = fmt.Sprintf("%d players", started-completed)
	}
	steps := make([]FunnelStepSummary, 0, len(def.Steps))
	for i, step := range def.Steps {
		count := counts[i]
		stepRate := 0.0
		if started > 0 {
			stepRate = float64(count) / float64(started)
		}
		steps = append(steps, FunnelStepSummary{
			ID:    strings.TrimSpace(step.ID),
			Label: strings.TrimSpace(step.Label),
			Count: count,
			Rate:  stepRate,
		})
	}
	return FunnelSummary{
		ID:          strings.TrimSpace(def.ID),
		Name:        strings.TrimSpace(def.Name),
		Description: strings.TrimSpace(def.Description),
		Entity:      "player",
		Started:     started,
		Completed:   completed,
		Rate:        rate,
		Dropoff:     dropoff,
		Steps:       steps,
	}
}

func funnelFieldValue(valueType string, raw json.RawMessage) (any, bool, error) {
	if !hasRawJSONValue(raw) {
		return nil, false, nil
	}
	switch strings.TrimSpace(valueType) {
	case "string":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, fmt.Errorf("%w: funnel field_value must be a string", ErrBadRequest)
		}
		return value, true, nil
	case "number":
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, fmt.Errorf("%w: funnel field_value must be a number", ErrBadRequest)
		}
		return value, true, nil
	case "bool":
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, fmt.Errorf("%w: funnel field_value must be a boolean", ErrBadRequest)
		}
		return value, true, nil
	default:
		return nil, false, fmt.Errorf("%w: unsupported funnel field type", ErrBadRequest)
	}
}

func hasRawJSONValue(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func funnelMode(def FunnelDefinition) string {
	mode := strings.TrimSpace(def.Mode)
	if mode == "" {
		return "ordered"
	}
	return mode
}

func trimmedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

var defaultIngestConfig = ProjectIngestConfig{
	MaxEventsPerBatch:       50,
	AcceptGzip:              true,
	AllowUnknownEventTypes:  true,
	AllowScreenshotFailures: true,
}

var defaultRetentionConfig = ProjectRetentionConfig{
	EventDays:     730,
	ReportDays:    1095,
	AccessLogDays: 14,
}

var defaultMapConfig = ProjectMapConfig{
	SpatialEnabled:   true,
	ZoneExtentM:      30000,
	ZoneHeatmapCellM: 300,
}

var defaultReportConfig = ProjectReportConfig{
	Statuses:         []string{"new", "seen", "reproduced", "fixed", "wont_fix", "needs_more_info"},
	Labels:           []string{"bug", "sentiment", "balance", "mission", "combat", "economy", "ui"},
	RateLimitSeconds: 60,
}

func newIngestToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := TelemetryTokenPrefix + base64.RawURLEncoding.EncodeToString(raw[:])
	return token, HashToken(token), nil
}

func ingestTokenSummary(id uuid.UUID, name string, enabled bool, expiresAt pgtype.Timestamptz, lastUsedAt pgtype.Timestamptz, createdAt time.Time) IngestTokenSummary {
	return IngestTokenSummary{
		ID:         id.String(),
		Name:       name,
		Enabled:    enabled,
		ExpiresAt:  optionalTime(expiresAt),
		LastUsedAt: optionalTime(lastUsedAt),
		CreatedAt:  formatTime(createdAt),
	}
}

func adminUserSummaryFromListRow(row dbq.AdminListUsersRow) AdminUserSummary {
	return AdminUserSummary{
		ID:          row.ID.String(),
		Email:       row.Email,
		Name:        row.Name,
		Picture:     row.PictureUrl,
		Role:        row.Role,
		Enabled:     row.Enabled,
		Provider:    row.Provider,
		LastLoginAt: optionalTime(row.LastLoginAt),
		CreatedAt:   formatTime(row.CreatedAt),
		UpdatedAt:   formatTime(row.UpdatedAt),
	}
}

func adminUserSummaryFromSetEnabledRow(row dbq.AdminSetUserEnabledRow) AdminUserSummary {
	return AdminUserSummary{
		ID:          row.ID.String(),
		Email:       row.Email,
		Name:        row.Name,
		Picture:     row.PictureUrl,
		Role:        row.Role,
		Enabled:     row.Enabled,
		Provider:    row.Provider,
		LastLoginAt: optionalTime(row.LastLoginAt),
		CreatedAt:   formatTime(row.CreatedAt),
		UpdatedAt:   formatTime(row.UpdatedAt),
	}
}

func agentAuthorizationSummaryFromListRow(row dbq.AdminListAgentAuthorizationsRow) (AgentAuthorizationSummary, error) {
	projectKeys := []string{}
	if len(row.ProjectKeys) > 0 {
		if err := json.Unmarshal(row.ProjectKeys, &projectKeys); err != nil {
			return AgentAuthorizationSummary{}, err
		}
	}
	return AgentAuthorizationSummary{
		ID:                   row.ID.String(),
		ClientID:             row.ClientID,
		ClientName:           row.ClientName,
		CreatedByAdminUserID: uuidString(row.CreatedByAdminUserID),
		CreatedByEmail:       textValue(row.CreatedByEmail),
		AllProjects:          row.AllProjects,
		ProjectKeys:          projectKeys,
		Scopes:               append([]string(nil), row.Scopes...),
		Enabled:              row.Enabled,
		ExpiresAt:            formatTime(row.ExpiresAt),
		ActivatedAt:          optionalTime(row.ActivatedAt),
		LastUsedAt:           optionalTime(row.LastUsedAt),
		CreatedAt:            formatTime(row.CreatedAt),
		UpdatedAt:            formatTime(row.UpdatedAt),
	}, nil
}

func agentAuthorizationSummaryFromSetEnabledRow(row dbq.AdminSetAgentAuthorizationEnabledRow) AgentAuthorizationSummary {
	return AgentAuthorizationSummary{
		ID:                   row.ID.String(),
		ClientID:             row.ClientID,
		ClientName:           row.ClientName,
		CreatedByAdminUserID: uuidString(row.CreatedByAdminUserID),
		AllProjects:          row.AllProjects,
		ProjectKeys:          []string{},
		Scopes:               append([]string(nil), row.Scopes...),
		Enabled:              row.Enabled,
		ExpiresAt:            formatTime(row.ExpiresAt),
		ActivatedAt:          optionalTime(row.ActivatedAt),
		LastUsedAt:           optionalTime(row.LastUsedAt),
		CreatedAt:            formatTime(row.CreatedAt),
		UpdatedAt:            formatTime(row.UpdatedAt),
	}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func optionalTime(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return formatTime(value.Time)
}

func validReportStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "new", "seen", "reproduced", "fixed", "wont_fix", "needs_more_info":
		return true
	default:
		return false
	}
}

func ParseLimit(value string, fallback int32) int32 {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return int32(parsed)
}

func ParseOffset(value string) int32 {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return 0
	}
	return int32(parsed)
}

func ParseTimeRange(projectKey string, fromRaw string, toRaw string) (TimeProjectFilter, error) {
	filter := DefaultTimeProjectFilter(projectKey)
	if fromRaw != "" {
		from, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			return filter, fmt.Errorf("%w: from must be RFC3339", ErrBadRequest)
		}
		filter.From = from
	}
	if toRaw != "" {
		to, err := time.Parse(time.RFC3339, toRaw)
		if err != nil {
			return filter, fmt.Errorf("%w: to must be RFC3339", ErrBadRequest)
		}
		filter.To = to
	}
	return filter, nil
}

type adminFieldFilter struct {
	Key         string
	ValueType   string
	HasValue    bool
	StringValue string
	NumberValue float64
	BoolValue   bool
}

func projectFieldFilter(queryFields json.RawMessage, fieldKey *string, fieldValue *string) (adminFieldFilter, bool, error) {
	if fieldKey == nil || strings.TrimSpace(*fieldKey) == "" {
		return adminFieldFilter{}, false, nil
	}

	key := strings.TrimSpace(*fieldKey)
	fields, err := projectQueryFields(queryFields)
	if err != nil {
		return adminFieldFilter{}, false, err
	}

	for _, field := range fields {
		if strings.TrimSpace(field.Key) != key {
			continue
		}
		if !field.Filterable {
			return adminFieldFilter{}, false, fmt.Errorf("%w: field is not filterable", ErrBadRequest)
		}

		filter := adminFieldFilter{
			Key:       key,
			ValueType: strings.TrimSpace(field.Type),
		}
		if fieldValue == nil || strings.TrimSpace(*fieldValue) == "" {
			return filter, true, nil
		}

		filter.HasValue = true
		value := strings.TrimSpace(*fieldValue)
		switch filter.ValueType {
		case "string":
			filter.StringValue = value
		case "number":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return adminFieldFilter{}, false, fmt.Errorf("%w: field_value must be a number", ErrBadRequest)
			}
			filter.NumberValue = parsed
		case "bool":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return adminFieldFilter{}, false, fmt.Errorf("%w: field_value must be a boolean", ErrBadRequest)
			}
			filter.BoolValue = parsed
		default:
			return adminFieldFilter{}, false, fmt.Errorf("%w: unsupported field type", ErrBadRequest)
		}
		return filter, true, nil
	}

	return adminFieldFilter{}, false, fmt.Errorf("%w: unknown field_key", ErrBadRequest)
}

func optionalTextPtr(value *string) pgtype.Text {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*value), Valid: true}
}

func optionalUUIDPtr(value *string) (pgtype.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.UUID{}, nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(*value))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: player_id must be a UUID", ErrBadRequest)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func clampLimit(value int32, maxValue int32) int32 {
	if value <= 0 {
		return maxValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxInt32(value int32, minValue int32) int32 {
	if value < minValue {
		return minValue
	}
	return value
}
