package schema

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsSource is a binary source of migration.
var migrationsSource source.Driver

func init() {
	var err error
	migrationsSource, err = iofs.New(migrationsFS, "migrations")
	if err != nil {
		log.Fatal(err)
	}
}

// PoolConfig controls the connection pool size for transient pools created
// during migrations. These connect directly to PG (bypassing PgBouncer), so
// keeping them small avoids exhausting backend connection slots.
type PoolConfig struct {
	MaxConns int32
	MinConns int32
}

// migrateURL converts a postgres:// connection string to pgx5:// so
// golang-migrate routes it through the pgx driver instead of lib/pq.
// pgx handles PlanetScale's direct-SSL requirement; lib/pq does not.
func migrateURL(connectionString string) string {
	if strings.HasPrefix(connectionString, "postgres://") {
		return "pgx5://" + strings.TrimPrefix(connectionString, "postgres://")
	}
	if strings.HasPrefix(connectionString, "postgresql://") {
		return "pgx5://" + strings.TrimPrefix(connectionString, "postgresql://")
	}
	return connectionString
}

// newCappedPool creates a pgxpool with explicit max/min connection limits.
func newCappedPool(ctx context.Context, connectionString string, cfg PoolConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	return pgxpool.NewWithConfig(ctx, poolConfig)
}

// Up migrates up to the given target, or to the latest if the target is -1
func Up(connectionString string, target int, poolCfg PoolConfig) error {
	m, err := migrate.NewWithSourceInstance("iofs", migrationsSource, migrateURL(connectionString))
	if err != nil {
		return fmt.Errorf("failed to migrate up: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if target < 0 {
		err = m.Up()
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("failed to migrate up: %w", err)
		}
	} else {
		err = m.Migrate(uint(target))
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("failed to migrate to version %d: %w", target, err)
		}
	}

	// Run River migrations (job queue schema)
	if err := migrateRiver(connectionString, poolCfg); err != nil {
		return fmt.Errorf("failed to run River migrations: %w", err)
	}

	return nil
}

// migrateRiver runs River job queue migrations using River's official Go API.
// This ensures we stay in sync with River's schema without maintaining our own copies.
func migrateRiver(connectionString string, poolCfg PoolConfig) error {
	ctx := context.Background()

	pool, err := newCappedPool(ctx, connectionString, poolCfg)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool for River migrations: %w", err)
	}
	defer pool.Close()

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("failed to create River migrator: %w", err)
	}

	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("failed to run River migrations: %w", err)
	}

	return nil
}

