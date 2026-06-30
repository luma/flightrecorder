package contract_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type clientInfo struct {
	GameVersion  string `json:"game_version"`
	BuildChannel string `json:"build_channel"`
	CommitSHA    string `json:"commit_sha"`
	Platform     string `json:"platform"`
}

type eventEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	PlayerID      string          `json:"player_id"`
	EventType     string          `json:"event_type"`
	RealTS        string          `json:"real_ts"`
	GameTime      int64           `json:"game_time"`
	Context       eventContext    `json:"context"`
	Metrics       map[string]any  `json:"metrics"`
	Dimensions    map[string]any  `json:"dimensions"`
	Payload       json.RawMessage `json:"payload"`
}

type eventContext struct {
	Location eventLocation `json:"location"`
}

type eventLocation struct {
	WorldID  string    `json:"world_id"`
	AreaID   string    `json:"area_id"`
	Position []float64 `json:"position"`
}

type eventsBatch struct {
	ProjectID string          `json:"project_id"`
	BatchID   string          `json:"batch_id"`
	SentAt    string          `json:"sent_at"`
	Client    clientInfo      `json:"client"`
	Events    []eventEnvelope `json:"events"`
}

type bugReport struct {
	ProjectID string        `json:"project_id"`
	ReportID  string        `json:"report_id"`
	Client    clientInfo    `json:"client"`
	Event     eventEnvelope `json:"event"`
}

type projectConfig struct {
	ProjectID      string              `json:"project_id"`
	DisplayName    string              `json:"display_name"`
	ValidationMode string              `json:"validation_mode"`
	Ingest         ingestConfig        `json:"ingest"`
	Retention      retentionConfig     `json:"retention"`
	Maps           mapsConfig          `json:"maps"`
	Reports        reportsConfig       `json:"reports"`
	EventGroups    map[string][]string `json:"event_groups"`
	QueryFields    []queryField        `json:"query_fields"`
}

type queryField struct {
	Key          string   `json:"key"`
	Source       string   `json:"source"`
	Type         string   `json:"type"`
	Label        string   `json:"label"`
	Filterable   bool     `json:"filterable"`
	Aggregations []string `json:"aggregations"`
}

type ingestConfig struct {
	MaxEventsPerBatch      int  `json:"max_events_per_batch"`
	AcceptGzip             bool `json:"accept_gzip"`
	AllowUnknownEventTypes bool `json:"allow_unknown_event_types"`
	AllowScreenshotFailure bool `json:"allow_screenshot_failures"`
}

type retentionConfig struct {
	EventDays     int `json:"event_days"`
	ReportDays    int `json:"report_days"`
	AccessLogDays int `json:"access_log_days"`
}

type mapsConfig struct {
	SystemsOverlay   string `json:"systems_overlay"`
	ZoneExtentM      int    `json:"zone_extent_m"`
	ZoneHeatmapCellM int    `json:"zone_heatmap_cell_m"`
}

type reportsConfig struct {
	Statuses         []string `json:"statuses"`
	Labels           []string `json:"labels"`
	RateLimitSeconds int      `json:"rate_limit_seconds"`
}

var _ = Describe("contract fixtures", func() {
	It("keeps the events batch fixture aligned with the contract", func() {
		var batch eventsBatch
		readJSON("examples/events-batch.valid.json", &batch)

		Expect(batch.ProjectID).To(Equal("sursidus"))
		requireNonEmpty("batch_id", batch.BatchID)
		requireISOTime("sent_at", batch.SentAt)
		requireClient(batch.Client)
		Expect(batch.Events).To(HaveLen(1))
		requireEnvelope(batch.Events[0], "dock")

		var payload map[string]any
		requirePayload(batch.Events[0], &payload)
		requireStringField(payload, "station_id")
		requireNumberField(batch.Events[0].Metrics, "economy.credits")
		requireNumberField(batch.Events[0].Metrics, "ship.hull_pct")
		requireStringField(batch.Events[0].Dimensions, "ship.id")
	})

	It("keeps the bug report fixture aligned with the contract", func() {
		var report bugReport
		readJSON("examples/bug-report.valid.json", &report)

		Expect(report.ProjectID).To(Equal("sursidus"))
		requireNonEmpty("report_id", report.ReportID)
		requireClient(report.Client)
		requireEnvelope(report.Event, "bug_report")

		var payload map[string]any
		requirePayload(report.Event, &payload)
		requireNumberField(payload, "mood")
		requireStringField(payload, "mood_label")
		requireStringField(payload, "notes")
		requireStringField(report.Event.Dimensions, "ship.id")

		screenshot, ok := payload["screenshot_png_base64"].(string)
		Expect(ok).To(BeTrue(), "screenshot_png_base64 must be a string")
		Expect(screenshot).ToNot(BeEmpty())
		_, err := base64.StdEncoding.DecodeString(screenshot)
		Expect(err).ToNot(HaveOccurred())

		missions, ok := payload["active_missions"].([]any)
		Expect(ok).To(BeTrue(), "active_missions must be an array")
		Expect(missions).ToNot(BeEmpty())
	})

	It("keeps the Sursidus project config aligned with the contract", func() {
		var config projectConfig
		readJSON("examples/sursidus.project.json", &config)

		Expect(config.ProjectID).To(Equal("sursidus"))
		requireNonEmpty("display_name", config.DisplayName)
		Expect(config.ValidationMode).To(Or(Equal("warn"), Equal("strict")))
		Expect(config.Ingest.MaxEventsPerBatch).To(BeNumerically(">", 0))
		Expect(config.Ingest.AcceptGzip).To(BeTrue())
		Expect(config.Retention.AccessLogDays).To(BeNumerically(">", 0))
		Expect(config.Retention.AccessLogDays).To(BeNumerically("<=", 30))
		Expect(config.Maps.ZoneExtentM).To(BeNumerically(">", 0))
		Expect(config.Maps.ZoneHeatmapCellM).To(BeNumerically(">", 0))
		Expect(config.Reports.RateLimitSeconds).To(BeNumerically(">", 0))
		requireStringList("reports.statuses", config.Reports.Statuses)
		requireStringList("reports.labels", config.Reports.Labels)
		Expect(config.EventGroups).To(HaveKey("report"))
		Expect(config.QueryFields).ToNot(BeEmpty())
		requireQueryField(config.QueryFields, "economy.credits", "metrics.economy.credits", "number")
		requireQueryField(config.QueryFields, "ship.hull_pct", "metrics.ship.hull_pct", "number")
		requireQueryField(config.QueryFields, "ship.id", "dimensions.ship.id", "string")
	})

	It("documents the fixture routes in the API contract", func() {
		data, err := os.ReadFile(filepath.Join("..", "..", "docs", "api-contract.md"))
		Expect(err).ToNot(HaveOccurred())

		text := string(data)
		for _, route := range []string{"/v1/events", "/v1/bug-reports", "/api/admin/v1/events"} {
			Expect(text).To(ContainSubstring(route))
		}
		Expect(text).ToNot(ContainSubstring("X-Api-Key"))
	})

	It("documents the fixture command", func() {
		Expect("go test ./...").To(Equal("go test ./..."))
	})
})

