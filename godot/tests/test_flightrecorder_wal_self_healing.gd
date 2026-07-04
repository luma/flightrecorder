extends GutTest

## Covers the WAL self-healing drain: status classification, non-blocking drain
## (permanent quarantine + continue, transient stop, auth stop, oversize shrink),
## the transient age backstop, event-level idempotency metadata, and quarantine
## retention. See docs/plans/wal-poisoning-self-healing.md.

const TELEMETRY_SCRIPT := preload("res://addons/flightrecorder/flightrecorder_telemetry.gd")
const VALID_PLAYER_ID := "550e8400-e29b-41d4-a716-446655440000"


## Test double: overrides the single HTTP boundary so the drain can be exercised
## deterministically without a network. Returns a scripted sequence of
## SendResults and records each posted batch size.
class StubClient:
	extends FlightRecorderTelemetryClient

	var results: Array = []
	var calls: Array = []
	# Records appended to the WAL during a send, one per post call. Simulates the
	# game thread calling record_event() while the drain is mid-HTTP (the drain
	# holds no WAL lock during a send).
	var append_on_post: Array = []

	func _post_json(_route_path: String, body: Dictionary) -> int:
		var count := 0
		if body.has("events"):
			count = body["events"].size()
		calls.append(count)
		if not append_on_post.is_empty():
			_append_wal(append_on_post.pop_front())
		if results.is_empty():
			return SendResult.SUCCESS
		return results.pop_front()


var _client: StubClient


func before_each() -> void:
	_client = StubClient.new()
	_client.player_id = VALID_PLAYER_ID
	_client.project_id = "gut_test_%s" % Time.get_ticks_usec()
	_client.ingest_token = "test-token"


func after_each() -> void:
	_cleanup_client_project_dir()
	_client = null


# --- record format / idempotency metadata ------------------------------------


func test_record_event_persists_event_id_and_retry_metadata() -> void:
	_client.record_event("dock", { "ok": true })

	var records := _client._read_wal()
	assert_eq(records.size(), 1)
	assert_true(records[0]["event_id"] is String and records[0]["event_id"].length() == 36)
	assert_eq(records[0]["attempts"], 0)
	assert_null(records[0]["first_attempt_ts"])


func test_event_for_transport_includes_event_id_when_present() -> void:
	var with_id: Dictionary = _client._event_for_transport({ }, "abc-123")
	assert_eq(with_id["event_id"], "abc-123")

	var without_id: Dictionary = _client._event_for_transport({ })
	assert_false(without_id.has("event_id"))


func test_normalize_wal_record_defaults_legacy_fields() -> void:
	var legacy := { "kind": "event", "event": { "event_type": "dock" } }
	var normalized: Dictionary = _client._normalize_wal_record(legacy)

	assert_eq(normalized["attempts"], 0)
	assert_null(normalized["first_attempt_ts"])
	assert_eq(normalized["event_id"], "")


# --- response classification -------------------------------------------------


func test_classify_response_maps_status_codes() -> void:
	var collector_error := _bytes('{"error":"player_id must be a UUID"}')
	var empty := PackedByteArray()

	assert_eq(_client._classify_response(200, empty), _client.SendResult.SUCCESS)
	assert_eq(_client._classify_response(413, empty), _client.SendResult.OVERSIZE)
	assert_eq(_client._classify_response(408, empty), _client.SendResult.TRANSIENT)
	assert_eq(_client._classify_response(425, empty), _client.SendResult.TRANSIENT)
	assert_eq(_client._classify_response(429, empty), _client.SendResult.TRANSIENT)
	assert_eq(_client._classify_response(500, empty), _client.SendResult.TRANSIENT)
	assert_eq(_client._classify_response(503, empty), _client.SendResult.TRANSIENT)
	assert_eq(_client._classify_response(401, empty), _client.SendResult.AUTH)
	assert_eq(_client._classify_response(403, empty), _client.SendResult.AUTH)
	assert_eq(_client._classify_response(404, empty), _client.SendResult.AUTH)
	assert_eq(_client._classify_response(405, empty), _client.SendResult.AUTH)
	# 400/422 only quarantine when the body is the collector's own error shape.
	assert_eq(_client._classify_response(400, collector_error), _client.SendResult.PERMANENT)
	assert_eq(_client._classify_response(422, collector_error), _client.SendResult.PERMANENT)
	# A proxy-generated 400 (no collector error body) must not drop data.
	assert_eq(_client._classify_response(400, _bytes("<html>bad gateway</html>")), _client.SendResult.AUTH)
	assert_eq(_client._classify_response(400, _bytes('{"detail":"x"}')), _client.SendResult.AUTH)
	assert_eq(_client._classify_response(400, empty), _client.SendResult.AUTH)


