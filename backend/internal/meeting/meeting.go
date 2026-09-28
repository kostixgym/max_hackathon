package meeting

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
)

// Forms of the meeting (docs/04, решение 23).
const (
	FormGISElectronic = "gis_electronic"
	FormPaperAbsentee = "paper_absentee"
)

// Statuses of the meeting. The stored status stays preparation until the result is
// fixed; notice, voting and counting are computed from the dates on every read.
const (
	StatusPreparation = "preparation"
	StatusNotice      = "notice"
	StatusVoting      = "voting"
	StatusCounting    = "counting"
	StatusCompleted   = "completed"
	StatusCanceled    = "canceled"
	statusCanceling   = "canceling"
)

// Statuses of a ballot in scope until 30.09 (the online declaration is not).
const (
	BallotNotVoted      = "not_voted"
	BallotPaperReceived = "paper_received"
	BallotCounted       = "counted"
)

// Domain errors.
var (
	// ErrNotFound means there is no such meeting, or its initiative is hidden.
	ErrNotFound = errors.New("meeting not found")
	// ErrInitiativeNotFound means the meeting is created for a missing initiative.
	ErrInitiativeNotFound = errors.New("initiative not found")
	// ErrBallotNotFound means the meeting has no such ballot.
	ErrBallotNotFound = errors.New("ballot not found")
	// ErrStaffOnly means only the staff running the meeting may do this (решение 79).
	ErrStaffOnly = errors.New("only the staff running the meeting may do this")
	// ErrNotMember means the viewer is neither the staff nor a verified resident of the house.
	ErrNotMember = errors.New("the meeting is visible to the residents of the house")
	// ErrWrongStage means the initiative is not waiting for a meeting.
	ErrWrongStage = errors.New("initiative stage does not allow a meeting")
	// ErrActiveMeetingExists means the initiative already has a meeting in progress.
	ErrActiveMeetingExists = errors.New("initiative already has an active meeting")
	// ErrInvalidForm means the form of the meeting is unknown.
	ErrInvalidForm = errors.New("unknown meeting form")
	// ErrInvalidDates means the dates break the order or the notice period; *DatesError
	// tells which rule.
	ErrInvalidDates = errors.New("invalid meeting dates")
	// ErrInvalidOfficers means the chair and the secretary are not two different owners
	// of the initiative's registry snapshot.
	ErrInvalidOfficers = errors.New("chair and secretary must be two different owners of the snapshot")
	// ErrAlreadyReceived means the ballot is already marked as received.
	ErrAlreadyReceived = errors.New("ballot is already received")
	// ErrVotingFinished means the voting is over: a paper ballot received after its end
	// does not count.
	ErrVotingFinished = errors.New("voting has finished")
	// ErrVotingNotFinished means the voting is still on: nothing is counted before its end.
	ErrVotingNotFinished = errors.New("voting has not finished")
	// ErrAlreadyFinalized means the result is fixed and nothing changes any more.
	ErrAlreadyFinalized = errors.New("the result is already fixed")
	// ErrNotFinalized means the result is not fixed yet.
	ErrNotFinalized = errors.New("the result is not fixed yet")
	// ErrBallotNotReceived means the paper ballot was not handed in during the voting.
	ErrBallotNotReceived = errors.New("the paper ballot is not received")
	// ErrInvalidDecisions means the decisions of a ballot do not cover every agenda
	// question exactly once with a known choice.
	ErrInvalidDecisions = errors.New("decisions must cover every agenda question once")
	// ErrInvalidGISResults means the official online aggregates are incomplete,
	// inconsistent or outside the area of the registry snapshot.
	ErrInvalidGISResults = errors.New("invalid GIS results")
	// ErrGISResultsNotAllowed means that the meeting has no online GIS channel.
	ErrGISResultsNotAllowed = errors.New("GIS results are not allowed for this meeting form")
	// ErrNotDemo means the action exists only in the demo house.
	ErrNotDemo = errors.New("available only in the demo house")
)

// Choices of a ballot decision.
const (
	ChoiceFor     = "for"
	ChoiceAgainst = "against"
	ChoiceAbstain = "abstain"
)

