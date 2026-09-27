package meeting

// Integration test of the meeting against PostgreSQL 18 (TEST_DATABASE_URL): a demo
// initiative on the demand stage → the meeting with the ballots of its snapshot →
// the card, the tracker and the paper ballots.
//
// The test can run again on the same database: its users, initiatives, meetings and
// jobs are unique per run and removed afterwards. Flats 12 and 13 of the demo house
// are reserved for it and freed at the start.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

var reservedFlats = []string{"12", "13"}

// env is the wired modules of one test run.
type env struct {
	pool     *pgxpool.Pool
	houses   *registry.Store
	users    *access.Store
	inits    *initiatives.Service
	meetings *Service
	house    registry.HouseSummary
	run      int64
}

func newEnv(t *testing.T) *env {
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
	// Cleanups run in reverse order: the pool closes after the rows are removed.
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	e := &env{pool: pool, houses: registry.NewStore(pool), run: time.Now().UnixNano()}
	tm := db.NewTransactionManager(pool)
	notifier := notify.NewStore(pool)
	catalog := rules.NewCatalog(pool)
	e.users = access.NewStore(pool, e.houses, initiatives.NewStore(pool))
	e.inits = initiatives.NewService(pool, tm, catalog, e.houses, e.users, notifier)
	e.meetings = NewService(pool, tm, e.inits, e.users, e.houses, notifier)

	slug, err := e.houses.SeedDemo(ctx, security.NewHasher([]byte("test-secret-test-secret-test-secret")), "", log)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.SeedCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	if e.house, err = e.houses.HouseBySlug(ctx, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM memberships
		WHERE premise_id IN (SELECT id FROM premises WHERE house_id = $1::uuid AND number = ANY($2))`,
		e.house.ID, reservedFlats); err != nil {
		t.Fatal(err)
	}

	return e
}

// user creates an account for the run and removes it afterwards with everything it
// started: initiatives with their meetings and jobs.
func (e *env) user(t *testing.T, n int64) access.User {
	t.Helper()
	ctx := context.Background()
	u, err := e.users.EnsureUser(ctx, e.run+n)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.removeUser(t, u.ID) })

	return u
}

func (e *env) removeUser(t *testing.T, userID string) {
	ctx := context.Background()
	var initiativeIDs []string
	rows, err := e.pool.Query(ctx, `SELECT id::text FROM initiatives WHERE initiator_user_id = $1::uuid`, userID)
	if err == nil {
		initiativeIDs, err = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if err != nil {
		t.Errorf("cleanup: initiatives of %s: %v", userID, err)

		return
	}

	const meetingsOf = `(SELECT id FROM meetings WHERE initiative_id = ANY($1::uuid[]))`
	// The final result is write-once (решение 61): its trigger does not fire in a
	// replica session, which only this cleanup opens, and only for these rows.
	err = pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM meeting_results WHERE meeting_id IN `+meetingsOf, initiativeIDs)

		return err
	})
	if err != nil {
		t.Errorf("cleanup: results of %s: %v", userID, err)
	}

	// The rest with the foreign keys on: agenda items, poll votes and ballot decisions
	// go by cascade, memberships and staff roles with the user.
	for _, sql := range []string{
		`DELETE FROM jobs WHERE payload->>'meeting_id' IN (SELECT id::text FROM meetings WHERE initiative_id = ANY($1::uuid[]))`,
		`DELETE FROM jobs WHERE payload->>'initiative_id' = ANY($1::text[])`,
		`DELETE FROM gis_result_entries WHERE meeting_id IN ` + meetingsOf,
		`DELETE FROM ballots WHERE meeting_id IN ` + meetingsOf,
		`DELETE FROM meetings WHERE initiative_id = ANY($1::uuid[])`,
		`DELETE FROM initiatives WHERE id = ANY($1::uuid[])`,
	} {
		if _, err := e.pool.Exec(ctx, sql, initiativeIDs); err != nil {
			t.Errorf("cleanup %q: %v", sql, err)
		}
	}
	if _, err := e.pool.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, userID); err != nil {
		t.Errorf("cleanup: user %s: %v", userID, err)
	}
}

