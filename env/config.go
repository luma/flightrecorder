package env

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// General configuration
	Environment string     `env:"ENVIRONMENT" envDefault:"development"`
	LogLevelRaw string     `env:"LOG_LEVEL" envDefault:"info"`
	LogLevel    slog.Level `env:"-"`

	// API configuration
	APIPort            int           `env:"API_PORT" envDefault:"8080"`
	APIBasePath        string        `env:"API_BASE_PATH" envDefault:"/"`
	APIExitGracePeriod time.Duration `env:"API_EXIT_GRACE_PERIOD" envDefault:"3s"`
	APIDomain          string        `env:"API_DOMAIN"`                                      // Public domain for OAuth callbacks (e.g., your-app.ngrok.io)
	WebBaseURL         string        `env:"WEB_BASE_URL" envDefault:"http://localhost:3000"` // Frontend URL for redirects

	// PostgreSQL configuration
	PostgresHost           string        `env:"POSTGRES_HOST" envDefault:"localhost"`
	PostgresPort           int           `env:"POSTGRES_PORT" envDefault:"5432"`
	PostgresUser           string        `env:"POSTGRES_USER" envDefault:"flightrecorder"`
	PostgresPassword       string        `env:"POSTGRES_PASSWORD,notEmpty" envDefault:"flightrecorder"`
	PostgresDB             string        `env:"POSTGRES_DB" envDefault:"flightrecorder"`
	PostgresSSLMode        string        `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	PostgresSSLRootCert    string        `env:"POSTGRES_SSL_ROOT_CERT"`
	PostgresSSLNegotiation string        `env:"POSTGRES_SSL_NEGOTIATION"` // "direct" for PlanetScale, empty for standard PostgreSQL
	PostgresMigrateHost    string        `env:"POSTGRES_MIGRATE_HOST"`    // Direct PG host for migrations (bypasses PgBouncer)
	PostgresMigratePort    int           `env:"POSTGRES_MIGRATE_PORT"`    // Direct PG port for migrations (bypasses PgBouncer)
	PostgresMaxConns       int           `env:"POSTGRES_MAX_CONNECTIONS" envDefault:"30"`
	PostgresMinConns       int           `env:"POSTGRES_MIN_CONNECTIONS" envDefault:"5"`
	PostgresConnTimeout    time.Duration `env:"POSTGRES_CONN_TIMEOUT" envDefault:"10s"`

	// Feature Flags
	EnablePprof bool `env:"ENABLE_PPROF" envDefault:"false"`

	// Ingestion configuration
	MaxEventsPerBatch       int    `env:"MAX_EVENTS_PER_BATCH" envDefault:"50"`
	ReportRateLimitSeconds  int    `env:"REPORT_RATE_LIMIT_SECONDS" envDefault:"60"`
	ReportStorageBackend    string `env:"REPORT_STORAGE_BACKEND" envDefault:"local"`
	ReportStorageDir        string `env:"REPORT_STORAGE_DIR" envDefault:"var/reports"`
	AllowScreenshotFailures bool   `env:"ALLOW_SCREENSHOT_FAILURES" envDefault:"true"`

	// R2 screenshot storage. R2 uses the S3 API, so these are S3-compatible
	// connection settings scoped to report screenshot objects.
	R2Endpoint        string `env:"R2_ENDPOINT"`
	R2Bucket          string `env:"R2_BUCKET"`
	R2AccessKeyID     string `env:"R2_ACCESS_KEY_ID"`
	R2SecretAccessKey string `env:"R2_SECRET_ACCESS_KEY"`
	R2Region          string `env:"R2_REGION" envDefault:"auto"`

	// Admin UI authentication.
	// WARNING: AdminSessionSecret must be overridden with a strong random value
	// in production — the default is a placeholder only safe for local development.
	// WARNING: AdminDevLogin defaults to true for convenience; set it to false in
	// production or any shared environment, as it bypasses credential verification.
	AdminSessionSecret   string        `env:"ADMIN_SESSION_SECRET" envDefault:"dev-admin-session-secret-change-me"`
	AdminAllowedEmails   string        `env:"ADMIN_ALLOWED_EMAILS" envDefault:"admin@example.com"`
	AdminDevLogin        bool          `env:"ADMIN_DEV_LOGIN" envDefault:"true"`
	AdminSessionDuration time.Duration `env:"ADMIN_SESSION_DURATION" envDefault:"12h"`
}

// MigrateConfig holds only the config needed to run database migrations.
type MigrateConfig struct {
	PostgresHost            string `env:"POSTGRES_HOST" envDefault:"localhost"`
	PostgresPort            int    `env:"POSTGRES_PORT" envDefault:"5432"`
	PostgresUser            string `env:"POSTGRES_USER" envDefault:"flightrecorder"`
	PostgresPassword        string `env:"POSTGRES_PASSWORD,notEmpty" envDefault:"flightrecorder"`
	PostgresDB              string `env:"POSTGRES_DB" envDefault:"flightrecorder"`
	PostgresSSLMode         string `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	PostgresSSLRootCert     string `env:"POSTGRES_SSL_ROOT_CERT"`
	PostgresSSLNegotiation  string `env:"POSTGRES_SSL_NEGOTIATION"`
	PostgresMigrateHost     string `env:"POSTGRES_MIGRATE_HOST"`
	PostgresMigratePort     int    `env:"POSTGRES_MIGRATE_PORT"`
	PostgresMigrateMaxConns int    `env:"POSTGRES_MIGRATE_MAX_CONNECTIONS" envDefault:"2"`
	PostgresMigrateMinConns int    `env:"POSTGRES_MIGRATE_MIN_CONNECTIONS" envDefault:"1"`
}

