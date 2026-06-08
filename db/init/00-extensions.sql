-- Extensions required by flightrecorder.
-- In production these should be managed by the database provisioning layer.
-- Locally they're applied by docker-compose on first boot.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
