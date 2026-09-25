package db

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"maxhackathon/backend/migrations"
)

// Integration test: needs a disposable PostgreSQL 18 in TEST_DATABASE_URL,
// e.g. `docker run --rm -e POSTGRES_PASSWORD=test -p 55432:5432 postgres:18-alpine`
// and TEST_DATABASE_URL=postgres://postgres:test@localhost:55432/postgres?sslmode=disable.
// Skipped when the variable is not set, so `go test ./...` works without a database.
func TestMigrateAndInvariants(t *testing.T) {
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
	// Second run must be a no-op.
	if err := Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	for _, table := range []string{
		"organizations", "users", "houses", "premises", "registry_uploads", "owners",
		"owner_records", "memberships", "org_members", "decision_types", "templates",
		"template_items", "initiatives", "agenda_items", "poll_votes", "demands",
		"meetings", "ballots", "ballot_decisions", "gis_result_entries", "meeting_results",
		"audit_logs", "registry_correction_requests",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s was not created", table)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // test cleanup

	var houseID, premiseID, ownerID, u1, u2 string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(tx.QueryRow(ctx, `INSERT INTO houses (address, region, invite_slug) VALUES ('test', 'test', 'slug-test') RETURNING id`).Scan(&houseID))
	must(tx.QueryRow(ctx, `INSERT INTO premises (house_id, number, kind) VALUES ($1, '1', 'residential') RETURNING id`, houseID).Scan(&premiseID))
	must(tx.QueryRow(ctx, `INSERT INTO owners (premise_id) VALUES ($1) RETURNING id`, premiseID).Scan(&ownerID))
	must(tx.QueryRow(ctx, `INSERT INTO users (max_user_id) VALUES (1) RETURNING id`).Scan(&u1))
	must(tx.QueryRow(ctx, `INSERT INTO users (max_user_id) VALUES (2) RETURNING id`).Scan(&u2))

	// UUIDv7: version nibble is 7.
	if !strings.HasPrefix(houseID[14:], "7") {
		t.Fatalf("id %s is not UUIDv7", houseID)
	}

	// The current registry version of a house must exist (composite FK).
	_, err = tx.Exec(ctx, "SAVEPOINT fk")
	must(err)
	if _, err = tx.Exec(ctx, `UPDATE houses SET current_registry_version = 3 WHERE id = $1`, houseID); err == nil {
		t.Fatal("current_registry_version pointing to a missing version must be rejected")
	}
	_, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT fk")
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO registry_uploads (house_id, version, total_area_centi, status) VALUES ($1, 1, 5230, 'applied')`, houseID)
	must(err)
	_, err = tx.Exec(ctx, `UPDATE houses SET current_registry_version = 1 WHERE id = $1`, houseID)
	must(err)

	insert := `INSERT INTO memberships (user_id, premise_id, owner_id, role, method, status)
	           VALUES ($1, $2, $3, 'owner', 'demo', 'verified')`
	_, err = tx.Exec(ctx, insert, u1, premiseID, ownerID)
	must(err)

	// Invariant 9: the second verified owner membership for the same owner is rejected.
	_, err = tx.Exec(ctx, "SAVEPOINT s")
	must(err)
	if _, err = tx.Exec(ctx, insert, u2, premiseID, ownerID); err == nil {
		t.Fatal("second verified membership for the same owner must be rejected")
	}
	_, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT s")
	must(err)

	// An owner membership without owner_id is rejected by the CHECK.
	if _, err = tx.Exec(ctx, `INSERT INTO memberships (user_id, premise_id, role, status) VALUES ($1, $2, 'owner', 'pending')`, u2, premiseID); err == nil {
		t.Fatal("owner membership without owner_id must be rejected")
	}
}