// PostgresMigrateURL returns the connection string for migrations.
func (c *MigrateConfig) PostgresMigrateURL() string {
	host := c.PostgresHost
	port := c.PostgresPort
	if c.PostgresMigrateHost != "" {
		host = c.PostgresMigrateHost
	}
	if c.PostgresMigratePort != 0 {
		port = c.PostgresMigratePort
	}
	return buildPostgresURL(c.PostgresUser, c.PostgresPassword, host, port, c.PostgresDB, c.PostgresSSLMode, c.PostgresSSLRootCert, c.PostgresSSLNegotiation)
}

// LoadMigrateConfig loads only the Postgres config needed for migrations.
func LoadMigrateConfig() (*MigrateConfig, error) {
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	cfg := &MigrateConfig{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadConfig loads configuration from environment variables, optionally from a .env file
func LoadConfig() (*Config, error) {
	// Load .env if present; not an error if it's missing (e.g. in CI or production
	// where env vars come from the process environment directly).
	if err := godotenv.Load(); err != nil {
		// godotenv returns a path error when the file doesn't exist.
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// Parse out a slog compatble log level
	if err := cfg.LogLevel.UnmarshalText([]byte(cfg.LogLevelRaw)); err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	return cfg, nil
}

// buildPostgresURL constructs a properly-escaped Postgres connection string.
// Credentials are percent-encoded so special characters (dots, @, etc.)
// in usernames or passwords don't corrupt URL parsing.
func buildPostgresURL(user, password, host string, port int, db, sslMode, sslRootCert, sslNegotiation string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   db,
	}
	q := url.Values{}
	q.Set("sslmode", sslMode)
	if sslRootCert != "" {
		q.Set("sslrootcert", sslRootCert)
	}
	if sslNegotiation != "" {
		q.Set("sslnegotiation", sslNegotiation)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// PostgresURL returns the PostgreSQL connection string
func (c *Config) PostgresURL() string {
	return buildPostgresURL(c.PostgresUser, c.PostgresPassword, c.PostgresHost, c.PostgresPort, c.PostgresDB, c.PostgresSSLMode, c.PostgresSSLRootCert, c.PostgresSSLNegotiation)
}

// PostgresMigrateURL returns a connection string for migrations, bypassing PgBouncer.
// Falls back to PostgresURL() if no migrate-specific overrides are set.
func (c *Config) PostgresMigrateURL() string {
	host := c.PostgresHost
	port := c.PostgresPort
	if c.PostgresMigrateHost != "" {
		host = c.PostgresMigrateHost
	}
	if c.PostgresMigratePort != 0 {
		port = c.PostgresMigratePort
	}
	return buildPostgresURL(c.PostgresUser, c.PostgresPassword, host, port, c.PostgresDB, c.PostgresSSLMode, c.PostgresSSLRootCert, c.PostgresSSLNegotiation)
}

func (c *Config) GetAPIPort() int {
	return c.APIPort
}

func (c *Config) GetAPIBasePath() string {
	return c.APIBasePath
}

func (c *Config) GetAPIExitGracePeriod() time.Duration {
	return c.APIExitGracePeriod
}

// Validate checks that all required configuration is present and consistent.
// Returns an error describing all missing/invalid configuration.
// NOTE: the validation body is currently empty — all constraints are enforced
// by env struct tags (e.g. notEmpty) at parse time in LoadConfig.
func (c *Config) Validate() error {
	var errors []string

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", join(errors, "\n  - "))
	}

	return nil
}

// join concatenates strings with a separator (simple helper to avoid importing strings).
func join(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
