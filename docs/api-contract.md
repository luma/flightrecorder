# API Contract

This contract defines the public ingestion API, admin API shape, authentication
model, response codes, idempotency behavior, and gzip support for
`flightrecorder`.

The API is project-scoped so one collector can accept telemetry for multiple
games. Sursidus is the first project, using `project_id: "sursidus"`.

## Conventions

- Public ingestion routes are versioned under `/v1`.
- Admin API routes are versioned under `/api/admin/v1`.
- Request and response bodies are JSON.
- Game clients may gzip request bodies. The collector must accept both plain
  JSON and `Content-Encoding: gzip`.
- Ingestion auth uses project-scoped bearer tokens.
- Admin auth uses OAuth-backed browser sessions, with users allowlisted in
  project or collector config.
- Server timestamps are ISO 8601 UTC strings.

## Ingestion Authentication

Telemetry ingestion uses bearer tokens, not OAuth.

```http
Authorization: Bearer <ingest_token>
Content-Type: application/json
Accept: application/json
```

Tokens are project scoped. The backend stores only token hashes.

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
  "server_time": "2026-06-06T11:42:06Z"
}
```

### Event Envelope

| Field | Type | Required | Notes |
|---|---|---|---|
| `schema_version` | integer | yes | Event envelope schema version. Current version is `2`. |
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
| Duplicate `batch_id` | `200 OK`; return the original accepted/rejected counts. |
| Empty `events` | `400 Bad Request`. |
| Batch above configured event limit | `413 Payload Too Large`. |
| Malformed event envelope | Store valid events, reject invalid events, increment `rejected`. |
| Unknown `event_type` | Accept and flag unless project validation mode is strict. |
| Missing `report_id` on `/v1/bug-reports` | `400 Bad Request`. |
| Invalid screenshot base64 | Accept the report if configured to allow screenshot failures, otherwise `400 Bad Request`. |

The game sender advances its local cursor only after a 2xx response. Any non-2xx
response or network failure keeps the events in the local write-ahead log.

## Admin API Shape

The admin API powers the future Vite dashboard. It returns JSON only.

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

Configured field filters use `field_key` and optional `field_value` query
parameters. `field_key` must match a project `query_fields` entry. `field_value`
is compared against the projected string value, number value, or bool value.
