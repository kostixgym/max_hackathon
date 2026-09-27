package registry

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"os"
	"strconv"
	"testing"

	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/migrations"
)

// Integration test (TEST_DATABASE_URL): the owners of the demo snapshot are the
// ballots of a meeting (Г1 of docs/plan-do-30-09.md).
func TestSnapshotOwners(t *testing.T) {
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
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	store := NewStore(pool)
	slug, err := store.SeedDemo(ctx, security.NewHasher([]byte("test-secret-test-secret-test-secret")), "", log)
	if err != nil {
		t.Fatal(err)
	}
	house, err := store.HouseBySlug(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := store.CurrentSnapshot(ctx, house.ID)
	if err != nil {
		t.Fatal(err)
	}

	owners, err := store.SnapshotOwners(ctx, snap.UploadID)
	if err != nil {
		t.Fatal(err)
	}

	// Every owner record of the version, and nothing lost on 1/2 and 1/3 shares: the
	// weights sum to the total area.
	var records int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM owner_records WHERE registry_upload_id = $1::uuid`,
		snap.UploadID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if len(owners) != records {
		t.Fatalf("owners = %d, want all %d records of the version", len(owners), records)
	}
	sum := new(big.Rat)
	for _, o := range owners {
		sum.Add(sum, big.NewRat(o.WeightNum, o.WeightDen))
	}
	if sum.Cmp(big.NewRat(snap.TotalAreaCenti, 1)) != 0 {
		t.Fatalf("sum of weights = %v, want the total %d", sum, snap.TotalAreaCenti)
	}

	// Entrance order; within an entrance flats by number, then other premises (the
	// shop Н1 of entrance 1).
	first, last := owners[0], owners[len(owners)-1]
	if first.Entrance == nil || *first.Entrance != 1 || first.PremiseNumber != "1" {
		t.Fatalf("first owner = flat %s, entrance %v; want flat 1 of entrance 1", first.PremiseNumber, first.Entrance)
	}
	if last.PremiseNumber != "60" || last.Kind != "municipality" {
		t.Fatalf("last owner = %+v; want the municipal flat 60", last)
	}
	for i := 1; i < len(owners); i++ {
		prev, cur := owners[i-1], owners[i]
		if *prev.Entrance > *cur.Entrance {
			t.Fatalf("entrance %d is listed before entrance %d", *prev.Entrance, *cur.Entrance)
		}
		if *prev.Entrance != *cur.Entrance {
			continue
		}
		prevN, prevNumeric := flatNumber(prev.PremiseNumber)
		curN, curNumeric := flatNumber(cur.PremiseNumber)
		if (!prevNumeric && curNumeric) || (prevNumeric && curNumeric && prevN > curN) {
			t.Fatalf("premise %s is listed before %s", prev.PremiseNumber, cur.PremiseNumber)
		}
	}
	var shop SnapshotOwner
	for i, o := range owners {
		if o.PremiseNumber == "Н1" {
			shop = o
			if next := owners[i+1]; next.PremiseNumber != "21" {
				t.Fatalf("after the shop comes flat %s, want 21 of the next entrance", next.PremiseNumber)
			}
		}
	}
	if shop.Kind != "organization" {
		t.Fatalf("the shop Н1 is missing: %+v", shop)
	}

	if none, err := store.SnapshotOwners(ctx, "00000000-0000-7000-8000-0000000000ff"); err != nil || len(none) != 0 {
		t.Fatalf("unknown version: %v, %v; want no owners", none, err)
	}
}

// flatNumber parses a numeric premise number; false for others like "Н1".
func flatNumber(s string) (int, bool) {
	n, err := strconv.Atoi(s)

	return n, err == nil
}
