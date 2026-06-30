## Reusable Godot client for the flightrecorder ingestion API.
##
## Add this script as an Autoload singleton, configure the endpoint, project ID,
## and ingest token, then call record_event() or submit_bug_report() from game
## code. Events are persisted to a small NDJSON write-ahead log before sending so
## they can drain on the next run after a crash or network outage.
class_name FlightRecorderTelemetryClient
extends Node

signal flush_completed(sent_records: int, remaining_records: int)
signal send_failed(message: String)

const SCHEMA_VERSION := 2
const DEFAULT_ENDPOINT_URL := "http://localhost:8080/"
const EVENTS_PATH := "/v1/events"
const BUG_REPORTS_PATH := "/v1/bug-reports"
const PROJECTS_DIR := "user://flightrecorder/projects"

@export var endpoint_url := DEFAULT_ENDPOINT_URL
@export var project_id := "sursidus"
@export var ingest_token := ""
@export var game_version := "dev"
@export var build_channel := "local"
@export var commit_sha := "dev"
@export var platform := OS.get_name().to_lower()
@export var opt_in_enabled := true
@export var batch_size := 25
@export var use_gzip := false
@export var report_cooldown_seconds := 60.0

var player_id := ""

var _wal_mutex := Mutex.new()
var _state_mutex := Mutex.new()
var _wake_sender := Semaphore.new()
var _sender_thread := Thread.new()
var _stop_sender := false
var _last_report_msec := -1


func _ready() -> void:
	_ensure_project_dir()
	player_id = _load_or_create_player_id()
	_start_sender()


func _exit_tree() -> void:
	_stop_sender_thread()


## Configures the client from a dictionary, useful for demo scenes, project
## settings, or game-specific settings menus.
func configure(options: Dictionary) -> void:
	var previous_project_id := project_id
	endpoint_url = str(options.get("endpoint_url", endpoint_url))
	project_id = str(options.get("project_id", project_id))
	ingest_token = str(options.get("ingest_token", ingest_token))
	game_version = str(options.get("game_version", game_version))
	build_channel = str(options.get("build_channel", build_channel))
	commit_sha = str(options.get("commit_sha", commit_sha))
	platform = str(options.get("platform", platform))
	opt_in_enabled = bool(options.get("opt_in_enabled", opt_in_enabled))
	batch_size = max(1, int(options.get("batch_size", batch_size)))
	use_gzip = bool(options.get("use_gzip", use_gzip))
	if project_id != previous_project_id or player_id == "":
		player_id = _load_or_create_player_id()


## Stores the player's telemetry preference. When disabled, new events and
## reports are ignored, but already queued data is left in the WAL.
func set_opt_in_enabled(enabled: bool) -> void:
	opt_in_enabled = enabled


## Queues a contract-shaped event and wakes the sender thread.
func record_event(event_type: String, payload: Dictionary = { }, context: Dictionary = { }) -> bool:
	if not opt_in_enabled:
		return false
	if ingest_token.strip_edges() == "":
		_report_send_failed("missing ingest token")
		return false

	var event := build_event(event_type, payload, context)
	_append_wal(
		{
			"kind": "event",
			"event": event,
		},
	)
	_wake_sender.post()
	return true


## Queues a bug/sentiment report. The screenshot should be PNG bytes encoded
## with Marshalls.raw_to_base64(), or an empty string if no screenshot is wanted.
func submit_bug_report(
		mood: int,
		mood_label: String,
		notes: String,
		screenshot_png_base64: String = "",
		context: Dictionary = { },
		extra_payload: Dictionary = { },
) -> bool:
	if not opt_in_enabled:
		return false
	if ingest_token.strip_edges() == "":
		_report_send_failed("missing ingest token")
		return false
	if not can_submit_report():
		_report_send_failed("report cooldown active")
		return false

	var payload := extra_payload.duplicate(true)
	payload["mood"] = clampi(mood, 1, 5)
	payload["mood_label"] = mood_label
	payload["notes"] = notes
	if screenshot_png_base64 != "":
		payload["screenshot_png_base64"] = screenshot_png_base64

	var event := build_event("bug_report", payload, context)
	var body := {
		"project_id": project_id,
		"report_id": _uuid_v4(),
		"client": _client_payload(),
		"event": event,
	}

	_last_report_msec = Time.get_ticks_msec()
	_append_wal(
		{
			"kind": "bug_report",
			"body": body,
		},
	)
	_wake_sender.post()
	return true


## Returns true when a player-initiated report may be sent.
func can_submit_report() -> bool:
	if _last_report_msec < 0:
		return true
	var elapsed := float(Time.get_ticks_msec() - _last_report_msec) / 1000.0
	return elapsed >= report_cooldown_seconds


