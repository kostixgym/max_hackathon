package access

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/migrations"
)

// Integration tests of the access module (need TEST_DATABASE_URL, see
// internal/platform/db/db_test.go). They run on the synthetic demo house.

// A valid id that no house or premise has.
const unknownID = "00000000-0000-7000-8000-000000000000"

// setup migrates the test database, seeds the demo house and wires the store to the
// real registry and initiatives modules, as main does.
func setup(t *testing.T) (context.Context, *pgxpool.Pool, *Store, registry.HouseSummary) {
	t.Helper()
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
	// Cleanups run in reverse order: the pool closes after the fixture has removed its rows.
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	houses := registry.NewStore(pool)
	slug, err := houses.SeedDemo(ctx, security.NewHasher([]byte("test-secret-test-secret-test-secret")), "", log)
	if err != nil {
		t.Fatal(err)
	}
	house, err := houses.HouseBySlug(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}

	return ctx, pool, NewStore(pool, houses, initiatives.NewStore(pool)), house
}

func TestOwnerDirectoryAccess(t *testing.T) {
	ctx, pool, store, house := setup(t)
	f := newFixture(t, ctx, pool, house.ID)

	flat45 := f.premise("45")
	guest := f.user(t, "45", "guest", "pending", false)
	resident := f.user(t, "45", "resident", "verified", false)
	ownerPending := f.user(t, "44", "owner", "pending", true)
	owner45 := f.user(t, "45", "owner", "verified", true)
	staff := f.staff(t)
	outsider := f.user(t, "", "", "", false)

	premiseCases := []struct {
		name    string
		userID  string
		premise string
		allowed bool
	}{
		{"guest of the flat is refused (anyone can claim any flat)", guest, flat45, false},
		{"verified resident of the flat", resident, flat45, true},
		{"verified owner of the flat", owner45, flat45, true},
		{"pending owner of their own flat", ownerPending, f.premise("44"), true},
		{"pending owner of another flat is refused", ownerPending, flat45, false},
		{"management company staff", staff, flat45, true},
		{"user without links", outsider, flat45, false},
	}
	for _, c := range premiseCases {
		t.Run("premise owners: "+c.name, func(t *testing.T) {
			owners, err := store.PremiseOwners(ctx, c.userID, c.premise)
			if c.allowed && (err != nil || len(owners) == 0) {
				t.Fatalf("want owners, got %d, %v", len(owners), err)
			}
			if !c.allowed && !errors.Is(err, ErrForbidden) {
				t.Fatalf("want ErrForbidden, got %v", err)
			}
		})
	}

	initiatorB := f.user(t, "2", "owner", "verified", true)
	f.initiative(t, initiatorB, "B")
	initiatorA := f.user(t, "3", "owner", "verified", true)
	f.initiative(t, initiatorA, "A")

	houseCases := []struct {
		name    string
		userID  string
		allowed bool
	}{
		{"verified owner who organizes nothing is refused", owner45, false},
		{"initiator of an active path-B initiative", initiatorB, true},
		{"initiator of a path-A initiative is refused (the company organizes)", initiatorA, false},
		{"management company staff", staff, true},
		{"verified resident is refused", resident, false},
	}
	for _, c := range houseCases {
		t.Run("officer candidates: "+c.name, func(t *testing.T) {
			owners, err := store.HouseOfficerCandidates(ctx, c.userID, house.ID)
			if c.allowed && (err != nil || len(owners) == 0) {
				t.Fatalf("want candidates, got %d, %v", len(owners), err)
			}
			if !c.allowed && !errors.Is(err, ErrForbidden) {
				t.Fatalf("want ErrForbidden, got %v", err)
			}
		})
	}

	viewCases := []struct {
		name    string
		userID  string
		allowed bool
	}{
		{"verified resident", resident, true},
		{"verified owner", owner45, true},
		{"management company staff (only sums in м², решение 43)", staff, true},
		{"guest is refused", guest, false},
		{"pending owner is refused", ownerPending, false},
		{"user without links is refused", outsider, false},
	}
	for _, c := range viewCases {
		t.Run("initiatives and poll progress: "+c.name, func(t *testing.T) {
			allowed, err := store.MayViewInitiatives(ctx, c.userID, house.ID)
			if err != nil || allowed != c.allowed {
				t.Fatalf("allowed = %v, %v; want %v", allowed, err, c.allowed)
			}
		})
	}

	t.Run("poll invitations go to verified owners only", func(t *testing.T) {
		recipients, err := store.VerifiedOwnerRecipients(ctx, house.ID)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, r := range recipients {
			got[r.UserID] = true
			if r.MaxUserID <= 0 || r.OwnerID == "" {
				t.Fatalf("incomplete recipient %+v", r)
			}
		}
		// Other tests may add owners of their own: only this fixture's users are checked.
		for _, id := range []string{owner45, initiatorA, initiatorB} {
			if !got[id] {
				t.Errorf("verified owner %s is not a recipient", id)
			}
		}
		for _, id := range []string{guest, resident, ownerPending, staff, outsider} {
			if got[id] {
				t.Errorf("user %s without a verified owner link is a recipient", id)
			}
		}
	})

	t.Run("unknown premise or house is not found", func(t *testing.T) {
		if _, err := store.PremiseOwners(ctx, staff, unknownID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("premise owners: want ErrNotFound, got %v", err)
		}
		if _, err := store.HouseOfficerCandidates(ctx, staff, unknownID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("officer candidates: want ErrNotFound, got %v", err)
		}
	})
}

