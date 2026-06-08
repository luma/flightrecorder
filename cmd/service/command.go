package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/urfave/cli/v3"

	"github.com/luma/flightrecorder/api"
	"github.com/luma/flightrecorder/db"
	"github.com/luma/flightrecorder/env"
	"github.com/luma/flightrecorder/services"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:   "serve",
		Usage:  "Start the flightrecorder server",
		Action: run,
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	cfg, err := env.LoadConfig()
	if err != nil {
		slog.Error("failed to start server", "err", err)
		return err
	}

	log := env.NewLogger(os.Stdout, cfg.LogLevel)

	if err := start(ctx, cmd, cfg, log); err != nil {
		log.Error("failed to start server", "err", err)
		return err
	}

	return nil
}

func start(parentCtx context.Context, cmd *cli.Command, cfg *env.Config, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	log.Info("Starting Service",
		"env", cfg.Environment,
		"api_addr", cfg.APIPort)

	// Create database pool
	dbConnectCtx, dbCancel := context.WithTimeout(ctx, cfg.PostgresConnTimeout)
	defer dbCancel()

	dbPool, pgxPool, err := CreateDatabasePool(dbConnectCtx, *cfg)
	if err != nil {
		return fmt.Errorf("failed to create database pool: %w", err)
	}
	if dbPool != nil {
		defer dbPool.Close()
		log.Info("Database connection established")
	}

	if pgxPool == nil {
		return errors.New("failed to create database pool")
	}

	authService := services.NewAuthService(dbPool)
	adminAuth := services.NewAdminAuthService(services.AdminAuthOptions{
		SessionSecret:   cfg.AdminSessionSecret,
		AllowedEmails:   cfg.AdminAllowedEmails,
		DevLogin:        cfg.AdminDevLogin,
		SessionDuration: cfg.AdminSessionDuration,
	})
	screenshotStore, err := CreateScreenshotStore(ctx, *cfg)
	if err != nil {
		return err
	}
	adminService := services.NewAdminService(dbPool, screenshotStore)
	ingestService := services.NewIngestService(services.IngestOptions{
		DB:                      dbPool,
		MaxEventsPerBatch:       cfg.MaxEventsPerBatch,
		ReportRateLimitSeconds:  cfg.ReportRateLimitSeconds,
		AllowScreenshotFailures: cfg.AllowScreenshotFailures,
		ScreenshotStore:         screenshotStore,
	})

	err = api.Run(api.Options{
		APIPort:            cfg.APIPort,
		APIBasePath:        cfg.APIBasePath,
		APIExitGracePeriod: cfg.APIExitGracePeriod,
		APIBaseURL:         apiBaseURL(*cfg),
		WebBaseURL:         cfg.WebBaseURL,
		EnablePprof:        cfg.EnablePprof,
		DBPool:             dbPool,
		PgxPool:            pgxPool,
		Log:                log,
		Ctx:                ctx,
		AuthService:        authService,
		IngestService:      ingestService,
		AdminAuth:          adminAuth,
		AdminService:       adminService,
	})
	if err != nil {
		return fmt.Errorf("running API server: %w", err)
	}

	log.Info("server stopped")

	return nil
}

// CreateDatabasePool creates a database connection pool from the configuration.
// Returns both the db.Pool interface and the underlying *pgxpool.Pool for River.
func CreateDatabasePool(ctx context.Context, cfg env.Config) (db.Pool, *pgxpool.Pool, error) {
	connConfig := db.ConnectConfig{
		ConnectionString: cfg.PostgresURL(),
		MaxConnections:   int32(cfg.PostgresMaxConns),
		MinConnections:   int32(cfg.PostgresMinConns),
	}

	// Skip database connection if no password is provided (for development/testing)
	if cfg.PostgresPassword == "" {
		return nil, nil, nil
	}

	pool, err := db.ConnectToPool(ctx, connConfig)
	if err != nil {
		return nil, nil, err
	}

	pgxPool, ok := pool.(*pgxpool.Pool)
	if !ok {
		return pool, nil, nil
	}

	return pool, pgxPool, nil
}

func CreateScreenshotStore(ctx context.Context, cfg env.Config) (services.ScreenshotStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.ReportStorageBackend)) {
	case "", "local", "filesystem", "fs":
		return services.LocalScreenshotStore{
			RootDir: cfg.ReportStorageDir,
		}, nil
	case "r2":
		return services.NewR2ScreenshotStore(ctx, services.R2ScreenshotStoreOptions{
			Endpoint:        cfg.R2Endpoint,
			Bucket:          cfg.R2Bucket,
			AccessKeyID:     cfg.R2AccessKeyID,
			SecretAccessKey: cfg.R2SecretAccessKey,
			Region:          cfg.R2Region,
		})
	default:
		return nil, fmt.Errorf("unsupported report storage backend %q", cfg.ReportStorageBackend)
	}
}

func apiBaseURL(cfg env.Config) string {
	if cfg.APIDomain != "" {
		return "https://" + cfg.APIDomain
	}
	return fmt.Sprintf("http://localhost:%d", cfg.APIPort)
}
