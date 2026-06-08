## Local end-to-end harness for the reusable flightrecorder Godot client.
##
## Run the flightrecorder service locally, create an ingest token in the admin
## Settings screen, paste it here, then use the buttons to emit contract-shaped
## sample telemetry and a bug report from Godot.
extends Control

const TELEMETRY_SCRIPT := preload("res://addons/flightrecorder/flightrecorder_telemetry.gd")

@onready var endpoint_edit: LineEdit = %Endpoint
@onready var project_edit: LineEdit = %Project
@onready var token_edit: LineEdit = %Token
@onready var status_label: Label = %Status
@onready var emit_button: Button = %EmitEvents
@onready var report_button: Button = %SubmitReport
@onready var flush_button: Button = %Flush

var _telemetry


func _ready() -> void:
	_telemetry = _get_or_create_telemetry()
	endpoint_edit.text = _telemetry.endpoint_url
	project_edit.text = _telemetry.project_id
	token_edit.text = _telemetry.ingest_token

	emit_button.pressed.connect(_on_emit_events_pressed)
	report_button.pressed.connect(_on_submit_report_pressed)
	flush_button.pressed.connect(_on_flush_pressed)
	_telemetry.flush_completed.connect(_on_flush_completed)
	_telemetry.send_failed.connect(_on_send_failed)
	_set_status("Ready. Start flightrecorder locally, paste an ingest token, then emit events.")


func _get_or_create_telemetry():
	var autoload := get_node_or_null("/root/FlightRecorderTelemetry")
	if autoload != null:
		return autoload

	var client = TELEMETRY_SCRIPT.new()
	client.name = "FlightRecorderTelemetry"
	add_child(client)
	return client


func _configure_client() -> bool:
	if token_edit.text.strip_edges() == "":
		_set_status("Paste an ingest token from the flightrecorder Settings tab first.")
		return false

	_telemetry.configure(
		{
			"endpoint_url": endpoint_edit.text.strip_edges(),
			"project_id": project_edit.text.strip_edges(),
			"ingest_token": token_edit.text.strip_edges(),
			"game_version": "godot-demo",
			"build_channel": "local",
			"opt_in_enabled": true,
		},
	)
	return true


func _on_emit_events_pressed() -> void:
	if not _configure_client():
		return

	var base_context := {
		"game_time": Time.get_ticks_msec() / 1000,
		"system_id": "lave",
		"zone_id": "lave_primary",
		"coordinates": [1240.5, -80.0, 330.2],
		"credits": 48200,
		"hull_pct": 0.94,
		"shield_pct": 1.0,
		"ship_id": "demo_ship",
	}
	_telemetry.record_event("game_continue", { "source": "godot_e2e_demo" }, base_context)
	_telemetry.record_event("dock", { "station_id": "demo_station" }, base_context)
	_telemetry.record_event(
		"buy_commodity",
		{
			"commodity": "food",
			"quantity": 4,
			"unit_price": 12,
		},
		base_context,
	)
	_telemetry.record_event("take_mission", { "mission_id": "demo_mission_1" }, base_context)
	_telemetry.record_event(
		"player_death",
		{ "cause": "demo_button" },
		{
			"game_time": Time.get_ticks_msec() / 1000,
			"system_id": "reorte",
			"zone_id": "reorte_open",
			"coordinates": [4200.1, 0.0, -810.3],
			"credits": 48000,
			"hull_pct": 0.0,
			"shield_pct": 0.0,
			"ship_id": "demo_ship",
		},
	)
	_telemetry.flush()
	_set_status("Queued sample events. Check Event Explorer after the sender drains.")


func _on_submit_report_pressed() -> void:
	if not _configure_client():
		return

	var screenshot: String = _telemetry.capture_viewport_png_base64()
	var context := {
		"game_time": Time.get_ticks_msec() / 1000,
		"system_id": "reorte",
		"zone_id": "reorte_open",
		"coordinates": [4200.1, 0.0, -810.3],
		"credits": 48200,
		"hull_pct": 0.72,
		"shield_pct": 0.0,
		"ship_id": "demo_ship",
	}
	var accepted: bool = _telemetry.submit_bug_report(
		2,
		"frustrated",
		"Godot local E2E demo report.",
		screenshot,
		context,
		{
			"active_missions": ["demo_mission_1"],
		},
	)
	if accepted:
		_set_status("Queued demo bug report. Check the Bug Reports inbox.")


func _on_flush_pressed() -> void:
	if not _configure_client():
		return
	_telemetry.flush()
	_set_status("Flush requested.")


func _on_flush_completed(sent_records: int, remaining_records: int) -> void:
	_set_status("Flush complete. Sent %s record(s), %s remaining." % [sent_records, remaining_records])


func _on_send_failed(message: String) -> void:
	_set_status("Send failed: %s" % message)


func _set_status(message: String) -> void:
	status_label.text = message