// Outcomes of a fixed meeting.
const (
	OutcomeHeld     = "held"
	OutcomeNoQuorum = "no_quorum"
)

// Initiatives is the initiatives module: a meeting belongs to an initiative and moves
// its stage in the same transaction as its own rows.
type Initiatives interface {
	Details(ctx context.Context, id string) (initiatives.Initiative, error)
	SetStageTx(ctx context.Context, tx pgx.Tx, id, from, to string, path *string) error
}

// Access answers who runs a meeting and who may see it (the access module).
type Access interface {
	ManagesAsStaff(ctx context.Context, userID, houseID string, initiatorUserID *string) (bool, error)
	MayViewInitiatives(ctx context.Context, userID, houseID string) (bool, error)
}

// Registry gives the house and the owners of the initiative's registry snapshot.
type Registry interface {
	House(ctx context.Context, id string) (registry.HouseRef, error)
	SnapshotOwners(ctx context.Context, uploadID string) ([]registry.SnapshotOwner, error)
}

// Queue is the notify module job queue.
type Queue interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, jobs ...notify.Job) error
}

// Service owns the meeting tables and the transaction boundary of meeting
// operations. Data of other modules is read before a transaction starts: a pool
// call inside it would wait for a second connection while holding the first.
type Service struct {
	pool        *pgxpool.Pool
	tm          *db.TransactionManager
	initiatives Initiatives
	access      Access
	registry    Registry
	queue       Queue
	now         func() time.Time
}

// NewService creates the meeting service.
func NewService(
	pool *pgxpool.Pool,
	tm *db.TransactionManager,
	inits Initiatives,
	acc Access,
	reg Registry,
	queue Queue,
) *Service {
	return &Service{
		pool: pool, tm: tm, initiatives: inits, access: acc, registry: reg, queue: queue,
		now: time.Now,
	}
}

// Meeting is a meeting with its status by the dates.
type Meeting struct {
	ID               string
	InitiativeID     string
	Attempt          int
	Form             string
	Status           string
	ChairOwnerID     string
	SecretaryOwnerID string
	NoticeAt         time.Time
	VotingStartsAt   time.Time
	VotingEndsAt     time.Time
	Outcome          *string // held or no_quorum after the result is fixed
	FinalizedAt      *time.Time
}

// VotingOver reports whether the voting has ended by now.
func (m Meeting) VotingOver(now time.Time) bool {
	return !now.Before(m.VotingEndsAt)
}

// effectiveStatus is the status the meeting has at the moment: a fixed or canceled
// meeting keeps the stored one, otherwise the dates decide (docs/04, решение 48).
func effectiveStatus(stored string, notice, starts, ends, now time.Time) string {
	switch stored {
	case StatusCompleted, StatusCanceled, statusCanceling:
		return stored
	}

	switch {
	case now.Before(notice):
		return StatusPreparation
	case now.Before(starts):
		return StatusNotice
	case now.Before(ends):
		return StatusVoting
	default:
		return StatusCounting
	}
}

// Officer is the chair or the secretary as the meeting card shows them.
type Officer struct {
	OwnerID    string
	MaskedName string
}

// Progress is the collection of the ballots: how many are in hand and the area of
// their owners. Votes are not counted before the voting ends (решение 26).
type Progress struct {
	BallotsTotal    int
	BallotsReceived int
	ParticipantsM2  *big.Rat
	TotalM2         *big.Rat
	QuorumAboveM2   *big.Rat
}

// View is the meeting card (GET /meetings/{id}).
type View struct {
	Meeting
	Title     string
	House     registry.HouseRef
	Chair     Officer
	Secretary Officer
	Agenda    []initiatives.AgendaItem
	Progress  Progress
	// IsAdmin: the viewer runs the meeting.
	IsAdmin bool
}

// querier is a pool or a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// loaded is a meeting with the initiative and the house it belongs to: access is
// checked by them.
type loaded struct {
	meeting    Meeting
	initiative initiatives.Initiative
	house      registry.HouseRef
}

