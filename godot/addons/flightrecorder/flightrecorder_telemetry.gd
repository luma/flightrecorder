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
## Emitted after a drain moves one or more records to the quarantine file. A
## non-zero count usually indicates a game-side code bug (wrong event shape) or a
## payload that is permanently too large; surface it as a data-quality metric.
signal records_quarantined(count: int)

const SCHEMA_VERSION := 2
const DEFAULT_ENDPOINT_URL := "http://localhost:8080/"
const EVENTS_PATH := "/v1/events"
const BUG_REPORTS_PATH := "/v1/bug-reports"
const PROJECTS_DIR := "user://flightrecorder/projects"

## Outcome of a single HTTP send, so the drain can tell a permanent record
## rejection apart from a transient outage, a config/routing error, and an
## oversize payload. Collapsing these into a bool is what let one bad batch
## freeze the whole queue (see docs/plans/wal-poisoning-self-healing.md).
enum SendResult { SUCCESS, TRANSIENT, PERMANENT, AUTH, OVERSIZE }

## Terminal state of a full drain pass, used by the sender loop to decide whether
## to back off and retry (TRANSIENT) or return to a blocking wait (everything
## else — only a new event, flush(), or restart can change those outcomes).
enum DrainOutcome { EMPTY, COMPLETED, TRANSIENT, AUTH }

const TRANSIENT_BACKOFF_BASE_MSEC := 1000
const TRANSIENT_BACKOFF_MAX_MSEC := 60000

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
## A record failing transiently (outage) for longer than this is quarantined as
## "stuck" so a permanent local problem cannot pin the queue forever. Default 7
## days: long enough that a normal outage never trips it.
@export var max_record_age_seconds := 604800
## Optional attempt-count backstop for transient failures (0 = unlimited; age is
## the primary bound). Only transient attempts are counted.
@export var max_transient_attempts := 0
## Quarantine file caps; the oldest lines are FIFO-trimmed when either is hit.
@export var max_quarantine_records := 1000
@export var max_quarantine_bytes := 5_242_880

var player_id := ""

var _wal_mutex := Mutex.new()
var _state_mutex := Mutex.new()
var _wake_sender := Semaphore.new()
var _sender_thread := Thread.new()
var _stop_sender := false
var _last_report_msec := -1
## Session-sticky, in-memory batch ceiling discovered from 413 responses. 0 means
## "unshrunk" — use the configured batch_size. Never persisted: a restart
## re-probes with one cheap 413, which self-corrects if the server raises its
## limit.
var _effective_batch_size := 0
## Consecutive TRANSIENT drain count, drives exponential backoff. Reset on any
## non-transient drain outcome.
var _transient_streak := 0
## Count of non-empty WAL lines the last `_read_wal` consumed. The drain releases
## `_wal_mutex` during its blocking HTTP sends, so the game thread can append new
## records meanwhile; the post-drain rewrite uses this to preserve those appends
## instead of truncating them away. Only the sender thread reads/rewrites, so a
## plain member is safe.
var _drain_line_count := 0


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
	max_record_age_seconds = int(options.get("max_record_age_seconds", max_record_age_seconds))
	max_transient_attempts = int(options.get("max_transient_attempts", max_transient_attempts))
	max_quarantine_records = int(options.get("max_quarantine_records", max_quarantine_records))
	max_quarantine_bytes = int(options.get("max_quarantine_bytes", max_quarantine_bytes))
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
			# event_id is the idempotency key: minted once here and persisted, so
			# a resend after a lost response never creates a duplicate on the
			# server (which dedups on (project_id, event_id)). attempts /
			# first_attempt_ts back the transient age/attempt backstop.
			"event_id": _uuid_v4(),
			"attempts": 0,
			"first_attempt_ts": null,
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
			# report_id (inside body) is the server-side idempotency key for the
			# report; event_id gives the underlying event the same guarantee.
			"event_id": _uuid_v4(),
			"attempts": 0,
			"first_attempt_ts": null,
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
		_run_drain_cycle()


func _should_stop_sender() -> bool:
	_state_mutex.lock()
	var should_stop := _stop_sender
	_state_mutex.unlock()
	return should_stop


## Drives repeated drains with transient backoff. A TRANSIENT drain does not
## return to the blocking wait (Semaphore has no timed wait, so an idle outage
## would never retry until a new event arrived); it sleeps a bounded, jittered
## interval and re-drains. Every other outcome returns to wait() — only a new
## event, flush(), or restart can change them.
func _run_drain_cycle() -> void:
	while true:
		if _should_stop_sender():
			return
		var outcome := _drain_wal()
		if outcome == DrainOutcome.TRANSIENT:
			_transient_streak += 1
			if not _sleep_backoff(_transient_streak):
				return
			continue
		_transient_streak = 0
		return


