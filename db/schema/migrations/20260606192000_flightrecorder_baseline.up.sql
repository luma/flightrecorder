-- flightrecorder baseline schema.
--
-- This project has not shipped yet, so the initial development migrations are
-- squashed into a single baseline. The schema below is the final shape expected
-- by the current API, admin UI, and Godot client.

-- UUID primary keys are generated in PostgreSQL so inserts can stay simple and
-- deterministic across API handlers, migrations, and local seed scripts.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Projects are the top-level tenant boundary. Project configuration is JSONB so
-- games can define their own telemetry schema, queryable fields, reports, maps,
-- and retention policy without requiring a table migration per game.
CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_key text NOT NULL UNIQUE,
    display_name text NOT NULL,
    validation_mode text NOT NULL DEFAULT 'warn'
        CHECK (validation_mode IN ('warn', 'strict')),
    ingest_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    retention_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    map_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    report_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    event_groups jsonb NOT NULL DEFAULT '{}'::jsonb,
    query_fields jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Admin users are intentionally small for v1: local dev sessions and future
-- OAuth flows both resolve to this table.
CREATE TABLE admin_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL UNIQUE,
    oauth_subject text UNIQUE,
    role text NOT NULL DEFAULT 'admin',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Ingest tokens are hashed at rest. token_hash is globally unique so token
-- authentication can update last_used_at and return the owning project in one
-- indexed statement.
CREATE TABLE ingest_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT true,
    expires_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ingest_tokens_project_id_idx
ON ingest_tokens(project_id);

-- Batches provide idempotency for client WAL flushes. A repeated batch_id for
-- the same project returns the original accepted/rejected counts instead of
-- inserting duplicate events.
CREATE TABLE batches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    batch_id text NOT NULL,
    accepted_count integer NOT NULL DEFAULT 0,
    rejected_count integer NOT NULL DEFAULT 0,
    request_meta jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, batch_id)
);

CREATE INDEX batches_project_created_idx
ON batches(project_id, created_at DESC);

-- Events are the high-volume fact table. Common dimensions are duplicated into
-- typed columns for fast filtering and grouping, while the full event payload is
-- retained as JSONB for inspection and export.
CREATE TABLE events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    batch_db_id uuid REFERENCES batches(id) ON DELETE SET NULL,
    player_id uuid NOT NULL,
    event_type text NOT NULL,
    real_ts timestamptz NOT NULL,
    game_time bigint NOT NULL,
    region_id text NOT NULL,
    zone_id text NOT NULL,
    coord_x double precision NOT NULL,
    coord_y double precision NOT NULL,
    coord_z double precision NOT NULL,
    game_version text NOT NULL,
    build_channel text NOT NULL,
    platform text NOT NULL,
    context jsonb NOT NULL DEFAULT '{}'::jsonb,
    metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
    dimensions jsonb NOT NULL DEFAULT '{}'::jsonb,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    event_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    validation_errors jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Dashboard and list views mostly filter by project and time, sometimes with
-- event type, player, or system constraints. These indexes avoid full project
-- scans once event volume reaches millions of rows.
CREATE INDEX events_project_real_ts_idx
ON events(project_id, real_ts DESC);

CREATE INDEX events_project_type_time_idx
ON events(project_id, event_type, real_ts DESC);

CREATE INDEX events_project_player_game_time_idx
ON events(project_id, player_id, game_time ASC);

CREATE INDEX events_project_player_real_ts_idx
ON events(project_id, player_id, real_ts DESC);

CREATE INDEX events_project_region_zone_idx
ON events(project_id, region_id, zone_id);

CREATE INDEX events_project_region_time_idx
ON events(project_id, region_id, real_ts DESC);

CREATE INDEX events_project_region_zone_time_idx
ON events(project_id, region_id, zone_id, real_ts DESC);

-- Project-defined query fields are projected from event JSON into typed columns.
-- This keeps the schema flexible for different games while giving the admin UI
-- btree indexes for common filters.
CREATE TABLE event_fields (
    event_id uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    field_key text NOT NULL,
    value_type text NOT NULL CHECK (value_type IN ('string', 'number', 'bool')),
    string_value text,
    number_value double precision,
    bool_value boolean,
    PRIMARY KEY (event_id, field_key)
);

CREATE INDEX event_fields_project_field_string_idx
ON event_fields(project_id, field_key, string_value) INCLUDE (event_id);

CREATE INDEX event_fields_project_field_number_idx
ON event_fields(project_id, field_key, number_value) INCLUDE (event_id);

CREATE INDEX event_fields_project_field_bool_idx
ON event_fields(project_id, field_key, bool_value) INCLUDE (event_id);

-- Bug reports are anchored to their source event so report detail pages can
-- show the nearby player timeline and original telemetry context.
CREATE TABLE bug_reports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    report_id text NOT NULL,
    event_id uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'new',
    labels text[] NOT NULL DEFAULT '{}'::text[],
    mood integer NOT NULL CHECK (mood >= 1 AND mood <= 5),
    mood_label text NOT NULL,
    notes_preview text NOT NULL DEFAULT '',
    screenshot_object_key text,
    screenshot_storage_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, report_id)
);

CREATE INDEX bug_reports_project_status_created_idx
ON bug_reports(project_id, status, created_at DESC);

CREATE INDEX bug_reports_project_created_idx
ON bug_reports(project_id, created_at DESC);

CREATE INDEX bug_reports_labels_gin_idx
ON bug_reports USING gin(labels);

CREATE TABLE report_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id uuid NOT NULL REFERENCES bug_reports(id) ON DELETE CASCADE,
    admin_user_id uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    note text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX report_notes_report_created_idx
ON report_notes(report_id, created_at DESC);
