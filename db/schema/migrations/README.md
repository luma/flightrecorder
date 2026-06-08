# Database Migrations

This directory contains SQL migration files for the Vigilkeeper database schema.

## Migration Files

Migrations are versioned using timestamps in the format `YYYYMMDDHHmmss_description.{up,down}.sql`.

- `20250921191007_security_audit_logs` - Security audit logging tables
- `20250922081430_multi_tenancy_tables` - Organizations, users, projects, agents
- `20250922113311_environment_variables` - Environment variable management
- `20251003100000_map_primitives` - Map primitive for ACID key-value storage
- `20251003100001_timeline_primitives` - Timeline primitive for immutable event streams
- `20251005120000_deployments` - Deployment state management for pipeline versions

## Running Migrations

### Prerequisites
- PostgreSQL 14+ installed and running
- Database created (e.g., `vigilkeeper_dev`, `vigilkeeper_test`)

### Manual Migration

Apply migrations in order:

```bash
# Navigate to migrations directory
cd pkg/db/schema/migrations

# Apply all up migrations
for file in *_*.up.sql; do
    psql -d vigilkeeper_dev -f "$file"
done

# Rollback all migrations (in reverse order)
for file in $(ls -r *_*.down.sql); do
    psql -d vigilkeeper_dev -f "$file"
done
```

### Using Migration Tools

The project uses migration files that can be applied with standard PostgreSQL tools or migration libraries.

## Storage Primitives Overview

### Map Primitive (`map_primitives` table)

Provides ACID-compliant key-value storage with:
- Multi-tenant isolation (scoped to project + keyspace)
- Automatic version tracking (increments on update)
- Actor attribution (agent that created/modified)
- JSONB value storage with GIN indexing
- Immutable audit trail

**Primary Key**: `(project_id, keyspace, subkey)`

**Key Indexes**:
- `idx_map_project_keyspace` - For keyspace-scoped queries
- `idx_map_value_gin` - For JSONB value queries
- `idx_map_actor` - For actor-based audit queries

### Timeline Primitive (`timeline_primitives` table)

Provides immutable append-only event streams with:
- Immutability enforced by database triggers
- Partitioning by timestamp for efficient archival
- Support for S3 archival with hot/cold tiering
- Legal Hold mode for regulatory compliance
- Monotonic version tracking

**Primary Key**: `(project_id, keyspace, entry_id)`

**Partitioning**: Monthly partitions (automatically managed)

**Key Indexes**:
- `idx_timeline_project_keyspace_timestamp` - For time-range queries
- `idx_timeline_version` - For version-based queries
- `idx_timeline_archived` - For hot tier filtering

**Immutability**:
- Updates to core fields (data, timestamp, etc.) are prevented by triggers
- Deletes are prevented by triggers
- Only archival metadata fields can be updated

## Testing

Migration tests are located in `pkg/storage/migrations_test.go`.

Run tests with:

```bash
# Run all storage tests including migrations
go test ./pkg/storage -v

# Or using ginkgo
ginkgo -r -v pkg/storage
```

### Test Database Setup

Tests require a PostgreSQL test database:

```bash
# Create test database
createdb vigilkeeper_test

# Set environment variables (optional)
export TEST_DB_HOST=localhost
export TEST_DB_NAME=vigilkeeper_test
export TEST_DB_USER=postgres
export TEST_DB_PASSWORD=postgres
```

## Schema Verification

After applying migrations, verify the schema:

```sql
-- Check Map primitive table
\d map_primitives

-- Check Timeline primitive table
\d timeline_primitives

-- List all partitions
SELECT tablename FROM pg_tables WHERE tablename LIKE 'timeline_primitives_%';

-- Check triggers
SELECT trigger_name, event_object_table
FROM information_schema.triggers
WHERE event_object_table IN ('map_primitives', 'timeline_primitives');
```
