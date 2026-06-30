package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	Summary(ctx context.Context, filter TimeProjectFilter) (SummaryResponse, error)
	ListEvents(ctx context.Context, filter EventListFilter) ([]EventSummary, error)
	PlayerTrace(ctx context.Context, projectKey string, playerID string, limit int32) ([]TraceEvent, error)
	SystemHeatmap(ctx context.Context, filter HeatmapFilter) ([]SystemHeatmapCell, error)
	ZoneHeatmap(ctx context.Context, filter ZoneHeatmapFilter) ([]ZoneHeatmapCell, error)
	Funnels(ctx context.Context, filter TimeProjectFilter) (FunnelsResponse, error)
	ListReports(ctx context.Context, filter ReportListFilter) ([]ReportSummary, error)
	GetReport(ctx context.Context, projectKey string, reportID string) (ReportDetail, error)
	ReportScreenshot(ctx context.Context, projectKey string, reportID string) (ScreenshotReadResult, error)
	UpdateReport(ctx context.Context, projectKey string, reportID string, req UpdateReportRequest) (ReportUpdateResponse, error)
	EventTypes(ctx context.Context, projectKey string) ([]EventTypeSummary, error)
	Settings(ctx context.Context, projectKey string) (SettingsResponse, error)
	CreateIngestToken(ctx context.Context, projectKey string, req CreateIngestTokenRequest) (CreateIngestTokenResponse, error)
	SetIngestTokenEnabled(ctx context.Context, projectKey string, tokenID string, enabled bool) (IngestTokenSummary, error)
}

type TimeProjectFilter struct {
	ProjectID string
	From      time.Time
	To        time.Time
}

type EventListFilter struct {
	TimeProjectFilter
	EventType    *string
	SystemID     *string
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
	SystemID string
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
	SystemID         string          `json:"system_id"`
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
	SystemID    string          `json:"system_id"`
	ZoneID      string          `json:"zone_id"`
	Coordinates []float64       `json:"coordinates,omitempty"`
	Context     json.RawMessage `json:"context"`
	Metrics     json.RawMessage `json:"metrics"`
	Dimensions  json.RawMessage `json:"dimensions"`
	Fields      json.RawMessage `json:"fields"`
	Payload     json.RawMessage `json:"payload"`
}

type SystemHeatmapCell struct {
	SystemID   string `json:"system_id"`
	EventType  string `json:"event_type"`
	EventCount int64  `json:"event_count"`
}

type ZoneHeatmapCell struct {
	SystemID   string `json:"system_id"`
	ZoneID     string `json:"zone_id"`
	GridX      int64  `json:"grid_x"`
	GridZ      int64  `json:"grid_z"`
	EventType  string `json:"event_type"`
	EventCount int64  `json:"event_count"`
}

type FunnelSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Started     int64   `json:"started"`
	Completed   int64   `json:"completed"`
	Rate        float64 `json:"rate"`
	Dropoff     string  `json:"dropoff"`
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
	SystemID            string          `json:"system_id"`
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
	ProjectID       string          `json:"project_id"`
	DisplayName     string          `json:"display_name"`
	ValidationMode  string          `json:"validation_mode"`
	IngestConfig    json.RawMessage `json:"ingest_config"`
	RetentionConfig json.RawMessage `json:"retention_config"`
	MapConfig       json.RawMessage `json:"map_config"`
	ReportConfig    json.RawMessage `json:"report_config"`
	EventGroups     json.RawMessage `json:"event_groups"`
	QueryFields     json.RawMessage `json:"query_fields"`
}

