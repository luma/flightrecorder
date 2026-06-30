extends GutTest

const TELEMETRY_SCRIPT := preload("res://addons/flightrecorder/flightrecorder_telemetry.gd")

var _client


func before_each() -> void:
	_client = autofree(TELEMETRY_SCRIPT.new())
	_client.player_id = "550e8400-e29b-41d4-a716-446655440000"
	_client.project_id = "gut_test_%s" % Time.get_ticks_usec()


func after_each() -> void:
	_cleanup_client_project_dir()


func test_build_event_emits_schema_v2_layers_from_compat_context() -> void:
	var event: Dictionary = _client.build_event(
		"dock",
		{
			"station_id": "demo_station",
		},
		{
			"game_time": 1843200,
			"system_id": "lave",
			"zone_id": "lave_primary",
			"coordinates": [1240.5, -80.0, 330.2],
			"credits": 48200,
			"hull_pct": 0.94,
			"shield_pct": 1.0,
			"ship_id": "cobra_mk3",
		},
	)

	assert_eq(event["schema_version"], 2)
	assert_eq(event["player_id"], "550e8400-e29b-41d4-a716-446655440000")
	assert_eq(event["event_type"], "dock")
	assert_eq(event["game_time"], 1843200)
	assert_eq(event["context"]["location"]["world_id"], "lave")
	assert_eq(event["context"]["location"]["area_id"], "lave_primary")
	assert_eq(event["context"]["location"]["position"], [1240.5, -80.0, 330.2])
	assert_eq(event["metrics"]["economy.credits"], 48200)
	assert_eq(event["metrics"]["ship.hull_pct"], 0.94)
	assert_eq(event["metrics"]["ship.shield_pct"], 1.0)
	assert_eq(event["dimensions"]["ship.id"], "cobra_mk3")
	assert_eq(event["payload"]["station_id"], "demo_station")
	assert_false(event.has("hull_pct"))
	assert_false(event.has("shield_pct"))
	assert_false(event.has("credits"))


