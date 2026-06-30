package services

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("admin projects", func() {
	It("normalizes create project requests into upsert params", func() {
		params, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example_game",
			DisplayName: "Example Game",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(params.ProjectKey).To(Equal("example_game"))
		Expect(params.DisplayName).To(Equal("Example Game"))
		Expect(params.ValidationMode).To(Equal("warn"))
		Expect(json.Valid(params.IngestConfig)).To(BeTrue())
		Expect(json.Valid(params.QueryFields)).To(BeTrue())
		Expect(string(params.EventGroups)).To(Equal("{}"))
		Expect(string(params.QueryFields)).To(Equal("[]"))
		Expect(string(params.Funnels)).To(Equal("[]"))
	})

	It("preserves explicit false booleans and zero values where allowed", func() {
		params, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			IngestConfig: ProjectIngestConfigInput{
				AcceptGzip:              boolPtr(false),
				AllowUnknownEventTypes:  boolPtr(false),
				AllowScreenshotFailures: boolPtr(false),
			},
			RetentionConfig: ProjectRetentionConfigInput{
				AccessLogDays: intPtr(0),
			},
			ReportConfig: ProjectReportConfigInput{
				RateLimitSeconds: intPtr(0),
			},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(string(params.IngestConfig)).To(ContainSubstring(`"accept_gzip":false`))
		Expect(string(params.IngestConfig)).To(ContainSubstring(`"allow_unknown_event_types":false`))
		Expect(string(params.IngestConfig)).To(ContainSubstring(`"allow_screenshot_failures":false`))
		Expect(string(params.RetentionConfig)).To(ContainSubstring(`"access_log_days":0`))
		Expect(string(params.ReportConfig)).To(ContainSubstring(`"rate_limit_seconds":0`))
	})

	It("rejects negative typed config values", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			RetentionConfig: ProjectRetentionConfigInput{
				EventDays: intPtr(-1),
			},
		})

		Expect(err).To(MatchError(ContainSubstring("retention days must be non-negative")))
	})

	It("validates report and event group shapes", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			ReportConfig: ProjectReportConfigInput{
				Statuses: []string{"new", "new"},
			},
		})

		Expect(err).To(MatchError(ContainSubstring("duplicate report statuses value")))

		_, err = createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			EventGroups: map[string][]string{
				"lifecycle": {"dock", "dock"},
			},
		})

		Expect(err).To(MatchError(ContainSubstring("duplicate event type in event group")))
	})

	It("rejects unsafe project keys", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "Example Game",
			DisplayName: "Example Game",
		})

		Expect(err).To(MatchError(ContainSubstring("project_id must use lowercase")))
	})

	It("does not substitute a default project key", func() {
		Expect(requiredProjectKey("")).To(BeEmpty())
		Expect(requiredProjectKey(" example ")).To(Equal("example"))
	})

	It("rejects unsupported query field types", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: []QueryFieldDefinition{{Key: "ship", Source: "dimensions.ship", Type: "object"}},
		})

		Expect(err).To(MatchError(ContainSubstring("query_fields type must be string, number, or bool")))
	})

	It("requires query field sources to include a root and dotted path", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: []QueryFieldDefinition{{Key: "ship.id", Source: "dimensions", Type: "string"}},
		})

		Expect(err).To(MatchError(ContainSubstring("query_fields source must use context, metrics, dimensions, or payload plus a dotted path")))
	})

	It("defaults query field labels to the key", func() {
		params, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: []QueryFieldDefinition{{
				Key:          "ship.id",
				Source:       "dimensions.ship.id",
				Type:         "string",
				Aggregations: []string{"not-a-real-aggregation"},
			}},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(string(params.QueryFields)).To(ContainSubstring(`"label":"ship.id"`))
		Expect(string(params.QueryFields)).To(ContainSubstring(`"not-a-real-aggregation"`))
	})

	It("rejects duplicate query field keys", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: []QueryFieldDefinition{
				{Key: "ship.id", Source: "dimensions.ship.id", Type: "string"},
				{Key: "ship.id", Source: "dimensions.ship.name", Type: "string"},
			},
		})

		Expect(err).To(MatchError(ContainSubstring("duplicate query_fields key")))
	})

	It("accepts valid project funnels", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: []QueryFieldDefinition{{Key: "ship.id", Source: "dimensions.ship.id", Type: "string", Filterable: true}},
			Funnels: []FunnelDefinition{{
				ID:     "first_ship",
				Name:   "First ship seen",
				Entity: "player",
				Steps: []FunnelStepConfig{{
					ID:    "seen",
					Label: "Seen",
					Match: FunnelEventMatcher{FieldKey: "ship.id", FieldValue: json.RawMessage(`"cobra"`)},
				}},
			}},
		})

		Expect(err).ToNot(HaveOccurred())
	})

	It("rejects funnel field matchers for non-filterable query fields", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: []QueryFieldDefinition{{Key: "ship.id", Source: "dimensions.ship.id", Type: "string", Filterable: false}},
			Funnels: []FunnelDefinition{{
				ID:     "first_ship",
				Name:   "First ship seen",
				Entity: "player",
				Steps: []FunnelStepConfig{{
					ID:    "seen",
					Label: "Seen",
					Match: FunnelEventMatcher{FieldKey: "ship.id"},
				}},
			}},
		})

		Expect(err).To(MatchError(ContainSubstring("funnel field_key must be filterable")))
	})

	It("builds unordered presence funnels as cumulative DB-side counts", func() {
		sql, args, err := buildFunnelCountsQuery(uuid.New(), time.Now().Add(-time.Hour), time.Now(), FunnelDefinition{
			ID:     "trade",
			Name:   "Trade",
			Entity: "player",
			Mode:   "unordered_presence",
			Steps: []FunnelStepConfig{
				{ID: "buy", Label: "Buy", Match: FunnelEventMatcher{EventType: "buy"}},
				{ID: "sell", Label: "Sell", Match: FunnelEventMatcher{EventType: "sell"}},
			},
		}, map[string]QueryFieldDefinition{})

		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(ContainSubstring("step_1 AS (SELECT DISTINCT e.player_id FROM events e"))
		Expect(sql).To(ContainSubstring("step_2 AS (SELECT DISTINCT e.player_id FROM events e JOIN step_1 prev ON prev.player_id = e.player_id"))
		Expect(sql).To(ContainSubstring("(SELECT count(*)::bigint FROM step_1) AS step_1_count"))
		Expect(sql).To(ContainSubstring("(SELECT count(*)::bigint FROM step_2) AS step_2_count"))
		Expect(sql).ToNot(ContainSubstring("e.game_time >= prev.first_game_time"))
		Expect(args).To(HaveLen(5))
	})

	It("builds ordered funnels as DB-side progression counts", func() {
		withinSeconds := int64(600)
		sql, args, err := buildFunnelCountsQuery(uuid.New(), time.Now().Add(-time.Hour), time.Now(), FunnelDefinition{
			ID:     "trade",
			Name:   "Trade",
			Entity: "player",
			Steps: []FunnelStepConfig{
				{ID: "buy", Label: "Buy", Match: FunnelEventMatcher{EventType: "buy"}},
				{ID: "sell", Label: "Sell", Match: FunnelEventMatcher{EventType: "sell"}, After: "buy", WithinSeconds: &withinSeconds},
			},
		}, map[string]QueryFieldDefinition{})

		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(ContainSubstring("SELECT DISTINCT ON (e.player_id) e.player_id, e.game_time AS first_game_time, e.real_ts AS first_real_ts"))
		Expect(sql).To(ContainSubstring("JOIN step_1 prev ON prev.player_id = e.player_id"))
		Expect(sql).To(ContainSubstring("e.game_time >= prev.first_game_time"))
		Expect(sql).To(ContainSubstring("e.real_ts <= prev.first_real_ts + ("))
		Expect(args).To(HaveLen(6))
	})

	It("builds typed field, event type, region, and zone funnel matchers", func() {
		sql, args, err := buildFunnelCountsQuery(uuid.New(), time.Now().Add(-time.Hour), time.Now(), FunnelDefinition{
			ID:     "ship_region",
			Name:   "Ship region",
			Entity: "player",
			Steps: []FunnelStepConfig{
				{
					ID:    "seen",
					Label: "Seen",
					Match: FunnelEventMatcher{
						EventTypes: []string{"dock", "undock"},
						FieldKey:   "ship.id",
						FieldValue: json.RawMessage(`"cobra"`),
						RegionID:   "lave",
						ZoneID:     "lave_primary",
					},
				},
			},
		}, map[string]QueryFieldDefinition{
			"ship.id": {Key: "ship.id", Type: "string", Filterable: true},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(ContainSubstring("e.event_type = ANY("))
		Expect(sql).To(ContainSubstring("e.region_id = "))
		Expect(sql).To(ContainSubstring("e.zone_id = "))
		Expect(sql).To(ContainSubstring("JOIN event_fields ef ON ef.event_id = e.id AND ef.project_id = e.project_id"))
		Expect(sql).To(ContainSubstring("ef.value_type = "))
		Expect(sql).To(ContainSubstring("ef.string_value = "))
		Expect(strings.Join(strings.Fields(sql), " ")).ToNot(ContainSubstring("event_type = dock"))
		Expect(args).To(HaveLen(9))
	})

	It("converts project settings rows through tolerant typed defaults", func() {
		settings := projectSettingsFromRaw(rawProjectSettings{
			ProjectKey:      "example",
			DisplayName:     "Example",
			ValidationMode:  "warn",
			IngestConfig:    json.RawMessage(`{"max_events_per_batch":"bad"}`),
			RetentionConfig: json.RawMessage(`{"event_days":90}`),
			MapConfig:       json.RawMessage(`{"spatial_enabled":false}`),
			ReportConfig:    json.RawMessage(`{"statuses":["new"],"labels":[]}`),
			EventGroups:     json.RawMessage(`{" report ":[" dock "]}`),
			QueryFields:     json.RawMessage(`[{"key":"ship.id","source":"dimensions.ship.id","type":"string"}]`),
			Funnels:         json.RawMessage(`not json`),
		})

		Expect(settings.IngestConfig.MaxEventsPerBatch).To(Equal(50))
		Expect(settings.RetentionConfig.EventDays).To(Equal(90))
		Expect(settings.RetentionConfig.ReportDays).To(Equal(1095))
		Expect(settings.MapConfig.SpatialEnabled).To(BeFalse())
		Expect(settings.ReportConfig.Statuses).To(Equal([]string{"new"}))
		Expect(settings.ReportConfig.Labels).To(BeEmpty())
		Expect(settings.EventGroups).To(Equal(map[string][]string{"report": {"dock"}}))
		Expect(settings.QueryFields).To(HaveLen(1))
		Expect(settings.QueryFields[0].Label).To(Equal("ship.id"))
		Expect(settings.Funnels).To(BeEmpty())
	})

	It("drops unknown stored config keys during typed settings conversion", func() {
		settings := projectSettingsFromRaw(rawProjectSettings{
			ProjectKey:      "example",
			DisplayName:     "Example",
			ValidationMode:  "warn",
			IngestConfig:    json.RawMessage(`{"max_events_per_batch":25,"unknown_ingest_key":true}`),
			RetentionConfig: json.RawMessage(`{"event_days":90,"report_days":180,"access_log_days":0,"unknown_retention_key":true}`),
			MapConfig:       json.RawMessage(`{"spatial_enabled":false,"zone_extent_m":1,"zone_heatmap_cell_m":1,"unknown_map_key":true}`),
			ReportConfig:    json.RawMessage(`{"statuses":["new"],"labels":[],"rate_limit_seconds":0,"unknown_report_key":true}`),
			EventGroups:     json.RawMessage(`{"lifecycle":["dock"],"unknown_group":["event"]}`),
			QueryFields:     json.RawMessage(`[{"key":"ship.id","source":"dimensions.ship.id","type":"string","unknown_query_key":true}]`),
			Funnels:         json.RawMessage(`[]`),
		})

		rawIngest, err := json.Marshal(settings.IngestConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rawIngest)).ToNot(ContainSubstring("unknown_ingest_key"))
		Expect(settings.IngestConfig.MaxEventsPerBatch).To(Equal(25))
		Expect(settings.RetentionConfig.AccessLogDays).To(Equal(0))
		Expect(settings.MapConfig.SpatialEnabled).To(BeFalse())
		Expect(settings.ReportConfig.RateLimitSeconds).To(Equal(0))
		Expect(settings.EventGroups).To(HaveKeyWithValue("unknown_group", []string{"event"}))
		Expect(settings.QueryFields).To(HaveLen(1))
		Expect(settings.QueryFields[0].Label).To(Equal("ship.id"))
	})
})

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
