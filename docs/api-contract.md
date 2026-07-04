# API Contract

This contract defines the public ingestion API, admin API shape, authentication
model, response codes, idempotency behavior, and gzip support for
`flightrecorder`.

The API is project-scoped so one collector can accept telemetry for multiple
games. Create projects in the admin UI before sending telemetry. The example
payloads below use `project_id: "sursidus"` because that is the checked-in
fixture project, not because the collector requires that ID.

## Conventions

- Public ingestion routes are versioned under `/v1`.
- Admin API routes are versioned under `/api/admin/v1`.
- Request and response bodies are JSON.
- Game clients may gzip request bodies. The collector must accept both plain
  JSON and `Content-Encoding: gzip`.
- Ingestion auth uses project-scoped bearer tokens.
- Admin auth uses OAuth-backed browser sessions, a configured email-domain
  gate, and invitation-managed users.
- MCP agent auth uses separate OAuth-issued bearer tokens.
- Server timestamps are ISO 8601 UTC strings.

## Ingestion Authentication

Telemetry ingestion uses bearer tokens, not OAuth.

```http
Authorization: Bearer <ingest_token>
Content-Type: application/json
Accept: application/json
```

Tokens are project scoped and prefixed with `fr_tel_`. The backend stores only
token hashes. MCP agent tokens are prefixed with `fr_agnt_` and are rejected by
ingestion routes.

## MCP Agent Authentication

Remote agents use the MCP endpoint at `/mcp`, OAuth authorization code + PKCE,
and project-scoped `fr_agnt_` bearer tokens. See [MCP Remote Agent
Access](mcp.md).

## `POST /v1/events`

Accepts a batch of telemetry events from a game client.

Request:

```json
{
  "project_id": "sursidus",
  "batch_id": "98bb9fc1-c09f-45f2-bcf5-20ac71c2381a",
  "sent_at": "2026-06-06T11:42:05Z",
  "client": {
    "game_version": "0.8.2",
    "build_channel": "early_access",
    "commit_sha": "abc123def456",
    "platform": "linux"
  },
  "events": [
    {
      "schema_version": 2,
      "event_id": "b0c4f6a2-1e3d-4a5b-8c9d-0e1f2a3b4c5d",
      "player_id": "550e8400-e29b-41d4-a716-446655440000",
      "event_type": "dock",
      "real_ts": "2026-06-06T11:42:00Z",
      "game_time": 1843200,
      "context": {
        "location": {
          "region_id": "lave",
          "zone_id": "lave_primary",
          "position": [1240.5, -80.0, 330.2]
        }
      },
      "metrics": {
        "economy.credits": 48200,
        "ship.hull_pct": 0.94,
        "ship.shield_pct": 1.0
      },
      "dimensions": {
        "ship.id": "cobra_mk3"
      },
      "payload": {
        "station_id": "stn_6a11eb1c-5cab-46f3-a911-0f8e8a14e6cd"
      }
    }
  ]
}
```

Response:

```json
{
  "accepted": 1,
  "rejected": 0,
  "batch_id": "98bb9fc1-c09f-45f2-bcf5-20ac71c2381a",
  "server_time": "2026-06-06T11:42:06Z",
  "rejections": []
}
```

A well-formed request always returns `200 OK` and reports per-event outcomes,
even when **every** event in the batch is invalid (see Validation And Response
Rules). Each rejected event appears in `rejections` with its `index`, a stable
machine-readable `reason` code, and a human-readable `message`:

```json
{
  "accepted": 0,
  "rejected": 1,
  "batch_id": "98bb9fc1-c09f-45f2-bcf5-20ac71c2381a",
  "server_time": "2026-06-06T11:42:06Z",
  "rejections": [
    { "index": 0, "reason": "player_id_not_uuid", "message": "player_id must be a UUID" }
  ]
}
```

Reason codes include `schema_version_unsupported`, `event_id_not_uuid`,
`player_id_not_uuid`, `event_type_missing`, `real_ts_invalid`,
`context_not_object`, `metrics_not_object`, `dimensions_not_object`,
`position_invalid`, and `payload_invalid`.

`rejections` is present only on **first** processing of a batch. A duplicate
`batch_id` replay returns the stored `accepted`/`rejected` counts *without* the
`rejections` array, so clients must not assume it is always present.

The ingestion endpoints tolerate unknown JSON fields (forward compatibility): an
unknown top-level or per-event field is ignored rather than rejected, so an
older collector never `400`s a newer client. The admin and MCP APIs reject
unknown fields.

