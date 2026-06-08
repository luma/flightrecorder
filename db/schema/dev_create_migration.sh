#!/bin/bash
# Creates a new migration in the migrations directory

# Check if migration name was provided
if [ -z "$1" ]; then
  echo "Error: Migration name is required"
  echo "Usage: ./dev_create_migration.sh <migration_name>"
  exit 1
fi

# Ensure the migrations directory exists
MIGRATIONS_DIR="$(dirname "$0")/migrations"
mkdir -p "$MIGRATIONS_DIR"

# Load environment variables if .env exists
if [ -f "$(dirname "$0")/../../../.env" ]; then
  source "$(dirname "$0")/../../../.env"
fi

# Create a timestamp for the migration
TIMESTAMP=$(date +%Y%m%d%H%M%S)
MIGRATION_NAME=$(echo "$1" | tr ' ' '_')
MIGRATION_PREFIX="${TIMESTAMP}_${MIGRATION_NAME}"

# Create up and down migration files
UP_FILE="${MIGRATIONS_DIR}/${MIGRATION_PREFIX}.up.sql"
DOWN_FILE="${MIGRATIONS_DIR}/${MIGRATION_PREFIX}.down.sql"

echo "-- Migration: ${MIGRATION_NAME}
-- Created at: $(date)
-- Up Migration

" > "$UP_FILE"

echo "-- Migration: ${MIGRATION_NAME}
-- Created at: $(date)
-- Down Migration

" > "$DOWN_FILE"

echo "Migration files created:"
echo "- $UP_FILE"
echo "- $DOWN_FILE"

# Make files executable
chmod +x "$UP_FILE" "$DOWN_FILE"
