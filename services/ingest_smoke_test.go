//go:build smoke

package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/env"
)

// Run with: go test -tags smoke ./services/ -run TestIngestSmoke -v
// Requires the dev Postgres (make dev-up) with migrations applied.
func TestIngestSmoke(t *testing.T) {
	ctx := context.Background()
	cfg, err := env.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	pool, err := db.ConnectToPool(ctx, db.ConnectConfig{
		ConnectionString: cfg.PostgresURL(),
		MaxConnections:   4,
		MinConnections:   1,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	projectKey := "smoke_" + uuid.NewString()[:8]
	var projectID uuid.UUID
	if err := pool.QueryRow(
		ctx,
		`INSERT INTO projects (project_key, display_name) VALUES ($1, $2) RETURNING id`,
		projectKey, "Smoke",
	).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	})

	svc := NewIngestService(IngestOptions{DB: pool})
	client := ClientInfo{GameVersion: "1.0.0", BuildChannel: "dev", Platform: "linux"}

	rawEvent := func(playerID, eventID string) EventEnvelope {
		body := map[string]any{
			"schema_version": 2,
			"player_id":      playerID,
			"event_type":     "dock",
			"real_ts":        "2026-06-06T11:42:00Z",
			"game_time":      1,
			"context":        map[string]any{},
			"metrics":        map[string]any{},
			"dimensions":     map[string]any{},
			"payload":        map[string]any{"ok": true},
		}
		if eventID != "" {
			body["event_id"] = eventID
		}
		raw, _ := json.Marshal(body)
		var e EventEnvelope
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		return e
	}

	countRejected := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM rejected_events WHERE project_id=$1`, projectID).Scan(&n); err != nil {
			t.Fatalf("count rejected: %v", err)
		}
		return n
	}
	countEvents := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE project_id=$1`, projectID).Scan(&n); err != nil {
			t.Fatalf("count events: %v", err)
		}
		return n
	}

	// 1. All-invalid batch → 200 with rejections, rows persisted, no events.
	badBatch := EventsRequest{
		ProjectID: projectKey, BatchID: uuid.NewString(), Client: client,
		Events: []EventEnvelope{rawEvent("commander-not-uuid", ""), rawEvent("also-bad", "")},
	}
	resp, err := svc.IngestEvents(ctx, projectID, badBatch)
	if err != nil {
		t.Fatalf("all-invalid batch returned error (should be 200): %v", err)
	}
	if resp.Accepted != 0 || resp.Rejected != 2 || len(resp.Rejections) != 2 {
		t.Fatalf("unexpected all-invalid resp: %+v", resp)
	}
	if resp.Rejections[0].Reason != reasonPlayerID {
		t.Fatalf("unexpected reason: %s", resp.Rejections[0].Reason)
	}
	if countRejected() != 2 {
		t.Fatalf("expected 2 rejected_events rows, got %d", countRejected())
	}
	if countEvents() != 0 {
		t.Fatalf("expected 0 events, got %d", countEvents())
	}

	// 2. Mixed batch with event_id idempotency: resend same batch content under a
	//    new batch_id → duplicate event_ids must not create duplicate rows.
	eid1 := uuid.NewString()
	eid2 := uuid.NewString()
	valid := "550e8400-e29b-41d4-a716-446655440000"
	mixed := func() EventsRequest {
		return EventsRequest{
			ProjectID: projectKey, BatchID: uuid.NewString(), Client: client,
			Events: []EventEnvelope{rawEvent(valid, eid1), rawEvent(valid, eid2), rawEvent("bad", "")},
		}
	}
	r1, err := svc.IngestEvents(ctx, projectID, mixed())
	if err != nil {
		t.Fatalf("mixed batch: %v", err)
	}
	if r1.Accepted != 2 || r1.Rejected != 1 {
		t.Fatalf("unexpected mixed resp: %+v", r1)
	}
	if countEvents() != 2 {
		t.Fatalf("expected 2 events after first mixed, got %d", countEvents())
	}
	// Resend (new batch_id, same event_ids) — simulates lost-response retry.
	if _, err := svc.IngestEvents(ctx, projectID, mixed()); err != nil {
		t.Fatalf("resend mixed batch: %v", err)
	}
	if countEvents() != 2 {
		t.Fatalf("event-level idempotency failed: expected 2 events, got %d", countEvents())
	}

	// 3. Bug-report idempotency: same report_id twice → no error, single row.
	reportID := uuid.NewString()
	bugReq := func() BugReportRequest {
		return BugReportRequest{
			ProjectID: projectKey, ReportID: reportID, Client: client,
			Event: func() EventEnvelope {
				e := rawEvent(valid, "")
				e.EventType = "bug_report"
				raw, _ := json.Marshal(map[string]any{
					"schema_version": 2, "player_id": valid, "event_type": "bug_report",
					"real_ts": "2026-06-06T11:42:00Z", "game_time": 1,
					"context": map[string]any{}, "metrics": map[string]any{}, "dimensions": map[string]any{},
					"payload": map[string]any{"mood": 3, "mood_label": "ok", "notes": "n"},
				})
				_ = json.Unmarshal(raw, &e)
				return e
			}(),
		}
	}
	if _, err := svc.SubmitBugReport(ctx, projectID, bugReq()); err != nil {
		t.Fatalf("first bug report: %v", err)
	}
	if _, err := svc.SubmitBugReport(ctx, projectID, bugReq()); err != nil {
		t.Fatalf("bug-report replay must not error (idempotent), got: %v", err)
	}
	var reports int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bug_reports WHERE project_id=$1`, projectID).Scan(&reports); err != nil {
		t.Fatalf("count reports: %v", err)
	}
	if reports != 1 {
		t.Fatalf("bug-report idempotency failed: expected 1 report, got %d", reports)
	}

	t.Log("smoke OK: all-invalid 200, rejected persisted, event idempotency, bug-report replay")
}
