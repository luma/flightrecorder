# Project Configuration

Project configuration lets `flightrecorder` stay reusable while supporting
game-specific schemas, maps, and report workflows.

Create and edit projects from the admin UI. The top-nav `Add Project` button
opens a wizard for identity, defaults, and schema. When there are no projects,
the dashboard shows an empty state and opens the wizard automatically.

The schema builders start empty for new projects. Event groups, query fields,
and funnels are optional and should be added only when they reflect concepts in
the game being integrated. `examples/sursidus.project.json` is a complete
example, not a default template.

## Shape

```json
{
  "project_id": "sursidus",
  "display_name": "Sursidus",
  "validation_mode": "warn",
  "ingest": {
    "max_events_per_batch": 50,
    "accept_gzip": true,
    "allow_unknown_event_types": true,
    "allow_screenshot_failures": true
  },
  "retention": {
    "event_days": 730,
    "report_days": 1095,
    "access_log_days": 14
  },
  "maps": {
    "spatial_enabled": true,
    "zone_extent_m": 30000,
    "zone_heatmap_cell_m": 300
  },
  "reports": {
    "statuses": ["new", "seen", "reproduced", "fixed", "wont_fix", "needs_more_info"],
    "labels": ["bug", "sentiment", "balance", "mission", "combat", "economy", "ui"],
    "rate_limit_seconds": 60
  },
  "event_groups": {
    "lifecycle": ["new_game", "game_continue", "game_exit", "dock", "undock"],
    "economy": ["buy_commodity", "sell_commodity", "buy_intel", "sell_intel", "purchase_ship", "change_equipment", "clear_bounty"],
    "mission": ["take_mission", "abandon_mission", "complete_mission", "complete_mission_objective", "mission_complication"],
    "combat": ["player_death", "player_kills_npc", "npc_enters_combat_with_player", "player_enters_combat_with_npc"],
    "legal": ["receive_bounty", "faction_rep_change"],
    "report": ["bug_report"]
  },
  "query_fields": [
    {
      "key": "economy.credits",
      "source": "metrics.economy.credits",
      "type": "number",
      "label": "Credits",
      "filterable": true,
      "aggregations": ["min", "max", "avg"]
    },
    {
      "key": "ship.id",
      "source": "dimensions.ship.id",
      "type": "string",
      "label": "Ship",
      "filterable": true,
      "aggregations": ["count"]
    }
  ],
  "funnels": [
    {
      "id": "first_trade_loop",
      "name": "First trade loop",
      "description": "buy commodity -> sell commodity",
      "entity": "player",
      "mode": "ordered",
      "steps": [
        { "id": "bought", "label": "Bought commodity", "match": { "event_type": "buy_commodity" } },
        { "id": "sold", "label": "Sold commodity", "match": { "event_type": "sell_commodity" }, "after": "bought" }
      ]
    }
  ]
}
```

## Fields

| Field | Notes |
|---|---|
| `project_id` | Stable lowercase identifier used in ingestion requests. |
| `display_name` | Human-facing project name in the admin UI. |
| `validation_mode` | `warn` accepts unknown event types and records validation warnings. `strict` rejects unknown event types. |
| `ingest.max_events_per_batch` | Upper bound for `/v1/events` batch size. |
| `ingest.accept_gzip` | Whether gzip request bodies are accepted. |
| `ingest.allow_unknown_event_types` | Per-project unknown event policy. |
| `ingest.allow_screenshot_failures` | Whether reports may be accepted when screenshot decoding/storage fails. |
| `retention.event_days` | Event-row retention target. |
| `retention.report_days` | Bug-report retention target. |
| `retention.access_log_days` | HTTP access log retention target. Keep short to avoid storing raw IPs longer than needed. |
| `maps.spatial_enabled` | Whether the game has physical regions/zones. When false, the admin hides the Regions and Zone map tabs. |
| `maps.zone_extent_m` | Expected width/depth of a zone in metres. |
| `maps.zone_heatmap_cell_m` | Bin size for zone heat-map aggregation. |
| `reports.statuses` | Allowed triage statuses. |
| `reports.labels` | Suggested report labels. |
| `reports.rate_limit_seconds` | Client/report anti-spam window. |
| `event_groups` | UI grouping metadata for filters and schema screens. |
| `query_fields` | Project-declared field projections. Each field has `key`, `source`, `type`, `label`, `filterable`, and `aggregations`. |
| `funnels` | Project-declared funnel definitions. Each funnel has `id`, `name`, `entity`, `mode`, and one or more `steps`. |

`query_fields.source` starts with one of `context`, `metrics`, `dimensions`, or
`payload`. The collector first checks for an exact key after the root, such as
`metrics.ship.hull_pct` reading `{ "ship.hull_pct": 0.94 }`, then falls back to
nested object traversal. This supports compact dotted metric keys and ordinary
nested JSON.

## Funnels

Funnels describe per-project concepts that should be tracked as stepwise player
progression. The admin UI exposes the same builder for new projects and
existing project schema settings.

Each funnel has:

| Field | Notes |
|---|---|
| `id` | Stable lowercase identifier used in API responses. |
| `name` | Human-facing label in the Funnels tab. |
| `description` | Optional context shown with the funnel. |
| `entity` | Currently always `player`. |
| `mode` | `ordered` or `unordered_presence`. |
| `enabled` | Optional; omitted or `true` means active. |
| `steps` | One or more funnel steps. |

Each step has:

| Field | Notes |
|---|---|
| `id` | Stable step identifier, unique within the funnel. |
| `label` | Human-facing step label. |
| `match` | Event matcher for this step. |
| `after` | Ordered funnels only; defaults to the previous step when omitted in the UI. |
| `within_seconds` | Ordered funnels only; optional maximum real-time gap after the prior step. |

Step matchers can combine:

- `event_type` or `event_types`
- `region_id`
- `zone_id`
- `field_key` and optional `field_value`

`field_key` must reference a `query_fields` entry with `filterable: true`.
`field_value` is typed from the query field definition, so number fields require
numeric values and bool fields require `true` or `false`.

`ordered` funnels count players who reach steps in sequence. Event ordering uses
`game_time`; `within_seconds` uses the event timestamps to bound the elapsed
real time between steps. `unordered_presence` funnels count cumulative step
presence in the selected time window without requiring ordering; step N counts
players that matched every step through N.

One-step funnels are valid. For example, a `first_report` funnel can use a
single `bug_report` step, making `started` and `completed` equal for that
funnel.

## Initial Sursidus Policy

- Use `validation_mode: "warn"` during early integration so newly added events
  do not get lost because the collector config is stale.
- Keep access logs short-lived or anonymized. Telemetry rows should not store raw
  IP addresses.
- Store screenshots outside analytical event tables. The database keeps only the
  object key.
- Load Sursidus map overlays from project config so the collector can stay open
  source without bundling Sursidus-only assets.
