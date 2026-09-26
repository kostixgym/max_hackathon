package registry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/migrations"
)

// Integration test, needs TEST_DATABASE_URL (see internal/platform/db/db_test.go).
// Uses its own database state: run against a disposable container.
func TestSeedDemoAndCurrentVersion(t *testing.T) {
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
	defer pool.Close()
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	store := NewStore(pool)
	hasher := security.NewHasher([]byte("test-secret-test-secret-test-secret"))

	// Two instances seeding at once (on a clean database this is the race from the 0.2 review):
	// both must get the same house.
	var wg sync.WaitGroup
	slugs, errs := make([]string, 2), make([]error, 2)
	for i := range 2 {
		wg.Go(func() { slugs[i], errs[i] = store.SeedDemo(ctx, hasher, "", log) })
	}
	wg.Wait()
	for i := range 2 {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
	}
	if slugs[0] != slugs[1] {
		t.Fatalf("concurrent seeds returned different houses: %q and %q", slugs[0], slugs[1])
	}
	slug := slugs[0]
	var demoHouses int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM houses WHERE is_demo`).Scan(&demoHouses); err != nil || demoHouses != 1 {
		t.Fatalf("demo houses = %d, err %v; want exactly 1", demoHouses, err)
	}
	// Idempotent: the second call keeps the existing demo house.
	again, err := store.SeedDemo(ctx, hasher, "", log)
	if err != nil || again != slug {
		t.Fatalf("second seed: slug %q, err %v; want %q", again, err, slug)
	}

	h, err := store.HouseBySlug(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	if !h.IsDemo || h.PremisesCount != 61 || h.RegistryVersion == nil || *h.RegistryVersion != 1 ||
		h.TotalAreaCenti == nil || *h.TotalAreaCenti != 300000 {
		t.Fatalf("unexpected demo house: %+v", h)
	}

	// Sum of stored weights equals the total area: nothing is lost on 1/2 and 1/3 shares.
	var num, den int64
	rows, err := pool.Query(ctx, `
		SELECT r.weight_num, r.weight_den FROM owner_records r
		JOIN registry_uploads u ON u.id = r.registry_upload_id
		JOIN houses h ON h.id = u.house_id WHERE h.invite_slug = $1`, slug)
	if err != nil {
		t.Fatal(err)
	}
	sum := new(big.Rat)
	for rows.Next() {
		if err := rows.Scan(&num, &den); err != nil {
			t.Fatal(err)
		}
		sum.Add(sum, big.NewRat(num, den))
	}
	if rows.Err() != nil || sum.Cmp(big.NewRat(300000, 1)) != 0 {
		t.Fatalf("sum of weights = %v сотых м², want 300000", sum)
	}

	if _, err := store.HouseBySlug(ctx, "no-such-slug"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// A preview version can never become current.
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO registry_uploads (house_id, version, total_area_centi, status)
			VALUES ($1, 2, 300000, 'preview')`, h.ID); err != nil {
			return err
		}

		return setCurrentVersion(ctx, tx, h.ID, 2)
	})
	if !errors.Is(err, ErrVersionNotApplied) {
		t.Fatalf("preview version became current: %v", err)
	}
}
