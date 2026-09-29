package access

// Integration tests of the staff part (К2 of docs/plan-do-30-09.md): the demo
// staff shortcut, IsStaffOf, the org list for the «Жилец / УК» switch and the
// demo isolation of the staff (решение 79).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

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
	orgID, err := houses.DemoOrgID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	store := NewStore(pool, houses, nil)

	// MAX ids unique per run: other test packages may add their own demo staff to the
	// same demo organization at the same time.
	run := time.Now().UnixNano()
	newUser := func(n int64) User {
		t.Helper()
		u, err := store.EnsureUser(ctx, run+n)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1::uuid`, u.ID)
		})

		return u
	}
	alice, bob := newUser(1), newUser(2)

	// Not staff yet: no orgs, no access to the house as staff.
	if staff, err := store.IsStaffOf(ctx, alice.ID, house.ID); err != nil || staff {
		t.Fatalf("before: staff = %v, %v", staff, err)
	}
	orgs, err := store.OrgsByUser(ctx, alice.ID)
	if err != nil || len(orgs) != 0 {
		t.Fatalf("before: orgs = %v, %v", orgs, err)
	}

	// An administrator grants an operator role in the demo organization.
	if err := store.SetOrgStaff(ctx, alice.MaxUserID, orgID, "operator", true); err != nil {
		t.Fatal(err)
	}
	orgs, err = store.OrgsByUser(ctx, alice.ID)
	if err != nil || len(orgs) != 1 {
		t.Fatalf("staff orgs = %+v, %v", orgs, err)
	}
	org := orgs[0]
	if org.Role != "operator" || org.Name == "" || org.Type != "uk" {
		t.Fatalf("org = %+v", org)
	}

	// Granting again updates one membership rather than creating a duplicate.
	if err := store.SetOrgStaff(ctx, alice.MaxUserID, orgID, "operator", true); err != nil {
		t.Fatalf("second confirm: %v", err)
	}

	if staff, err := store.IsStaffOf(ctx, alice.ID, house.ID); err != nil || !staff {
		t.Fatalf("after: staff = %v, %v", staff, err)
	}
	orgs, err = store.OrgsByUser(ctx, alice.ID)
	if err != nil || len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Fatalf("after: orgs = %+v, %v", orgs, err)
	}

	// A foreign organization is closed: OrgByUser answers ErrForbidden.
	if _, err := store.OrgByUser(ctx, alice.ID, "00000000-0000-7000-8000-0000000000ea"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign org: %v", err)
	}

	// An unknown or malformed house is no staff house, and not a 500.
	for _, id := range []string{"00000000-0000-7000-8000-0000000000eb", "not-a-uuid"} {
		if staff, err := store.IsStaffOf(ctx, alice.ID, id); err != nil || staff {
			t.Fatalf("house %q: staff = %v, %v", id, staff, err)
		}
	}

	// Решение 79: in the demo house both testers are staff of the one demo
	// organization, but each acts and is notified only on the initiatives they lead.
	if err := store.SetOrgStaff(ctx, bob.MaxUserID, orgID, "operator", true); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		user      User
		initiator *string
		manages   bool
	}{
		{alice, &alice.ID, true},
		{bob, &alice.ID, false},
		{bob, &bob.ID, true},
		{alice, nil, false}, // the initiator deleted the account
	} {
		manages, err := store.ManagesAsStaff(ctx, c.user.ID, house.ID, c.initiator)
		if err != nil || manages != c.manages {
			t.Fatalf("demo: ManagesAsStaff(%s, initiator %v) = %v, %v; want %v", c.user.ID, c.initiator, manages, err, c.manages)
		}
	}
	ids, err := store.StaffRecipients(ctx, house.ID, &alice.ID)
	if err != nil || !slices.Equal(ids, []int64{alice.MaxUserID}) {
		t.Fatalf("demo recipients of alice's initiative = %v, %v; want only alice %d", ids, err, alice.MaxUserID)
	}

	// A real house: any staff of its organization acts on any initiative of the
	// house, and all of them are notified.
	realHouse := newRealHouse(t, ctx, store, run)
	carol, dave := newUser(3), newUser(4)
	for _, u := range []User{carol, dave} {
		if _, err := pool.Exec(ctx, `INSERT INTO org_members (user_id, org_id, role) VALUES ($1::uuid, $2::uuid, 'operator')`,
			u.ID, realHouse.orgID); err != nil {
			t.Fatal(err)
		}
	}
	if manages, err := store.ManagesAsStaff(ctx, dave.ID, realHouse.id, &carol.ID); err != nil || !manages {
		t.Fatalf("real house: dave manages carol's initiative = %v, %v; want true", manages, err)
	}
	if manages, err := store.ManagesAsStaff(ctx, alice.ID, realHouse.id, &alice.ID); err != nil || manages {
		t.Fatalf("real house: demo staff manages it = %v, %v; want false", manages, err)
	}
	ids, err = store.StaffRecipients(ctx, realHouse.id, &carol.ID)
	if err != nil || !slices.Contains(ids, carol.MaxUserID) || !slices.Contains(ids, dave.MaxUserID) || len(ids) != 2 {
		t.Fatalf("real house recipients = %v, %v; want carol and dave", ids, err)
	}
}

type testHouse struct{ id, orgID string }

// newRealHouse adds a house of an ordinary (not demo) organization and removes it
// after the test.
func newRealHouse(t *testing.T, ctx context.Context, store *Store, run int64) testHouse {
	t.Helper()
	var h testHouse
	if err := store.pool.QueryRow(ctx, `INSERT INTO organizations (type, name) VALUES ('uk', $1) RETURNING id::text`,
		fmt.Sprintf("УК «Тест %d»", run)).Scan(&h.orgID); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `
		INSERT INTO houses (org_id, address, region, invite_slug)
		VALUES ($1::uuid, 'г. Казань, ул. Тестовая, д. 1', 'Республика Татарстан', $2)
		RETURNING id::text`, h.orgID, fmt.Sprintf("real-house-%d", run)).Scan(&h.id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM houses WHERE id = $1::uuid`, h.id)
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, h.orgID)
	})

	return h
}