// The home page of the mini-app joins data of two modules: links (access) and
// premises, houses, owners (registry).
func TestMembershipsByUser(t *testing.T) {
	ctx, pool, store, house := setup(t)
	f := newFixture(t, ctx, pool, house.ID)

	userID := f.user(t, "45", "owner", "verified", true)
	f.link(t, userID, "46", "guest", "pending", false)
	f.link(t, userID, "47", "resident", "revoked", false)

	got, err := store.MembershipsByUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 links (the revoked one is hidden), got %+v", got)
	}

	owner, guest := got[0], got[1]
	if owner.Premise.Number != "45" || owner.Role != "owner" || owner.Premise.DisplayAreaCenti == nil ||
		owner.Premise.House.ID != house.ID || owner.Premise.House.Address != house.Address {
		t.Fatalf("flat 45 of the demo house expected first, got %+v", owner)
	}
	if owner.Owner == nil || owner.Owner.PremiseNumber != "45" || owner.Owner.WeightNum <= 0 ||
		!strings.HasSuffix(owner.Owner.MaskedName, ".") {
		t.Fatalf("owner facts of the current version with a masked name expected, got %+v", owner.Owner)
	}
	if guest.Premise.Number != "46" || guest.Role != "guest" || guest.Owner != nil {
		t.Fatalf("guest link to flat 46 without owner facts expected, got %+v", guest)
	}

	none, err := store.MembershipsByUser(ctx, f.user(t, "", "", "", false))
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("user without links: want an empty list, got %v, %v", none, err)
	}
}

