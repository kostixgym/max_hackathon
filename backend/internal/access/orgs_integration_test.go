package access

// Integration tests of the staff part (К2 of docs/plan-do-30-09.md): the demo
// staff shortcut, IsStaffOf and the org list for the «Жилец / УК» switch.

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/migrations"
)

func TestDemoStaff(t *testing.T) {
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
	// t.Cleanup, not defer: the row cleanups registered below must run before the
	// pool closes, or they silently fail on a closed pool (found on a re-run).
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	houses := registry.NewStore(pool)
	slug, err := houses.SeedDemo(ctx, security.NewHasher([]byte("test-secret-test-secret-test-secret")), "staff-demo", log)
	if err != nil {
		t.Fatal(err)
	}
	house, err := houses.HouseBySlug(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}

	store := NewStore(pool, houses, nil)

	user, err := store.EnsureUser(ctx, 901)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE max_user_id = 901`)
	})

	// Not staff yet: no orgs, no access to the house as staff.
	if staff, err := store.IsStaffOf(ctx, user.ID, house.ID); err != nil || staff {
		t.Fatalf("before: staff = %v, %v", staff, err)
	}
	orgs, err := store.OrgsByUser(ctx, user.ID)
	if err != nil || len(orgs) != 0 {
		t.Fatalf("before: orgs = %v, %v", orgs, err)
	}

	// The demo shortcut makes the user an operator of the demo organization.
	org, err := store.ConfirmDemoStaff(ctx, user.ID, house.ID)
	if err != nil {
		t.Fatal(err)
	}
	if org.Role != "operator" || org.Name == "" || org.Type != "uk" {
		t.Fatalf("org = %+v", org)
	}

	// It is idempotent: the second call keeps one membership.
	if _, err := store.ConfirmDemoStaff(ctx, user.ID, house.ID); err != nil {
		t.Fatalf("second confirm: %v", err)
	}

	if staff, err := store.IsStaffOf(ctx, user.ID, house.ID); err != nil || !staff {
		t.Fatalf("after: staff = %v, %v", staff, err)
	}
	orgs, err = store.OrgsByUser(ctx, user.ID)
	if err != nil || len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Fatalf("after: orgs = %+v, %v", orgs, err)
	}

	// The bot uses this to notify the staff about a demand.
	ids, err := store.StaffMaxUserIDs(ctx, org.ID)
	if err != nil || len(ids) != 1 || ids[0] != 901 {
		t.Fatalf("staff max ids = %v, %v", ids, err)
	}

	// A foreign organization is closed: OrgByUser answers ErrForbidden.
	if _, err := store.OrgByUser(ctx, user.ID, "00000000-0000-7000-8000-0000000000ea"); err != ErrForbidden {
		t.Fatalf("foreign org: %v", err)
	}

	// An unknown house and a house without an organization are not staff houses.
	if staff, err := store.IsStaffOf(ctx, user.ID, "00000000-0000-7000-8000-0000000000eb"); err != nil || staff {
		t.Fatalf("unknown house: staff = %v, %v", staff, err)
	}
}