## Wakes the sender thread. This is useful after starting the game, changing
## network state, or creating an ingest token during local development.
func flush() -> void:
	_wake_sender.post()


## Builds an event envelope without writing it to disk. Game integrations can
## use this for tests or to inspect the outgoing contract.
func build_event(event_type: String, payload: Dictionary = { }, context: Dictionary = { }) -> Dictionary:
	var event_context := _event_context_from_context(context)
	var metrics := _metrics_from_context(context)
	var dimensions := _dimensions_from_context(context)
	return {
		"schema_version": SCHEMA_VERSION,
		"player_id": str(context.get("player_id", player_id)),
		"event_type": event_type,
		"real_ts": _utc_now(),
		"game_time": int(context.get("game_time", 0)),
		"context": event_context,
		"metrics": metrics,
		"dimensions": dimensions,
		"payload": payload,
	}


## Captures the current viewport as PNG base64. Call this on the main thread.
func capture_viewport_png_base64() -> String:
	var viewport := get_viewport()
	if viewport == null:
		return ""
	var image := viewport.get_texture().get_image()
	if image == null:
		return ""
	var png_bytes := image.save_png_to_buffer()
	return Marshalls.raw_to_base64(png_bytes)


func _start_sender() -> void:
	_stop_sender = false
	if not _sender_thread.is_started():
		_sender_thread.start(Callable(self, "_sender_loop"))
	_wake_sender.post()


func _stop_sender_thread() -> void:
	_state_mutex.lock()
	_stop_sender = true
	_state_mutex.unlock()
	_wake_sender.post()
	if _sender_thread.is_started():
		_sender_thread.wait_to_finish()


func _sender_loop() -> void:
	while true:
		_wake_sender.wait()
		if _should_stop_sender():
			return
		_drain_wal()


func _should_stop_sender() -> bool:
	_state_mutex.lock()
	var should_stop := _stop_sender
	_state_mutex.unlock()
	return should_stop


func _drain_wal() -> void:
	if ingest_token.strip_edges() == "":
		return

	var records := _read_wal()
	if records.is_empty():
		return

	var remaining: Array[Dictionary] = []
	var pending_event_records: Array[Dictionary] = []
	var pending_events: Array[Dictionary] = []
	var sent_count := 0
	var failed := false

	for index in range(records.size()):
		var record := records[index]
		var kind := str(record.get("kind", ""))

		if kind == "event":
			pending_event_records.append(record)
			pending_events.append(record.get("event", { }))
			if pending_events.size() >= batch_size:
				if _send_event_batch(pending_events):
					sent_count += pending_event_records.size()
					pending_event_records.clear()
					pending_events.clear()
				else:
					remaining.append_array(pending_event_records)
					remaining.append_array(records.slice(index + 1))
					failed = true
					break
		elif kind == "bug_report":
			if not pending_events.is_empty():
				if _send_event_batch(pending_events):
					sent_count += pending_event_records.size()
					pending_event_records.clear()
					pending_events.clear()
				else:
					remaining.append_array(pending_event_records)
					remaining.append_array(records.slice(index))
					failed = true
					break

			if _post_json(BUG_REPORTS_PATH, _bug_report_body_for_transport(record.get("body", { }))):
				sent_count += 1
			else:
				remaining.append(record)
				remaining.append_array(records.slice(index + 1))
				failed = true
				break
		else:
			sent_count += 1

	if not failed and not pending_events.is_empty():
		if _send_event_batch(pending_events):
			sent_count += pending_event_records.size()
		else:
			remaining.append_array(pending_event_records)

	_rewrite_wal(remaining)
	call_deferred("_emit_flush_completed", sent_count, remaining.size())


func _send_event_batch(events: Array[Dictionary]) -> bool:
	if events.is_empty():
		return true
	var normalized_events: Array[Dictionary] = []
	for event in events:
		normalized_events.append(_event_for_transport(event))
	var body := {
		"project_id": project_id,
		"batch_id": _uuid_v4(),
		"sent_at": _utc_now(),
		"client": _client_payload(),
		"events": normalized_events,
	}
	return _post_json(EVENTS_PATH, body)


func _bug_report_body_for_transport(body: Dictionary) -> Dictionary:
	var output := body.duplicate(true)
	if output.get("event") is Dictionary:
		output["event"] = _event_for_transport(output["event"])
		var event: Dictionary = output["event"]
		if event.get("payload") is Dictionary:
			var payload: Dictionary = event["payload"].duplicate(true)
			if payload.has("mood"):
				payload["mood"] = int(payload.get("mood", 0))
			event["payload"] = payload
	return output