## Sleeps the backoff interval for the given streak in short, stop-checked steps.
## Returns false if a stop was requested mid-sleep.
func _sleep_backoff(streak: int) -> bool:
	var shift := mini(streak - 1, 6)
	var base := mini(TRANSIENT_BACKOFF_BASE_MSEC << shift, TRANSIENT_BACKOFF_MAX_MSEC)
	var jitter := float(base) * 0.2
	var delay_msec := int(maxf(0.0, float(base) + randf_range(-jitter, jitter)))
	var waited := 0
	while waited < delay_msec:
		if _should_stop_sender():
			return false
		OS.delay_msec(50)
		waited += 50
	return true


## Drains the WAL once, in order, without head-of-line blocking:
## SUCCESS drops the batch; PERMANENT quarantines the offending records and keeps
## going; OVERSIZE shrinks the batch (quarantining a lone record that still 413s);
## TRANSIENT stops and keeps the remainder (aging out records stuck too long);
## AUTH stops and keeps everything (a config change is required).
func _drain_wal() -> int:
	if ingest_token.strip_edges() == "":
		return DrainOutcome.AUTH

	var records := _read_wal()
	if records.is_empty():
		return DrainOutcome.EMPTY

	var remaining: Array[Dictionary] = []
	var quarantined: Array[Dictionary] = []
	var sent_count := 0
	var outcome := DrainOutcome.COMPLETED
	var stopped := false
	var index := 0

	while index < records.size():
		var record := records[index]
		if stopped:
			remaining.append(record)
			index += 1
			continue

		var kind := str(record.get("kind", ""))
		if kind == "event":
			var effective := _current_effective_batch_size()
			var group: Array[Dictionary] = []
			var scan := index
			while scan < records.size() and str(records[scan].get("kind", "")) == "event" and group.size() < effective:
				group.append(records[scan])
				scan += 1

			var events: Array[Dictionary] = []
			for group_record in group:
				events.append(group_record.get("event", { }))
			var result := _send_event_batch(events, group)

			match result:
				SendResult.SUCCESS:
					sent_count += group.size()
					index = scan
				SendResult.PERMANENT:
					for group_record in group:
						quarantined.append(_quarantine_entry(group_record, "permanent"))
					index = scan
				SendResult.OVERSIZE:
					if group.size() <= 1:
						# Cannot shrink further: a single record that still 413s
						# can never fit. Quarantine it so it can't block the drain.
						if group.size() == 1:
							quarantined.append(_quarantine_entry(group[0], "oversize"))
						index += 1
					else:
						# Shrink and retry the same records with a smaller batch;
						# do not advance the index.
						_shrink_effective_batch_size()
				SendResult.TRANSIENT:
					for group_record in group:
						_bump_transient(group_record)
					for group_record in group:
						if _is_stuck(group_record):
							quarantined.append(_quarantine_entry(group_record, "stuck"))
						else:
							remaining.append(group_record)
					stopped = true
					outcome = DrainOutcome.TRANSIENT
					index = scan
				SendResult.AUTH:
					remaining.append_array(group)
					stopped = true
					outcome = DrainOutcome.AUTH
					index = scan
		elif kind == "bug_report":
			var body := _bug_report_body_for_transport(record.get("body", { }), str(record.get("event_id", "")))
			var result := _post_json(BUG_REPORTS_PATH, body)
			match result:
				SendResult.SUCCESS:
					sent_count += 1
					index += 1
				SendResult.PERMANENT:
					quarantined.append(_quarantine_entry(record, "permanent"))
					index += 1
				SendResult.OVERSIZE:
					# A single report that is too large can never be sent.
					quarantined.append(_quarantine_entry(record, "oversize"))
					index += 1
				SendResult.TRANSIENT:
					_bump_transient(record)
					if _is_stuck(record):
						quarantined.append(_quarantine_entry(record, "stuck"))
					else:
						remaining.append(record)
					stopped = true
					outcome = DrainOutcome.TRANSIENT
					index += 1
				SendResult.AUTH:
					remaining.append(record)
					stopped = true
					outcome = DrainOutcome.AUTH
					index += 1
		else:
			# Unknown record kind: drop it (as the original drain did) so a
			# malformed WAL line can't wedge the queue.
			sent_count += 1
			index += 1

	if not quarantined.is_empty():
		_append_quarantine(quarantined)
	_rewrite_wal(remaining, _drain_line_count)

	call_deferred("_emit_flush_completed", sent_count, remaining.size())
	if not quarantined.is_empty():
		call_deferred("_emit_records_quarantined", quarantined.size())

	if outcome == DrainOutcome.COMPLETED and remaining.is_empty():
		return DrainOutcome.EMPTY
	return outcome


