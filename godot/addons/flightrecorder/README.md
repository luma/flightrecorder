# flightrecorder Godot Addon

This addon sends privacy-respecting gameplay telemetry and opt-in reports to a
flightrecorder collector.

## Install

Copy this directory into a Godot project:

```text
res://addons/flightrecorder/
```

Enable the plugin:

```text
Project -> Project Settings -> Plugins -> flightrecorder -> Enable
```

The plugin registers an Autoload singleton named `FlightRecorderTelemetry`.

## Configure

Create a project in the flightrecorder admin UI first. If the collector has no
projects, the dashboard shows an empty state and opens the Add Project wizard.
The project wizard includes the baseline `bug_report` event group and report
query fields needed by the feedback Reports tab. Add any additional event
groups, query fields, and funnels that match your game.

Then configure the addon with the same project ID and an ingest token from that
project's Settings tab. Ingest tokens are prefixed with `fr_tel_`:

```gdscript
FlightRecorderTelemetry.configure({
    "endpoint_url": "https://collector.example.com/",
    "project_id": "my-game",
    "ingest_token": "<token>",
    "game_version": "0.8.2",
    "build_channel": "early_access",
    "commit_sha": "abc123def456",
})
```

## Events And Project Schema

`record_event()` accepts an event type, event payload, and optional telemetry
layers:

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
    },
    "dimensions": {
        "ship.id": "cobra_mk3",
    },
})
```

The collector stores the full event JSON. Project `query_fields` tell the admin
UI which values should be projected into typed filterable fields. Project
`funnels` are also configured server-side; the addon only sends events.

Funnels can match event types, regions, zones, and filterable query fields. Use
`ordered` funnels when step sequence matters, or `unordered_presence` when a
player only needs to have matched each step in the selected time window.

## Local Files

The addon keeps unsent records in a project-specific write-ahead log:

```text
user://flightrecorder/projects/<project_id>/wal.ndjson
```

The anonymous player ID helper is project-specific too:

```text
user://flightrecorder/projects/<project_id>/player_id.txt
```

Changing `project_id` switches both paths, so each collector project gets its
own WAL and player ID.