func _event_for_transport(event: Dictionary) -> Dictionary:
	var output := event.duplicate(true)
	output["schema_version"] = int(output.get("schema_version", SCHEMA_VERSION))
	output["game_time"] = int(output.get("game_time", 0))
	return output


func _post_json(route_path: String, body: Dictionary) -> bool:
	var url := _join_url(endpoint_url, route_path)
	var parsed := _parse_url(url)
	if parsed.is_empty():
		_report_send_failed("invalid endpoint: %s" % url)
		return false

	var client := HTTPClient.new()
	var tls_options: TLSOptions = null
	if parsed["scheme"] == "https":
		tls_options = TLSOptions.client()

	var err := client.connect_to_host(parsed["host"], parsed["port"], tls_options)
	if err != OK:
		_report_send_failed("connect failed: %s" % error_string(err))
		return false

	while client.get_status() in [HTTPClient.STATUS_RESOLVING, HTTPClient.STATUS_CONNECTING]:
		client.poll()
		OS.delay_msec(20)

	if client.get_status() != HTTPClient.STATUS_CONNECTED:
		_report_send_failed("connect failed with status %s" % client.get_status())
		return false

	var json := JSON.stringify(body)
	var body_bytes := json.to_utf8_buffer()
	var headers := PackedStringArray(
		[
			"Authorization: Bearer %s" % ingest_token,
			"Content-Type: application/json",
			"Accept: application/json",
		],
	)
	if use_gzip:
		body_bytes = body_bytes.compress(FileAccess.COMPRESSION_GZIP)
		headers.append("Content-Encoding: gzip")

	err = client.request_raw(HTTPClient.METHOD_POST, parsed["request_path"], headers, body_bytes)
	if err != OK:
		_report_send_failed("request failed: %s" % error_string(err))
		return false

	while client.get_status() == HTTPClient.STATUS_REQUESTING:
		client.poll()
		OS.delay_msec(20)

	while client.get_status() == HTTPClient.STATUS_BODY:
		client.poll()
		client.read_response_body_chunk()
		OS.delay_msec(10)

	var response_code := client.get_response_code()
	if response_code < 200 or response_code >= 300:
		_report_send_failed("collector returned HTTP %s" % response_code)
		return false
	return true


func _report_send_failed(message: String) -> void:
	call_deferred("_emit_send_failed", message)


func _emit_send_failed(message: String) -> void:
	send_failed.emit(message)


func _emit_flush_completed(sent_records: int, remaining_records: int) -> void:
	flush_completed.emit(sent_records, remaining_records)


func _append_wal(record: Dictionary) -> void:
	_wal_mutex.lock()
	_ensure_project_dir()
	var wal_path := _wal_path()
	var file := FileAccess.open(wal_path, FileAccess.READ_WRITE)
	if file == null:
		file = FileAccess.open(wal_path, FileAccess.WRITE)
	else:
		file.seek_end()
	if file != null:
		file.store_line(JSON.stringify(record))
		file.flush()
	_wal_mutex.unlock()


func _read_wal() -> Array[Dictionary]:
	var records: Array[Dictionary] = []
	_wal_mutex.lock()
	var wal_path := _wal_path()
	if not FileAccess.file_exists(wal_path):
		_wal_mutex.unlock()
		return records
	var file := FileAccess.open(wal_path, FileAccess.READ)
	if file == null:
		_wal_mutex.unlock()
		return records
	while not file.eof_reached():
		var line := file.get_line().strip_edges()
		if line == "":
			continue
		var parsed = JSON.parse_string(line)
		if typeof(parsed) == TYPE_DICTIONARY:
			records.append(parsed)
	_wal_mutex.unlock()
	return records


func _rewrite_wal(records: Array[Dictionary]) -> void:
	_wal_mutex.lock()
	_ensure_project_dir()
	var file := FileAccess.open(_wal_path(), FileAccess.WRITE)
	if file != null:
		for record in records:
			file.store_line(JSON.stringify(record))
		file.flush()
	_wal_mutex.unlock()


func _ensure_project_dir() -> void:
	DirAccess.make_dir_recursive_absolute(ProjectSettings.globalize_path(_project_dir()))


func _project_dir() -> String:
	return "%s/%s" % [PROJECTS_DIR, _safe_project_id()]


func _wal_path() -> String:
	return "%s/wal.ndjson" % _project_dir()


func _player_id_path() -> String:
	return "%s/player_id.txt" % _project_dir()


