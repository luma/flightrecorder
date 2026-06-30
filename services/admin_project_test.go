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
		Expect(string(params.Funnels)).To(Equal("[]"))
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
			QueryFields: json.RawMessage(`[{"key":"ship","source":"dimensions.ship","type":"object"}]`),
		})

		Expect(err).To(MatchError(ContainSubstring("query_fields type must be string, number, or bool")))
	})

	It("accepts valid project funnels", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: json.RawMessage(`[{"key":"ship.id","source":"dimensions.ship.id","type":"string","filterable":true}]`),
			Funnels: json.RawMessage(`[
				{
					"id":"first_ship",
					"name":"First ship seen",
					"entity":"player",
					"steps":[{"id":"seen","label":"Seen","match":{"field_key":"ship.id","field_value":"cobra"}}]
				}
			]`),
		})

		Expect(err).ToNot(HaveOccurred())
	})

	It("rejects funnel field matchers for non-filterable query fields", func() {
		_, err := createProjectParams(CreateProjectRequest{
			ProjectID:   "example",
			DisplayName: "Example",
			QueryFields: json.RawMessage(`[{"key":"ship.id","source":"dimensions.ship.id","type":"string","filterable":false}]`),
			Funnels: json.RawMessage(`[
				{
					"id":"first_ship",
					"name":"First ship seen",
					"entity":"player",
					"steps":[{"id":"seen","label":"Seen","match":{"field_key":"ship.id"}}]
				}
			]`),
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
})