func test_build_event_preserves_explicit_generic_context_metrics_and_dimensions() -> void:
	var event: Dictionary = _client.build_event(
		"wave_complete",
		{
			"wave_result": "clean",
		},
		{
			"player_id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			"game_time": 99,
			"context": {
				"location": {
					"world_id": "arena_1",
					"area_id": "room_a",
					"position": [1.0, 2.0, 3.0],
				},
				"difficulty": "hard",
			},
			"metrics": {
				"character.level": 12,
				"wave.time_ms": 45200,
			},
			"dimensions": {
				"class.id": "pilot",
			},
		},
	)

	assert_eq(event["player_id"], "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	assert_eq(event["context"]["location"]["world_id"], "arena_1")
	assert_eq(event["context"]["difficulty"], "hard")
	assert_eq(event["metrics"]["character.level"], 12)
	assert_eq(event["metrics"]["wave.time_ms"], 45200)
	assert_eq(event["dimensions"]["class.id"], "pilot")


func test_build_event_clamps_sursidus_percent_metrics() -> void:
	var event: Dictionary = _client.build_event(
		"player_death",
		{ },
		{
			"hull_pct": -2.0,
			"shield_pct": 4.0,
		},
	)

	assert_eq(event["metrics"]["ship.hull_pct"], 0.0)
	assert_eq(event["metrics"]["ship.shield_pct"], 1.0)


func test_event_for_transport_restores_contract_integer_fields_after_json_parse() -> void:
	var event: Dictionary = _client.build_event(
		"dock",
		{ },
		{
			"game_time": 1843200,
		},
	)
	var parsed: Dictionary = JSON.parse_string(JSON.stringify(event))
	var transport_event: Dictionary = _client._event_for_transport(parsed)

	assert_eq(transport_event["schema_version"], 2)
	assert_eq(transport_event["game_time"], 1843200)
	assert_typeof(transport_event["schema_version"], TYPE_INT)
	assert_typeof(transport_event["game_time"], TYPE_INT)


func test_bug_report_body_for_transport_restores_nested_event_integer_fields_after_json_parse() -> void:
	var body := {
		"project_id": "gut_test",
		"report_id": "report-1",
		"client": _client._client_payload(),
		"event": _client.build_event(
			"bug_report",
			{
				"mood": 2,
				"mood_label": "frustrated",
				"notes": "hello",
			},
			{
				"game_time": 99,
			},
		),
	}
	var parsed: Dictionary = JSON.parse_string(JSON.stringify(body))
	var transport_body: Dictionary = _client._bug_report_body_for_transport(parsed)

	assert_eq(transport_body["event"]["schema_version"], 2)
	assert_eq(transport_body["event"]["game_time"], 99)
	assert_eq(transport_body["event"]["payload"]["mood"], 2)
	assert_typeof(transport_body["event"]["schema_version"], TYPE_INT)
	assert_typeof(transport_body["event"]["game_time"], TYPE_INT)
	assert_typeof(transport_body["event"]["payload"]["mood"], TYPE_INT)


func test_configure_sanitizes_project_specific_paths() -> void:
	_client.project_id = "Sursidus Demo / QA"

	assert_eq(_client._safe_project_id(), "sursidus_demo___qa")
	assert_eq(
		_client._wal_path(),
		"user://flightrecorder/projects/sursidus_demo___qa/wal.ndjson",
	)
	assert_eq(
		_client._player_id_path(),
		"user://flightrecorder/projects/sursidus_demo___qa/player_id.txt",
	)


func test_parse_url_supports_localhost_with_port_and_path() -> void:
	var parsed: Dictionary = _client._parse_url("http://localhost:3000/v1/events")

	assert_eq(parsed["scheme"], "http")
	assert_eq(parsed["host"], "localhost")
	assert_eq(parsed["port"], 3000)
	assert_eq(parsed["request_path"], "/v1/events")


func test_join_url_normalizes_slashes() -> void:
	assert_eq(_client._join_url("http://localhost:3000/", "/v1/events"), "http://localhost:3000/v1/events")
	assert_eq(_client._join_url("http://localhost:3000", "v1/events"), "http://localhost:3000/v1/events")


func test_report_cooldown_defaults_to_available() -> void:
	assert_true(_client.can_submit_report())

	_client._last_report_msec = Time.get_ticks_msec()

	assert_false(_client.can_submit_report())


func test_record_event_writes_event_to_project_wal() -> void:
	_client.ingest_token = "test-token"

	var accepted: bool = _client.record_event(
		"dock",
		{
			"station_id": "demo_station",
		},
		{
			"system_id": "lave",
			"zone_id": "lave_primary",
			"coordinates": [1.0, 2.0, 3.0],
		},
	)

	var records: Array[Dictionary] = _client._read_wal()
	assert_true(accepted)
	assert_eq(records.size(), 1)
	assert_eq(records[0]["kind"], "event")
	assert_eq(records[0]["event"]["event_type"], "dock")
	assert_eq(records[0]["event"]["context"]["location"]["world_id"], "lave")


func test_submit_bug_report_writes_report_body_to_project_wal() -> void:
	_client.ingest_token = "test-token"

	var accepted: bool = _client.submit_bug_report(
		2,
		"frustrated",
		"Report note",
		"",
		{
			"system_id": "reorte",
			"zone_id": "reorte_open",
			"coordinates": [4.0, 5.0, 6.0],
		},
		{
			"active_missions": ["demo_mission_1"],
		},
	)

	var records: Array[Dictionary] = _client._read_wal()
	assert_true(accepted)
	assert_eq(records.size(), 1)
	assert_eq(records[0]["kind"], "bug_report")
	assert_eq(records[0]["body"]["project_id"], _client.project_id)
	assert_eq(records[0]["body"]["client"]["game_version"], _client.game_version)
	assert_eq(records[0]["body"]["client"]["commit_sha"], _client.commit_sha)
	assert_eq(records[0]["body"]["event"]["event_type"], "bug_report")
	assert_eq(records[0]["body"]["event"]["payload"]["mood"], 2.0)
	assert_eq(records[0]["body"]["event"]["payload"]["notes"], "Report note")
	assert_eq(records[0]["body"]["event"]["payload"]["active_missions"], ["demo_mission_1"])


func _cleanup_client_project_dir() -> void:
	if _client == null:
		return
	if not _client._safe_project_id().begins_with("gut_test_"):
		return
	var project_dir := ProjectSettings.globalize_path(_client._project_dir())
	var dir := DirAccess.open(project_dir)
	if dir == null:
		return
	dir.list_dir_begin()
	var file_name := dir.get_next()
	while file_name != "":
		if not dir.current_is_dir():
			DirAccess.remove_absolute("%s/%s" % [project_dir, file_name])
		file_name = dir.get_next()
	dir.list_dir_end()
	DirAccess.remove_absolute(project_dir)
