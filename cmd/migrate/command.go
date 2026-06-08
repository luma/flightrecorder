package migrate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/urfave/cli/v3"

	"github.com/luma/flightrecorder/db/schema"
	"github.com/luma/flightrecorder/env"
)

// handleClearDirtyState checks if the clear-dirty-state flag is set and clears it if needed
func handleClearDirtyState(cmd *cli.Command, cfg *env.MigrateConfig) error {
	if cmd.Bool("clear-dirty-state") {
		if err := schema.ClearDirtyState(cfg.PostgresMigrateURL()); err != nil {
			return fmt.Errorf("failed to clear dirty state: %w", err)
		}
		fmt.Println("Dirty migration state cleared successfully")
	}
	return nil
}

// Command creates the migrate command
func Command() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Manage database migrations",
		Commands: []*cli.Command{
			newUpCommand(),
			newDownCommand(),
			newResetCommand(),
			newDestroyCommand(),
			newInfoCommand(),
			newLogCommand(),
		},
	}
}

// newUpCommand creates the migrate up command
func newUpCommand() *cli.Command {
	return &cli.Command{
		Name:  "up",
		Usage: "Apply migrations up to target version (or latest)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "target",
				Usage: "Target migration version (optional)",
			},
			&cli.BoolFlag{
				Name:  "ci",
				Usage: "CI-friendly output (JSON format, no color)",
			},
			&cli.BoolFlag{
				Name:  "clear-dirty-state",
				Usage: "Clear dirty migration state before running command",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := env.LoadMigrateConfig()
			if err != nil {
				return err
			}

			// Handle clearing dirty state if requested
			if err := handleClearDirtyState(cmd, cfg); err != nil {
				return err
			}

			target := -1 // Default to latest
			targetStr := cmd.String("target")
			if targetStr != "" {
				targetUint, err := strconv.ParseUint(targetStr, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid target version '%s': %w", targetStr, err)
				}
				target = int(targetUint)
			}

			err = schema.Up(cfg.PostgresMigrateURL(), target, schema.PoolConfig{
				MaxConns: int32(cfg.PostgresMigrateMaxConns),
				MinConns: int32(cfg.PostgresMigrateMinConns),
			})
			if err != nil {
				return fmt.Errorf("failed to create migrate instance: %w", err)
			}

			if cmd.Bool("ci") {
				fmt.Println(`{"status":"success","message":"Migration completed successfully"}`)
			} else {
				fmt.Println("Migration completed successfully")
			}

			return nil
		},
	}
}

// newDownCommand creates the migrate down command
func newDownCommand() *cli.Command {
	return &cli.Command{
		Name:      "down",
		Usage:     "Rollback migrations down to target version",
		ArgsUsage: "<target>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "ci",
				Usage: "CI-friendly output (JSON format, no color)",
			},
			&cli.BoolFlag{
				Name:  "clear-dirty-state",
				Usage: "Clear dirty migration state before running command",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return errors.New("target version is required")
			}

			targetStr := cmd.Args().First()
			target, err := strconv.ParseUint(targetStr, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid target version '%s': %w", targetStr, err)
			}

			cfg, err := env.LoadMigrateConfig()
			if err != nil {
				return err
			}

			// Handle clearing dirty state if requested
			if err := handleClearDirtyState(cmd, cfg); err != nil {
				return err
			}

			err = schema.Down(cfg.PostgresMigrateURL(), int(target))
			if err != nil {
				return fmt.Errorf("failed to create migrate instance: %w", err)
			}

			if cmd.Bool("ci") {
				fmt.Println(`{"status":"success","message":"Migration rollback completed successfully"}`)
			} else {
				fmt.Println("Migration rollback completed successfully")
			}

			return nil
		},
	}
}

// newResetCommand creates the migrate reset command
func newResetCommand() *cli.Command {
	return &cli.Command{
		Name:  "reset",
		Usage: "Drop everything and re-apply all migrations",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Skip confirmation prompt",
			},
			&cli.BoolFlag{
				Name:  "ci",
				Usage: "CI-friendly output (JSON format, no color)",
			},
			&cli.BoolFlag{
				Name:  "clear-dirty-state",
				Usage: "Clear dirty migration state before running command",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if !cmd.Bool("force") && !cmd.Bool("ci") {
				fmt.Print("WARNING: This will destroy all data in the database. Continue? [y/N] ")
				var response string
				_, _ = fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Migration reset cancelled")
					return nil
				}
			}

			cfg, err := env.LoadMigrateConfig()
			if err != nil {
				return err
			}

			// Handle clearing dirty state if requested
			if err := handleClearDirtyState(cmd, cfg); err != nil {
				return err
			}

			err = schema.Reset(cfg.PostgresMigrateURL(), schema.PoolConfig{
				MaxConns: int32(cfg.PostgresMigrateMaxConns),
				MinConns: int32(cfg.PostgresMigrateMinConns),
			})
			if err != nil {
				return fmt.Errorf("failed to create migrate instance: %w", err)
			}

			if cmd.Bool("ci") {
				fmt.Println(`{"status":"success","message":"Migration reset completed successfully"}`)
			} else {
				fmt.Println("Migration reset completed successfully")
			}

			return nil
		},
	}
}

