package poll

// Integration test of the whole support-poll flow against PostgreSQL 18:
// demo confirmations → initiative from the template → poll start with job
// enqueueing → weighted votes (a 1/3 share, one user with two flats, a vote
// change with the survey reset) → live progress → the queue and markers.
// Skipped without TEST_DATABASE_URL, like the other integration tests.
//
// The test can run again on the same database: its users, jobs and markers are
// unique per run and removed afterwards. Flats 7, 22 and 43 of the demo house are
// reserved for it and freed at the start.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
	"maxhackathon/backend/migrations"
)

var reservedFlats = []string{"7", "22", "43"}

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
	// Cleanups run in reverse order: the pool closes after the rows are removed.
	t.Cleanup(pool.Close)
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

	run := time.Now().UnixNano()
	probeType := fmt.Sprintf("probe-%d", run)
	botID := run
	var userIDs []string
	var initiativeID string
	t.Cleanup(func() {
		exec := func(sql string, args ...any) {
			if _, err := pool.Exec(ctx, sql, args...); err != nil {
				t.Errorf("cleanup %q: %v", sql, err)
			}
		}
		exec(`DELETE FROM jobs WHERE type = $1`, probeType)
		if initiativeID != "" {
			exec(`DELETE FROM jobs WHERE dedup_key LIKE $1`, notify.TypePollInvite+":"+initiativeID+":%")
		}
		// Poll votes and agenda items go with the initiatives, links with the users.
		exec(`DELETE FROM initiatives WHERE initiator_user_id = ANY($1::uuid[])`, userIDs)
		exec(`DELETE FROM users WHERE id = ANY($1::uuid[])`, userIDs)
		exec(`DELETE FROM bot_markers WHERE bot_user_id = $1`, botID)
	})

	if _, err := pool.Exec(ctx, `
		DELETE FROM memberships
		WHERE premise_id IN (SELECT id FROM premises WHERE house_id = $1::uuid AND number = ANY($2))`,
		house.ID, reservedFlats); err != nil {
		t.Fatal(err)
	}

	newUser := func(n int64) access.User {
		t.Helper()
		u, err := users.EnsureUser(ctx, run+n)
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)

		return u
	}
	alice, bob, carol, dave := newUser(1), newUser(2), newUser(3), newUser(4)

	// Alice and Bob are the co-owners of flat 43 (1/2 each of 48.00 м²), Carol owns
	// 1/3 of flat 7 (48.00 м²) and 1/3 of flat 22 (46.00 м²). Dave is an outsider.
	for _, c := range []struct {
		user  access.User
		flat  string
		index int
	}{{alice, "43", 1}, {bob, "43", 2}, {carol, "7", 1}, {carol, "22", 1}} {
		if _, err := users.ConfirmDemoOwner(ctx, c.user.ID, house.ID, c.flat, c.index); err != nil {
			t.Fatalf("demo owner of flat %s: %v", c.flat, err)
		}
	}

	// The same owner record cannot be confirmed by two accounts (invariant 9).
	if _, err := users.ConfirmDemoOwner(ctx, dave.ID, house.ID, "43", 1); !errors.Is(err, access.ErrOwnerTaken) {
		t.Fatalf("second account on the same owner: %v, want ErrOwnerTaken", err)
	}

	initiative, err := initService.CreateFromTemplate(ctx, initiatives.CreateInput{
		HouseID: house.ID, InitiatorUserID: alice.ID, TemplateCode: rules.TemplateVideoSurveillance,
		Title: "  Камеры в подъезде  ", Description: "3 камеры, хранение 30 дней",
		Params: map[string]any{"camera_count": 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	initiativeID = initiative.ID
	if initiative.Stage != "draft" || initiative.RegistryUploadID == "" || initiative.Description == "" ||
		initiative.Title != "Камеры в подъезде" || initiative.RegistryVersion != 1 {
		t.Fatalf("initiative = %+v", initiative)
	}
	// The agenda starts with the procedural question: the protocol names the chair and the secretary.
	if items := initiative.AgendaItems; len(items) != 2 ||
		items[0].MajorityRule != string(rules.MajorityOfParticipants) || items[1].MajorityRule != string(rules.TwoThirdsOfAll) {
		t.Fatalf("agenda = %+v", initiative.AgendaItems)
	}
	var cameras string
	if err := pool.QueryRow(ctx, `SELECT params->>'camera_count' FROM initiatives WHERE id = $1::uuid`,
		initiative.ID).Scan(&cameras); err != nil || cameras != "3" {
		t.Fatalf("stored params: camera_count = %q, %v", cameras, err)
	}

	// Решение 2: the snapshot is taken when the poll starts. The draft is pointed to
	// another version of the registry; the start moves it to the current one.
	var otherUpload string
	if err := pool.QueryRow(ctx, `
		INSERT INTO registry_uploads (house_id, version, total_area_centi, status)
		SELECT $1::uuid, max(version) + 1, 100, 'preview' FROM registry_uploads WHERE house_id = $1::uuid
		RETURNING id::text`, house.ID).Scan(&otherUpload); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM registry_uploads WHERE id = $1::uuid`, otherUpload); err != nil {
			t.Errorf("cleanup registry upload: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `UPDATE initiatives SET registry_upload_id = $2::uuid WHERE id = $1::uuid`,
		initiative.ID, otherUpload); err != nil {
		t.Fatal(err)
	}

	// Only the initiator starts the poll.
	if _, err := initService.StartPoll(ctx, initiative.ID, bob.ID, time.Time{}); !errors.Is(err, initiatives.ErrNotInitiator) {
		t.Fatalf("bob start poll: %v", err)
	}
	started, err := initService.StartPoll(ctx, initiative.ID, alice.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if started.Stage != "poll" || started.PollEndsAt == nil || started.RegistryUploadID != initiative.RegistryUploadID {
		t.Fatalf("started = %+v, want the poll on the current registry version %s", started, initiative.RegistryUploadID)
	}
	if _, err := initService.Get(ctx, "not-a-uuid"); !errors.Is(err, initiatives.ErrNotFound) {
		t.Fatalf("malformed id: %v, want ErrNotFound", err)
	}

	// Invitations: in the demo house only the initiator gets one (решение 72), the other
	// testers — Bob, Carol — do not receive Alice's text.
	for _, c := range []struct {
		user access.User
		want int
	}{{alice, 1}, {bob, 0}, {carol, 0}, {dave, 0}} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE dedup_key = $1`,
			fmt.Sprintf("%s:%s:%s", notify.TypePollInvite, initiative.ID, c.user.ID)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != c.want {
			t.Fatalf("poll_invite jobs of user %s = %d, want %d", c.user.ID, n, c.want)
		}
	}

	// Quiet hours (решение 29): a poll started at night reaches the owners at 09:00 of the house.
	var runAt time.Time
	if err := pool.QueryRow(ctx, `SELECT run_at FROM jobs WHERE dedup_key = $1`,
		fmt.Sprintf("%s:%s:%s", notify.TypePollInvite, initiative.ID, alice.ID)).Scan(&runAt); err != nil {
		t.Fatal(err)
	}
	houseRef, err := houses.House(ctx, house.ID)
	if err != nil {
		t.Fatal(err)
	}
	if until, quiet := notify.QuietHoursEnd(time.Now(), houseRef.Location()); quiet {
		if !runAt.Equal(until) {
			t.Fatalf("invitation at night: run_at = %s, want the end of the quiet hours %s", runAt, until)
		}
	} else if time.Since(runAt) > time.Minute || time.Until(runAt) > time.Minute {
		t.Fatalf("invitation in the day time: run_at = %s, want now", runAt)
	}

	vote := func(u access.User, choice string, survey *Survey) CastResult {
		t.Helper()
		res, err := polls.CastVote(ctx, CastInput{InitiativeID: initiative.ID, UserID: u.ID, Choice: choice, Survey: survey})
		if err != nil {
			t.Fatalf("vote of %s: %v", u.ID, err)
		}

		return res
	}
	surveyOf := func(u access.User) (channel *string, willing bool) {
		t.Helper()
		if err := pool.QueryRow(ctx, `
			SELECT official_channel, willing_to_help FROM poll_votes
			WHERE initiative_id = $1::uuid AND user_id = $2::uuid`,
			initiative.ID, u.ID).Scan(&channel, &willing); err != nil {
			t.Fatal(err)
		}

		return channel, willing
	}

	// Alice: 24.00 м² «за» with the survey from the mini-app.
	vote(alice, ChoiceFor, &Survey{OfficialChannel: ChannelPaper, WillingToHelp: true})
	// Pressing «Поддерживаю» in the chat again does not erase the survey.
	vote(alice, ChoiceFor, nil)
	if channel, willing := surveyOf(alice); channel == nil || *channel != ChannelPaper || !willing {
		t.Fatalf("alice survey after a repeated chat vote: channel=%v willing=%v, want kept", channel, willing)
	}

	// Carol votes with both flats: 16.00 + 15.33… = 94/3 м².
	carolVote := vote(carol, ChoiceFor, nil)
	if got := big.NewRat(carolVote.WeightNum, carolVote.WeightDen*100); got.Cmp(big.NewRat(94, 3)) != 0 ||
		!strings.Contains(carolVote.PremiseNumber, "7") || !strings.Contains(carolVote.PremiseNumber, "22") {
		t.Fatalf("carol = %+v (%v м²), want 94/3 м² for flats 7 and 22", carolVote, got)
	}

	progress, err := polls.Progress(ctx, initiative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := progress.ForM2(); got.Cmp(big.NewRat(24*3+94, 3)) != 0 {
		t.Fatalf("for = %v м², want 166/3", got)
	}
	if progress.VotesFor != 3 || progress.DemandReached() {
		t.Fatalf("progress = %+v", progress)
	}

	// Bob votes against; Alice changes her mind: the survey resets (invariant 7).
	vote(bob, ChoiceAgainst, nil)
	vote(alice, ChoiceAgainst, nil)
	if channel, willing := surveyOf(alice); channel != nil || willing {
		t.Fatalf("alice survey after «против»: channel=%v willing=%v, want reset", channel, willing)
	}

	progress, err = polls.Progress(ctx, initiative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := progress.ForM2(); got.Cmp(big.NewRat(94, 3)) != 0 {
		t.Fatalf("for after the change = %v м², want 94/3", got)
	}
	if got := progress.AgainstM2(); got.Cmp(big.NewRat(48, 1)) != 0 {
		t.Fatalf("against after the change = %v м², want 48", got)
	}
	if progress.VotesFor != 2 || progress.VotesAgainst != 2 {
		t.Fatalf("votes = %d/%d, want 2/2", progress.VotesFor, progress.VotesAgainst)
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

	testQueue(t, ctx, pool, notifier, probeType)

	// Poller markers survive a restart.
	if err := notifier.SaveMarker(ctx, botID, 42); err != nil {
		t.Fatal(err)
	}
	marker, err := notifier.LoadMarker(ctx, botID)
	if err != nil || marker != 42 {
		t.Fatalf("marker = %d, %v", marker, err)
	}
	unknown, err := notifier.LoadMarker(ctx, botID+1)
	if err != nil || unknown != 0 {
		t.Fatalf("unknown bot marker = %d, %v", unknown, err)
	}

	// The rules catalog is idempotent and readable.
	if err := catalog.SeedCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	tpl, err := catalog.TemplateByCode(ctx, rules.TemplateVideoSurveillance)
	if err != nil {
		t.Fatal(err)
	}
	if len(tpl.Items) != 2 || tpl.Items[1].MajorityRule != string(rules.TwoThirdsOfAll) ||
		!strings.Contains(tpl.Items[1].LegalReference, "ч. 1 ст. 46") {
		t.Fatalf("template = %+v", tpl)
	}

	// One account creates at most MaxInitiativesPerDay initiatives in 24 hours: each
	// may message the owners of the house.
	for i := 1; i < initiatives.MaxInitiativesPerDay; i++ {
		if _, err := initService.CreateFromTemplate(ctx, initiatives.CreateInput{
			HouseID: house.ID, InitiatorUserID: alice.ID, TemplateCode: rules.TemplateVideoSurveillance,
			Title: fmt.Sprintf("Инициатива %d", i),
		}); err != nil {
			t.Fatalf("initiative %d: %v", i, err)
		}
	}
	if _, err := initService.CreateFromTemplate(ctx, initiatives.CreateInput{
		HouseID: house.ID, InitiatorUserID: alice.ID, TemplateCode: rules.TemplateVideoSurveillance, Title: "Лишняя",
	}); !errors.Is(err, initiatives.ErrTooManyInitiatives) {
		t.Fatalf("initiative over the daily limit: %v, want ErrTooManyInitiatives", err)
	}
}

// testQueue checks deduplication and the life cycle of a job. Probe jobs are due
// long ago and claimed one at a time, so jobs of other tests are never taken.
func testQueue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, notifier *notify.Store, probeType string) {
	t.Helper()

	longAgo := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	key := func(name string) string { return probeType + ":" + name }
	enqueue := func(name string, maxAttempts int) {
		t.Helper()
		if err := notifier.Enqueue(ctx, notify.Job{Type: probeType, DedupKey: key(name), RunAt: longAgo}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE jobs SET max_attempts = $2 WHERE dedup_key = $1`, key(name), maxAttempts); err != nil {
			t.Fatal(err)
		}
	}
	claimOne := func() notify.ClaimedJob {
		t.Helper()
		claimed, err := notifier.Claim(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(claimed) != 1 || claimed[0].Type != probeType {
			t.Fatalf("claimed %+v, want one %s job", claimed, probeType)
		}

		return claimed[0]
	}
	status := func(name string) string {
		t.Helper()
		var s string
		if err := pool.QueryRow(ctx, `SELECT status FROM jobs WHERE dedup_key = $1`, key(name)).Scan(&s); err != nil {
			t.Fatal(err)
		}

		return s
	}

	// Deduplication: a second job with the same key is dropped.
	enqueue("done", 5)
	enqueue("done", 5)
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE type = $1`, probeType).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("dedup: %d jobs, want 1", n)
	}

	// queued → running → done.
	if err := notifier.Complete(ctx, claimOne().ID); err != nil {
		t.Fatal(err)
	}
	if s := status("done"); s != "done" {
		t.Fatalf("status = %s, want done", s)
	}

	// A finished job keeps the MAX id of its addressee: it is purged after 30 days.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET updated_at = now() - interval '31 days' WHERE dedup_key = $1`, key("done")); err != nil {
		t.Fatal(err)
	}
	if _, err := notifier.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE dedup_key = $1`, key("done")).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("a job finished 31 days ago was not purged")
	}

	// A failing job without attempts left fails for good.
	enqueue("failed", 1)
	if err := notifier.Retry(ctx, claimOne().ID, errors.New("boom"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if s := status("failed"); s != "failed" {
		t.Fatalf("status = %s, want failed", s)
	}

	// A job stuck in 'running' after a crash goes back to the queue, and fails once
	// its attempts are spent (a job that kills the worker must not loop forever).
	enqueue("stuck", 2)
	for _, want := range []string{"queued", "failed"} {
		stuck := claimOne()
		if _, err := pool.Exec(ctx, `UPDATE jobs SET updated_at = now() - interval '11 minutes' WHERE id = $1::uuid`, stuck.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := notifier.ResetStale(ctx); err != nil {
			t.Fatal(err)
		}
		if s := status("stuck"); s != want {
			t.Fatalf("stale job after attempt %d: status = %s, want %s", stuck.Attempts, s, want)
		}
	}
}