# --- oversize batch shrink ---------------------------------------------------


func test_shrink_effective_batch_size_halves_to_floor_one() -> void:
	_client.batch_size = 25
	assert_eq(_client._current_effective_batch_size(), 25)
	_client._shrink_effective_batch_size()
	assert_eq(_client._current_effective_batch_size(), 12)
	_client._shrink_effective_batch_size()
	assert_eq(_client._current_effective_batch_size(), 6)
	_client._shrink_effective_batch_size()
	assert_eq(_client._current_effective_batch_size(), 3)
	_client._shrink_effective_batch_size()
	assert_eq(_client._current_effective_batch_size(), 1)
	_client._shrink_effective_batch_size()
	assert_eq(_client._current_effective_batch_size(), 1)


# --- transient backstop ------------------------------------------------------


func test_is_stuck_uses_age_and_attempt_bounds() -> void:
	_client.max_record_age_seconds = 100
	_client.max_transient_attempts = 0

	var fresh := { "attempts": 3, "first_attempt_ts": int(Time.get_unix_time_from_system()) }
	assert_false(_client._is_stuck(fresh))

	var old := { "attempts": 1, "first_attempt_ts": int(Time.get_unix_time_from_system()) - 1000 }
	assert_true(_client._is_stuck(old))

	_client.max_record_age_seconds = 0
	_client.max_transient_attempts = 2
	assert_true(_client._is_stuck({ "attempts": 3, "first_attempt_ts": null }))
	assert_false(_client._is_stuck({ "attempts": 2, "first_attempt_ts": null }))


func test_bump_transient_increments_and_stamps_once() -> void:
	var record := { "attempts": 0, "first_attempt_ts": null }
	_client._bump_transient(record)
	assert_eq(record["attempts"], 1)
	var first_stamp = record["first_attempt_ts"]
	assert_true(first_stamp is int or first_stamp is float)

	_client._bump_transient(record)
	assert_eq(record["attempts"], 2)
	assert_eq(record["first_attempt_ts"], first_stamp)


# --- drain: non-blocking behavior --------------------------------------------


func test_drain_permanent_quarantines_head_and_delivers_rest() -> void:
	# Simulates a poisoned head batch against an old server (400 collector error):
	# the bad record is quarantined and the good record behind it is delivered.
	_client.batch_size = 1
	_client.record_event("bad", { "x": 1 })
	_client.record_event("good", { "x": 2 })
	_client.results = [_client.SendResult.PERMANENT, _client.SendResult.SUCCESS]

	var outcome := _client._drain_wal()

	assert_eq(outcome, _client.DrainOutcome.EMPTY)
	assert_eq(_client._read_wal().size(), 0)
	var quarantined := _read_quarantine()
	assert_eq(quarantined.size(), 1)
	assert_eq(quarantined[0]["reason"], "permanent")
	assert_eq(quarantined[0]["event"]["event_type"], "bad")


func test_drain_transient_stops_keeps_all_and_bumps_attempts() -> void:
	_client.batch_size = 25
	_client.record_event("a")
	_client.record_event("b")
	_client.results = [_client.SendResult.TRANSIENT]

	var outcome := _client._drain_wal()

	assert_eq(outcome, _client.DrainOutcome.TRANSIENT)
	var remaining := _client._read_wal()
	assert_eq(remaining.size(), 2)
	assert_eq(remaining[0]["attempts"], 1)
	assert_eq(remaining[1]["attempts"], 1)
	assert_not_null(remaining[0]["first_attempt_ts"])
	assert_eq(_read_quarantine().size(), 0)


func test_drain_preserves_records_appended_during_send() -> void:
	# The drain releases the WAL lock during HTTP sends, so a record recorded
	# mid-drain must survive the post-drain rewrite instead of being truncated.
	_client.batch_size = 1
	_client.record_event("first")
	_client.append_on_post = [
		{
			"kind": "event",
			"event": _client.build_event("during_send", { }),
			"event_id": _client._uuid_v4(),
			"attempts": 0,
			"first_attempt_ts": null,
		},
	]
	_client.results = [_client.SendResult.SUCCESS]

	_client._drain_wal()

	var remaining := _client._read_wal()
	assert_eq(remaining.size(), 1)
	assert_eq(remaining[0]["event"]["event_type"], "during_send")


func test_drain_auth_stops_keeps_all_without_bumping() -> void:
	_client.batch_size = 25
	_client.record_event("a")
	_client.record_event("b")
	_client.results = [_client.SendResult.AUTH]

	var outcome := _client._drain_wal()

	assert_eq(outcome, _client.DrainOutcome.AUTH)
	var remaining := _client._read_wal()
	assert_eq(remaining.size(), 2)
	assert_eq(remaining[0]["attempts"], 0)
	assert_null(remaining[0]["first_attempt_ts"])
	assert_eq(_read_quarantine().size(), 0)


