-- name: AdminSummary :one
SELECT
    count(*)::bigint AS event_count,
    count(DISTINCT player_id)::bigint AS player_count,
    count(*) FILTER (WHERE event_type = 'game_continue')::bigint AS session_count,
    count(*) FILTER (WHERE event_type = 'player_death')::bigint AS death_count,
    (
        SELECT count(*)::bigint
        FROM bug_reports br
        WHERE br.project_id = $1
          AND br.created_at >= $2
          AND br.created_at <= $3
    ) AS report_count
FROM events
WHERE events.project_id = $1
  AND real_ts >= $2
  AND real_ts <= $3;

-- name: AdminListEvents :many
SELECT
    id,
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
    COALESCE(
        (
            SELECT jsonb_object_agg(
                ef.field_key,
                CASE ef.value_type
                    WHEN 'number' THEN to_jsonb(ef.number_value)
                    WHEN 'bool' THEN to_jsonb(ef.bool_value)
                    ELSE to_jsonb(ef.string_value)
                END
            )
            FROM event_fields ef
            WHERE ef.event_id = events.id
        ),
        '{}'::jsonb
    )::jsonb AS fields,
    validation_errors
FROM events
WHERE events.project_id = $1
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('region_id')::text IS NULL OR region_id = sqlc.narg('region_id'))
  AND (sqlc.narg('zone_id')::text IS NULL OR zone_id = sqlc.narg('zone_id'))
  AND (sqlc.narg('player_id')::uuid IS NULL OR player_id = sqlc.narg('player_id')::uuid)
  AND (sqlc.narg('game_version')::text IS NULL OR game_version = sqlc.narg('game_version'))
  AND (sqlc.narg('build_channel')::text IS NULL OR build_channel = sqlc.narg('build_channel'))
  AND real_ts >= $2
  AND real_ts <= $3
ORDER BY real_ts DESC
LIMIT $4
OFFSET $5;

-- name: AdminListEventsByField :many
SELECT
    events.id,
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
    COALESCE(
        (
            SELECT jsonb_object_agg(
                ef.field_key,
                CASE ef.value_type
                    WHEN 'number' THEN to_jsonb(ef.number_value)
                    WHEN 'bool' THEN to_jsonb(ef.bool_value)
                    ELSE to_jsonb(ef.string_value)
                END
            )
            FROM event_fields ef
            WHERE ef.event_id = events.id
        ),
        '{}'::jsonb
    )::jsonb AS fields,
    validation_errors
FROM event_fields filter_field
JOIN events ON events.id = filter_field.event_id
WHERE filter_field.project_id = $1
  AND filter_field.field_key = sqlc.arg('field_key')
  AND filter_field.value_type = sqlc.arg('field_value_type')
  AND (
      NOT sqlc.arg('has_field_value')::bool
      OR (filter_field.value_type = 'string' AND filter_field.string_value = sqlc.arg('field_string_value')::text)
      OR (filter_field.value_type = 'number' AND filter_field.number_value = sqlc.arg('field_number_value')::double precision)
      OR (filter_field.value_type = 'bool' AND filter_field.bool_value = sqlc.arg('field_bool_value')::boolean)
  )
  AND events.project_id = $1
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('region_id')::text IS NULL OR region_id = sqlc.narg('region_id'))
  AND (sqlc.narg('zone_id')::text IS NULL OR zone_id = sqlc.narg('zone_id'))
  AND (sqlc.narg('player_id')::uuid IS NULL OR player_id = sqlc.narg('player_id')::uuid)
  AND (sqlc.narg('game_version')::text IS NULL OR game_version = sqlc.narg('game_version'))
  AND (sqlc.narg('build_channel')::text IS NULL OR build_channel = sqlc.narg('build_channel'))
  AND real_ts >= $2
  AND real_ts <= $3
ORDER BY real_ts DESC
LIMIT $4
OFFSET $5;

