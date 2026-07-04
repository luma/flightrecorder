package api

import (
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/luma/flightrecorder/services"
)

func TestDecodeJSONBodyRejectsUnknownNestedProjectConfigKeys(t *testing.T) {
	ctx := requestContextWithBody(`{
		"project_id": "example",
		"display_name": "Example",
		"validation_mode": "warn",
		"ingest_config": {
			"max_event_per_batch": 50
		},
		"retention_config": {},
		"map_config": {},
		"report_config": {},
		"event_groups": {},
		"query_fields": [],
		"funnels": []
	}`)

	var req services.CreateProjectRequest
	err := decodeJSONBody(ctx, &req)
	if err == nil {
		t.Fatal("expected unknown nested project config key to fail decoding")
	}
	if !strings.Contains(err.Error(), "unknown field \"max_event_per_batch\"") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestDecodeJSONBodyAllowsRawTelemetryKeys(t *testing.T) {
	ctx := requestContextWithBody(`{
		"project_id": "example",
		"batch_id": "batch-1",
		"sent_at": "2026-06-06T11:42:00Z",
		"client": {
			"game_version": "0.1.0",
			"build_channel": "dev",
			"commit_sha": "abc123",
			"platform": "linux"
		},
		"events": [{
			"schema_version": 2,
			"player_id": "550e8400-e29b-41d4-a716-446655440000",
			"event_type": "dock",
			"real_ts": "2026-06-06T11:42:00Z",
			"game_time": 1843200,
			"context": {
				"custom_context_key": {"nested": true}
			},
			"metrics": {
				"arbitrary.metric": 123
			},
			"dimensions": {
				"arbitrary_dimension": "value"
			},
			"payload": {
				"game_specific_payload": ["ok"]
			},
			"future_event_envelope_key": "preserved by EventEnvelope"
		}]
	}`)

	var req services.EventsRequest
	if err := decodeJSONBody(ctx, &req); err != nil {
		t.Fatalf("expected raw telemetry keys to decode, got %v", err)
	}
	if len(req.Events) != 1 {
		t.Fatalf("expected one decoded event, got %d", len(req.Events))
	}
	if !strings.Contains(string(req.Events[0].Context), "custom_context_key") {
		t.Fatalf("expected raw context JSON to be preserved, got %s", req.Events[0].Context)
	}
	if !strings.Contains(string(req.Events[0].Raw), "future_event_envelope_key") {
		t.Fatalf("expected raw event envelope JSON to be preserved, got %s", req.Events[0].Raw)
	}
}

func TestDecodeIngestBodyToleratesUnknownTopLevelField(t *testing.T) {
	body := `{
		"project_id": "example",
		"batch_id": "batch-1",
		"future_top_level_field": "ignored by lenient ingest decoder",
		"client": {
			"game_version": "0.1.0",
			"build_channel": "dev",
			"commit_sha": "abc123",
			"platform": "linux"
		},
		"events": []
	}`

	var lenient services.EventsRequest
	if err := decodeIngestBody(requestContextWithBody(body), &lenient); err != nil {
		t.Fatalf("expected lenient ingest decoder to tolerate unknown top-level field, got %v", err)
	}
	if lenient.BatchID != "batch-1" {
		t.Fatalf("expected batch_id to decode, got %q", lenient.BatchID)
	}

	// The same unknown top-level field must still be rejected by the strict
	// decoder used for admin/MCP routes.
	var strict services.EventsRequest
	if err := decodeJSONBody(requestContextWithBody(body), &strict); err == nil {
		t.Fatal("expected strict decoder to reject unknown top-level field")
	}
}

func requestContextWithBody(body string) *app.RequestContext {
	ctx := &app.RequestContext{}
	ctx.Request.SetBodyString(body)
	return ctx
}