// newDestroyCommand creates the migrate destroy command
func newDestroyCommand() *cli.Command {
	return &cli.Command{
		Name:  "destroy",
		Usage: "Drop all tables and data (destructive)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Skip confirmation prompt",
			},
			&cli.BoolFlag{
				Name:  "ci",
				Usage: "CI-friendly output (JSON format, no color)",
			},
			&cli.BoolFlag{
				Name:  "clear-dirty-state",
				Usage: "Clear dirty migration state before running command",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if !cmd.Bool("force") && !cmd.Bool("ci") {
				fmt.Print("WARNING: This will destroy all data in the database. Continue? [y/N] ")
				var response string
				_, _ = fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Migration destroy cancelled")
					return nil
				}
			}

			cfg, err := env.LoadMigrateConfig()
			if err != nil {
				return err
			}

			// Handle clearing dirty state if requested
			if err := handleClearDirtyState(cmd, cfg); err != nil {
				return err
			}

			err = schema.Destroy(cfg.PostgresMigrateURL())
			if err != nil {
				return fmt.Errorf("failed to create migrate instance: %w", err)
			}

			if cmd.Bool("ci") {
				fmt.Println(`{"status":"success","message":"Migration destroy completed successfully"}`)
			} else {
				fmt.Println("Migration destroy completed successfully")
			}

			return nil
		},
	}
}

// newInfoCommand creates the migrate info command
func newInfoCommand() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Show migration status information",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "ci",
				Usage: "CI-friendly output (JSON format, no color)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := env.LoadMigrateConfig()
			if err != nil {
				return err
			}

			version, dirty, err := schema.Info(cfg.PostgresMigrateURL())
			if err != nil {
				if errors.Is(err, migrate.ErrNilVersion) {
					if cmd.Bool("ci") {
						fmt.Println(`{"status":"success","version":"none","dirty":false}`)
					} else {
						fmt.Println("No migrations have been applied")
					}
					return nil
				}
				return fmt.Errorf("failed to get migration version: %w", err)
			}

			if cmd.Bool("ci") {
				fmt.Printf(`{"status":"success","version":"%d","dirty":%t}`, version, dirty)
				fmt.Println()
			} else {
				fmt.Printf("Current migration version: %d\n", version)
				if dirty {
					fmt.Println("WARNING: The database schema is in a dirty state")
				}
			}

			return nil
		},
	}
}

// newLogCommand creates the migrate log command
func newLogCommand() *cli.Command {
	return &cli.Command{
		Name:  "log",
		Usage: "Show migration history",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "before",
				Usage: "Show migrations before this version",
			},
			&cli.StringFlag{
				Name:  "after",
				Usage: "Show migrations after this version",
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "Maximum number of entries to show",
				Value: 10,
			},
			&cli.BoolFlag{
				Name:  "ci",
				Usage: "CI-friendly output (JSON format, no color)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// First get current version
			cfg, err := env.LoadMigrateConfig()
			if err != nil {
				return err
			}

			var filters schema.LogFilters
			// Before filter
			if cmd.IsSet("before") {
				beforeStr := cmd.String("before")
				before, err := strconv.ParseUint(beforeStr, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid before version '%s': %w", beforeStr, err)
				}

				filters.Before = int(before)
			}

			// After filter
			if cmd.IsSet("after") {
				afterStr := cmd.String("after")
				after, err := strconv.ParseUint(afterStr, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid after version '%s': %w", afterStr, err)
				}

				filters.After = int(after)
			}

			filters.Limit = cmd.Int("limit")

			migrations, err := schema.Log(cfg.PostgresMigrateURL(), filters)
			// m, err := migrate.New("file://schema/migrations", cfg.PostgresMigrateURL())
			if err != nil {
				return fmt.Errorf("failed to create migrate instance: %w", err)
			}

			// Display results
			if cmd.Bool("ci") {
				fmt.Print(`{"status":"success","migrations":[`)
				for i, migration := range migrations {
					if i > 0 {
						fmt.Print(",")
					}
					fmt.Printf(`{"version":"%d","name":"%s","applied":%t}`,
						migration.Version, migration.Name, migration.Applied)
				}
				fmt.Println("]}")
			} else {
				if len(migrations) == 0 {
					fmt.Println("No migrations found")
					return nil
				}

				fmt.Println("Migration history:")
				fmt.Println("------------------")
				for _, migration := range migrations {
					status := "[ ]"
					if migration.Applied {
						status = "[✓]"
					}
					// Convert version to readable timestamp
					versionTime := time.Date(
						int(migration.Version/10000000000),
						time.Month((migration.Version/100000000)%100),
						int((migration.Version/1000000)%100),
						int((migration.Version/10000)%100),
						int((migration.Version/100)%100),
						int(migration.Version%100),
						0, time.UTC,
					)
					timeStr := versionTime.Format("2006-01-02 15:04:05")
					fmt.Printf("%s %s (%d) - %s\n",
						status, timeStr, migration.Version, migration.Name)
				}
			}

			return nil
		},
	}
}