-- name: AdminPlayerTrace :many
SELECT
    id,
    event_type,
    real_ts,
    game_time,
    region_id,
    zone_id,
    coord_x,
    coord_y,
    coord_z,
    context,
    metrics,
    dimensions,
    COALESCE(
        (
            SELECT jsonb_object_agg(
                ef.field_key,
                CASE ef.value_type
                    WHEN 'number' THEN to_jsonb(ef.number_value)
                    WHEN 'bool' THEN to_jsonb(ef.bool_value)
                    ELSE to_jsonb(ef.string_value)
                END
            )
            FROM event_fields ef
            WHERE ef.event_id = events.id
        ),
        '{}'::jsonb
    )::jsonb AS fields,
    payload
FROM events
WHERE events.project_id = $1
  AND player_id = $2
ORDER BY game_time ASC
LIMIT $3;

-- name: AdminRegionHeatmap :many
SELECT
    region_id,
    event_type,
    count(*)::bigint AS event_count
FROM events
WHERE events.project_id = $1
  AND real_ts >= $2
  AND real_ts <= $3
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('game_version')::text IS NULL OR game_version = sqlc.narg('game_version'))
  AND (sqlc.narg('build_channel')::text IS NULL OR build_channel = sqlc.narg('build_channel'))
GROUP BY region_id, event_type
ORDER BY event_count DESC;

-- name: AdminRegionHeatmapByField :many
SELECT
    region_id,
    event_type,
    count(*)::bigint AS event_count
FROM event_fields filter_field
JOIN events ON events.id = filter_field.event_id
WHERE filter_field.project_id = $1
  AND filter_field.field_key = sqlc.arg('field_key')
  AND filter_field.value_type = sqlc.arg('field_value_type')
  AND (
      NOT sqlc.arg('has_field_value')::bool
      OR (filter_field.value_type = 'string' AND filter_field.string_value = sqlc.arg('field_string_value')::text)
      OR (filter_field.value_type = 'number' AND filter_field.number_value = sqlc.arg('field_number_value')::double precision)
      OR (filter_field.value_type = 'bool' AND filter_field.bool_value = sqlc.arg('field_bool_value')::boolean)
  )
  AND events.project_id = $1
  AND real_ts >= $2
  AND real_ts <= $3
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('game_version')::text IS NULL OR game_version = sqlc.narg('game_version'))
  AND (sqlc.narg('build_channel')::text IS NULL OR build_channel = sqlc.narg('build_channel'))
GROUP BY region_id, event_type
ORDER BY event_count DESC;

-- name: AdminZoneHeatmap :many
SELECT
    region_id,
    zone_id,
    round(coord_x / $4)::bigint AS grid_x,
    round(coord_z / $4)::bigint AS grid_z,
    event_type,
    count(*)::bigint AS event_count
FROM events
WHERE events.project_id = $1
  AND real_ts >= $2
  AND real_ts <= $3
  AND region_id = $5
  AND (sqlc.narg('zone_id')::text IS NULL OR zone_id = sqlc.narg('zone_id'))
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('game_version')::text IS NULL OR game_version = sqlc.narg('game_version'))
  AND (sqlc.narg('build_channel')::text IS NULL OR build_channel = sqlc.narg('build_channel'))
GROUP BY region_id, zone_id, grid_x, grid_z, event_type
ORDER BY event_count DESC;

-- name: AdminZoneHeatmapByField :many
SELECT
    region_id,
    zone_id,
    round(coord_x / $4)::bigint AS grid_x,
    round(coord_z / $4)::bigint AS grid_z,
    event_type,
    count(*)::bigint AS event_count
FROM event_fields filter_field
JOIN events ON events.id = filter_field.event_id
WHERE filter_field.project_id = $1
  AND filter_field.field_key = sqlc.arg('field_key')
  AND filter_field.value_type = sqlc.arg('field_value_type')
  AND (
      NOT sqlc.arg('has_field_value')::bool
      OR (filter_field.value_type = 'string' AND filter_field.string_value = sqlc.arg('field_string_value')::text)
      OR (filter_field.value_type = 'number' AND filter_field.number_value = sqlc.arg('field_number_value')::double precision)
      OR (filter_field.value_type = 'bool' AND filter_field.bool_value = sqlc.arg('field_bool_value')::boolean)
  )
  AND events.project_id = $1
  AND real_ts >= $2
  AND real_ts <= $3
  AND region_id = $5
  AND (sqlc.narg('zone_id')::text IS NULL OR zone_id = sqlc.narg('zone_id'))
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('game_version')::text IS NULL OR game_version = sqlc.narg('game_version'))
  AND (sqlc.narg('build_channel')::text IS NULL OR build_channel = sqlc.narg('build_channel'))
