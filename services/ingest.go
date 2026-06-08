package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/db/dbq"
)

var (
	// ErrForbidden is intentionally vague — it covers both "project not found"
	// and "token belongs to a different project" to prevent project key
	// enumeration attacks via differing error messages.
	ErrForbidden       = errors.New("project token cannot access requested project")
	ErrBadRequest      = errors.New("bad request")
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrRateLimited     = errors.New("rate limited")
)

type Ingest interface {
	IngestEvents(ctx context.Context, authProjectID uuid.UUID, req EventsRequest) (EventsResponse, error)
	SubmitBugReport(ctx context.Context, authProjectID uuid.UUID, req BugReportRequest) (BugReportResponse, error)
}

type ClientInfo struct {
	GameVersion  string `json:"game_version"`
	BuildChannel string `json:"build_channel"`
	Platform     string `json:"platform"`
}

// EventEnvelope is the top-level wrapper for every event sent by the game client.
// Raw holds the original verbatim JSON from the inbound request. It is populated
// by UnmarshalJSON but excluded from marshaling (json:"-"), so the original bytes
// can be persisted without re-serialization.
type EventEnvelope struct {
	Raw           json.RawMessage `json:"-"`
	SchemaVersion int             `json:"schema_version"`
	CommanderID   string          `json:"commander_id"`
	EventType     string          `json:"event_type"`
	RealTS        string          `json:"real_ts"`
	GameTime      int64           `json:"game_time"`
	Context       json.RawMessage `json:"context"`
	Metrics       json.RawMessage `json:"metrics"`
	Dimensions    json.RawMessage `json:"dimensions"`
	Payload       json.RawMessage `json:"payload"`
}

func (e *EventEnvelope) UnmarshalJSON(data []byte) error {
	type eventEnvelope EventEnvelope
	var decoded eventEnvelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = EventEnvelope(decoded)
	// append to a zero-length slice forces a fresh allocation — defensive copy
	// so callers cannot alias the original buffer.
	e.Raw = append(e.Raw[:0], data...)
	return nil
}

type EventLocation struct {
	WorldID  string    `json:"world_id"`
	AreaID   string    `json:"area_id"`
	Position []float64 `json:"position"`
}

type EventContext struct {
	Location EventLocation `json:"location"`
}

type QueryFieldDefinition struct {
	Key          string   `json:"key"`
	Source       string   `json:"source"`
	Type         string   `json:"type"`
	Label        string   `json:"label"`
	Filterable   bool     `json:"filterable"`
	Aggregations []string `json:"aggregations"`
}

type eventFieldProjection struct {
	key         string
	valueType   string
	stringValue pgtype.Text
	numberValue pgtype.Float8
	boolValue   pgtype.Bool
}

type EventsRequest struct {
	ProjectID string          `json:"project_id"`
	BatchID   string          `json:"batch_id"`
	SentAt    string          `json:"sent_at"`
	Client    ClientInfo      `json:"client"`
	Events    []EventEnvelope `json:"events"`
}

type EventsResponse struct {
	Accepted   int32  `json:"accepted"`
	Rejected   int32  `json:"rejected"`
	BatchID    string `json:"batch_id"`
	ServerTime string `json:"server_time"`
}

type BugReportRequest struct {
	ProjectID string        `json:"project_id"`
	ReportID  string        `json:"report_id"`
	Client    ClientInfo    `json:"client"`
	Event     EventEnvelope `json:"event"`
}

type BugReportResponse struct {
	Accepted            bool   `json:"accepted"`
	ReportID            string `json:"report_id"`
	ScreenshotObjectKey string `json:"screenshot_object_key,omitempty"`
	ServerTime          string `json:"server_time"`
}

type ScreenshotStore interface {
	StorePNG(ctx context.Context, projectKey string, reportID string, eventTime time.Time, png []byte) (string, error)
	ReadPNG(ctx context.Context, key string) (ScreenshotReadResult, error)
}

type IngestOptions struct {
	DB                      db.Pool
	MaxEventsPerBatch       int
	ReportRateLimitSeconds  int
	AllowScreenshotFailures bool
	ScreenshotStore         ScreenshotStore
}