// Down migrates Down to the given target.
func Down(connectionString string, target int) error {
	m, err := migrate.NewWithSourceInstance("iofs", migrationsSource, migrateURL(connectionString))
	if err != nil {
		return fmt.Errorf("failed to migrate up: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	err = m.Migrate(uint(target))
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to migrate to version %d: %w", target, err)
	}

	return nil
}

// Reset drops the schema and then reapplies all migrations
func Reset(connectionString string, poolCfg PoolConfig) error {
	if err := forceCleanDatabase(connectionString, poolCfg); err != nil {
		return fmt.Errorf("failed to force clean database: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", migrationsSource, migrateURL(connectionString))
	if err != nil {
		return fmt.Errorf("failed to create fresh migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to migrate up: %w", err)
	}

	if err := migrateRiver(connectionString, poolCfg); err != nil {
		return fmt.Errorf("failed to run River migrations: %w", err)
	}

	return nil
}

// forceCleanDatabase performs a comprehensive cleanup of the database using pgx.
func forceCleanDatabase(connectionString string, poolCfg PoolConfig) error {
	ctx := context.Background()

	pool, err := newCappedPool(ctx, connectionString, poolCfg)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cleanupQueries := []string{
		`DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN SELECT viewname FROM pg_views WHERE schemaname = 'public' LOOP
				EXECUTE format('DROP VIEW IF EXISTS %I CASCADE', r.viewname);
			END LOOP;
		END $$;`,

		`DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' LOOP
				EXECUTE format('DROP TABLE IF EXISTS %I CASCADE', r.tablename);
			END LOOP;
		END $$;`,

		`DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN SELECT proname, oidvectortypes(proargtypes) as argtypes
					 FROM pg_proc INNER JOIN pg_namespace ns ON (pg_proc.pronamespace = ns.oid)
					 WHERE ns.nspname = 'public'
					 AND NOT EXISTS (
						 SELECT 1 FROM pg_depend d
						 WHERE d.objid = pg_proc.oid
						 AND d.deptype = 'e'
					 ) LOOP
				EXECUTE format('DROP FUNCTION IF EXISTS %I(%s) CASCADE', r.proname, r.argtypes);
			END LOOP;
		END $$;`,

		`DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN SELECT typname FROM pg_type WHERE typnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
					 AND typtype = 'e' LOOP
				EXECUTE format('DROP TYPE IF EXISTS %I CASCADE', r.typname);
			END LOOP;
		END $$;`,

		`DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN SELECT sequencename FROM pg_sequences WHERE schemaname = 'public' LOOP
				EXECUTE format('DROP SEQUENCE IF EXISTS %I CASCADE', r.sequencename);
			END LOOP;
		END $$;`,
	}

	for _, query := range cleanupQueries {
		if _, err := tx.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to execute cleanup query: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit cleanup transaction: %w", err)
	}

	return nil
}

// Destroy drops the schema.
func Destroy(connectionString string) error {
	m, err := migrate.NewWithSourceInstance("iofs", migrationsSource, migrateURL(connectionString))
	if err != nil {
		return fmt.Errorf("failed to migrate up: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err = m.Drop(); err != nil {
		return fmt.Errorf("failed to drop everything: %w", err)
	}

	return nil
}

// Info returns the current migration version and dirty state.
func Info(connectionString string) (version uint, dirty bool, err error) {
	m, err := migrate.NewWithSourceInstance("iofs", migrationsSource, migrateURL(connectionString))
	if err != nil {
		return 0, false, fmt.Errorf("failed to migrate up: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	version, dirty, err = m.Version()
	if err != nil {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}

type MigrationInfo struct {
	Version    uint
	Name       string
	Applied    bool
	AppliedAt  string
	Direction  string
	SourceFile string
}

type LogFilters struct {
	// Before filters migrations to only those with a version less than this version. This will be
	// ignored if Before is 0
	Before int

	// After filters migrations to only those with a version greater than this version. This will be
	// ignored if After is 0
	After int

	// Limit the maximum number of migrations to show
	Limit int
}

func Log(connectionString string, filters LogFilters) ([]MigrationInfo, error) {
	m, err := migrate.NewWithSourceInstance("iofs", migrationsSource, migrateURL(connectionString))
	if err != nil {
		return nil, fmt.Errorf("failed to migrate up: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	currentVersion, _, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return nil, fmt.Errorf("failed to get migration version: %w", err)
	}

	var migrations []MigrationInfo

	if err := fs.WalkDir(migrationsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}

		if d.IsDir() {
			return nil
		}

		// Parse filename to get version
		// Expected format: YYYYMMDDHHMMSS_name.(up|down).sql
		name := d.Name()
		if len(name) < 24 || name[14] != '_' || (name[len(name)-7:] != ".up.sql" && name[len(name)-9:] != ".down.sql") {
			return nil
		}

		versionStr := name[:14]
		version, err := strconv.ParseUint(versionStr, 10, 64)
		if err != nil {
			return nil
		}

		direction := "up"
		if name[len(name)-9:] == ".down.sql" {
			direction = "down"
		}

		// Only add up migrations to the list for display
		if direction == "up" {
			applied := uint(version) <= currentVersion
			appliedAt := ""
			if applied {
				appliedAt = "unknown"
			}

			migrationName := name[15 : len(name)-7]

			migrations = append(migrations, MigrationInfo{
				Version:    uint(version),
				Name:       migrationName,
				Applied:    applied,
				AppliedAt:  appliedAt,
				Direction:  direction,
				SourceFile: name,
			})
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to walk migrations directory: %w", err)
	}

	var filteredMigrations []MigrationInfo
	for _, migration := range migrations {
		if filters.Before > 0 {
			if migration.Version >= uint(filters.Before) {
				continue
			}
		}

		if filters.After > 0 {
			if migration.Version <= uint(filters.After) {
				continue
			}
		}

		filteredMigrations = append(filteredMigrations, migration)
	}

	// Sort by version (desc)
	for i := 0; i < len(filteredMigrations)-1; i++ {
		for j := i + 1; j < len(filteredMigrations); j++ {
			if filteredMigrations[i].Version < filteredMigrations[j].Version {
				filteredMigrations[i], filteredMigrations[j] = filteredMigrations[j], filteredMigrations[i]
			}
		}
	}

	if filters.Limit > 0 && filters.Limit < len(filteredMigrations) {
		filteredMigrations = filteredMigrations[:filters.Limit]
	}

	return filteredMigrations, nil
}

// ClearDirtyState resets a dirty migration state so that `migrate up` can
// proceed. When a migration fails, golang-migrate records the version as
// dirty. We force to the PREVIOUS version (not the dirty one) so the failed
// migration is re-attempted on the next `up`.
func ClearDirtyState(connectionString string) error {
	m, err := migrate.NewWithSourceInstance("iofs", migrationsSource, migrateURL(connectionString))
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	version, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return nil
		}
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if !dirty {
		return nil
	}

	// Find the migration version immediately before the dirty one.
	// Force to that version so the failed migration runs again on next `up`.
	prevVersion, err := findPreviousVersion(version)
	if err != nil {
		return fmt.Errorf("failed to find previous version for %d: %w", version, err)
	}

	// -1 means the dirty migration was the very first one. Use golang-migrate's
	// special NilVersion (-1) which represents "no migrations applied".
	if err := m.Force(prevVersion); err != nil {
		return fmt.Errorf("failed to force migration version to %d: %w", prevVersion, err)
	}

	return nil
}

// findPreviousVersion walks the embedded migrations to find the version
// immediately before the given one. Returns -1 if there is no previous
// version (the dirty migration was the first one).
func findPreviousVersion(dirtyVersion uint) (int, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return 0, fmt.Errorf("reading migrations dir: %w", err)
	}

	var versions []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		// Migration filenames are like "20260304174241_initial_schema.up.sql"
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}

	sort.Ints(versions)

	prev := -1
	for _, v := range versions {
		if uint(v) >= dirtyVersion {
			break
		}
		prev = v
	}
	return prev, nil
}