func _send_event_batch(events: Array[Dictionary], records: Array[Dictionary] = []) -> int:
	if events.is_empty():
		return SendResult.SUCCESS
	var normalized_events: Array[Dictionary] = []
	for i in range(events.size()):
		var event_id := ""
		if i < records.size():
			event_id = str(records[i].get("event_id", ""))
		normalized_events.append(_event_for_transport(events[i], event_id))
	var body := {
		"project_id": project_id,
		"batch_id": _uuid_v4(),
		"sent_at": _utc_now(),
		"client": _client_payload(),
		"events": normalized_events,
	}
	return _post_json(EVENTS_PATH, body)


func _bug_report_body_for_transport(body: Dictionary, event_id: String = "") -> Dictionary:
	var output := body.duplicate(true)
	if output.get("event") is Dictionary:
		output["event"] = _event_for_transport(output["event"], event_id)
		var event: Dictionary = output["event"]
		if event.get("payload") is Dictionary:
			var payload: Dictionary = event["payload"].duplicate(true)
			if payload.has("mood"):
				payload["mood"] = int(payload.get("mood", 0))
			event["payload"] = payload
	return output


func _event_for_transport(event: Dictionary, event_id: String = "") -> Dictionary:
	var output := event.duplicate(true)
	output["schema_version"] = int(output.get("schema_version", SCHEMA_VERSION))
	output["game_time"] = int(output.get("game_time", 0))
	# event_id rides inside the event envelope. The server's custom envelope
	# unmarshaller tolerates it even on older, strict builds (it falls back to
	# batch-level dedup), so sending it is always safe.
	if event_id != "":
		output["event_id"] = event_id
	return output


func _post_json(route_path: String, body: Dictionary) -> int:
	var url := _join_url(endpoint_url, route_path)
	var parsed := _parse_url(url)
	if parsed.is_empty():
		# A malformed endpoint_url is a config error, not a record problem.
		_report_send_failed("invalid endpoint: %s" % url)
		return SendResult.AUTH

	var client := HTTPClient.new()
	var tls_options: TLSOptions = null
	if parsed["scheme"] == "https":
		tls_options = TLSOptions.client()

	var err := client.connect_to_host(parsed["host"], parsed["port"], tls_options)
	if err != OK:
		_report_send_failed("connect failed: %s" % error_string(err))
		return SendResult.TRANSIENT

	while client.get_status() in [HTTPClient.STATUS_RESOLVING, HTTPClient.STATUS_CONNECTING]:
		client.poll()
		OS.delay_msec(20)

	if client.get_status() != HTTPClient.STATUS_CONNECTED:
		_report_send_failed("connect failed with status %s" % client.get_status())
		return SendResult.TRANSIENT

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
		return SendResult.TRANSIENT

	while client.get_status() == HTTPClient.STATUS_REQUESTING:
		client.poll()
		OS.delay_msec(20)

	var response_body := PackedByteArray()
	while client.get_status() == HTTPClient.STATUS_BODY:
		client.poll()
		var chunk := client.read_response_body_chunk()
		if chunk.size() > 0:
			response_body.append_array(chunk)
		OS.delay_msec(10)

	return _classify_response(client.get_response_code(), response_body)


