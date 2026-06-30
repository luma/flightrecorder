# flightrecorder

`flightrecorder` is a lightweight telemetry collector and review console for
games.

It is designed for small teams that want useful player-behaviour signal without
shipping personal identifiers, buying into a product-analytics platform, or
building every dashboard from scratch. Game clients send privacy-respecting
events and opt-in bug/sentiment reports. The collector validates those requests,
stores analytical event rows, stores report screenshots outside the event table,
and serves a compact admin UI for exploring what happened.

The first production user is Sursidus, but the project should stay reusable:
project-specific maps, event names, and validation policy belong in project
configuration, not in hardcoded collector logic.

## Current Surface

The service currently includes:

- Public ingestion API for `/v1/events` and `/v1/bug-reports`.
- Postgres-backed event, batch, token, bug report, and admin tables.
- A Vite admin dashboard embedded into the Go binary and served by the API.
- Admin auth for local development and admin API routes for review workflows.
- Project configuration contract and executable request fixtures.
- Local filesystem screenshot storage, with R2-compatible object storage support
  behind the same storage interface.
- A reusable Godot client asset and local E2E demo project under `godot/`; see
  `godot/README.md`.

## Repository Layout

```text
.
├── main.go
├── cmd/
│   ├── migrate/
│   ├── service/
│   └── version/
├── api/
│   ├── auth/
│   └── spa/
├── services/
├── db/
│   ├── dbq/                 # sqlc-generated code; do not edit by hand
│   ├── queries/             # raw sqlc query files
│   └── schema/
│       └── migrations/
├── env/
├── web-vite/                # admin SPA embedded into the Go binary
├── godot/                   # reusable Godot client asset and E2E demo
├── docs/
│   ├── api-contract.md
│   └── project-config.md
├── docker-compose.yaml
├── examples/
│   ├── bug-report.valid.json
│   ├── events-batch.valid.json
│   └── sursidus.project.json
└── internal/
    └── contract/
        └── contract_test.go
```

## Local Development

### Prerequisites

- Go 1.25 or newer.
- Docker with Docker Compose.
- Bun, used by `web-vite` when building the embedded admin SPA.

Run every command below from the repository root.

### 1. Start Postgres

```bash
make dev-up
```

This starts the `postgres` service from `docker-compose.yaml` and exposes it on
`localhost:5432`.

The default app config already matches the compose database:

```text
API_PORT=8080
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=flightrecorder
POSTGRES_PASSWORD=flightrecorder
POSTGRES_DB=flightrecorder
POSTGRES_SSL_MODE=disable
REPORT_STORAGE_BACKEND=local
REPORT_STORAGE_DIR=var/reports
ADMIN_DEV_LOGIN=true
ADMIN_ALLOWED_EMAILS=admin@example.com
```

You can override any value with environment variables or a local `.env` file.
The `.env` file is loaded automatically by the Go service and migration
commands. To start from the checked-in local defaults:

```bash
cp .env.example .env
```

### 2. Apply Database Migrations

```bash
make migrate-up
```

If you want to inspect the local database directly:

```bash
docker compose exec postgres psql -U flightrecorder -d flightrecorder
```

### 3. Bootstrap a Local Project

Migrations create the schema, but they do not seed a project. Add the local
Sursidus project before opening the admin Settings screen or submitting
fixtures. The values below mirror `examples/sursidus.project.json`:

```bash
docker compose exec postgres psql -U flightrecorder -d flightrecorder
```

Then run this SQL:

