# flightrecorder Godot Client

This directory contains a reusable Godot 4.6 client asset and a small local
end-to-end demo project for `flightrecorder`.

## Contents

```text
godot/
├── project.godot
├── addons/
│   └── flightrecorder/
│       ├── flightrecorder_telemetry.gd
│       ├── plugin.cfg
│       └── plugin.gd
└── demo/
    ├── flightrecorder_e2e_demo.gd
    └── flightrecorder_e2e_demo.tscn
```

`addons/flightrecorder/flightrecorder_telemetry.gd` is the reusable client. It
is exposed as the `FlightRecorderTelemetry` Autoload in the demo project.

## Tests

GUT 9.6.0 lives under `res://addons/gut/`. Run the addon tests from the repo
root with:

```bash
make -C godot test
```

Run one file with:

```bash
make -C godot test-file FILE=res://tests/test_flightrecorder_telemetry.gd
```

The full-suite target writes Godot logs to `godot/.godot-test/`.

## Install In An Existing Godot Project

1. Copy `godot/addons/flightrecorder/` into your project as:

   ```text
   res://addons/flightrecorder/
   ```

2. Enable the plugin in Godot:

   ```text
   Project -> Project Settings -> Plugins -> flightrecorder -> Enable
   ```

   The plugin registers an Autoload singleton named
   `FlightRecorderTelemetry`.

3. If you do not want to enable the plugin, add the Autoload manually:

   ```text
   Name: FlightRecorderTelemetry
   Path: res://addons/flightrecorder/flightrecorder_telemetry.gd
   ```

4. Configure the client at startup or from your settings UI:

   ```gdscript
   FlightRecorderTelemetry.configure({
       "endpoint_url": "http://localhost:8080/",
       "project_id": "my-game",
       "ingest_token": "<token>",
       "game_version": "0.8.2",
       "build_channel": "local",
       "commit_sha": "abc123def456",
   })
   ```

   The `project_id` must match a project created in the flightrecorder admin UI.
   New projects start with empty event groups, query fields, and funnels; add
   only the fields and funnels your game needs.

5. Record events from gameplay code:

   ```gdscript
   FlightRecorderTelemetry.record_event("dock", {
   	"station_id": "demo_station",
   }, {
   	"context": {
   		"location": {
   			"region_id": "lave",
   			"zone_id": "lave_primary",
   			"position": [1240.5, -80.0, 330.2],
   		},
   	},
   	"metrics": {
   		"economy.credits": 48200,
   		"ship.hull_pct": 0.94,
   		"ship.shield_pct": 1.0,
   	},
   	"dimensions": {
   		"ship.id": "cobra_mk3",
   	},
   })
   ```

## Local E2E Demo

1. Start local Postgres and the collector from the `flightrecorder` repo root:

   ```bash
   cp .env.example .env
   make dev-up
   make serve
   ```

   The service runs database migrations on startup. You can run
   `make migrate-up` first if you want to check migrations separately.

2. Open `http://localhost:8080` and sign in with `admin@example.com`.

3. If no projects exist, the admin UI opens the Add Project wizard. Create a
   project whose ID matches the demo config, or update the demo config to match
   the project you create.

4. Add any query fields and funnels you want to inspect. For the sample events
   below, useful query fields are `metrics.economy.credits`,
   `metrics.ship.hull_pct`, `metrics.ship.shield_pct`, and
   `dimensions.ship.id`. A simple funnel can match `dock` after `undock`, or a
   one-step `bug_report` funnel for submitted reports.

5. Create an ingest token in the Settings tab.

6. Open the `godot/` directory in Godot 4.6.

7. Run the demo scene, paste the ingest token, and click `Emit Events` or
   `Submit Report`.

8. Return to the admin UI and check Event Explorer, Player Trace, heat-map
   tables, funnels, and Bug Reports.

## Endpoint Settings

For local development, use the Go service directly:

```text
http://localhost:8080/
```

You can also use the Vite dev server when it is running and proxying `/v1` to
the Go service:

```text
http://localhost:3000/
```

The client appends versioned API paths such as `/v1/events` and
`/v1/bug-reports`.

## Bug Reports

```gdscript
var screenshot := FlightRecorderTelemetry.capture_viewport_png_base64()
FlightRecorderTelemetry.submit_bug_report(
	2,
	"frustrated",
	"The demo report button worked, but I am roleplaying distress.",
	screenshot,
	{
		"context": {
			"location": {
				"region_id": "reorte",
				"zone_id": "reorte_open",
				"position": [4200.1, 0.0, -810.3],
			},
		},
		"metrics": {
			"economy.credits": 48200,
			"ship.hull_pct": 0.72,
			"ship.shield_pct": 0.0,
		},
		"dimensions": {
			"ship.id": "demo_ship",
		},
	},
	{
		"active_missions": ["demo_mission_1"],
	}
)
```

## Write-Ahead Log

Events and reports are written before the sender thread attempts delivery. The
WAL is project-specific:

```text
user://flightrecorder/projects/<project_id>/wal.ndjson
```

The stable anonymous player ID helper is project-specific too:

```text
user://flightrecorder/projects/<project_id>/player_id.txt
```

The sender removes records only after the collector returns a 2xx response.
Truncated or malformed final lines are skipped when reading the log.
