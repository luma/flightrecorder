DROP TABLE IF EXISTS project_rejection_acks;
DROP TABLE IF EXISTS rejected_events;
DROP INDEX IF EXISTS events_project_client_event_id_key;
ALTER TABLE events DROP COLUMN IF EXISTS client_event_id;