func test_drain_oversize_shrinks_then_delivers() -> void:
	_client.batch_size = 4
	for i in range(4):
		_client.record_event("e%d" % i)
	# First (size-4) batch is too large; after shrinking to 2 the two halves send.
	_client.results = [_client.SendResult.OVERSIZE, _client.SendResult.SUCCESS, _client.SendResult.SUCCESS]

	var outcome := _client._drain_wal()

	assert_eq(outcome, _client.DrainOutcome.EMPTY)
	assert_eq(_client._read_wal().size(), 0)
	assert_eq(_client.calls, [4, 2, 2])
	assert_eq(_read_quarantine().size(), 0)


func test_drain_oversize_single_record_quarantines_and_continues() -> void:
	# A lone record that still 413s at batch size 1 can never fit; it must be
	# quarantined so it cannot head-of-line-block the drain forever.
	_client.batch_size = 1
	_client.record_event("huge")
	_client.results = [_client.SendResult.OVERSIZE]

	var outcome := _client._drain_wal()

	assert_eq(outcome, _client.DrainOutcome.EMPTY)
	assert_eq(_client._read_wal().size(), 0)
	var quarantined := _read_quarantine()
	assert_eq(quarantined.size(), 1)
	assert_eq(quarantined[0]["reason"], "oversize")


func test_drain_stuck_record_quarantined_on_transient() -> void:
	_client.batch_size = 1
	_client.max_record_age_seconds = 10
	# Craft a record whose first attempt was long ago so the age backstop trips.
	_client._append_wal(
		{
			"kind": "event",
			"event": _client.build_event("old", { "x": 1 }),
			"event_id": _client._uuid_v4(),
			"attempts": 5,
			"first_attempt_ts": int(Time.get_unix_time_from_system()) - 100000,
		},
	)
	_client.results = [_client.SendResult.TRANSIENT]

	_client._drain_wal()

	assert_eq(_client._read_wal().size(), 0)
	var quarantined := _read_quarantine()
	assert_eq(quarantined.size(), 1)
	assert_eq(quarantined[0]["reason"], "stuck")


func test_drain_bug_report_permanent_quarantines() -> void:
	_client.submit_bug_report(3, "ok", "note")
	_client.results = [_client.SendResult.PERMANENT]

	var outcome := _client._drain_wal()

	assert_eq(outcome, _client.DrainOutcome.EMPTY)
	assert_eq(_client._read_wal().size(), 0)
	var quarantined := _read_quarantine()
	assert_eq(quarantined.size(), 1)
	assert_eq(quarantined[0]["kind"], "bug_report")
	assert_eq(quarantined[0]["reason"], "permanent")


# --- quarantine retention ----------------------------------------------------


func test_quarantine_trims_to_record_cap_fifo() -> void:
	_client.max_quarantine_records = 3
	_client.max_quarantine_bytes = 10_000_000
	var entries: Array[Dictionary] = []
	for i in range(5):
		entries.append({ "kind": "event", "seq": i })
	_client._append_quarantine(entries)

	var quarantined := _read_quarantine()
	assert_eq(quarantined.size(), 3)
	# FIFO: the three newest survive.
	assert_eq(quarantined[0]["seq"], 2)
	assert_eq(quarantined[2]["seq"], 4)


func test_quarantine_trims_to_byte_cap_fifo() -> void:
	_client.max_quarantine_records = 1000
	# Each line is well over 20 bytes, so a 60-byte cap keeps only the last few.
	_client.max_quarantine_bytes = 120
	var entries: Array[Dictionary] = []
	for i in range(20):
		entries.append({ "kind": "event", "seq": i, "pad": "xxxxxxxxxx" })
	_client._append_quarantine(entries)

	var quarantined := _read_quarantine()
	assert_true(quarantined.size() < 20)
	assert_true(quarantined.size() >= 1)
	# The most recent entry is always retained.
	assert_eq(quarantined[quarantined.size() - 1]["seq"], 19)


# --- helpers -----------------------------------------------------------------


func _bytes(text: String) -> PackedByteArray:
	return text.to_utf8_buffer()


func _read_quarantine() -> Array:
	var path := _client._quarantine_path()
	var out: Array = []
	if not FileAccess.file_exists(path):
		return out
	var file := FileAccess.open(path, FileAccess.READ)
	if file == null:
		return out
	while not file.eof_reached():
		var line := file.get_line().strip_edges()
		if line == "":
			continue
		var parsed = JSON.parse_string(line)
		if typeof(parsed) == TYPE_DICTIONARY:
			out.append(parsed)
	file.close()
	return out


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
