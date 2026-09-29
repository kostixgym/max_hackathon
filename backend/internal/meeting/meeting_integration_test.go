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
	"fmt"
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
	orgID    string
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
	if e.orgID, err = e.houses.DemoOrgID(ctx); err != nil {
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
		if err := e.users.SetOrgStaff(ctx, u.MaxUserID, e.orgID, "operator", true); err != nil {
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

// decisionsFor gives every question of the agenda a choice, in the agenda order.
func decisionsFor(agenda []initiatives.AgendaItem, choices ...string) []Decision {
	result := make([]Decision, 0, len(agenda))
	for i, item := range agenda {
		result = append(result, Decision{AgendaItemID: item.ID, Choice: choices[i]})
	}

	return result
}

// The rest of path A: the demo accelerators, the decisions of a paper ballot, the
// preview, the write-once result and the data for the protocol.
func TestMeetingResultIntegration(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	alice, bob := e.user(t, 11), e.user(t, 12)
	if _, err := e.users.ConfirmDemoOwner(ctx, alice.ID, e.house.ID, "12", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.users.ConfirmDemoOwner(ctx, bob.ID, e.house.ID, "13", 1); err != nil {
		t.Fatal(err)
	}
	if err := e.users.SetOrgStaff(ctx, alice.MaxUserID, e.orgID, "operator", true); err != nil {
		t.Fatal(err)
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

	now := time.Now()
	view, err := e.meetings.Create(ctx, CreateInput{
		InitiativeID: initiative.ID, ByUserID: alice.ID, Form: FormGISElectronic,
		NoticeAt: now, VotingStartsAt: now.Add(time.Hour), VotingEndsAt: now.Add(2 * time.Hour),
		ChairOwnerID: owners[2].ID, SecretaryOwnerID: owners[3].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	tracker, err := e.meetings.Tracker(ctx, view.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	var bobBallot, unreceived TrackerRow
	for _, r := range tracker.Ballots {
		if r.PremiseNumber == "13" && bobBallot.BallotID == "" {
			bobBallot = r
		}
	}
	unreceived = tracker.Ballots[0]
	allFor := decisionsFor(view.Agenda, ChoiceFor, ChoiceFor)

	// Nothing is counted while the voting is on.
	if _, err := e.meetings.RecordDecisions(ctx, bobBallot.BallotID, alice.ID, allFor); !errors.Is(err, ErrVotingNotFinished) {
		t.Fatalf("decisions during the voting: %v", err)
	}
	if _, err := e.meetings.Finalize(ctx, view.ID, alice.ID); !errors.Is(err, ErrVotingNotFinished) {
		t.Fatalf("finalize during the voting: %v", err)
	}
	if _, err := e.meetings.FillBallots(ctx, view.ID, alice.ID); !errors.Is(err, ErrVotingNotFinished) {
		t.Fatalf("fill during the voting: %v", err)
	}
	if _, err := e.meetings.ReceiveBallot(ctx, view.ID, bobBallot.BallotID, alice.ID); err != nil {
		t.Fatal(err)
	}

	// The demo accelerator ends the voting now, and only for the administrator.
	if _, err := e.meetings.FinishVoting(ctx, view.ID, bob.ID); !errors.Is(err, ErrStaffOnly) {
		t.Fatalf("finish by a resident: %v", err)
	}
	finished, err := e.meetings.FinishVoting(ctx, view.ID, alice.ID)
	if err != nil || finished.Status != StatusCounting || finished.VotingEndsAt.After(time.Now()) ||
		finished.VotingStartsAt.Before(finished.NoticeAt) || !finished.VotingEndsAt.After(finished.VotingStartsAt) {
		t.Fatalf("finished: %s, %v – %v – %v, %v", finished.Status, finished.NoticeAt, finished.VotingStartsAt,
			finished.VotingEndsAt, err)
	}
	if _, err := e.meetings.ProtocolData(ctx, view.ID); !errors.Is(err, ErrNotFinalized) {
		t.Fatalf("protocol before the result: %v", err)
	}

	// Decisions: only of a ballot handed in, on every question, by the administrator;
	// a second entry corrects the first.
	for _, c := range []struct {
		name     string
		ballot   string
		user     access.User
		decision []Decision
		want     error
	}{
		{"ballot not handed in", unreceived.BallotID, alice, allFor, ErrBallotNotReceived},
		{"one question of two", bobBallot.BallotID, alice, allFor[:1], ErrInvalidDecisions},
		{"a resident", bobBallot.BallotID, bob, allFor, ErrStaffOnly},
		{"unknown ballot", "00000000-0000-7000-8000-0000000000eb", alice, allFor, ErrBallotNotFound},
	} {
		if _, err := e.meetings.RecordDecisions(ctx, c.ballot, c.user.ID, c.decision); !errors.Is(err, c.want) {
			t.Fatalf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := e.meetings.RecordDecisions(ctx, bobBallot.BallotID, alice.ID,
		decisionsFor(view.Agenda, ChoiceFor, ChoiceAgainst)); err != nil {
		t.Fatal(err)
	}
	recorded, err := e.meetings.RecordDecisions(ctx, bobBallot.BallotID, alice.ID,
		decisionsFor(view.Agenda, ChoiceFor, ChoiceAbstain))
	if err != nil || recorded.Status != BallotCounted || recorded.Decisions[1].Choice != ChoiceAbstain {
		t.Fatalf("corrected decisions: %+v, %v", recorded, err)
	}

	// The demo fill: the quorum and 2/3 of all, Bob's own decision kept, idempotent.
	if _, err := e.meetings.FillBallots(ctx, view.ID, bob.ID); !errors.Is(err, ErrStaffOnly) {
		t.Fatalf("fill by a resident: %v", err)
	}
	filled, err := e.meetings.FillBallots(ctx, view.ID, alice.ID)
	if err != nil || filled.Progress.ParticipantsM2.Cmp(big.NewRat(2400, 1)) < 0 {
		t.Fatalf("filled: %v м², %v", filled.Progress.ParticipantsM2, err)
	}
	again, err := e.meetings.FillBallots(ctx, view.ID, alice.ID)
	if err != nil || again.Progress.ParticipantsM2.Cmp(filled.Progress.ParticipantsM2) != 0 {
		t.Fatalf("second fill changed the participants: %v → %v, %v", filled.Progress.ParticipantsM2,
			again.Progress.ParticipantsM2, err)
	}
	if _, err := e.meetings.Preview(ctx, view.ID, bob.ID); !errors.Is(err, ErrStaffOnly) {
		t.Fatalf("preview by a resident: %v", err)
	}
	preview, err := e.meetings.Preview(ctx, view.ID, alice.ID)
	if err != nil || !preview.QuorumReached || !preview.Items[0].Accepted || !preview.Items[1].Accepted ||
		preview.Items[1].AbstainM2.Cmp(bobBallot.WeightM2) < 0 {
		t.Fatalf("preview: quorum %v, accepted %v/%v, abstain %v; %v", preview.QuorumReached,
			preview.Items[0].Accepted, preview.Items[1].Accepted, preview.Items[1].AbstainM2, err)
	}

	// The result is fixed once, by the administrator, and closes everything.
	if _, err := e.meetings.Finalize(ctx, view.ID, bob.ID); !errors.Is(err, ErrStaffOnly) {
		t.Fatalf("finalize by a resident: %v", err)
	}
	// Two clicks at once: one result, the other click is told it is fixed already.
	var wg sync.WaitGroup
	finals, errs := make([]Final, 2), make([]error, 2)
	for i := range 2 {
		wg.Go(func() { finals[i], errs[i] = e.meetings.Finalize(ctx, view.ID, alice.ID) })
	}
	wg.Wait()
	var final Final
	switch {
	case errs[0] == nil && errors.Is(errs[1], ErrAlreadyFinalized):
		final = finals[0]
	case errs[1] == nil && errors.Is(errs[0], ErrAlreadyFinalized):
		final = finals[1]
	default:
		t.Fatalf("concurrent finalize: %v, %v; want one result and ErrAlreadyFinalized", errs[0], errs[1])
	}
	if final.Outcome != OutcomeHeld || final.Result.ParticipantsM2.Cmp(preview.ParticipantsM2) != 0 {
		t.Fatalf("final: %+v", final)
	}
	for name, err := range map[string]error{
		"second finalize": func() error { _, err := e.meetings.Finalize(ctx, view.ID, alice.ID); return err }(),
		"decisions after": func() error {
			_, err := e.meetings.RecordDecisions(ctx, bobBallot.BallotID, alice.ID, allFor)
			return err
		}(),
		"fill after":   func() error { _, err := e.meetings.FillBallots(ctx, view.ID, alice.ID); return err }(),
		"finish after": func() error { _, err := e.meetings.FinishVoting(ctx, view.ID, alice.ID); return err }(),
	} {
		if !errors.Is(err, ErrAlreadyFinalized) {
			t.Fatalf("%s: %v, want ErrAlreadyFinalized", name, err)
		}
	}
	if stage, err := e.inits.Stage(ctx, initiative.ID); err != nil || stage != initiatives.StageCompleted {
		t.Fatalf("initiative stage = %s, %v; want completed", stage, err)
	}
	var jobs int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE dedup_key = $1`,
		notify.TypeMeetingFinalized+":"+view.ID).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("meeting_finalized jobs = %d, %v; want 1", jobs, err)
	}
	if card, err := e.meetings.Get(ctx, view.ID, bob.ID); err != nil || card.Status != StatusCompleted ||
		card.Outcome == nil || *card.Outcome != OutcomeHeld || card.FinalizedAt == nil {
		t.Fatalf("card after the result: %+v, %v", card.Meeting, err)
	}

	// Write-once (решение 61): the database refuses to change the fixed result.
	if _, err := e.pool.Exec(ctx, `UPDATE meeting_results SET accepted = NOT accepted WHERE meeting_id = $1::uuid`,
		view.ID); err == nil || !strings.Contains(err.Error(), "write-once") {
		t.Fatalf("changing the fixed result: %v, want the write-once error", err)
	}

	// Дима's read API: the protocol data and the meeting of the card.
	p, err := e.meetings.ProtocolData(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Outcome != OutcomeHeld || !p.QuorumReached || p.ParticipantsM2.Cmp(final.Result.ParticipantsM2) != 0 ||
		len(p.Items) != len(view.Agenda) || !p.Items[1].Accepted || p.Items[1].ForM2.Cmp(final.Result.Items[1].ForM2) != 0 ||
		p.Chair.FullName != owners[2].FullName || p.Secretary.FullName != owners[3].FullName || p.HouseAddress == "" {
		t.Fatalf("protocol data = %+v", p)
	}
	ref, ok, err := e.meetings.ActiveMeeting(ctx, initiative.ID)
	if err != nil || !ok || ref.ID != view.ID || ref.Status != StatusCompleted {
		t.Fatalf("active meeting = %+v, %v, %v", ref, ok, err)
	}
	if _, ok, err := e.meetings.ActiveMeeting(ctx, "00000000-0000-7000-8000-0000000000ea"); ok || err != nil {
		t.Fatalf("meeting of an unknown initiative: %v, %v", ok, err)
	}
}

// Official GIS aggregates can be corrected before finalization and are combined
// with counted paper ballots in the preview and the write-once result.
func TestGISResultsIntegration(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	alice, bob := e.user(t, 21), e.user(t, 22)
	if _, err := e.users.ConfirmDemoOwner(ctx, alice.ID, e.house.ID, "12", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.users.ConfirmDemoOwner(ctx, bob.ID, e.house.ID, "13", 1); err != nil {
		t.Fatal(err)
	}
	if err := e.users.SetOrgStaff(ctx, alice.MaxUserID, e.orgID, "operator", true); err != nil {
		t.Fatal(err)
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

	now := time.Now()
	view, err := e.meetings.Create(ctx, CreateInput{
		InitiativeID: initiative.ID, ByUserID: alice.ID, Form: FormGISElectronic,
		NoticeAt: now, VotingStartsAt: now.Add(time.Hour), VotingEndsAt: now.Add(2 * time.Hour),
		ChairOwnerID: owners[2].ID, SecretaryOwnerID: owners[3].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	tracker, err := e.meetings.Tracker(ctx, view.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	var bobBallot TrackerRow
	for _, row := range tracker.Ballots {
		if row.PremiseNumber == "13" {
			bobBallot = row
			break
		}
	}
	if bobBallot.BallotID == "" {
		t.Fatal("Bob's ballot not found")
	}
	if _, err := e.meetings.ReceiveBallot(ctx, view.ID, bobBallot.BallotID, alice.ID); err != nil {
		t.Fatal(err)
	}

	results := func(participants, officersFor, officersAgainst, officersAbstain,
		camerasFor, camerasAgainst, camerasAbstain int64,
	) GISResults {
		return GISResults{
			OnlineParticipantsM2: big.NewRat(participants, 1),
			Entries: []GISResultEntry{
				{AgendaItemID: view.Agenda[0].ID, ForM2: big.NewRat(officersFor, 1),
					AgainstM2: big.NewRat(officersAgainst, 1), AbstainM2: big.NewRat(officersAbstain, 1)},
				{AgendaItemID: view.Agenda[1].ID, ForM2: big.NewRat(camerasFor, 1),
					AgainstM2: big.NewRat(camerasAgainst, 1), AbstainM2: big.NewRat(camerasAbstain, 1)},
			},
		}
	}
	first := results(1800, 1000, 500, 300, 1600, 100, 100)
	if _, err := e.meetings.RecordGISResults(ctx, view.ID, alice.ID, first); !errors.Is(err, ErrVotingNotFinished) {
		t.Fatalf("GIS results during voting: %v, want ErrVotingNotFinished", err)
	}
	if _, err := e.meetings.FinishVoting(ctx, view.ID, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.meetings.RecordGISResults(ctx, view.ID, bob.ID, first); !errors.Is(err, ErrStaffOnly) {
		t.Fatalf("GIS results by resident: %v, want ErrStaffOnly", err)
	}
	if _, err := e.meetings.RecordGISResults(ctx, view.ID, alice.ID, first); err != nil {
		t.Fatal(err)
	}

	// A second PUT corrects all aggregates without adding duplicate rows.
	corrected := results(1800, 1100, 400, 300, 1700, 50, 50)
	saved, err := e.meetings.RecordGISResults(ctx, view.ID, alice.ID, corrected)
	if err != nil || len(saved.Entries) != 2 || saved.Entries[0].ForM2.Cmp(big.NewRat(1100, 1)) != 0 {
		t.Fatalf("corrected GIS results: %+v, %v", saved, err)
	}
	var rows, participantsNum, participantsDen int64
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM gis_result_entries WHERE meeting_id = $1::uuid`,
		view.ID).Scan(&rows); err != nil || rows != int64(len(view.Agenda)) {
		t.Fatalf("GIS rows = %d, %v", rows, err)
	}
	if err := e.pool.QueryRow(ctx, `
		SELECT online_participants_weight_num, online_participants_weight_den
		FROM meetings WHERE id = $1::uuid`, view.ID).Scan(&participantsNum, &participantsDen); err != nil ||
		participantsNum != 180000 || participantsDen != 1 {
		t.Fatalf("stored GIS participants = %d/%d, %v", participantsNum, participantsDen, err)
	}

	if _, err := e.meetings.RecordDecisions(ctx, bobBallot.BallotID, alice.ID,
		decisionsFor(view.Agenda, ChoiceFor, ChoiceFor)); err != nil {
		t.Fatal(err)
	}
	preview, err := e.meetings.Preview(ctx, view.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantParticipants := new(big.Rat).Add(big.NewRat(1800, 1), bobBallot.WeightM2)
	wantOfficersFor := new(big.Rat).Add(big.NewRat(1100, 1), bobBallot.WeightM2)
	if preview.ParticipantsM2.Cmp(wantParticipants) != 0 || preview.Items[0].ForM2.Cmp(wantOfficersFor) != 0 ||
		preview.Items[1].AgainstM2.Cmp(big.NewRat(50, 1)) != 0 || !preview.QuorumReached {
		t.Fatalf("combined preview = %+v", preview)
	}

	final, err := e.meetings.Finalize(ctx, view.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Result.ParticipantsM2.Cmp(preview.ParticipantsM2) != 0 ||
		final.Result.Items[0].ForM2.Cmp(preview.Items[0].ForM2) != 0 {
		t.Fatalf("final differs from preview: final=%+v preview=%+v", final.Result, preview)
	}
	if _, err := e.meetings.RecordGISResults(ctx, view.ID, alice.ID, corrected); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("GIS correction after finalize: %v, want ErrAlreadyFinalized", err)
	}

	// A paper-only meeting never accepts GIS aggregates.
	paperInitiative := e.demandInitiative(t, alice.ID)
	paper, err := e.meetings.Create(ctx, CreateInput{
		InitiativeID: paperInitiative.ID, ByUserID: alice.ID, Form: FormPaperAbsentee,
		NoticeAt: now, VotingStartsAt: now.Add(time.Hour), VotingEndsAt: now.Add(2 * time.Hour),
		ChairOwnerID: owners[2].ID, SecretaryOwnerID: owners[3].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.meetings.RecordGISResults(ctx, paper.ID, alice.ID, corrected); !errors.Is(err, ErrGISResultsNotAllowed) {
		t.Fatalf("GIS results for paper meeting: %v, want ErrGISResultsNotAllowed", err)
	}
}

// realHouse is an ordinary (not demo) house of its own organization with a two-flat
// registry.
type realHouse struct {
	id, orgID, uploadID string
}

// newRealHouse adds the house and removes it after the test. Create it before the
// users: cleanups run in reverse order, so their initiatives go first.
func (e *env) newRealHouse(t *testing.T) realHouse {
	t.Helper()
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	var h realHouse
	must(e.pool.QueryRow(ctx, `INSERT INTO organizations (type, name) VALUES ('uk', $1) RETURNING id::text`,
		fmt.Sprintf("УК «Тест %d»", e.run)).Scan(&h.orgID))
	must(e.pool.QueryRow(ctx, `
		INSERT INTO houses (org_id, address, region, invite_slug)
		VALUES ($1::uuid, 'г. Казань, ул. Тестовая, д. 2', 'Республика Татарстан', $2)
		RETURNING id::text`, h.orgID, fmt.Sprintf("meeting-real-%d", e.run)).Scan(&h.id))
	t.Cleanup(func() {
		for _, sql := range []string{
			`UPDATE houses SET current_registry_version = NULL WHERE id = $1::uuid`,
			`DELETE FROM registry_uploads WHERE house_id = $1::uuid`, // owner records go by cascade
			`DELETE FROM owners WHERE premise_id IN (SELECT id FROM premises WHERE house_id = $1::uuid)`,
			`DELETE FROM premises WHERE house_id = $1::uuid`,
			`DELETE FROM houses WHERE id = $1::uuid`,
		} {
			if _, err := e.pool.Exec(context.Background(), sql, h.id); err != nil {
				t.Errorf("cleanup %q: %v", sql, err)
			}
		}
		if _, err := e.pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, h.orgID); err != nil {
			t.Errorf("cleanup of the organization: %v", err)
		}
	})

	must(e.pool.QueryRow(ctx, `
		INSERT INTO registry_uploads (house_id, version, total_area_centi, status)
		VALUES ($1::uuid, 1, 10000, 'applied') RETURNING id::text`, h.id).Scan(&h.uploadID))
	_, err := e.pool.Exec(ctx, `UPDATE houses SET current_registry_version = 1 WHERE id = $1::uuid`, h.id)
	must(err)
	for i, name := range []string{"Орлов Пётр Ильич", "Зайцева Анна Олеговна"} {
		var premiseID, ownerID string
		must(e.pool.QueryRow(ctx, `
			INSERT INTO premises (house_id, number, kind, entrance, floor)
			VALUES ($1::uuid, $2, 'residential', 1, 1) RETURNING id::text`, h.id, fmt.Sprint(i+1)).Scan(&premiseID))
		must(e.pool.QueryRow(ctx, `INSERT INTO owners (premise_id) VALUES ($1::uuid) RETURNING id::text`,
			premiseID).Scan(&ownerID))
		_, err := e.pool.Exec(ctx, `
			INSERT INTO owner_records (owner_id, registry_upload_id, full_name, share_num, share_den,
			                           weight_num, weight_den, owner_kind)
			VALUES ($1::uuid, $2::uuid, $3, 1, 1, 5000, 1, 'person')`, ownerID, h.uploadID, name)
		must(err)
	}

	return h
}

// In an ordinary house the notice comes 10 days before the voting, any staff of the
// organization runs the meeting, and the demo accelerators do not exist.
func TestMeetingInRealHouseIntegration(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	h := e.newRealHouse(t)
	carol, dan := e.user(t, 21), e.user(t, 22)
	for _, u := range []access.User{carol, dan} {
		if _, err := e.pool.Exec(ctx, `INSERT INTO org_members (user_id, org_id, role) VALUES ($1::uuid, $2::uuid, 'operator')`,
			u.ID, h.orgID); err != nil {
			t.Fatal(err)
		}
	}
	in, err := e.inits.CreateFromTemplate(ctx, initiatives.CreateInput{
		HouseID: h.id, InitiatorUserID: carol.ID, TemplateCode: rules.TemplateVideoSurveillance,
		Title:  "Камеры во дворе",
		Params: json.RawMessage(`{"camera_count": 2, "payment_method": "special_assessment"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.inits.StartPoll(ctx, in.ID, carol.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	pathA := "A"
	if err := e.inits.SetStage(ctx, in.ID, initiatives.StagePoll, initiatives.StageDemand, &pathA); err != nil {
		t.Fatal(err)
	}
	owners, err := e.houses.SnapshotOwners(ctx, h.uploadID)
	if err != nil || len(owners) != 2 {
		t.Fatalf("owners = %d, %v", len(owners), err)
	}

	now := time.Now()
	input := CreateInput{
		InitiativeID: in.ID, ByUserID: dan.ID, Form: FormPaperAbsentee,
		NoticeAt: now, VotingStartsAt: now.Add(9 * 24 * time.Hour), VotingEndsAt: now.Add(20 * 24 * time.Hour),
		ChairOwnerID: owners[0].ID, SecretaryOwnerID: owners[1].ID,
	}
	var dates *DatesError
	if _, err := e.meetings.Create(ctx, input); !errors.As(err, &dates) || dates.Reason != DatesNoticePeriod {
		t.Fatalf("voting 9 days after the notice: %v, want the notice period", err)
	}

	// Dan is not the initiator but staff of the house's organization: he may run it.
	input.VotingStartsAt = now.Add(10 * 24 * time.Hour)
	view, err := e.meetings.Create(ctx, input)
	if err != nil || !view.IsAdmin || view.Status != StatusNotice || view.Progress.BallotsTotal != 2 {
		t.Fatalf("meeting of a real house: %+v, %v", view.Meeting, err)
	}
	if card, err := e.meetings.Get(ctx, view.ID, carol.ID); err != nil || !card.IsAdmin {
		t.Fatalf("the other staff member: admin %v, %v", card.IsAdmin, err)
	}
	if _, err := e.meetings.FinishVoting(ctx, view.ID, dan.ID); !errors.Is(err, ErrNotDemo) {
		t.Fatalf("finish voting in a real house: %v, want ErrNotDemo", err)
	}
	if _, err := e.meetings.FillBallots(ctx, view.ID, dan.ID); !errors.Is(err, ErrNotDemo) {
		t.Fatalf("fill ballots in a real house: %v, want ErrNotDemo", err)
	}
}
