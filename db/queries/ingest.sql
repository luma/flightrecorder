-- name: GetProjectByKey :one
SELECT id, project_key, display_name, validation_mode, query_fields
FROM projects
WHERE project_key = $1;

-- name: GetBatchByProjectAndBatchID :one
SELECT id, accepted_count, rejected_count
FROM batches
WHERE project_id = $1
  AND batch_id = $2;

-- name: CreateBatch :one
INSERT INTO batches (
    project_id,
    batch_id,
    accepted_count,
    rejected_count,
    request_meta
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING id, accepted_count, rejected_count;

-- name: CreateEvent :one
INSERT INTO events (
    project_id,
    batch_db_id,
    player_id,
    event_type,
    real_ts,
    game_time,
    region_id,
    zone_id,
    coord_x,
    coord_y,
    coord_z,
    game_version,
    build_channel,
    commit_sha,
    platform,
    context,
    metrics,
    dimensions,
    payload,
    event_json,
    validation_errors
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
)
RETURNING id;

-- name: CreateEventField :exec
INSERT INTO event_fields (
    event_id,
    project_id,
    field_key,
    value_type,
    string_value,
    number_value,
    bool_value
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (event_id, field_key) DO UPDATE
SET value_type = EXCLUDED.value_type,
    string_value = EXCLUDED.string_value,
    number_value = EXCLUDED.number_value,
    bool_value = EXCLUDED.bool_value;

-- name: CountRecentBugReportsByPlayer :one
SELECT count(*)::bigint
FROM bug_reports br
JOIN events e ON e.id = br.event_id
WHERE br.project_id = $1
  AND e.player_id = $2
  AND br.created_at > now() - make_interval(secs => $3);

-- name: CreateBugReport :one
INSERT INTO bug_reports (
    project_id,
    report_id,
    event_id,
    mood,
    mood_label,
    notes_preview,
    screenshot_object_key,
    screenshot_storage_error
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, screenshot_object_key;