// EnsureUser runs on every API request: a known user must be read, not written.
func TestEnsureUser(t *testing.T) {
	ctx, pool, store, _ := setup(t)
	maxID := time.Now().UnixNano()
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE max_user_id IN ($1, $2)`, maxID, maxID+1); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	first, err := store.EnsureUser(ctx, maxID)
	if err != nil {
		t.Fatal(err)
	}
	version := rowVersion(t, ctx, pool, first.ID)

	again, err := store.EnsureUser(ctx, maxID)
	if err != nil || again.ID != first.ID {
		t.Fatalf("second visit: got %q, %v; want the same user %q", again.ID, err, first.ID)
	}
	if v := rowVersion(t, ctx, pool, first.ID); v != version {
		t.Fatalf("the row of a known user was rewritten: xmin %s -> %s", version, v)
	}

	// Parallel first requests of a new user (the mini-app loads several screens at once).
	const n = 8
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			u, err := store.EnsureUser(ctx, maxID+1)
			ids[i], errs[i] = u.ID, err
		})
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("parallel first visits must end with one user: ids %v, errors %v", ids, errs)
		}
	}
}

// rowVersion returns xmin of the user's row: it changes whenever the row is written.
func rowVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID string) string {
	t.Helper()

	var xmin string
	if err := pool.QueryRow(ctx, `SELECT xmin::text FROM users WHERE id = $1::uuid`, userID).Scan(&xmin); err != nil {
		t.Fatal(err)
	}

	return xmin
}

// fixture creates users, links and initiatives on the demo house and removes them
// after the test, so the test can run again against the same database.
type fixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	houseID string
	users   []string
}

func newFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, houseID string) *fixture {
	f := &fixture{ctx: ctx, pool: pool, houseID: houseID}
	t.Cleanup(func() {
		// Memberships and staff links go with the users (ON DELETE CASCADE).
		if _, err := pool.Exec(ctx, `DELETE FROM initiatives WHERE initiator_user_id = ANY($1::uuid[])`, f.users); err != nil {
			t.Errorf("cleanup initiatives: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, f.users); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	return f
}

func (f *fixture) premise(number string) string {
	var id string
	_ = f.pool.QueryRow(f.ctx, `SELECT id::text FROM premises WHERE house_id = $1 AND number = $2`, f.houseID, number).Scan(&id)

	return id
}

// user creates a user; with a flat number it also links the user to the flat.
func (f *fixture) user(t *testing.T, flat, role, status string, withOwner bool) string {
	t.Helper()

	var userID string
	if err := f.pool.QueryRow(f.ctx, `
		INSERT INTO users (max_user_id)
		VALUES ((SELECT coalesce(max(max_user_id), 1000) + 1 FROM users)) RETURNING id::text`,
	).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	f.users = append(f.users, userID)
	if flat != "" {
		f.link(t, userID, flat, role, status, withOwner)
	}

	return userID
}

// link links the user to the flat. withOwner links an owner membership to a free
// owner of the flat.
func (f *fixture) link(t *testing.T, userID, flat, role, status string, withOwner bool) {
	t.Helper()

	var ownerID *string
	if withOwner {
		var id string
		// An owner of the flat without a verified link yet (invariant 9).
		if err := f.pool.QueryRow(f.ctx, `
			SELECT o.id::text FROM owners o
			WHERE o.premise_id = $1
			  AND NOT EXISTS (SELECT 1 FROM memberships m
			                  WHERE m.owner_id = o.id AND m.status = 'verified')
			ORDER BY o.id LIMIT 1`, f.premise(flat)).Scan(&id); err != nil {
			t.Fatalf("free owner of flat %s: %v", flat, err)
		}
		ownerID = &id
	}

	if _, err := f.pool.Exec(f.ctx, `
		INSERT INTO memberships (user_id, premise_id, owner_id, role, method, status)
		VALUES ($1, $2, $3, $4, 'demo', $5)`,
		userID, f.premise(flat), ownerID, role, status); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) staff(t *testing.T) string {
	t.Helper()

	userID := f.user(t, "", "", "", false)
	if _, err := f.pool.Exec(f.ctx, `
		INSERT INTO org_members (user_id, org_id, role)
		SELECT $1, org_id, 'operator' FROM houses WHERE id = $2`, userID, f.houseID); err != nil {
		t.Fatal(err)
	}

	return userID
}

func (f *fixture) initiative(t *testing.T, initiatorID, path string) {
	t.Helper()

	if _, err := f.pool.Exec(f.ctx, `
		INSERT INTO initiatives (house_id, title, stage, path, registry_upload_id, poll_ends_at,
		                         initiator_user_id, author_user_id)
		SELECT h.id, 'Камеры в подъезде', 'poll', $2, ru.id, now() + interval '7 days', $3, $3
		FROM houses h
		JOIN registry_uploads ru ON ru.house_id = h.id AND ru.version = h.current_registry_version
		WHERE h.id = $1`, f.houseID, path, initiatorID); err != nil {
		t.Fatal(err)
	}
}