GROUP BY region_id, zone_id, grid_x, grid_z, event_type
ORDER BY event_count DESC;

-- name: AdminListReports :many
SELECT
    br.id,
    br.report_id,
    br.status,
    br.labels,
    br.mood,
    br.mood_label,
    br.notes_preview,
    br.screenshot_object_key,
    br.created_at,
    e.player_id,
    e.real_ts,
    e.game_time,
    e.region_id,
    e.zone_id,
    e.context,
    e.metrics,
    e.dimensions,
    e.payload
FROM bug_reports br
JOIN events e ON e.id = br.event_id
WHERE br.project_id = $1
  AND (sqlc.narg('status')::text IS NULL OR br.status = sqlc.narg('status'))
ORDER BY br.created_at DESC
LIMIT $2
OFFSET $3;

-- name: AdminListReportsByLabel :many
SELECT
    br.id,
    br.report_id,
    br.status,
    br.labels,
    br.mood,
    br.mood_label,
    br.notes_preview,
    br.screenshot_object_key,
    br.created_at,
    e.player_id,
    e.real_ts,
    e.game_time,
    e.region_id,
    e.zone_id,
    e.context,
    e.metrics,
    e.dimensions,
    e.payload
FROM bug_reports br
JOIN events e ON e.id = br.event_id
WHERE br.project_id = $1
  AND br.labels @> ARRAY[sqlc.arg('label')::text]
  AND (sqlc.narg('status')::text IS NULL OR br.status = sqlc.narg('status'))
ORDER BY br.created_at DESC
LIMIT $2
OFFSET $3;

-- name: AdminGetReport :one
SELECT
    br.id,
    br.report_id,
    br.status,
    br.labels,
    br.mood,
    br.mood_label,
    br.notes_preview,
    br.screenshot_object_key,
    br.screenshot_storage_error,
    br.created_at,
    e.player_id,
    e.real_ts,
    e.game_time,
    e.region_id,
    e.zone_id,
    e.coord_x,
    e.coord_y,
    e.coord_z,
    e.context,
    e.metrics,
    e.dimensions,
    e.payload
FROM bug_reports br
JOIN events e ON e.id = br.event_id
WHERE br.project_id = $1
  AND br.report_id = $2;

-- name: AdminListReportNotes :many
SELECT
    id,
    note,
    created_at
FROM report_notes
WHERE report_id = $1
ORDER BY created_at DESC;

-- name: AdminReportTrace :many
SELECT
    e.id,
    e.event_type,
    e.real_ts,
    e.game_time,
    e.region_id,
    e.zone_id,
    e.context,
    e.metrics,
    e.dimensions,
    e.payload
FROM bug_reports br
JOIN events report_event ON report_event.id = br.event_id
JOIN events e ON e.project_id = report_event.project_id
             AND e.player_id = report_event.player_id
             AND e.game_time >= report_event.game_time - $3
             AND e.game_time <= report_event.game_time + $3
WHERE br.project_id = $1
  AND br.report_id = $2
ORDER BY e.game_time ASC;

-- name: AdminUpdateReport :one
UPDATE bug_reports
SET status = $3,
    labels = $4,
    updated_at = now()
WHERE project_id = $1
  AND report_id = $2
RETURNING id, report_id, status, labels, updated_at;

-- name: AdminCreateReportNote :one
INSERT INTO report_notes (
    report_id,
    note
) VALUES (
    $1, $2
)
RETURNING id, note, created_at;

-- name: AdminEventTypes :many
SELECT
    event_type,
    count(*)::bigint AS event_count,
    max(real_ts)::timestamptz AS last_seen_at,
    (
        SELECT payload
        FROM events sample
        WHERE sample.project_id = e.project_id
          AND sample.event_type = e.event_type
        ORDER BY sample.real_ts DESC
        LIMIT 1
    ) AS sample_payload
FROM events e
WHERE e.project_id = $1
GROUP BY e.project_id, e.event_type
ORDER BY event_count DESC;