### Event Envelope

| Field | Type | Required | Notes |
|---|---|---|---|
| `schema_version` | integer | yes | Event envelope schema version. Current version is `2`. |
| `event_id` | string | no | Client-generated idempotency key (UUID). When present, the collector dedups on `(project_id, event_id)`, so a resend after a lost response never creates a duplicate. Omitted by older clients, which fall back to batch-level dedup. |
| `player_id` | string | yes | Game-generated playthrough/player ID. |
| `event_type` | string | yes | Stable event type name. |
| `real_ts` | string | yes | Client event time, ISO 8601 UTC. |
| `game_time` | integer | yes | In-game seconds elapsed. |
| `context` | object | no | Generic context. `context.location.region_id`, `zone_id`, and `position` power map/trace screens when present. |
| `metrics` | object | no | Project-declared numeric state such as Sursidus credits, hull, and shields. |
| `dimensions` | object | no | Project-declared string/bool grouping values such as ship ID or station ID. |
| `payload` | object | yes | Event-specific data. |

The collector stores the raw event JSON as the source of truth. Project
configuration can declare `query_fields` that project selected values from
`context`, `metrics`, `dimensions`, or `payload` into typed query rows. This is
how Sursidus gets first-class reporting for `economy.credits`,
`ship.hull_pct`, `ship.shield_pct`, and `ship.id` without making those fields
universal for every game.

## `POST /v1/bug-reports`

Accepts a user-initiated report. The report contains one `bug_report` event
envelope plus optional inline screenshot data. The collector writes screenshot
bytes to object storage or a configured local data directory, then stores only
the resulting object key in the database.

Request:

```json
{
  "project_id": "sursidus",
  "report_id": "52b12975-a875-4aef-a6a3-b326af3e47ce",
  "client": {
    "game_version": "0.8.2",
    "build_channel": "early_access",
    "commit_sha": "abc123def456",
    "platform": "linux"
  },
  "event": {
    "schema_version": 2,
    "event_id": "3f2a7b1c-9d8e-4f6a-b1c2-d3e4f5a6b7c8",
    "player_id": "550e8400-e29b-41d4-a716-446655440000",
    "event_type": "bug_report",
    "real_ts": "2026-06-06T11:55:00Z",
    "game_time": 1843200,
    "context": {
      "location": {
        "region_id": "reorte",
        "zone_id": "reorte_open",
        "position": [4200.1, 0.0, -810.3]
      }
    },
    "metrics": {
      "economy.credits": 48200,
      "ship.hull_pct": 0.72,
      "ship.shield_pct": 0.0
    },
    "dimensions": {
      "ship.id": "corsair_mk2"
    },
    "payload": {
      "mood": 1,
      "mood_label": "unhappy",
      "notes": "The gate toll did not deduct from my credits but I got transited anyway",
      "screenshot_png_base64": "iVBORw0KGgo=",
      "active_missions": ["reorte_12_1"]
    }
  }
}
```

Response:

```json
{
  "accepted": true,
  "report_id": "52b12975-a875-4aef-a6a3-b326af3e47ce",
  "screenshot_object_key": "bug-reports/sursidus/2026/06/06/52b12975.png",
  "server_time": "2026-06-06T11:55:02Z"
}
```

## Validation And Response Rules

| Rule | Behavior |
|---|---|
| Missing or invalid bearer token | `401 Unauthorized`. |
| Token valid but not allowed for `project_id` | `403 Forbidden`. |
| Missing `project_id` | `400 Bad Request`. |
| Unknown `project_id` | `401 Unauthorized` or `403 Forbidden`, depending on token validity. |
| Missing `batch_id` on `/v1/events` | `400 Bad Request`. |
| Duplicate `batch_id` | `200 OK`; return the stored accepted/rejected counts *without* `rejections`. |
| Empty `events` | `400 Bad Request`. |
| Batch above configured event limit | `413 Payload Too Large`. |
| Malformed event envelope (some events invalid) | `200 OK`. Store valid events, list invalid ones in `rejections`, increment `rejected`. |
| All events invalid | `200 OK` with `accepted: 0`, `rejected: N`, and a populated `rejections` array — *not* `400`. A record-level rejection must never fail the whole batch, or one bad event could poison an unpatchable client's queue. |
| Duplicate `event_id` (within a batch or across a resend) | Deduplicated on `(project_id, event_id)`; counted as accepted, stored once. |
| Unknown JSON field (top-level or per-event) on ingestion | Ignored (forward compatibility). Admin/MCP APIs reject unknown fields. |
| Unknown `event_type` | Accept and flag unless project validation mode is strict. |
| Missing `report_id` on `/v1/bug-reports` | `400 Bad Request`. |
| Duplicate `report_id` on `/v1/bug-reports` | `200 OK`; the stored report is replayed idempotently (no duplicate, no `500`). |
| Invalid screenshot base64 | Accept the report if configured to allow screenshot failures, otherwise `400 Bad Request`. |

