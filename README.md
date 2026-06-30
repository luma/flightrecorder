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

### 2. Optional: Check Database Migrations

```bash
make migrate-up
```

The service runs `schema.Up()` when it starts, so `make serve` will bring the
database to the latest schema before binding the API port. Running
`make migrate-up` explicitly is optional, but useful when you want migration
failures to surface before starting the server.

If you want to inspect the local database directly:

```bash
docker compose exec postgres psql -U flightrecorder -d flightrecorder
```

### 3. Start the Service

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

### 4. Create a Local Project

Migrations create the schema, but they do not seed projects. Use `Add Project`
in the top nav. If no projects exist, the dashboard shows an empty state and
opens the Add Project wizard automatically.

The wizard has three sections:

- `Identity`: project ID, display name, and validation mode.
- `Defaults`: ingest limits, retention, map behavior, report statuses/labels,
  and bug-report rate limit.
- `Schema`: event groups, query fields, and funnels.

The schema section starts empty. Add only the event groups, query fields, and
funnels that make sense for the game you are integrating. The Sursidus example
configuration remains available in `examples/sursidus.project.json` as a
reference, not as the default starting point.

### 5. Create an Ingest Token

In the admin UI:

1. Open the `Settings` tab.
2. Enter a token name such as `local-dev`.
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

The checked-in fixtures use `project_id: "sursidus"` and Sursidus-shaped event
names/fields. Either create a local project with ID `sursidus` and matching
schema/funnels, or edit the fixture JSON to use your own project ID and event
shape.

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
```

The next `make serve` will run migrations automatically. You can still run
`make migrate-up` manually after a reset if you want to validate the database
before starting the service.

## Deployment Notes

The service runs application and River migrations on startup by calling
`schema.Up()` before it creates the long-lived API database pool. The database
role used at startup must have DDL privileges for tables, indexes, migration
metadata, River tables, and `CREATE EXTENSION IF NOT EXISTS pgcrypto` on a fresh
database.

For PlanetScale Postgres, route normal service traffic through PgBouncer and
run migrations through a direct connection:

```text
POSTGRES_PORT=6432
POSTGRES_MIGRATE_PORT=5432
POSTGRES_MIGRATE_MAX_CONNECTIONS=2
POSTGRES_MIGRATE_MIN_CONNECTIONS=1
```

`POSTGRES_MIGRATE_HOST` is also supported when the direct endpoint has a
different hostname. Keep the migration pool small so deploys do not exhaust
backend connection slots.

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