-- name: AdminProjectSettings :one
SELECT
    id,
    project_key,
    display_name,
    validation_mode,
    ingest_config,
    retention_config,
    map_config,
    report_config,
    event_groups,
    query_fields,
    funnels
FROM projects
WHERE project_key = $1;

-- name: AdminListProjects :many
SELECT
    project_key,
    display_name,
    validation_mode,
    created_at,
    updated_at
FROM projects
ORDER BY display_name ASC, project_key ASC;

-- name: AdminUpsertProject :one
INSERT INTO projects (
    project_key,
    display_name,
    validation_mode,
    ingest_config,
    retention_config,
    map_config,
    report_config,
    event_groups,
    query_fields,
    funnels
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (project_key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    validation_mode = EXCLUDED.validation_mode,
    ingest_config = EXCLUDED.ingest_config,
    retention_config = EXCLUDED.retention_config,
    map_config = EXCLUDED.map_config,
    report_config = EXCLUDED.report_config,
    event_groups = EXCLUDED.event_groups,
    query_fields = EXCLUDED.query_fields,
    funnels = EXCLUDED.funnels,
    updated_at = now()
RETURNING
    id,
    project_key,
    display_name,
    validation_mode,
    ingest_config,
    retention_config,
    map_config,
    report_config,
    event_groups,
    query_fields,
    funnels;

-- name: AdminListIngestTokens :many
SELECT
    id,
    name,
    enabled,
    expires_at,
    last_used_at,
    created_at
FROM ingest_tokens
WHERE project_id = $1
ORDER BY created_at DESC;

-- name: AdminCreateIngestToken :one
INSERT INTO ingest_tokens (
    project_id,
    name,
    token_hash
) VALUES (
    $1, $2, $3
)
RETURNING id, name, enabled, expires_at, last_used_at, created_at;

-- name: AdminSetIngestTokenEnabled :one
UPDATE ingest_tokens
SET enabled = $3
WHERE project_id = $1
  AND id = $2
RETURNING id, name, enabled, expires_at, last_used_at, created_at;

-- name: AdminCountUsers :one
SELECT count(*)::bigint
FROM admin_users;

-- name: AdminCountEnabledUsers :one
SELECT count(*)::bigint
FROM admin_users
WHERE enabled = true;

-- name: AdminGetUserByID :one
SELECT
    id,
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at,
    created_at,
    updated_at
FROM admin_users
WHERE id = $1;

-- name: AdminGetUserByEmail :one
SELECT
    id,
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at,
    created_at,
    updated_at
FROM admin_users
WHERE email = $1;

-- name: AdminGetUserBySubject :one
SELECT
    id,
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at,
    created_at,
    updated_at
FROM admin_users
WHERE oauth_subject = $1;

-- name: AdminCreateUser :one
INSERT INTO admin_users (
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at
) VALUES (
    $1, $2, 'admin', true, $3, $4, $5, now()
)
RETURNING
    id,
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at,
    created_at,
    updated_at;

-- name: AdminRefreshUserLogin :one
UPDATE admin_users
SET oauth_subject = COALESCE(sqlc.narg('oauth_subject'), oauth_subject),
    name = sqlc.arg('name'),
    picture_url = sqlc.arg('picture_url'),
    provider = sqlc.arg('provider'),
    last_login_at = now(),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING
    id,
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at,
    created_at,
    updated_at;

-- name: AdminListUsers :many
SELECT
    id,
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at,
    created_at,
    updated_at
FROM admin_users
ORDER BY email ASC;

-- name: AdminSetUserEnabled :one
UPDATE admin_users
SET enabled = $2,
    updated_at = now()
WHERE id = $1
RETURNING
    id,
    email,
    oauth_subject,
    role,
    enabled,
    name,
    picture_url,
    provider,
    last_login_at,
    created_at,
    updated_at;

-- name: AdminCreateInvitation :one
INSERT INTO admin_invitations (
    email,
    token_hash,
    created_by_admin_user_id,
    expires_at
) VALUES (
    $1, $2, $3, now() + interval '48 hours'
)
RETURNING
    id,
    email,
    expires_at,
    accepted_at,
    deleted_at,
    created_at;

-- name: MCPUpsertOAuthClient :one
INSERT INTO mcp_oauth_clients (
    client_id,
    client_name,
    redirect_uris,
    client_uri,
    logo_uri
) VALUES (
    $1, $2, $3, $4, $5
)
ON CONFLICT (client_id) DO UPDATE
SET client_name = EXCLUDED.client_name,
    redirect_uris = EXCLUDED.redirect_uris,
    client_uri = EXCLUDED.client_uri,
    logo_uri = EXCLUDED.logo_uri,
    updated_at = now()
RETURNING
    client_id,
    client_name,
    redirect_uris,
    client_uri,
    logo_uri,
    created_at,
    updated_at;

-- name: MCPGetOAuthClient :one
SELECT
    client_id,
    client_name,
    redirect_uris,
    client_uri,
    logo_uri,
    created_at,
    updated_at
FROM mcp_oauth_clients
WHERE client_id = $1;

-- name: MCPCreateAgentAuthorization :one
INSERT INTO agent_authorizations (
    client_id,
    client_name,
    created_by_admin_user_id,
    all_projects,
    scopes,
    expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING
    id,
    client_id,
    client_name,
    created_by_admin_user_id,
    all_projects,
    scopes,
    enabled,
    expires_at,
    activated_at,
    last_used_at,
    created_at,
    updated_at;

-- name: MCPCreateAgentAuthorizationProject :exec
INSERT INTO agent_authorization_projects (
    agent_authorization_id,
    project_id
) VALUES (
    $1, $2
);

-- name: MCPCreateOAuthCode :exec
INSERT INTO mcp_oauth_codes (
    code_hash,
    client_id,
    redirect_uri,
    code_challenge,
    code_challenge_method,
    resource,
    scopes,
    admin_user_id,
    agent_authorization_id,
    expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
);

-- name: MCPConsumeOAuthCode :one
UPDATE mcp_oauth_codes
SET consumed_at = now()
WHERE code_hash = $1
  AND client_id = $2
  AND redirect_uri = $3
  AND resource = $4
  AND consumed_at IS NULL
  AND expires_at > now()
RETURNING
    client_id,
    redirect_uri,
    code_challenge,
    code_challenge_method,
    resource,
    scopes,
    admin_user_id,
    agent_authorization_id,
    expires_at,
    consumed_at,
    created_at;

-- name: MCPActivateAgentAuthorization :one
UPDATE agent_authorizations
SET token_hash = $2,
    activated_at = now(),
    updated_at = now()
WHERE id = $1
  AND token_hash IS NULL
  AND enabled = true
  AND expires_at > now()
RETURNING
    id,
    client_id,
    client_name,
    created_by_admin_user_id,
    all_projects,
    scopes,
    enabled,
    expires_at,
    activated_at,
    last_used_at,
    created_at,
    updated_at;

-- name: MCPValidateAgentToken :one
UPDATE agent_authorizations aa
SET last_used_at = now()
FROM admin_users au
WHERE aa.token_hash = $1
  AND aa.enabled = true
  AND aa.expires_at > now()
  AND aa.created_by_admin_user_id IS NOT NULL
  AND au.id = aa.created_by_admin_user_id
  AND au.enabled = true
RETURNING
    aa.id,
    aa.client_id,
    aa.client_name,
    aa.created_by_admin_user_id,
    aa.all_projects,
    aa.scopes,
    aa.enabled,
    aa.expires_at,
    aa.activated_at,
    aa.last_used_at,
    aa.created_at,
    aa.updated_at;

-- name: MCPListAgentAuthorizationProjects :many
SELECT
    p.id,
    p.project_key
FROM agent_authorization_projects aap
JOIN projects p ON p.id = aap.project_id
WHERE aap.agent_authorization_id = $1
ORDER BY p.project_key ASC;

-- name: AdminListAgentAuthorizations :many
SELECT
    aa.id,
    aa.client_id,
    aa.client_name,
    aa.created_by_admin_user_id,
    creator.email AS created_by_email,
    aa.all_projects,
    aa.scopes,
    aa.enabled,
    aa.expires_at,
    aa.activated_at,
    aa.last_used_at,
    aa.created_at,
    aa.updated_at,
    COALESCE(
        (
            SELECT jsonb_agg(p.project_key ORDER BY p.project_key)
            FROM agent_authorization_projects aap
            JOIN projects p ON p.id = aap.project_id
            WHERE aap.agent_authorization_id = aa.id
        ),
        '[]'::jsonb
    )::jsonb AS project_keys
FROM agent_authorizations aa
LEFT JOIN admin_users creator ON creator.id = aa.created_by_admin_user_id
ORDER BY aa.created_at DESC;

-- name: AdminSetAgentAuthorizationEnabled :one
UPDATE agent_authorizations
SET enabled = $2,
    updated_at = now()
WHERE id = $1
RETURNING
    id,
    client_id,
    client_name,
    created_by_admin_user_id,
    all_projects,
    scopes,
    enabled,
    expires_at,
    activated_at,
    last_used_at,
    created_at,
    updated_at;

-- name: MCPCleanupExpiredOAuthState :exec
DELETE FROM agent_authorizations aa
WHERE aa.token_hash IS NULL
  AND EXISTS (
      SELECT 1
      FROM mcp_oauth_codes code
      WHERE code.agent_authorization_id = aa.id
        AND code.expires_at < now()
  );

-- name: MCPCleanupExpiredOAuthCodes :exec
DELETE FROM mcp_oauth_codes
WHERE expires_at < now();

-- name: AdminListActiveInvitations :many
SELECT
    i.id,
    i.email,
    i.expires_at,
    i.created_at,
    creator.email AS created_by_email
FROM admin_invitations i
LEFT JOIN admin_users creator ON creator.id = i.created_by_admin_user_id
WHERE i.accepted_at IS NULL
  AND i.deleted_at IS NULL
  AND i.expires_at > now()
ORDER BY i.created_at DESC;

-- name: AdminDeleteInvitation :one
UPDATE admin_invitations
SET deleted_at = now()
WHERE id = $1
  AND accepted_at IS NULL
  AND deleted_at IS NULL
RETURNING
    id,
    email,
    expires_at,
    accepted_at,
    deleted_at,
    created_at;

-- name: AdminAcceptInvitation :one
UPDATE admin_invitations
SET accepted_at = now(),
    accepted_by_admin_user_id = $3
WHERE email = $1
  AND token_hash = $2
  AND accepted_at IS NULL
  AND deleted_at IS NULL
  AND expires_at > now()
RETURNING
    id,
    email,
    expires_at,
    accepted_at,
    deleted_at,
    created_at;

-- name: AdminRejectedEventGroups :many
SELECT
    re.event_type,
    re.reason_code,
    re.reason_message,
    re.game_version,
    re.build_channel,
    count(*)::bigint AS event_count,
    min(re.created_at)::timestamptz AS first_seen_at,
    max(re.created_at)::timestamptz AS last_seen_at,
    (
        SELECT sample.raw_event
        FROM rejected_events sample
        WHERE sample.project_id = re.project_id
          AND sample.event_type = re.event_type
          AND sample.reason_code = re.reason_code
          AND sample.game_version = re.game_version
        ORDER BY sample.created_at DESC
        LIMIT 1
    ) AS sample_event
FROM rejected_events re
WHERE re.project_id = $1
GROUP BY re.project_id, re.event_type, re.reason_code, re.reason_message, re.game_version, re.build_channel
ORDER BY last_seen_at DESC;

-- name: AdminCountActiveRejectionGroups :one
SELECT count(*)::bigint
FROM (
    SELECT re.event_type, re.reason_code, re.game_version
    FROM rejected_events re
    WHERE re.project_id = $1
    GROUP BY re.event_type, re.reason_code, re.game_version
    HAVING max(re.created_at) >= now() - interval '24 hours'
       AND max(re.created_at) > COALESCE(
            (SELECT acknowledged_at FROM project_rejection_acks WHERE project_id = $1),
            'epoch'::timestamptz
       )
) active_groups;

-- name: AdminAcknowledgeRejectedEvents :exec
INSERT INTO project_rejection_acks (project_id, acknowledged_at)
VALUES ($1, now())
ON CONFLICT (project_id) DO UPDATE SET acknowledged_at = now();
