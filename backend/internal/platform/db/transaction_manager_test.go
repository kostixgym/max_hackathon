package db

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/migrations"
)

func TestTransactionManagerCommitAndRollback(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	pool, err := Connect(ctx, url, log)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	manager := NewTransactionManager(pool)
	baseID := time.Now().UnixNano()
	rollbackID := baseID
	commitID := baseID + 1
	defer func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM users WHERE max_user_id IN ($1, $2)`, rollbackID, commitID)
	}()

	wantErr := errors.New("stop transaction")
	err = manager.WithinTransaction(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, execErr := tx.Exec(txCtx, `INSERT INTO users (max_user_id) VALUES ($1)`, rollbackID); execErr != nil {
			return execErr
		}

		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("rollback transaction error = %v, want %v", err, wantErr)
	}
	assertUserCount(t, ctx, pool, rollbackID, 0)

	if err := manager.WithinTransaction(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, execErr := tx.Exec(txCtx, `INSERT INTO users (max_user_id) VALUES ($1)`, commitID)

		return execErr
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	assertUserCount(t, ctx, pool, commitID, 1)
}

func assertUserCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, maxUserID int64, want int) {
	t.Helper()

	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE max_user_id = $1`, maxUserID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("user count for max_user_id %d = %d, want %d", maxUserID, got, want)
	}
}
