package services

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ingest validation", func() {
	validEvent := func(eventType string) EventEnvelope {
		return EventEnvelope{
			SchemaVersion: 2,
			PlayerID:      "550e8400-e29b-41d4-a716-446655440000",
			EventType:     eventType,
			RealTS:        "2026-06-06T11:42:00Z",
			GameTime:      1843200,
			Context:       json.RawMessage(`{"location":{"region_id":"lave","zone_id":"lave_primary","position":[1240.5,-80.0,330.2]}}`),
			Metrics:       json.RawMessage(`{"economy.credits":48200,"ship.hull_pct":0.94,"ship.shield_pct":1.0}`),
			Dimensions:    json.RawMessage(`{"ship.id":"cobra_mk3"}`),
			Payload:       json.RawMessage(`{"station_id":"stn_6a11eb1c-5cab-46f3-a911-0f8e8a14e6cd"}`),
		}
	}

	It("accepts a valid event envelope", func() {
		Expect(validateEvent(validEvent("dock"))).To(Succeed())
	})

	It("rejects malformed event envelopes", func() {
		event := validEvent("dock")
		event.Context = json.RawMessage(`{"location":{"region_id":"lave","zone_id":"lave_primary","position":[1,2]}}`)

		Expect(validateEvent(event)).To(MatchError(ContainSubstring("context.location.position must contain x, y, z")))
	})

	It("counts malformed events as rejected without dropping valid siblings", func() {
		bad := validEvent("dock")
		bad.PlayerID = "not-a-uuid"

		valid, rejected := validateEvents([]EventEnvelope{validEvent("dock"), bad})

		Expect(valid).To(HaveLen(1))
		Expect(rejected).To(Equal(1))
	})

	It("validates bug report payload mood bounds", func() {
		_, err := bugReportPayload(json.RawMessage(`{"mood":0,"mood_label":"bad","notes":""}`))

		Expect(err).To(MatchError(ContainSubstring("mood must be between 1 and 5")))
	})

	It("normalizes bug report payloads before storing analytical events", func() {
		payload, err := normalizedBugReportPayload(
			json.RawMessage(`{"mood":1,"mood_label":"unhappy","notes":"oops","screenshot_png_base64":"abc"}`),
			"bug-reports/sursidus/2026/06/06/report.png",
		)

		Expect(err).ToNot(HaveOccurred())
		var fields map[string]any
		Expect(json.Unmarshal(payload, &fields)).To(Succeed())
		Expect(fields["screenshot_png_base64"]).To(BeNil())
		Expect(fields["screenshot_object_key"]).To(Equal("bug-reports/sursidus/2026/06/06/report.png"))
	})

	It("projects configured event fields from dotted keys", func() {
		queryFields := json.RawMessage(`[
			{"key":"economy.credits","source":"metrics.economy.credits","type":"number"},
			{"key":"ship.id","source":"dimensions.ship.id","type":"string"}
		]`)

		fields, err := projectEventFields(queryFields, validEvent("dock"))

		Expect(err).ToNot(HaveOccurred())
		Expect(fields).To(HaveLen(2))
		Expect(fields[0].key).To(Equal("economy.credits"))
		Expect(fields[0].numberValue.Float64).To(Equal(48200.0))
		Expect(fields[1].key).To(Equal("ship.id"))
		Expect(fields[1].stringValue.String).To(Equal("cobra_mk3"))
	})

	It("preserves raw event JSON for unknown future envelope fields", func() {
		var event EventEnvelope
		Expect(json.Unmarshal(
			[]byte(`{
				"schema_version":2,
				"player_id":"550e8400-e29b-41d4-a716-446655440000",
				"event_type":"dock",
				"real_ts":"2026-06-06T11:42:00Z",
				"game_time":1843200,
				"context":{"location":{"region_id":"lave","zone_id":"lave_primary","position":[1,2,3]}},
				"metrics":{},
				"dimensions":{},
				"payload":{},
				"future_field":"still here"
			}`),
			&event,
		)).To(Succeed())

		eventJSON := normalizedEventJSON(event, event.Context, event.Metrics, event.Dimensions, event.Payload)

		var stored map[string]any
		Expect(json.Unmarshal(eventJSON, &stored)).To(Succeed())
		Expect(stored).To(HaveKeyWithValue("future_field", "still here"))
	})

	It("sanitizes bug report payloads inside preserved raw event JSON", func() {
		var event EventEnvelope
		Expect(json.Unmarshal(
			[]byte(`{
				"schema_version":2,
				"player_id":"550e8400-e29b-41d4-a716-446655440000",
				"event_type":"bug_report",
				"real_ts":"2026-06-06T11:42:00Z",
				"game_time":1843200,
				"context":{"location":{"region_id":"lave","zone_id":"lave_primary","position":[1,2,3]}},
				"metrics":{},
				"dimensions":{},
				"payload":{"mood":1,"mood_label":"bad","notes":"oops","screenshot_png_base64":"abc"},
				"future_field":"still here"
			}`),
			&event,
		)).To(Succeed())
		payload, err := normalizedBugReportPayload(event.Payload, "bug-reports/sursidus/report.png")
		Expect(err).ToNot(HaveOccurred())

		eventJSON := eventRawWithPayload(event.Raw, payload)

		var stored map[string]any
		Expect(json.Unmarshal(eventJSON, &stored)).To(Succeed())
		Expect(stored).To(HaveKeyWithValue("future_field", "still here"))
		storedPayload, ok := stored["payload"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(storedPayload["screenshot_png_base64"]).To(BeNil())
		Expect(storedPayload["screenshot_object_key"]).To(Equal("bug-reports/sursidus/report.png"))
	})
})
