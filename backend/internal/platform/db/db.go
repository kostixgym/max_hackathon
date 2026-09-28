// Package db connects to PostgreSQL and applies embedded migrations.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Connect opens a pool and waits until the database accepts connections:
// in docker compose the service may start a few seconds before PostgreSQL is ready.
func Connect(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(60 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) {
			pool.Close()

			return nil, fmt.Errorf("database is not reachable: %w", err)
		}
		log.Info("waiting for database", "err", err)

		select {
		case <-ctx.Done():
			pool.Close()

			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Migrate applies all pending migrations from the embedded filesystem.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS, log *slog.Logger) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	// Advisory lock in PostgreSQL: two instances starting at once (or two test packages
	// running in parallel) apply migrations one after another, not concurrently.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("migration locker: %w", err)
	}

	// Timestamped migrations from parallel branches may reach a database after a newer one.
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations,
		goose.WithSessionLocker(locker), goose.WithAllowOutofOrder(true))
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, r := range results {
		log.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration.String())
	}

	return nil
}