type CreateProjectRequest struct {
	ProjectID       string          `json:"project_id"`
	DisplayName     string          `json:"display_name"`
	ValidationMode  string          `json:"validation_mode"`
	IngestConfig    json.RawMessage `json:"ingest_config"`
	RetentionConfig json.RawMessage `json:"retention_config"`
	MapConfig       json.RawMessage `json:"map_config"`
	ReportConfig    json.RawMessage `json:"report_config"`
	EventGroups     json.RawMessage `json:"event_groups"`
	QueryFields     json.RawMessage `json:"query_fields"`
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

type adminService struct {
	queries         *dbq.Queries
	screenshotStore ScreenshotStore
}

func NewAdminService(pool db.Pool, screenshotStore ScreenshotStore) Admin {
	return &adminService{
		queries:         dbq.New(pool),
		screenshotStore: screenshotStore,
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
	params, err := createProjectParams(req)
	if err != nil {
		return ProjectSettings{}, err
	}
	row, err := s.queries.AdminUpsertProject(ctx, params)
	if err != nil {
		return ProjectSettings{}, err
	}
	return ProjectSettings{
		ProjectID:       row.ProjectKey,
		DisplayName:     row.DisplayName,
		ValidationMode:  row.ValidationMode,
		IngestConfig:    row.IngestConfig,
		RetentionConfig: row.RetentionConfig,
		MapConfig:       row.MapConfig,
		ReportConfig:    row.ReportConfig,
		EventGroups:     row.EventGroups,
		QueryFields:     row.QueryFields,
	}, nil
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
			SystemID:         optionalTextPtr(filter.SystemID),
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
				SystemID:         row.SystemID,
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
		SystemID:     optionalTextPtr(filter.SystemID),
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
			SystemID:         row.SystemID,
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
			SystemID:    row.SystemID,
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

func (s *adminService) SystemHeatmap(ctx context.Context, filter HeatmapFilter) ([]SystemHeatmapCell, error) {
	project, err := s.loadProject(ctx, filter.ProjectID)
	if err != nil {
		return nil, err
	}
	fieldFilter, hasFieldFilter, err := projectFieldFilter(project.QueryFields, filter.FieldKey, filter.FieldValue)
	if err != nil {
		return nil, err
	}
	if hasFieldFilter {
		rows, err := s.queries.AdminSystemHeatmapByField(ctx, dbq.AdminSystemHeatmapByFieldParams{
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
		out := make([]SystemHeatmapCell, 0, len(rows))
		for _, row := range rows {
			out = append(out, SystemHeatmapCell{
				SystemID:   row.SystemID,
				EventType:  row.EventType,
				EventCount: row.EventCount,
			})
		}
		return out, nil
	}
	rows, err := s.queries.AdminSystemHeatmap(ctx, dbq.AdminSystemHeatmapParams{
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
	out := make([]SystemHeatmapCell, 0, len(rows))
	for _, row := range rows {
		out = append(out, SystemHeatmapCell{
			SystemID:   row.SystemID,
			EventType:  row.EventType,
			EventCount: row.EventCount,
		})
	}
	return out, nil
}

func (s *adminService) ZoneHeatmap(ctx context.Context, filter ZoneHeatmapFilter) ([]ZoneHeatmapCell, error) {
	if filter.SystemID == "" {
		return nil, fmt.Errorf("%w: system_id is required", ErrBadRequest)
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
			SystemID:         filter.SystemID,
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
				SystemID:   row.SystemID,
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
		SystemID:     filter.SystemID,
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
			SystemID:   row.SystemID,
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
	counts, err := s.queries.AdminFunnelCounts(ctx, dbq.AdminFunnelCountsParams{
		ProjectID: project.ID,
		RealTs:    filter.From,
		RealTs_2:  filter.To,
	})
	if err != nil {
		return FunnelsResponse{}, err
	}
	return FunnelsResponse{
		Funnels: []FunnelSummary{
			funnel("onboarding_first_return", "Onboarding: first station return", "continue -> undock -> dock", counts.OnboardingStarted, counts.OnboardingCompleted),
			funnel("first_trade_loop", "First trade loop", "buy commodity -> sell commodity", counts.TradeStarted, counts.TradeCompleted),
			funnel("first_mission_loop", "First mission loop", "take mission -> complete mission", counts.MissionStarted, counts.MissionCompleted),
			funnel("first_combat_survival", "First combat survival", "combat start -> no death in window", counts.CombatStarted, counts.CombatCompleted),
			funnel("first_station_return", "First station return", "undock -> dock", counts.StationReturnStarted, counts.StationReturnCompleted),
			funnel("first_report", "First player report", "bug_report submissions by time window", counts.ReportStarted, counts.ReportCompleted),
		},
	}, nil
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
				SystemID:            row.SystemID,
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
			SystemID:            row.SystemID,
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
			SystemID:   traceRow.SystemID,
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
			SystemID:            row.SystemID,
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
		Project: ProjectSettings{
			ProjectID:       settings.ProjectKey,
			DisplayName:     settings.DisplayName,
			ValidationMode:  settings.ValidationMode,
			IngestConfig:    settings.IngestConfig,
			RetentionConfig: settings.RetentionConfig,
			MapConfig:       settings.MapConfig,
			ReportConfig:    settings.ReportConfig,
			EventGroups:     settings.EventGroups,
			QueryFields:     settings.QueryFields,
		},
		Tokens: tokens,
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

	ingestConfig, err := normalizeJSONConfig("ingest_config", req.IngestConfig, defaultIngestConfigJSON, "object")
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	retentionConfig, err := normalizeJSONConfig("retention_config", req.RetentionConfig, defaultRetentionConfigJSON, "object")
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	mapConfig, err := normalizeJSONConfig("map_config", req.MapConfig, defaultMapConfigJSON, "object")
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	reportConfig, err := normalizeJSONConfig("report_config", req.ReportConfig, defaultReportConfigJSON, "object")
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	eventGroups, err := normalizeJSONConfig("event_groups", req.EventGroups, defaultEventGroupsJSON, "object")
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}
	queryFields, err := normalizeJSONConfig("query_fields", req.QueryFields, defaultQueryFieldsJSON, "array")
	if err != nil {
		return dbq.AdminUpsertProjectParams{}, err
	}

	if err := validateQueryFields(queryFields); err != nil {
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

func normalizeJSONConfig(name string, value json.RawMessage, fallback string, wantKind string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" {
		trimmed = fallback
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("%w: %s must be valid JSON", ErrBadRequest, name)
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("%w: %s must be valid JSON", ErrBadRequest, name)
	}
	switch wantKind {
	case "object":
		if _, ok := decoded.(map[string]any); !ok {
			return nil, fmt.Errorf("%w: %s must be a JSON object", ErrBadRequest, name)
		}
	case "array":
		if _, ok := decoded.([]any); !ok {
			return nil, fmt.Errorf("%w: %s must be a JSON array", ErrBadRequest, name)
		}
	}
	return json.RawMessage(trimmed), nil
}

func validateQueryFields(value json.RawMessage) error {
	var fields []QueryFieldDefinition
	if err := json.Unmarshal(value, &fields); err != nil {
		return fmt.Errorf("%w: query_fields must match the project field schema", ErrBadRequest)
	}
	for _, field := range fields {
		if strings.TrimSpace(field.Key) == "" || strings.TrimSpace(field.Source) == "" {
			return fmt.Errorf("%w: query_fields require key and source", ErrBadRequest)
		}
		switch strings.TrimSpace(field.Type) {
		case "string", "number", "bool":
		default:
			return fmt.Errorf("%w: query_fields type must be string, number, or bool", ErrBadRequest)
		}
	}
	return nil
}

const defaultIngestConfigJSON = `{
  "max_events_per_batch": 50,
  "accept_gzip": true,
  "allow_unknown_event_types": true,
  "allow_screenshot_failures": true
}`

const defaultRetentionConfigJSON = `{
  "event_days": 730,
  "report_days": 1095,
  "access_log_days": 14
}`

const defaultMapConfigJSON = `{
  "systems_overlay": "",
  "zone_extent_m": 30000,
  "zone_heatmap_cell_m": 300
}`

const defaultReportConfigJSON = `{
  "statuses": ["new", "seen", "reproduced", "fixed", "wont_fix", "needs_more_info"],
  "labels": ["bug", "sentiment", "balance", "mission", "combat", "economy", "ui"],
  "rate_limit_seconds": 60
}`

const defaultEventGroupsJSON = `{
  "lifecycle": ["new_game", "game_continue", "game_exit", "dock", "undock"],
  "report": ["bug_report"]
}`

const defaultQueryFieldsJSON = `[]`

func newIngestToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := "fr_" + base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
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

func optionalTime(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return formatTime(value.Time)
}

func funnel(id string, name string, description string, started int64, completed int64) FunnelSummary {
	rate := 0.0
	if started > 0 {
		rate = float64(completed) / float64(started)
	}
	dropoff := "none"
	if started > completed {
		dropoff = "incomplete"
	}
	return FunnelSummary{
		ID:          id,
		Name:        name,
		Description: description,
		Started:     started,
		Completed:   completed,
		Rate:        rate,
		Dropoff:     dropoff,
	}
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
	var fields []QueryFieldDefinition
	if err := json.Unmarshal(queryFields, &fields); err != nil {
		return adminFieldFilter{}, false, fmt.Errorf("project query_fields must be an array: %w", err)
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