## Maps an HTTP response to a SendResult. A permanent record rejection is only
## inferred from a 400/422 that carries the collector's own {"error": …} body;
## any other 4xx (including 404/405 routing errors and proxy-generated 400s) is
## treated as a fixable config problem (AUTH), never as a reason to drop data.
func _classify_response(code: int, response_body: PackedByteArray) -> int:
	# A dropped/half-open connection yields no HTTP status (code 0). That is a
	# transient transport failure, not a config error — retry with backoff.
	if code < 100:
		_report_send_failed("collector connection dropped before response")
		return SendResult.TRANSIENT
	if code >= 200 and code < 300:
		return SendResult.SUCCESS
	if code == 413:
		return SendResult.OVERSIZE
	if code == 408 or code == 425 or code == 429 or code >= 500:
		_report_send_failed("collector transient error HTTP %s" % code)
		return SendResult.TRANSIENT
	if code == 401 or code == 403 or code == 404 or code == 405:
		_report_send_failed("collector rejected request (auth/endpoint) HTTP %s" % code)
		return SendResult.AUTH
	if code == 400 or code == 422:
		if _is_collector_error_body(response_body):
			return SendResult.PERMANENT
		_report_send_failed("collector returned HTTP %s (non-collector body)" % code)
		return SendResult.AUTH
	# Any other status: treat conservatively as config/routing, keep the data.
	_report_send_failed("collector returned HTTP %s" % code)
	return SendResult.AUTH


func _is_collector_error_body(response_body: PackedByteArray) -> bool:
	if response_body.is_empty():
		return false
	var text := response_body.get_string_from_utf8().strip_edges()
	# Cheap guard so a non-JSON proxy/CDN body (e.g. an HTML error page) never
	# reaches the parser. Use the JSON instance parser, which reports failure via
	# a return code instead of pushing an engine error to the log.
	if not text.begins_with("{"):
		return false
	var json := JSON.new()
	if json.parse(text) != OK:
		return false
	var data = json.get_data()
	return data is Dictionary and data.has("error")


func _report_send_failed(message: String) -> void:
	call_deferred("_emit_send_failed", message)


func _emit_send_failed(message: String) -> void:
	send_failed.emit(message)


func _emit_flush_completed(sent_records: int, remaining_records: int) -> void:
	flush_completed.emit(sent_records, remaining_records)


func _emit_records_quarantined(count: int) -> void:
	records_quarantined.emit(count)


## Returns the batch ceiling to use right now: the configured batch_size unless a
## 413 has shrunk it this session.
func _current_effective_batch_size() -> int:
	var configured := maxi(1, batch_size)
	if _effective_batch_size <= 0 or _effective_batch_size > configured:
		return configured
	return _effective_batch_size


## Halves the effective batch size (floor 1) after a 413 and reports it so the
## integrator can lower batch_size at the source.
func _shrink_effective_batch_size() -> void:
	var current := _current_effective_batch_size()
	_effective_batch_size = maxi(1, current / 2)
	_report_send_failed("batch too large; reduced batch size to %d" % _effective_batch_size)


## Records a transient failure on a WAL record: increments attempts and stamps
## first_attempt_ts once, so the age backstop measures time since the first real
## send attempt (not time on disk).
func _bump_transient(record: Dictionary) -> void:
	record["attempts"] = int(record.get("attempts", 0)) + 1
	if record.get("first_attempt_ts", null) == null:
		record["first_attempt_ts"] = int(_unix_now())


## True when a record has been failing transiently long enough (or, if enabled,
## often enough) that it should be given up on rather than block the queue.
func _is_stuck(record: Dictionary) -> bool:
	var first_attempt = record.get("first_attempt_ts", null)
	if first_attempt != null and max_record_age_seconds > 0:
		if _unix_now() - float(first_attempt) >= float(max_record_age_seconds):
			return true
	if max_transient_attempts > 0 and int(record.get("attempts", 0)) > max_transient_attempts:
		return true
	return false


## Wraps a record for the quarantine file: the verbatim record plus the reason it
## was given up on and its attempt count.
func _quarantine_entry(record: Dictionary, reason: String) -> Dictionary:
	var entry := record.duplicate(true)
	entry["quarantined_at"] = _utc_now()
	entry["reason"] = reason
	entry["attempts"] = int(record.get("attempts", 0))
	return entry


func _append_quarantine(entries: Array[Dictionary]) -> void:
	if entries.is_empty():
		return
	_wal_mutex.lock()
	_ensure_project_dir()
	var quarantine_path := _quarantine_path()
	var file := FileAccess.open(quarantine_path, FileAccess.READ_WRITE)
	if file == null:
		file = FileAccess.open(quarantine_path, FileAccess.WRITE)
	else:
		file.seek_end()
	if file != null:
		for entry in entries:
			file.store_line(JSON.stringify(entry))
		file.flush()
		file.close()
	_wal_mutex.unlock()
	_trim_quarantine()