```sql
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
)
VALUES (
    'sursidus',
    'Sursidus',
    'warn',
    '{
        "max_events_per_batch": 50,
        "accept_gzip": true,
        "allow_unknown_event_types": true,
        "allow_screenshot_failures": true
    }'::jsonb,
    '{
        "event_days": 730,
        "report_days": 1095,
        "access_log_days": 14
    }'::jsonb,
    '{
        "spatial_enabled": true,
        "zone_extent_m": 30000,
        "zone_heatmap_cell_m": 300
    }'::jsonb,
    '{
        "statuses": ["new", "seen", "reproduced", "fixed", "wont_fix", "needs_more_info"],
        "labels": ["bug", "sentiment", "balance", "mission", "combat", "economy", "ui"],
        "rate_limit_seconds": 60
    }'::jsonb,
    '{
        "lifecycle": ["game_continue", "game_exit", "dock", "undock"],
        "economy": ["buy_commodity", "sell_commodity", "buy_intel", "sell_intel", "purchase_ship", "change_equipment", "clear_bounty"],
        "mission": ["take_mission", "abandon_mission", "complete_mission", "complete_mission_objective", "mission_complication"],
        "combat": ["player_death", "player_kills_npc", "npc_enters_combat_with_player", "player_enters_combat_with_npc"],
        "legal": ["receive_bounty", "faction_rep_change"],
        "report": ["bug_report"]
    }'::jsonb,
    '[
        {
            "key": "economy.credits",
            "source": "metrics.economy.credits",
            "type": "number",
            "label": "Credits",
            "filterable": true,
            "aggregations": ["min", "max", "avg"]
        },
        {
            "key": "ship.hull_pct",
            "source": "metrics.ship.hull_pct",
            "type": "number",
            "label": "Hull",
            "filterable": true,
            "aggregations": ["min", "avg", "histogram"]
        },
        {
            "key": "ship.shield_pct",
            "source": "metrics.ship.shield_pct",
            "type": "number",
            "label": "Shield",
            "filterable": true,
            "aggregations": ["min", "avg", "histogram"]
        },
        {
            "key": "ship.id",
            "source": "dimensions.ship.id",
            "type": "string",
            "label": "Ship",
            "filterable": true,
            "aggregations": ["count"]
        }
    ]'::jsonb,
    '[
        {
            "id": "onboarding_first_return",
            "name": "Onboarding: first station return",
            "description": "continue -> undock -> dock",
            "entity": "player",
            "mode": "unordered_presence",
            "steps": [
                { "id": "continued", "label": "Continued", "match": { "event_type": "game_continue" } },
                { "id": "undocked", "label": "Undocked", "match": { "event_type": "undock" } },
                { "id": "docked", "label": "Docked", "match": { "event_type": "dock" } }
            ]
        },
        {
            "id": "first_trade_loop",
            "name": "First trade loop",
            "description": "buy commodity -> sell commodity",
            "entity": "player",
            "mode": "unordered_presence",
            "steps": [
                { "id": "bought", "label": "Bought commodity", "match": { "event_type": "buy_commodity" } },
                { "id": "sold", "label": "Sold commodity", "match": { "event_type": "sell_commodity" } }
            ]
        },
        {
            "id": "first_mission_loop",
            "name": "First mission loop",
            "description": "take mission -> complete mission",
            "entity": "player",
            "mode": "unordered_presence",
            "steps": [
                { "id": "took", "label": "Took mission", "match": { "event_type": "take_mission" } },
                { "id": "completed", "label": "Completed mission", "match": { "event_type": "complete_mission" } }
            ]
        },
        {
            "id": "first_combat_entry",
            "name": "First combat entry",
            "description": "combat start",
            "entity": "player",
            "mode": "unordered_presence",
            "steps": [
                { "id": "started", "label": "Started combat", "match": { "event_type": "combat_start" } }
            ]
        },
        {
            "id": "first_station_return",
            "name": "First station return",
            "description": "undock -> dock",
            "entity": "player",
            "mode": "unordered_presence",
            "steps": [
                { "id": "undocked", "label": "Undocked", "match": { "event_type": "undock" } },
                { "id": "docked", "label": "Docked", "match": { "event_type": "dock" } }
            ]
        },
        {
            "id": "first_report",
            "name": "First player report",
            "description": "bug_report submissions by time window",
            "entity": "player",
            "mode": "unordered_presence",
            "steps": [
                { "id": "reported", "label": "Reported", "match": { "event_type": "bug_report" } }
            ]
        }
    ]'::jsonb
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
    updated_at = now();
```

Type `\q` to leave `psql`.

### 4. Start the Service

```bash
make serve
```

The service listens on `http://localhost:8080`.

Health checks:

```bash
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/v1/ready
```

Open the admin UI:

```text
http://localhost:8080
```

With the default local config, sign in with:

```text
admin@example.com
```

The local login is controlled by `ADMIN_DEV_LOGIN=true`. For shared or
production-like environments, set `ADMIN_DEV_LOGIN=false`, replace
`ADMIN_SESSION_SECRET`, and configure the real auth path before exposing the
service.

### 5. Create an Ingest Token

In the admin UI:

1. Open the `Settings` tab.
2. Enter a token name such as `sursidus-dev`.
3. Click `Create Token`.
4. Copy the token immediately. Only the hash is stored after creation.

Export it in your shell:

```bash
export FR_INGEST_TOKEN='<paste-token-here>'
```

### 6. Submit Example Data

Send the valid event batch fixture:

```bash
curl -sS -X POST http://localhost:8080/v1/events \
  -H "Authorization: Bearer $FR_INGEST_TOKEN" \
  -H "Content-Type: application/json" \
  --data @examples/events-batch.valid.json
```

Send the valid bug report fixture:

```bash
curl -sS -X POST http://localhost:8080/v1/bug-reports \
  -H "Authorization: Bearer $FR_INGEST_TOKEN" \
  -H "Content-Type: application/json" \
  --data @examples/bug-report.valid.json
```

Bug reports are rate-limited by player. If you resubmit the same report
fixture repeatedly, wait at least 60 seconds or edit the fixture's player and
report IDs.

Refresh the admin UI and use the `Events`, `Reports`, `Trace`, `Regions`, `Zone`,
`Funnels`, and `Schema` tabs to inspect the submitted data.

### Local Storage

With `REPORT_STORAGE_BACKEND=local`, report screenshots are written under
`var/reports/`, which is ignored by Git. This is the recommended local-dev
backend.

For production object storage, set `REPORT_STORAGE_BACKEND=r2` and provide:

```text
R2_ENDPOINT=
R2_BUCKET=
R2_ACCESS_KEY_ID=
R2_SECRET_ACCESS_KEY=
R2_REGION=auto
```

### Reset or Stop Local Dev Services

Stop Postgres while keeping the database volume:

```bash
make dev-down
```

Delete the database volume and start fresh:

```bash
make dev-reset
make migrate-up
```

## Verify

```bash
make test
```

## Design Principles

- **Games first.** Coordinates, zones, missions, player traces, and screenshot
  reports are first-class concerns.
- **Privacy by default.** The collector is built around project-scoped ingest
  tokens and game-generated player IDs, not platform accounts or machine
  fingerprints.
- **Reusable, not generic mush.** Sursidus-specific data is supported through
  project configuration and examples, while the collector remains useful for
  other games.
- **Single deployable.** The Go binary serves ingestion routes, admin API
  routes, and the built frontend assets.
- **Observable contracts.** Example requests are executable fixtures, so contract
  drift shows up in tests instead of in production.