// demandInitiative creates a video-surveillance initiative of the initiator and moves
// it to the demand stage, as Дима's demand does.
func (e *env) demandInitiative(t *testing.T, initiatorID string) initiatives.Initiative {
	t.Helper()
	ctx := context.Background()
	in, err := e.inits.CreateFromTemplate(ctx, initiatives.CreateInput{
		HouseID: e.house.ID, InitiatorUserID: initiatorID, TemplateCode: rules.TemplateVideoSurveillance,
		Title:  "Камеры в подъездах",
		Params: json.RawMessage(`{"camera_count": 4, "payment_method": "management_bill"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.inits.StartPoll(ctx, in.ID, initiatorID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	pathA := "A"
	if err := e.inits.SetStage(ctx, in.ID, initiatives.StagePoll, initiatives.StageDemand, &pathA); err != nil {
		t.Fatal(err)
	}

	return in
}

func TestMeetingCreateAndBallotsIntegration(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// Alice owns flat 12 and is the demo staff: she runs her own initiative. Bob is a
	// resident (1/2 of flat 13), Dave the demo staff of someone else's initiative,
	// Carol an outsider.
	alice, bob, dave, carol := e.user(t, 1), e.user(t, 2), e.user(t, 3), e.user(t, 4)
	if _, err := e.users.ConfirmDemoOwner(ctx, alice.ID, e.house.ID, "12", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.users.ConfirmDemoOwner(ctx, bob.ID, e.house.ID, "13", 1); err != nil {
		t.Fatal(err)
	}
	for _, u := range []access.User{alice, dave} {
		if _, err := e.users.ConfirmDemoStaff(ctx, u.ID, e.house.ID); err != nil {
			t.Fatal(err)
		}
	}

	initiative := e.demandInitiative(t, alice.ID)
	snap, err := e.houses.CurrentSnapshot(ctx, e.house.ID)
	if err != nil {
		t.Fatal(err)
	}
	owners, err := e.houses.SnapshotOwners(ctx, snap.UploadID)
	if err != nil {
		t.Fatal(err)
	}
	chair, secretary := owners[0].ID, owners[1].ID

	now := time.Now()
	valid := CreateInput{
		InitiativeID: initiative.ID, ByUserID: alice.ID, Form: FormPaperAbsentee,
		NoticeAt: now, VotingStartsAt: now.Add(time.Minute), VotingEndsAt: now.Add(time.Hour),
		ChairOwnerID: chair, SecretaryOwnerID: secretary,
	}
	with := func(change func(*CreateInput)) CreateInput {
		in := valid
		change(&in)

		return in
	}

	// Who and what is refused before anything is written.
	for _, c := range []struct {
		name string
		in   CreateInput
		want error
	}{
		{"staff of another initiative (решение 79)", with(func(in *CreateInput) { in.ByUserID = dave.ID }), ErrStaffOnly},
		{"a resident", with(func(in *CreateInput) { in.ByUserID = bob.ID }), ErrStaffOnly},
		{"unknown initiative", with(func(in *CreateInput) { in.InitiativeID = "00000000-0000-7000-8000-0000000000ee" }), ErrInitiativeNotFound},
		{"one person as chair and secretary", with(func(in *CreateInput) { in.SecretaryOwnerID = chair }), ErrInvalidOfficers},
		{"chair outside the snapshot", with(func(in *CreateInput) { in.ChairOwnerID = "00000000-0000-7000-8000-0000000000ef" }), ErrInvalidOfficers},
		{"unknown form", with(func(in *CreateInput) { in.Form = "in_person" }), ErrInvalidForm},
		{"notice in the past", with(func(in *CreateInput) { in.NoticeAt = now.Add(-time.Hour) }), ErrInvalidDates},
	} {
		if _, err := e.meetings.Create(ctx, c.in); !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}

	// Two administrators press «Создать» at once: exactly one meeting appears.
	var wg sync.WaitGroup
	views, errs := make([]View, 2), make([]error, 2)
	for i := range 2 {
		wg.Go(func() { views[i], errs[i] = e.meetings.Create(ctx, valid) })
	}
	wg.Wait()
	var created View
	switch {
	case errs[0] == nil && errs[1] != nil:
		created = views[0]
	case errs[1] == nil && errs[0] != nil:
		created = views[1]
	default:
		t.Fatalf("concurrent creation: errs = %v, %v; want exactly one success", errs[0], errs[1])
	}
	for _, err := range errs {
		if err != nil && !errors.Is(err, ErrWrongStage) && !errors.Is(err, ErrActiveMeetingExists) {
			t.Fatalf("the losing creation: %v", err)
		}
	}
	if _, err := e.meetings.Create(ctx, valid); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("second meeting: %v, want ErrWrongStage", err)
	}

	// The card of the administrator.
	if created.Attempt != 1 || created.Status != StatusNotice || !created.IsAdmin || created.Title != initiative.Title {
		t.Fatalf("created = %+v", created.Meeting)
	}
	if created.Chair.OwnerID != chair || !strings.Contains(created.Chair.MaskedName, ".") || created.Secretary.OwnerID != secretary {
		t.Fatalf("officers = %+v, %+v", created.Chair, created.Secretary)
	}
	if len(created.Agenda) == 0 || created.Agenda[0].ID == "" {
		t.Fatalf("agenda without ids: %+v", created.Agenda)
	}
	if p := created.Progress; p.BallotsTotal != len(owners) || p.BallotsReceived != 0 || p.ParticipantsM2.Sign() != 0 ||
		p.TotalM2.Cmp(big.NewRat(3000, 1)) != 0 {
		t.Fatalf("progress = %+v, want %d ballots of 3000 м²", p, len(owners))
	}

	// Rows: a ballot per owner of the snapshot with its weight and a unique QR token,
	// the initiative on the meeting stage, the announcement job.
	var ballots, tokens int
	var num, den int64
	sum := new(big.Rat)
	rows, err := e.pool.Query(ctx, `SELECT weight_num, weight_den FROM ballots WHERE meeting_id = $1::uuid`, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		if err := rows.Scan(&num, &den); err != nil {
			t.Fatal(err)
		}
		sum.Add(sum, big.NewRat(num, den))
		ballots++
	}
	if ballots != len(owners) || sum.Cmp(big.NewRat(snap.TotalAreaCenti, 1)) != 0 {
		t.Fatalf("ballots = %d weighing %v, want %d weighing %d", ballots, sum, len(owners), snap.TotalAreaCenti)
	}
	if err := e.pool.QueryRow(ctx, `SELECT count(DISTINCT qr_token) FROM ballots WHERE meeting_id = $1::uuid`,
		created.ID).Scan(&tokens); err != nil || tokens != ballots {
		t.Fatalf("distinct QR tokens = %d, %v; want %d", tokens, err, ballots)
	}
	if stage, err := e.inits.Stage(ctx, initiative.ID); err != nil || stage != initiatives.StageMeeting {
		t.Fatalf("initiative stage = %s, %v; want meeting", stage, err)
	}
	var jobs int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE type = $1 AND dedup_key = $2`,
		notify.TypeMeetingCreated, notify.TypeMeetingCreated+":"+created.ID).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("meeting_created jobs = %d, %v; want 1", jobs, err)
	}

	// Who sees the card: residents and the staff, not outsiders.
	for _, c := range []struct {
		name  string
		user  access.User
		admin bool
		err   error
	}{
		{"resident", bob, false, nil},
		{"staff of the house", dave, false, nil},
		{"outsider", carol, false, ErrNotMember},
	} {
		v, err := e.meetings.Get(ctx, created.ID, c.user.ID)
		if !errors.Is(err, c.err) || (err == nil && v.IsAdmin != c.admin) {
			t.Fatalf("%s: admin %v, err %v", c.name, v.IsAdmin, err)
		}
	}
	for _, id := range []string{"00000000-0000-7000-8000-0000000000ed", "not-a-uuid"} {
		if _, err := e.meetings.Get(ctx, id, alice.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("meeting %q: %v, want ErrNotFound", id, err)
		}
	}

	// The tracker is the administrator's: every ballot in the round order, no choices.
	if _, err := e.meetings.Tracker(ctx, created.ID, bob.ID); !errors.Is(err, ErrStaffOnly) {
		t.Fatalf("tracker of a resident: %v", err)
	}
	tracker, err := e.meetings.Tracker(ctx, created.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracker.Ballots) != len(owners) || tracker.Ballots[0].PremiseNumber != "1" || tracker.Received != 0 {
		t.Fatalf("tracker: %d rows, first flat %s, received %d", len(tracker.Ballots), tracker.Ballots[0].PremiseNumber, tracker.Received)
	}
	var bobBallot TrackerRow
	for _, r := range tracker.Ballots {
		if r.PremiseNumber == "13" && bobBallot.BallotID == "" {
			bobBallot = r
		}
	}
	if bobBallot.Status != BallotNotVoted || bobBallot.Entrance == nil || *bobBallot.Entrance != 1 {
		t.Fatalf("flat 13 in the tracker: %+v", bobBallot)
	}

	// Paper ballots: received once, only by the administrator, only of this meeting.
	if _, err := e.meetings.ReceiveBallot(ctx, created.ID, bobBallot.BallotID, bob.ID); !errors.Is(err, ErrStaffOnly) {
		t.Fatalf("receive by a resident: %v", err)
	}
	received, err := e.meetings.ReceiveBallot(ctx, created.ID, bobBallot.BallotID, alice.ID)
	if err != nil || received.Status != BallotPaperReceived || received.ReceivedAt.IsZero() {
		t.Fatalf("receive: %+v, %v", received, err)
	}
	if _, err := e.meetings.ReceiveBallot(ctx, created.ID, bobBallot.BallotID, alice.ID); !errors.Is(err, ErrAlreadyReceived) {
		t.Fatalf("second receive: %v", err)
	}
	if _, err := e.meetings.ReceiveBallot(ctx, created.ID, "00000000-0000-7000-8000-0000000000ec", alice.ID); !errors.Is(err, ErrBallotNotFound) {
		t.Fatalf("foreign ballot: %v", err)
	}
	card, err := e.meetings.Get(ctx, created.ID, bob.ID)
	if err != nil || card.Progress.BallotsReceived != 1 || card.Progress.ParticipantsM2.Cmp(bobBallot.WeightM2) != 0 {
		t.Fatalf("progress after a ballot: %+v, %v; want 1 ballot of %s м²", card.Progress, err, bobBallot.WeightM2.FloatString(2))
	}

	// After the end of the voting a paper ballot no longer counts.
	late := *e.meetings
	late.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if v, err := late.Get(ctx, created.ID, alice.ID); err != nil || v.Status != StatusCounting {
		t.Fatalf("status after the end: %s, %v; want counting", v.Status, err)
	}
	if _, err := late.ReceiveBallot(ctx, created.ID, tracker.Ballots[0].BallotID, alice.ID); !errors.Is(err, ErrVotingFinished) {
		t.Fatalf("receive after the end: %v, want ErrVotingFinished", err)
	}
}