## FIFO-trims the quarantine file to both the record and byte caps. The file
## lives on a player machine and a fully poisoned client can grow it fast.
func _trim_quarantine() -> void:
	_wal_mutex.lock()
	var quarantine_path := _quarantine_path()
	if not FileAccess.file_exists(quarantine_path):
		_wal_mutex.unlock()
		return
	var read_file := FileAccess.open(quarantine_path, FileAccess.READ)
	if read_file == null:
		_wal_mutex.unlock()
		return
	var lines: Array[String] = []
	var total_bytes := 0
	while not read_file.eof_reached():
		var line := read_file.get_line().strip_edges()
		if line == "":
			continue
		lines.append(line)
		total_bytes += _line_byte_size(line)
	read_file.close()

	var trimmed := false
	while lines.size() > max_quarantine_records and lines.size() > 0:
		var dropped: String = lines.pop_front()
		total_bytes -= _line_byte_size(dropped)
		trimmed = true
	while total_bytes > max_quarantine_bytes and lines.size() > 1:
		var dropped: String = lines.pop_front()
		total_bytes -= _line_byte_size(dropped)
		trimmed = true

	if trimmed:
		var write_file := FileAccess.open(quarantine_path, FileAccess.WRITE)
		if write_file != null:
			for line in lines:
				write_file.store_line(line)
			write_file.flush()
			write_file.close()
	_wal_mutex.unlock()


## UTF-8 byte size of a stored line (payloads may be non-ASCII), plus one for the
## trailing newline, so the byte cap is measured in real bytes not code points.
func _line_byte_size(line: String) -> int:
	return line.to_utf8_buffer().size() + 1


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
	var line_count := 0
	var wal_path := _wal_path()
	if not FileAccess.file_exists(wal_path):
		_drain_line_count = 0
		_wal_mutex.unlock()
		return records
	var file := FileAccess.open(wal_path, FileAccess.READ)
	if file == null:
		_drain_line_count = 0
		_wal_mutex.unlock()
		return records
	while not file.eof_reached():
		var line := file.get_line().strip_edges()
		if line == "":
			continue
		# Count every non-empty line, including ones that fail to parse (they are
		# dropped from records but still occupy a line), so the tail-preserving
		# rewrite lines up with the raw file.
		line_count += 1
		var parsed = JSON.parse_string(line)
		if typeof(parsed) == TYPE_DICTIONARY:
			records.append(_normalize_wal_record(parsed))
	_drain_line_count = line_count
	_wal_mutex.unlock()
	return records


## Defaults the retry-metadata fields so old WAL files (written before this
## format) load and drain without error. event_id is left empty for legacy
## records, which keeps them on at-least-once batch-level dedup.
func _normalize_wal_record(record: Dictionary) -> Dictionary:
	if not record.has("attempts"):
		record["attempts"] = 0
	if not record.has("first_attempt_ts"):
		record["first_attempt_ts"] = null
	if not record.has("event_id"):
		record["event_id"] = ""
	return record


## Rewrites the WAL after a drain. `remaining` is the drained snapshot's kept
## records; `consumed_lines` is how many non-empty lines that snapshot read. The
## game thread may have appended new records during the drain's unmutexed HTTP
## sends, so we re-read under the lock and keep every line beyond `consumed_lines`
## verbatim — otherwise the truncating rewrite would silently drop them.
func _rewrite_wal(remaining: Array[Dictionary], consumed_lines: int) -> void:
	_wal_mutex.lock()
	_ensure_project_dir()
	var wal_path := _wal_path()
	var tail: Array[String] = []
	if FileAccess.file_exists(wal_path):
		var read_file := FileAccess.open(wal_path, FileAccess.READ)
		if read_file != null:
			var index := 0
			while not read_file.eof_reached():
				var line := read_file.get_line().strip_edges()
				if line == "":
					continue
				if index >= consumed_lines:
					tail.append(line)
				index += 1
			read_file.close()
	var file := FileAccess.open(wal_path, FileAccess.WRITE)
	if file != null:
		for record in remaining:
			file.store_line(JSON.stringify(record))
		for line in tail:
			file.store_line(line)
		file.flush()
		file.close()
	_wal_mutex.unlock()


func _ensure_project_dir() -> void:
	DirAccess.make_dir_recursive_absolute(ProjectSettings.globalize_path(_project_dir()))


func _project_dir() -> String:
	return "%s/%s" % [PROJECTS_DIR, _safe_project_id()]


func _wal_path() -> String:
	return "%s/wal.ndjson" % _project_dir()


func _quarantine_path() -> String:
	return "%s/wal.quarantine.ndjson" % _project_dir()


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


func _unix_now() -> float:
	return Time.get_unix_time_from_system()


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
