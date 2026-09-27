package initiatives

// Integration tests of SetStageTx (К0 of docs/plan-do-30-09.md): the stage
// transitions of path A go through the caller's transaction, with an optimistic
// from-check. Skipped without TEST_DATABASE_URL like the other integration tests.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/migrations"
)

func TestSetStageTx(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := db.Connect(ctx, url, log)
	if err != nil {
		t.Fatal(err)
	}
	// t.Cleanup, not defer: see orgs_integration_test.go.
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	houses := registry.NewStore(pool)
	slug, err := houses.SeedDemo(ctx, security.NewHasher([]byte("test-secret-test-secret-test-secret")), "stage-tx", log)
	if err != nil {
		t.Fatal(err)
	}
	house, err := houses.HouseBySlug(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}

	// A unique title keeps initiatives of repeated runs apart; the test removes them.
	title := time.Now().Format("setstage-150405.000")
	snap, err := houses.CurrentSnapshot(ctx, house.ID)
	if err != nil {
		t.Fatal(err)
	}
	var initiativeID string
	err = pool.QueryRow(ctx, `
		INSERT INTO initiatives (house_id, title, stage, registry_upload_id)
		VALUES ($1::uuid, $2, 'draft', $3::uuid)
		RETURNING id::text`, house.ID, title, snap.UploadID).Scan(&initiativeID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM initiatives WHERE id = $1::uuid`, initiativeID)
	})

	svc := NewService(pool, db.NewTransactionManager(pool), nil, houses, nil, nil)

	// The transitions of path A that SetStageTx serves. draft → poll belongs to
	// StartPoll: the schema requires poll_ends_at together with the poll stage.
	startedAt := time.Now().Add(24 * time.Hour)
	if _, err := pool.Exec(ctx, `
		UPDATE initiatives SET stage = 'poll', poll_ends_at = $2 WHERE id = $1::uuid`,
		initiativeID, startedAt); err != nil {
		t.Fatal(err)
	}

	pathA := "A"

	// poll → demand picks the way A; demand → meeting keeps it.
	if err := svc.SetStage(ctx, initiativeID, StagePoll, StageDemand, &pathA); err != nil {
		t.Fatalf("poll → demand: %v", err)
	}
	if err := svc.SetStage(ctx, initiativeID, StageDemand, StageMeeting, nil); err != nil {
		t.Fatalf("demand → meeting: %v", err)
	}
	if err := svc.SetStage(ctx, initiativeID, StageMeeting, StageCompleted, nil); err != nil {
		t.Fatalf("meeting → completed: %v", err)
	}

	got, err := svc.Stage(ctx, initiativeID)
	if err != nil || got != StageCompleted {
		t.Fatalf("stage = %q, %v; want %q", got, err, StageCompleted)
	}
	var path *string
	if err := pool.QueryRow(ctx, `SELECT path::text FROM initiatives WHERE id = $1::uuid`, initiativeID).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if path == nil || *path != "A" {
		t.Fatalf("path = %v, want A (nil keeps the stored path)", path)
	}

	// The optimistic check: a transition from a passed stage cannot repeat.
	if err := svc.SetStage(ctx, initiativeID, StagePoll, StageMeeting, nil); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("stale transition: %v, want ErrWrongStage", err)
	}

	// A same-stage move is a mistake, not a no-op.
	if err := svc.SetStage(ctx, initiativeID, StageMeeting, StageMeeting, nil); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("same stage: %v, want ErrWrongStage", err)
	}

	// An unknown initiative is the same wrong-stage error, not a 500.
	if err := svc.SetStage(ctx, "00000000-0000-7000-8000-0000000000ff", StagePoll, StageDemand, nil); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("unknown initiative: %v", err)
	}
}
