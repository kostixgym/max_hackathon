package poll

// Integration test of the whole support-poll flow against PostgreSQL 18:
// demo confirmations → initiative from the template → poll start with job
// enqueueing → weighted votes (including a 1/3 share and a vote change with the
// survey reset) → live progress → the access rules. Skipped without
// TEST_DATABASE_URL, like the other integration tests.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
	"maxhackathon/backend/migrations"
)

func TestPollFlowIntegration(t *testing.T) {
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

	hasher := security.NewHasher([]byte("test-secret-test-secret-test-secret"))
	houses := registry.NewStore(pool)
	tm := db.NewTransactionManager(pool)
	notifier := notify.NewStore(pool)
	catalog := rules.NewCatalog(pool)
	users := access.NewStore(pool, houses, initiatives.NewStore(pool))
	initService := initiatives.NewService(pool, tm, catalog, houses, users, notifier)
	polls := NewStore(pool, initService, users, houses)

	slug, err := houses.SeedDemo(ctx, hasher, "", log)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.SeedCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	house, err := houses.HouseBySlug(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}

	// Three testers: two co-owners of flat 43 (1/2 each, 48.00 м² flat) and one
	// owner of flat 7 (1/3 share, 48.00 м² flat), plus an outsider.
	alice, err := users.EnsureUser(ctx, 101)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.EnsureUser(ctx, 102)
	if err != nil {
		t.Fatal(err)
	}
	carol, err := users.EnsureUser(ctx, 103)
	if err != nil {
		t.Fatal(err)
	}
	dave, err := users.EnsureUser(ctx, 104)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := users.ConfirmDemoOwner(ctx, alice.ID, house.ID, "43", 1); err != nil {
		t.Fatalf("alice demo owner: %v", err)
	}
	if _, err := users.ConfirmDemoOwner(ctx, bob.ID, house.ID, "43", 2); err != nil {
		t.Fatalf("bob demo owner: %v", err)
	}
	if _, err := users.ConfirmDemoOwner(ctx, carol.ID, house.ID, "7", 1); err != nil {
		t.Fatalf("carol demo owner: %v", err)
	}

	// The same owner record cannot be confirmed by two accounts (invariant 9).
	if _, err := users.ConfirmDemoOwner(ctx, dave.ID, house.ID, "43", 1); err == nil {
		t.Fatal("second account on the same owner must be rejected")
	}

	initiative, err := initService.CreateFromTemplate(ctx, initiatives.CreateInput{
		HouseID: house.ID, InitiatorUserID: alice.ID,
		TemplateCode: "cctv", Title: "Камеры в подъезде", Description: "3 камеры, хранение 30 дней",
	})
	if err != nil {
		t.Fatal(err)
	}
	if initiative.Stage != "draft" || initiative.RegistryUploadID == "" {
		t.Fatalf("initiative = %+v", initiative)
	}

	// Only the initiator starts the poll.
	if _, err := initService.StartPoll(ctx, initiative.ID, bob.ID, time.Time{}); !errors.Is(err, initiatives.ErrNotInitiator) {
		t.Fatalf("bob start poll: %v", err)
	}
	started, err := initService.StartPoll(ctx, initiative.ID, alice.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if started.Stage != "poll" || started.PollEndsAt == nil {
		t.Fatalf("started = %+v", started)
	}

	// Invitations: one job per verified owner (three at the moment of the start).
	var invitations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE type = 'poll_invite'`).Scan(&invitations); err != nil {
		t.Fatal(err)
	}
	if invitations != 3 {
		t.Fatalf("poll_invite jobs = %d, want 3", invitations)
	}

	// Votes: alice 24.00 м² for, carol 16.00 м² for (48 * 1/3).
	if _, err := polls.CastVote(ctx, CastInput{InitiativeID: initiative.ID, UserID: alice.ID, Choice: ChoiceFor,
		OfficialChannel: ChannelPaper, WillingToHelp: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := polls.CastVote(ctx, CastInput{InitiativeID: initiative.ID, UserID: carol.ID, Choice: ChoiceFor}); err != nil {
		t.Fatal(err)
	}

	progress, err := polls.Progress(ctx, initiative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := progress.ForM2(); got.Cmp(big.NewRat(40, 1)) != 0 {
		t.Fatalf("for = %v м², want 40", got)
	}
	if progress.VotesFor != 2 || progress.DemandReached() {
		t.Fatalf("progress = %+v", progress)
	}

	// Bob votes against; alice changes her mind: the survey resets (invariant 7).
	if _, err := polls.CastVote(ctx, CastInput{InitiativeID: initiative.ID, UserID: bob.ID, Choice: ChoiceAgainst}); err != nil {
		t.Fatal(err)
	}
	if _, err := polls.CastVote(ctx, CastInput{InitiativeID: initiative.ID, UserID: alice.ID, Choice: ChoiceAgainst}); err != nil {
		t.Fatal(err)
	}

	var willing bool
	var channel *string
	if err := pool.QueryRow(ctx, `
		SELECT willing_to_help, official_channel FROM poll_votes
		WHERE initiative_id = $1::uuid AND user_id = $2::uuid`,
		initiative.ID, alice.ID).Scan(&willing, &channel); err != nil {
		t.Fatal(err)
	}
	if willing || channel != nil {
		t.Fatalf("alice survey after the change: willing=%v channel=%v, want reset", willing, channel)
	}

	progress, err = polls.Progress(ctx, initiative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := progress.ForM2(); got.Cmp(big.NewRat(16, 1)) != 0 {
		t.Fatalf("for after the change = %v м², want 16", got)
	}
	if got := progress.AgainstM2(); got.Cmp(big.NewRat(48, 1)) != 0 {
		t.Fatalf("against after the change = %v м², want 48", got)
	}
	if progress.VotesFor != 1 || progress.VotesAgainst != 2 {
		t.Fatalf("votes = %d/%d", progress.VotesFor, progress.VotesAgainst)
	}

	// An outsider without a verified link cannot vote.
	if _, err := polls.CastVote(ctx, CastInput{InitiativeID: initiative.ID, UserID: dave.ID, Choice: ChoiceFor}); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("dave vote: %v", err)
	}

	// The poll freezes on the way to the meeting: votes are no longer taken.
	if _, err := pool.Exec(ctx, `UPDATE initiatives SET stage = 'meeting' WHERE id = $1::uuid`, initiative.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := polls.CastVote(ctx, CastInput{InitiativeID: initiative.ID, UserID: carol.ID, Choice: ChoiceAgainst}); !errors.Is(err, ErrPollClosed) {
		t.Fatalf("vote on a frozen poll: %v", err)
	}

	// The queue: deduplication and the claim lifecycle.
	if err := notifier.Enqueue(ctx, notify.Job{Type: "probe", DedupKey: "probe:1"}); err != nil {
		t.Fatal(err)
	}
	if err := notifier.Enqueue(ctx, notify.Job{Type: "probe", DedupKey: "probe:1"}); err != nil {
		t.Fatal(err)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE type = 'probe'`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("dedup: %d jobs, want 1", queued)
	}

	claimed, err := notifier.Claim(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var probe *string
	for i := range claimed {
		if claimed[i].Type == "probe" {
			probe = &claimed[i].ID
		}
	}
	if probe == nil {
		t.Fatalf("probe job not claimed: %+v", claimed)
	}
	if err := notifier.Complete(ctx, *probe); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1::uuid`, *probe).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "done" {
		t.Fatalf("status = %s", status)
	}

	// A failing job with one attempt budget goes to 'failed'.
	if err := notifier.Enqueue(ctx, notify.Job{Type: "probe", DedupKey: "probe:2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET max_attempts = 1 WHERE dedup_key = 'probe:2'`); err != nil {
		t.Fatal(err)
	}
	claimed, err = notifier.Claim(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("second claim = %+v", claimed)
	}
	if err := notifier.Retry(ctx, claimed[0].ID, errors.New("boom"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM jobs WHERE dedup_key = 'probe:2'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("failed status = %s", status)
	}

	// Poller markers survive a restart.
	if err := notifier.SaveMarker(ctx, 555, 42); err != nil {
		t.Fatal(err)
	}
	marker, err := notifier.LoadMarker(ctx, 555)
	if err != nil || marker != 42 {
		t.Fatalf("marker = %d, %v", marker, err)
	}
	zero, err := notifier.LoadMarker(ctx, 556)
	if err != nil || zero != 0 {
		t.Fatalf("unknown bot marker = %d, %v", zero, err)
	}

	// The rules catalog is idempotent and readable.
	if err := catalog.SeedCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	tpl, err := catalog.TemplateByCode(ctx, "cctv")
	if err != nil {
		t.Fatal(err)
	}
	if len(tpl.Items) != 1 || tpl.Items[0].MajorityRule != string(rules.TwoThirdsOfAll) {
		t.Fatalf("template = %+v", tpl)
	}

	_ = pgx.ErrNoRows // keep the import for direct row checks
}