type ingestService struct {
	db                      db.Pool
	queries                 *dbq.Queries
	maxEventsPerBatch       int
	reportRateLimitSeconds  int
	allowScreenshotFailures bool
	screenshotStore         ScreenshotStore
}

func NewIngestService(opts IngestOptions) Ingest {
	maxEvents := opts.MaxEventsPerBatch
	if maxEvents <= 0 {
		maxEvents = 50
	}
	reportRateLimit := opts.ReportRateLimitSeconds
	if reportRateLimit <= 0 {
		reportRateLimit = 60
	}

	return &ingestService{
		db:                      opts.DB,
		queries:                 dbq.New(opts.DB),
		maxEventsPerBatch:       maxEvents,
		reportRateLimitSeconds:  reportRateLimit,
		allowScreenshotFailures: opts.AllowScreenshotFailures,
		screenshotStore:         opts.ScreenshotStore,
	}
}

func (s *ingestService) IngestEvents(ctx context.Context, authProjectID uuid.UUID, req EventsRequest) (EventsResponse, error) {
	if strings.TrimSpace(req.ProjectID) == "" || strings.TrimSpace(req.BatchID) == "" {
		return EventsResponse{}, fmt.Errorf("%w: project_id and batch_id are required", ErrBadRequest)
	}
	if len(req.Events) == 0 {
		return EventsResponse{}, fmt.Errorf("%w: events must not be empty", ErrBadRequest)
	}
	if len(req.Events) > s.maxEventsPerBatch {
		return EventsResponse{}, fmt.Errorf("%w: batch contains %d events, max is %d", ErrPayloadTooLarge, len(req.Events), s.maxEventsPerBatch)
	}
	if err := validateClient(req.Client); err != nil {
		return EventsResponse{}, err
	}
	if req.SentAt != "" {
		if _, err := parseTime(req.SentAt); err != nil {
			return EventsResponse{}, fmt.Errorf("%w: sent_at must be RFC3339 UTC", ErrBadRequest)
		}
	}

	project, err := s.loadAuthorizedProject(ctx, authProjectID, req.ProjectID)
	if err != nil {
		return EventsResponse{}, err
	}

	existing, err := s.queries.GetBatchByProjectAndBatchID(ctx, dbq.GetBatchByProjectAndBatchIDParams{
		ProjectID: project.ID,
		BatchID:   req.BatchID,
	})
	if err == nil {
		return EventsResponse{
			Accepted:   existing.AcceptedCount,
			Rejected:   existing.RejectedCount,
			BatchID:    req.BatchID,
			ServerTime: serverTime(),
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return EventsResponse{}, err
	}

	validEvents, rejected := validateEvents(req.Events)
	if len(validEvents) == 0 {
		return EventsResponse{}, fmt.Errorf("%w: no valid events in batch", ErrBadRequest)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EventsResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)
	requestMeta, err := json.Marshal(map[string]any{
		"sent_at": req.SentAt,
		"client":  req.Client,
	})
	if err != nil {
		return EventsResponse{}, err
	}

	batch, err := q.CreateBatch(ctx, dbq.CreateBatchParams{
		ProjectID:     project.ID,
		BatchID:       req.BatchID,
		AcceptedCount: int32(len(validEvents)),
		RejectedCount: int32(rejected),
		RequestMeta:   requestMeta,
	})
	if err != nil {
		return EventsResponse{}, err
	}

	for _, event := range validEvents {
		eventID, err := q.CreateEvent(ctx, createEventParams(project.ID, nullableUUID(batch.ID), req.Client, event, nil))
		if err != nil {
			return EventsResponse{}, err
		}
		if err := createEventFieldProjections(ctx, q, project.ID, eventID, project.QueryFields, event); err != nil {
			return EventsResponse{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return EventsResponse{}, err
	}

	return EventsResponse{
		Accepted:   int32(len(validEvents)),
		Rejected:   int32(rejected),
		BatchID:    req.BatchID,
		ServerTime: serverTime(),
	}, nil
}

func (s *ingestService) SubmitBugReport(ctx context.Context, authProjectID uuid.UUID, req BugReportRequest) (BugReportResponse, error) {
	if strings.TrimSpace(req.ProjectID) == "" || strings.TrimSpace(req.ReportID) == "" {
		return BugReportResponse{}, fmt.Errorf("%w: project_id and report_id are required", ErrBadRequest)
	}
	if err := validateClient(req.Client); err != nil {
		return BugReportResponse{}, err
	}
	if strings.TrimSpace(req.Event.EventType) != "bug_report" {
		return BugReportResponse{}, fmt.Errorf("%w: event_type must be bug_report", ErrBadRequest)
	}
	if err := validateEvent(req.Event); err != nil {
		return BugReportResponse{}, err
	}

	project, err := s.loadAuthorizedProject(ctx, authProjectID, req.ProjectID)
	if err != nil {
		return BugReportResponse{}, err
	}

	commanderID, err := uuid.Parse(req.Event.CommanderID)
	if err != nil {
		return BugReportResponse{}, fmt.Errorf("%w: commander_id must be a UUID", ErrBadRequest)
	}

	recent, err := s.queries.CountRecentBugReportsByCommander(ctx, dbq.CountRecentBugReportsByCommanderParams{
		ProjectID:   project.ID,
		CommanderID: commanderID,
		Secs:        float64(s.reportRateLimitSeconds),
	})
	if err != nil {
		return BugReportResponse{}, err
	}
	if recent > 0 {
		return BugReportResponse{}, fmt.Errorf("%w: only one report is allowed every %d seconds", ErrRateLimited, s.reportRateLimitSeconds)
	}

	reportPayload, err := bugReportPayload(req.Event.Payload)
	if err != nil {
		return BugReportResponse{}, err
	}

	eventTime, err := parseTime(req.Event.RealTS)
	if err != nil {
		return BugReportResponse{}, fmt.Errorf("%w: real_ts must be RFC3339 UTC", ErrBadRequest)
	}

	screenshotKey, screenshotErr := s.storeScreenshot(ctx, project.ProjectKey, req.ReportID, eventTime, reportPayload.ScreenshotPNGBase64)
	if screenshotErr != nil && !s.allowScreenshotFailures {
		return BugReportResponse{}, fmt.Errorf("%w: screenshot storage failed: %v", ErrBadRequest, screenshotErr)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BugReportResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)
	storedEvent := req.Event
	storedEvent.Payload, err = normalizedBugReportPayload(req.Event.Payload, screenshotKey)
	if err != nil {
		return BugReportResponse{}, err
	}
	storedEvent.Raw = eventRawWithPayload(req.Event.Raw, storedEvent.Payload)

	eventID, err := q.CreateEvent(ctx, createEventParams(project.ID, pgtype.UUID{}, req.Client, storedEvent, nil))
	if err != nil {
		return BugReportResponse{}, err
	}
	if err := createEventFieldProjections(ctx, q, project.ID, eventID, project.QueryFields, storedEvent); err != nil {
		return BugReportResponse{}, err
	}

	storageError := pgtype.Text{}
	if screenshotErr != nil {
		storageError = pgtype.Text{String: screenshotErr.Error(), Valid: true}
	}
	report, err := q.CreateBugReport(ctx, dbq.CreateBugReportParams{
		ProjectID:              project.ID,
		ReportID:               req.ReportID,
		EventID:                eventID,
		Mood:                   int32(reportPayload.Mood),
		MoodLabel:              reportPayload.MoodLabel,
		NotesPreview:           truncate(reportPayload.Notes, 500),
		ScreenshotObjectKey:    optionalText(screenshotKey),
		ScreenshotStorageError: storageError,
	})
	if err != nil {
		return BugReportResponse{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return BugReportResponse{}, err
	}

	objectKey := screenshotKey
	if report.ScreenshotObjectKey.Valid {
		objectKey = report.ScreenshotObjectKey.String
	}
	return BugReportResponse{
		Accepted:            true,
		ReportID:            req.ReportID,
		ScreenshotObjectKey: objectKey,
		ServerTime:          serverTime(),
	}, nil
}

func (s *ingestService) loadAuthorizedProject(ctx context.Context, authProjectID uuid.UUID, projectKey string) (dbq.GetProjectByKeyRow, error) {
	project, err := s.queries.GetProjectByKey(ctx, projectKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbq.GetProjectByKeyRow{}, fmt.Errorf("%w: unknown project_id", ErrForbidden)
		}
		return dbq.GetProjectByKeyRow{}, err
	}
	if project.ID != authProjectID {
		return dbq.GetProjectByKeyRow{}, ErrForbidden
	}
	return project, nil
}

func (s *ingestService) storeScreenshot(ctx context.Context, projectKey string, reportID string, eventTime time.Time, encoded string) (string, error) {
	if strings.TrimSpace(encoded) == "" {
		return "", nil
	}
	if s.screenshotStore == nil {
		return "", errors.New("screenshot store is not configured")
	}
	png, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return s.screenshotStore.StorePNG(ctx, projectKey, reportID, eventTime, png)
}

type bugReportPayloadFields struct {
	Mood                int    `json:"mood"`
	MoodLabel           string `json:"mood_label"`
	Notes               string `json:"notes"`
	ScreenshotPNGBase64 string `json:"screenshot_png_base64"`
}

func bugReportPayload(payload json.RawMessage) (bugReportPayloadFields, error) {
	var fields bugReportPayloadFields
	if err := json.Unmarshal(payload, &fields); err != nil {
		return fields, fmt.Errorf("%w: bug report payload must be an object", ErrBadRequest)
	}
	if fields.Mood < 1 || fields.Mood > 5 {
		return fields, fmt.Errorf("%w: mood must be between 1 and 5", ErrBadRequest)
	}
	if strings.TrimSpace(fields.MoodLabel) == "" {
		return fields, fmt.Errorf("%w: mood_label is required", ErrBadRequest)
	}
	return fields, nil
}

func normalizedBugReportPayload(payload json.RawMessage, screenshotObjectKey string) (json.RawMessage, error) {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, fmt.Errorf("%w: bug report payload must be an object", ErrBadRequest)
	}
	fields["screenshot_png_base64"] = nil
	if screenshotObjectKey != "" {
		fields["screenshot_object_key"] = screenshotObjectKey
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateEvents(events []EventEnvelope) ([]EventEnvelope, int) {
	valid := make([]EventEnvelope, 0, len(events))
	rejected := 0
	for _, event := range events {
		if err := validateEvent(event); err != nil {
			rejected++
			continue
		}
		valid = append(valid, event)
	}
	return valid, rejected
}

func validateEvent(event EventEnvelope) error {
	if event.SchemaVersion != 2 {
		return fmt.Errorf("%w: schema_version must be 2", ErrBadRequest)
	}
	if _, err := uuid.Parse(event.CommanderID); err != nil {
		return fmt.Errorf("%w: commander_id must be a UUID", ErrBadRequest)
	}
	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("%w: event_type is required", ErrBadRequest)
	}
	if _, err := parseTime(event.RealTS); err != nil {
		return fmt.Errorf("%w: real_ts must be RFC3339 UTC", ErrBadRequest)
	}
	if err := validateJSONObject(event.Context, "context", false); err != nil {
		return err
	}
	if err := validateJSONObject(event.Metrics, "metrics", false); err != nil {
		return err
	}
	if err := validateJSONObject(event.Dimensions, "dimensions", false); err != nil {
		return err
	}
	location, err := eventLocation(event.Context)
	if err != nil {
		return err
	}
	if len(location.Position) != 0 && len(location.Position) != 3 {
		return fmt.Errorf("%w: context.location.position must contain x, y, z", ErrBadRequest)
	}
	if err := validateJSONObject(event.Payload, "payload", true); err != nil {
		return err
	}
	return nil
}

func validateClient(client ClientInfo) error {
	if strings.TrimSpace(client.GameVersion) == "" ||
		strings.TrimSpace(client.BuildChannel) == "" ||
		strings.TrimSpace(client.Platform) == "" {
		return fmt.Errorf("%w: client.game_version, client.build_channel, and client.platform are required", ErrBadRequest)
	}
	return nil
}

// createEventParams maps a validated event onto the database insert parameters.
// Parse errors are intentionally ignored: every input must pass validateEvent
// and validateClient before reaching this function, so uuid.Parse, parseTime,
// and eventLocation cannot fail on pre-validated data.
func createEventParams(projectID uuid.UUID, batchID pgtype.UUID, client ClientInfo, event EventEnvelope, validationErrors []string) dbq.CreateEventParams {
	commanderID, _ := uuid.Parse(event.CommanderID)
	realTS, _ := parseTime(event.RealTS)
	location, _ := eventLocation(event.Context)
	contextJSON := normalizedJSONObject(event.Context)
	metricsJSON := normalizedJSONObject(event.Metrics)
	dimensionsJSON := normalizedJSONObject(event.Dimensions)
	payload := normalizedJSONObject(event.Payload)
	eventJSON := normalizedEventJSON(event, contextJSON, metricsJSON, dimensionsJSON, payload)
	errorsJSON, _ := json.Marshal(validationErrors)

	return dbq.CreateEventParams{
		ProjectID:        projectID,
		BatchDbID:        batchID,
		CommanderID:      commanderID,
		EventType:        event.EventType,
		RealTs:           realTS,
		GameTime:         event.GameTime,
		SystemID:         location.WorldID,
		ZoneID:           location.AreaID,
		CoordX:           location.Position[0],
		CoordY:           location.Position[1],
		CoordZ:           location.Position[2],
		GameVersion:      client.GameVersion,
		BuildChannel:     client.BuildChannel,
		Platform:         client.Platform,
		Context:          contextJSON,
		Metrics:          metricsJSON,
		Dimensions:       dimensionsJSON,
		Payload:          payload,
		EventJson:        eventJSON,
		ValidationErrors: errorsJSON,
	}
}

func createEventFieldProjections(ctx context.Context, q *dbq.Queries, projectID uuid.UUID, eventID uuid.UUID, queryFields json.RawMessage, event EventEnvelope) error {
	projections, err := projectEventFields(queryFields, event)
	if err != nil {
		return err
	}
	for _, projection := range projections {
		if err := q.CreateEventField(ctx, dbq.CreateEventFieldParams{
			EventID:     eventID,
			ProjectID:   projectID,
			FieldKey:    projection.key,
			ValueType:   projection.valueType,
			StringValue: projection.stringValue,
			NumberValue: projection.numberValue,
			BoolValue:   projection.boolValue,
		}); err != nil {
			return err
		}
	}
	return nil
}

func projectEventFields(queryFields json.RawMessage, event EventEnvelope) ([]eventFieldProjection, error) {
	if len(queryFields) == 0 || string(queryFields) == "null" {
		return nil, nil
	}
	var fields []QueryFieldDefinition
	if err := json.Unmarshal(queryFields, &fields); err != nil {
		return nil, fmt.Errorf("project query_fields must be an array: %w", err)
	}
	out := make([]eventFieldProjection, 0, len(fields))
	for _, field := range fields {
		key := strings.TrimSpace(field.Key)
		source := strings.TrimSpace(field.Source)
		valueType := strings.TrimSpace(field.Type)
		if key == "" || source == "" || valueType == "" {
			continue
		}
		value, ok := eventSourceValue(event, source)
		if !ok {
			continue
		}
		projection, ok := newEventFieldProjection(key, valueType, value)
		if ok {
			out = append(out, projection)
		}
	}
	return out, nil
}

func newEventFieldProjection(key string, valueType string, value any) (eventFieldProjection, bool) {
	projection := eventFieldProjection{key: key, valueType: valueType}
	switch valueType {
	case "string":
		strValue, ok := value.(string)
		if !ok {
			return eventFieldProjection{}, false
		}
		projection.stringValue = pgtype.Text{String: strValue, Valid: true}
	case "number":
		numberValue, ok := value.(float64)
		if !ok {
			return eventFieldProjection{}, false
		}
		projection.numberValue = pgtype.Float8{Float64: numberValue, Valid: true}
	case "bool":
		boolValue, ok := value.(bool)
		if !ok {
			return eventFieldProjection{}, false
		}
		projection.boolValue = pgtype.Bool{Bool: boolValue, Valid: true}
	default:
		return eventFieldProjection{}, false
	}
	return projection, true
}

func eventSourceValue(event EventEnvelope, source string) (any, bool) {
	root, path, ok := strings.Cut(source, ".")
	if !ok {
		return nil, false
	}
	var raw json.RawMessage
	switch root {
	case "context":
		raw = event.Context
	case "metrics":
		raw = event.Metrics
	case "dimensions":
		raw = event.Dimensions
	case "payload":
		raw = event.Payload
	default:
		return nil, false
	}
	return rawObjectValue(raw, path)
}

// rawObjectValue extracts a value from a JSON object by key or dot-separated path.
// An exact key match (e.g. "a.b" as a literal key) takes priority over the
// nested path traversal (a → b). Keep field keys unambiguous to avoid surprises.
func rawObjectValue(raw json.RawMessage, path string) (any, bool) {
	var values map[string]any
	if err := json.Unmarshal(normalizedJSONObject(raw), &values); err != nil {
		return nil, false
	}
	if value, ok := values[path]; ok {
		return value, true
	}
	parts := strings.Split(path, ".")
	var current any = values
	for _, part := range parts {
		currentMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = currentMap[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func eventLocation(rawContext json.RawMessage) (EventLocation, error) {
	location := EventLocation{
		WorldID:  "unknown",
		AreaID:   "unknown",
		Position: []float64{0, 0, 0},
	}
	if len(rawContext) == 0 || string(rawContext) == "null" {
		return location, nil
	}
	var context EventContext
	if err := json.Unmarshal(rawContext, &context); err != nil {
		return location, fmt.Errorf("%w: context must be an object", ErrBadRequest)
	}
	if strings.TrimSpace(context.Location.WorldID) != "" {
		location.WorldID = context.Location.WorldID
	}
	if strings.TrimSpace(context.Location.AreaID) != "" {
		location.AreaID = context.Location.AreaID
	}
	if len(context.Location.Position) > 0 {
		location.Position = context.Location.Position
	}
	return location, nil
}

func validateJSONObject(raw json.RawMessage, name string, required bool) error {
	if len(raw) == 0 || string(raw) == "null" {
		if required {
			return fmt.Errorf("%w: %s is required", ErrBadRequest, name)
		}
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("%w: %s must be an object", ErrBadRequest, name)
	}
	return nil
}

func normalizedJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

func normalizedEventJSON(event EventEnvelope, contextJSON json.RawMessage, metricsJSON json.RawMessage, dimensionsJSON json.RawMessage, payload json.RawMessage) json.RawMessage {
	if len(event.Raw) > 0 {
		return event.Raw
	}
	eventJSON, err := json.Marshal(map[string]any{
		"schema_version": event.SchemaVersion,
		"commander_id":   event.CommanderID,
		"event_type":     event.EventType,
		"real_ts":        event.RealTS,
		"game_time":      event.GameTime,
		"context":        json.RawMessage(contextJSON),
		"metrics":        json.RawMessage(metricsJSON),
		"dimensions":     json.RawMessage(dimensionsJSON),
		"payload":        json.RawMessage(payload),
	})
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return eventJSON
}

func eventRawWithPayload(raw json.RawMessage, payload json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil
	}
	var payloadValue any
	if err := json.Unmarshal(payload, &payloadValue); err != nil {
		return nil
	}
	event["payload"] = payloadValue
	updated, err := json.Marshal(event)
	if err != nil {
		return nil
	}
	return updated
}

func optionalText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func nullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339, value)
}

func serverTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// truncate trims value to at most limit bytes. This is a byte-count limit, not
// a rune count — it can split a multi-byte UTF-8 character if the limit falls
// mid-sequence. Callers storing in text columns should be aware of this.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
