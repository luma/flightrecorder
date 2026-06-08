-- Drop in reverse dependency order so the baseline can be cleanly rolled back
-- by local development databases and test fixtures.
DROP TABLE IF EXISTS report_notes;
DROP TABLE IF EXISTS bug_reports;
DROP TABLE IF EXISTS event_fields;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS batches;
DROP TABLE IF EXISTS ingest_tokens;
DROP TABLE IF EXISTS admin_users;
DROP TABLE IF EXISTS projects;

-- pgcrypto may be shared by other schemas in a database, so leave the extension
-- installed when rolling back this application's objects.