The game sender advances its local cursor only after a 2xx response. Any non-2xx
response or network failure keeps the events in the local write-ahead log. The
client classifies non-2xx outcomes: transient errors (timeouts, `408/425/429`,
`5xx`) are retried with backoff; config/routing errors (`401/403/404/405`, or a
`400/422` whose body is not the collector's `{"error": …}` shape) stop the drain
without dropping data; and a `400/422` carrying the collector's error body — from
an older server that still hard-rejects — quarantines just the offending records
so the rest of the queue drains.

## Admin API Shape

The admin API powers the embedded Vite dashboard. It returns JSON only.

| Route | Purpose |
|---|---|
| `GET /api/admin/v1/summary` | Time-windowed totals for events, players, sessions, deaths, reports, and opt-in counts. |
| `GET /api/admin/v1/events` | Paginated event search by type, region, zone, player, version, channel, configured field, and time range. |
| `GET /api/admin/v1/players/{player_id}/trace` | Chronological playthrough trace. |
| `GET /api/admin/v1/heatmap/regions` | Per-region event counts for the regions map colouring, with the same event/version/channel/configured-field filters. |
| `GET /api/admin/v1/heatmap/zones` | Binned zone-space `(x, z)` counts, with the same event/version/channel/configured-field filters. |
| `GET /api/admin/v1/funnels` | Named funnel results. |
| `GET /api/admin/v1/reports` | Bug/sentiment report inbox. |
| `GET /api/admin/v1/reports/{report_id}` | Report detail with screenshot URL and surrounding trace. |
| `PATCH /api/admin/v1/reports/{report_id}` | Update report status, labels, and internal notes. |
| `GET /api/admin/v1/event-types` | Known event types, sample payloads, counts, and validation errors. |
| `GET /api/admin/v1/rejected-events` | Rejected-event groups (by event type, reason code, and game version) with counts, first/last seen, a sample event, and `active_group_count` (groups active in the last 24h newer than the last acknowledgement). Powers the Data Quality page. |
| `GET /api/admin/v1/rejected-events/count` | `active_group_count` only, for the nav badge. |
| `POST /api/admin/v1/rejected-events/acknowledge` | Acknowledge current rejection activity for the project (shared across users), clearing the nav badge until a group recurs. |
| `GET /api/admin/v1/projects` | Active projects for the top-nav project switcher. |
| `POST /api/admin/v1/projects` | Create or update a project, including defaults, event groups, query fields, and funnels. |
| `GET /api/admin/v1/settings` | Current project config and ingest tokens. |
| `POST /api/admin/v1/settings/ingest-tokens` | Create an ingest token for the active project. |
| `PATCH /api/admin/v1/settings/ingest-tokens/{token_id}` | Enable or disable an ingest token. |

Configured field filters use `field_key` and optional `field_value` query
parameters. `field_key` must match a project `query_fields` entry. `field_value`
is compared against the projected string value, number value, or bool value.

`GET /api/admin/v1/funnels` evaluates the active project's configured
`funnels`. Each result includes the existing summary fields (`started`,
`completed`, `rate`, and `dropoff`) plus `steps`, an ordered list of per-step
counts and rates. Project settings responses include the raw `funnels`
configuration so the dashboard can display and edit project-specific funnel
definitions.

Configured funnels support two modes:

- `ordered`: later steps must occur after earlier steps by `game_time`.
  Optional `within_seconds` bounds elapsed real time between ordered steps.
- `unordered_presence`: ordering is ignored. Step N counts players who matched
  every configured step from the first step through N in the selected time
  window.

Funnel step matchers can use `event_type`, `event_types`, `region_id`,
`zone_id`, and filterable project query fields. A matcher `field_key` must refer
to a project `query_fields` entry marked `filterable: true`.