func (s *Service) load(ctx context.Context, meetingID string) (loaded, error) {
	m, err := s.meeting(ctx, s.pool, meetingID)
	if err != nil {
		return loaded{}, err
	}
	in, err := s.initiatives.Details(ctx, m.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		// A hidden initiative hides its meetings too.
		return loaded{}, ErrNotFound
	}
	if err != nil {
		return loaded{}, err
	}
	house, err := s.registry.House(ctx, in.HouseID)
	if err != nil {
		return loaded{}, err
	}

	return loaded{meeting: m, initiative: in, house: house}, nil
}

// runs reports whether the user runs the meeting: the staff of the house's
// organization who acts on the initiative (решение 79).
func (s *Service) runs(ctx context.Context, userID string, l loaded) (bool, error) {
	if l.initiative.Path != nil && *l.initiative.Path == "B" &&
		l.initiative.InitiatorUserID != nil && *l.initiative.InitiatorUserID == userID {
		return true, nil
	}
	return s.access.ManagesAsStaff(ctx, userID, l.house.ID, l.initiative.InitiatorUserID)
}

func (s *Service) meeting(ctx context.Context, q querier, id string) (Meeting, error) {
	var m Meeting
	if !validID(id) {
		return m, ErrNotFound
	}
	var stored string
	err := q.QueryRow(ctx, `
		SELECT id::text, initiative_id::text, attempt, form, status, chair_owner_id::text,
		       secretary_owner_id::text, notice_at, voting_starts_at, voting_ends_at, outcome, finalized_at
		FROM meetings
		WHERE id = $1::uuid`, id,
	).Scan(&m.ID, &m.InitiativeID, &m.Attempt, &m.Form, &stored, &m.ChairOwnerID,
		&m.SecretaryOwnerID, &m.NoticeAt, &m.VotingStartsAt, &m.VotingEndsAt, &m.Outcome, &m.FinalizedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, fmt.Errorf("get meeting: %w", err)
	}
	m.Status = effectiveStatus(stored, m.NoticeAt, m.VotingStartsAt, m.VotingEndsAt, s.now())

	return m, nil
}

// ballot is a ballot row: the weight is copied from the snapshot when the meeting is
// created, in hundredths of м² as an exact fraction.
type ballot struct {
	ID         string
	OwnerID    string
	Status     string
	Weight     registry.Weight
	ReceivedAt *time.Time
}

// inHand reports whether the paper ballot has been handed in.
func (b ballot) inHand() bool {
	return b.Status == BallotPaperReceived || b.Status == BallotCounted
}

func (s *Service) ballots(ctx context.Context, q querier, meetingID string) ([]ballot, error) {
	rows, err := q.Query(ctx, `
		SELECT id::text, owner_id::text, status, weight_num, weight_den, received_at
		FROM ballots
		WHERE meeting_id = $1::uuid`, meetingID)
	if err != nil {
		return nil, fmt.Errorf("list ballots: %w", err)
	}
	defer rows.Close()

	result := make([]ballot, 0)
	for rows.Next() {
		var b ballot
		if err := rows.Scan(&b.ID, &b.OwnerID, &b.Status, &b.Weight.Num, &b.Weight.Den, &b.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scan ballot: %w", err)
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ballots: %w", err)
	}

	return result, nil
}

// progress sums the ballots in hand against the total area of the snapshot.
func progress(ballots []ballot, totalAreaCenti int64) Progress {
	p := Progress{
		BallotsTotal:   len(ballots),
		ParticipantsM2: new(big.Rat),
		TotalM2:        registry.CentiToM2(totalAreaCenti),
	}
	for _, b := range ballots {
		if b.inHand() {
			p.BallotsReceived++
			p.ParticipantsM2.Add(p.ParticipantsM2, b.Weight.M2())
		}
	}
	p.QuorumAboveM2 = rules.ForTotal(p.TotalM2).QuorumAbove

	return p
}

// validID reports whether id can be a UUID; ids come from URLs.
func validID(id string) bool {
	var u pgtype.UUID

	return u.Scan(id) == nil && u.Valid
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
