-- WAL self-healing: event-level idempotency + rejected-event visibility.
--
-- 1. client_event_id gives events an idempotency key that is stable across WAL
--    resend attempts, so a committed batch whose response was lost does not
--    produce duplicate rows. Nullable + partial unique so old clients (which
--    send no event_id) behave exactly as before.
ALTER TABLE events ADD COLUMN client_event_id uuid;

CREATE UNIQUE INDEX events_project_client_event_id_key
    ON events (project_id, client_event_id)
    WHERE client_event_id IS NOT NULL;

-- 2. rejected_events records events the collector refused, for operator
--    visibility. It is deliberately separate from the events fact table: the
--    typed, NOT NULL columns on events cannot hold a malformed event, and
--    coercing garbage would pollute every dashboard, heatmap, funnel, and trace.
--    validateEvent returns a single (first) reason per event, so a single
--    reason_code/message pair is stored rather than an array — this keeps the
--    aggregation btree-indexable.
CREATE TABLE rejected_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    batch_db_id uuid REFERENCES batches(id) ON DELETE SET NULL,
    event_type text NOT NULL DEFAULT '',
    reason_code text NOT NULL DEFAULT '',
    reason_message text NOT NULL DEFAULT '',
    raw_event jsonb NOT NULL DEFAULT '{}'::jsonb,
    game_version text NOT NULL DEFAULT '',
    build_channel text NOT NULL DEFAULT '',
    commit_sha text NOT NULL DEFAULT '',
    platform text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Serves the created_at prune and the recent-activity badge count.
CREATE INDEX rejected_events_project_created_idx
    ON rejected_events (project_id, created_at DESC);

-- Serves the (event_type, reason_code, game_version) aggregation for the
-- Data Quality view.
CREATE INDEX rejected_events_project_group_idx
    ON rejected_events (project_id, event_type, reason_code, game_version);

-- 3. project_rejection_acks stores a per-project (shared across all operators)
--    acknowledgement timestamp so the nav badge only re-alerts when a rejection
--    group recurs after being acknowledged.
CREATE TABLE project_rejection_acks (
    project_id uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    acknowledged_at timestamptz NOT NULL DEFAULT now()
);