func _safe_project_id() -> String:
	var safe := project_id.strip_edges().to_lower()
	var output := ""
	for index in range(safe.length()):
		var character := safe.substr(index, 1)
		if character.is_valid_identifier() or character.is_valid_int() or character in ["-", "_"]:
			output += character
		else:
			output += "_"
	if output == "":
		output = "default"
	return output


func _load_or_create_player_id() -> String:
	_ensure_project_dir()
	var player_id_path := _player_id_path()
	if FileAccess.file_exists(player_id_path):
		var existing := FileAccess.get_file_as_string(player_id_path).strip_edges()
		if existing != "":
			return existing

	var new_id := _uuid_v4()
	var file := FileAccess.open(player_id_path, FileAccess.WRITE)
	if file != null:
		file.store_string(new_id)
		file.flush()
	return new_id


func _client_payload() -> Dictionary:
	return {
		"game_version": game_version,
		"build_channel": build_channel,
		"commit_sha": commit_sha,
		"platform": platform,
	}


func _event_context_from_context(context: Dictionary) -> Dictionary:
	if context.has("context") and context["context"] is Dictionary:
		return context["context"].duplicate(true)

	var event_context := context.duplicate(true)
	event_context.erase("player_id")
	event_context.erase("game_time")
	event_context.erase("metrics")
	event_context.erase("dimensions")
	event_context.erase("credits")
	event_context.erase("hull_pct")
	event_context.erase("shield_pct")
	event_context.erase("ship_id")
	event_context["location"] = {
		"region_id": str(context.get("region_id", "unknown")),
		"zone_id": str(context.get("zone_id", "unknown")),
		"position": _coordinates_from_context(context),
	}
	event_context.erase("region_id")
	event_context.erase("zone_id")
	event_context.erase("coordinates")
	return event_context


func _metrics_from_context(context: Dictionary) -> Dictionary:
	var metrics := { }
	if context.has("metrics") and context["metrics"] is Dictionary:
		metrics = context["metrics"].duplicate(true)
	if context.has("credits"):
		metrics["economy.credits"] = int(context["credits"])
	if context.has("hull_pct"):
		metrics["ship.hull_pct"] = clampf(float(context["hull_pct"]), 0.0, 1.0)
	if context.has("shield_pct"):
		metrics["ship.shield_pct"] = clampf(float(context["shield_pct"]), 0.0, 1.0)
	return metrics


func _dimensions_from_context(context: Dictionary) -> Dictionary:
	var dimensions := { }
	if context.has("dimensions") and context["dimensions"] is Dictionary:
		dimensions = context["dimensions"].duplicate(true)
	if context.has("ship_id"):
		dimensions["ship.id"] = str(context["ship_id"])
	return dimensions


func _coordinates_from_context(context: Dictionary) -> Array[float]:
	var coords = context.get("coordinates", [0.0, 0.0, 0.0])
	if coords is Vector3:
		return [coords.x, coords.y, coords.z]
	if coords is Array and coords.size() >= 3:
		return [float(coords[0]), float(coords[1]), float(coords[2])]
	return [0.0, 0.0, 0.0]


func _utc_now() -> String:
	return "%sZ" % Time.get_datetime_string_from_system(true)


func _join_url(base_url: String, route_path: String) -> String:
	var base := base_url.strip_edges()
	while base.ends_with("/"):
		base = base.substr(0, base.length() - 1)
	if not route_path.begins_with("/"):
		route_path = "/%s" % route_path
	return "%s%s" % [base, route_path]


func _parse_url(url: String) -> Dictionary:
	var scheme_end := url.find("://")
	if scheme_end < 0:
		return { }
	var scheme := url.substr(0, scheme_end).to_lower()
	var rest := url.substr(scheme_end + 3)
	var path_start := rest.find("/")
	var host_port := rest
	var request_path := "/"
	if path_start >= 0:
		host_port = rest.substr(0, path_start)
		request_path = rest.substr(path_start)
	if host_port == "":
		return { }

	var host := host_port
	var port := 80
	if scheme == "https":
		port = 443
	var port_start := host_port.rfind(":")
	if port_start > 0:
		host = host_port.substr(0, port_start)
		port = int(host_port.substr(port_start + 1))
	if host == "":
		return { }

	return {
		"scheme": scheme,
		"host": host,
		"port": port,
		"request_path": request_path,
	}


func _uuid_v4() -> String:
	var crypto := Crypto.new()
	var bytes := crypto.generate_random_bytes(16)
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	var hex := bytes.hex_encode()
	return "%s-%s-%s-%s-%s" % [
		hex.substr(0, 8),
		hex.substr(8, 4),
		hex.substr(12, 4),
		hex.substr(16, 4),
		hex.substr(20, 12),
	]