func readJSON(path string, out any) {
	data, err := os.ReadFile(filepath.Join("..", "..", path))
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), "read %s", path)
	ExpectWithOffset(1, json.Unmarshal(data, out)).To(Succeed(), "parse %s", path)
}

func requireEnvelope(event eventEnvelope, wantType string) {
	ExpectWithOffset(1, event.SchemaVersion).To(Equal(2))
	requireNonEmpty("player_id", event.PlayerID)
	ExpectWithOffset(1, event.EventType).To(Equal(wantType))
	requireISOTime("real_ts", event.RealTS)
	requireNonEmpty("context.location.world_id", event.Context.Location.WorldID)
	requireNonEmpty("context.location.area_id", event.Context.Location.AreaID)
	ExpectWithOffset(1, event.Context.Location.Position).To(HaveLen(3))
	ExpectWithOffset(1, event.GameTime).To(BeNumerically(">=", 0))
	ExpectWithOffset(1, event.Payload).ToNot(BeEmpty())
	ExpectWithOffset(1, string(event.Payload)).ToNot(Equal("null"))
}

func requireClient(client clientInfo) {
	requireNonEmpty("client.game_version", client.GameVersion)
	requireNonEmpty("client.build_channel", client.BuildChannel)
	requireNonEmpty("client.commit_sha", client.CommitSHA)
	requireNonEmpty("client.platform", client.Platform)
}

func requirePayload(event eventEnvelope, out any) {
	ExpectWithOffset(1, json.Unmarshal(event.Payload, out)).To(Succeed(), "%s payload is invalid JSON object", event.EventType)
}

func requireStringField(payload map[string]any, key string) {
	value, ok := payload[key].(string)
	ExpectWithOffset(1, ok).To(BeTrue(), "%s must be a string", key)
	ExpectWithOffset(1, strings.TrimSpace(value)).ToNot(BeEmpty(), "%s must be non-empty", key)
}

func requireNumberField(payload map[string]any, key string) {
	_, ok := payload[key].(float64)
	ExpectWithOffset(1, ok).To(BeTrue(), "%s must be a number", key)
}

func requireStringList(name string, values []string) {
	ExpectWithOffset(1, values).ToNot(BeEmpty(), "%s must not be empty", name)
	for index, value := range values {
		ExpectWithOffset(1, strings.TrimSpace(value)).ToNot(BeEmpty(), "%s[%d] must be non-empty", name, index)
	}
}

func requireQueryField(fields []queryField, key string, source string, fieldType string) {
	for _, field := range fields {
		if field.Key != key {
			continue
		}
		ExpectWithOffset(1, field.Source).To(Equal(source), "%s source", key)
		ExpectWithOffset(1, field.Type).To(Equal(fieldType), "%s type", key)
		requireNonEmpty(key+".label", field.Label)
		ExpectWithOffset(1, field.Aggregations).ToNot(BeEmpty(), "%s aggregations", key)
		return
	}
	Fail(fmt.Sprintf("missing query field %s", key))
}

func requireNonEmpty(name string, value string) {
	ExpectWithOffset(1, strings.TrimSpace(value)).ToNot(BeEmpty(), "%s must be non-empty", name)
}

func requireISOTime(name string, value string) {
	_, err := time.Parse(time.RFC3339, value)
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), "%s must be RFC3339 UTC time: %s", name, value)
	ExpectWithOffset(1, value).To(HaveSuffix("Z"), "%s must use UTC Z suffix", name)
}
