package meeting

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/registry"
)

// noticePeriod: the notice of a meeting reaches the owners at least 10 days before
// it (ст. 45 ч. 4 ЖК), so the voting starts no earlier.
const noticePeriod = 10 * 24 * time.Hour

// noticeSkew tolerates the clock of the client: a notice «now» that arrives a few
// seconds late is not a date in the past.
const noticeSkew = 5 * time.Minute

// Reasons of a DatesError.
const (
	DatesRequired           = "required"
	DatesNoticeInPast       = "notice_in_past"
	DatesStartsBeforeNotice = "starts_before_notice"
	DatesEndsBeforeStarts   = "ends_before_starts"
	DatesNoticePeriod       = "notice_period"
)

// DatesError tells which rule the dates of a meeting break; it is ErrInvalidDates.
type DatesError struct {
	Reason string
}

func (e *DatesError) Error() string { return fmt.Sprintf("%v: %s", ErrInvalidDates, e.Reason) }

func (e *DatesError) Unwrap() error { return ErrInvalidDates }

// checkDates validates the dates of a new meeting. In the demo house the voting may
// start right after the notice: the jury cannot wait 10 days.
func checkDates(now time.Time, demo bool, notice, starts, ends time.Time) error {
	reason := ""
	switch {
	case notice.IsZero() || starts.IsZero() || ends.IsZero():
		reason = DatesRequired
	case notice.Before(now.Add(-noticeSkew)):
		reason = DatesNoticeInPast
	case starts.Before(notice):
		reason = DatesStartsBeforeNotice
	case !ends.After(starts):
		reason = DatesEndsBeforeStarts
	case !demo && starts.Before(notice.Add(noticePeriod)):
		reason = DatesNoticePeriod
	}
	if reason != "" {
		return &DatesError{Reason: reason}
	}

	return nil
}

// CreateInput is the form of a new meeting.
type CreateInput struct {
	InitiativeID     string
	ByUserID         string
	Form             string
	NoticeAt         time.Time
	VotingStartsAt   time.Time
	VotingEndsAt     time.Time
	ChairOwnerID     string
	SecretaryOwnerID string
}

// Create creates the meeting of an initiative that waits for it after the demand
// (path A, docs/02 шаг 3): the staff running the initiative becomes its
// administrator. In one transaction: the meeting, a ballot for every owner of the
// initiative's registry snapshot (the weight is copied, решение 2), the initiative
// moves to the meeting stage, and the bot job announces it.
func (s *Service) Create(ctx context.Context, in CreateInput) (View, error) {
	initiative, err := s.initiatives.Details(ctx, in.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		return View{}, ErrInitiativeNotFound
	}
	if err != nil {
		return View{}, err
	}
	house, err := s.registry.House(ctx, initiative.HouseID)
	if err != nil {
		return View{}, err
	}
	runs, err := s.runs(ctx, in.ByUserID, loaded{initiative: initiative, house: house})
	if err != nil {
		return View{}, err
	}
	if !runs {
		return View{}, ErrStaffOnly
	}
	if initiative.Stage != initiatives.StageDemand {
		return View{}, fmt.Errorf("%w: %s", ErrWrongStage, initiative.Stage)
	}
	if in.Form != FormGISElectronic && in.Form != FormPaperAbsentee {
		return View{}, fmt.Errorf("%w: %q", ErrInvalidForm, in.Form)
	}
	if err := checkDates(s.now(), house.IsDemo, in.NoticeAt, in.VotingStartsAt, in.VotingEndsAt); err != nil {
		return View{}, err
	}

	owners, err := s.registry.SnapshotOwners(ctx, initiative.RegistryUploadID)
	if err != nil {
		return View{}, err
	}
	if len(owners) == 0 {
		return View{}, fmt.Errorf("registry snapshot %s has no owners", initiative.RegistryUploadID)
	}
	if !officersValid(owners, in.ChairOwnerID, in.SecretaryOwnerID) {
		return View{}, ErrInvalidOfficers
	}

	ownerIDs := make([]string, len(owners))
	nums := make([]int64, len(owners))
	dens := make([]int64, len(owners))
	tokens := make([]string, len(owners))
	for i, o := range owners {
		ownerIDs[i], nums[i], dens[i] = o.ID, o.WeightNum, o.WeightDen
		// The token goes into the QR code of the paper ballot: random, not derived from ids.
		tokens[i] = rand.Text()
	}

	var meetingID string
	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// The stage moves first: it locks the initiative, so a concurrent creation waits
		// here and then fails on the stage it no longer has.
		if err := s.initiatives.SetStageTx(ctx, tx, initiative.ID,
			initiatives.StageDemand, initiatives.StageMeeting, nil); err != nil {
			if errors.Is(err, initiatives.ErrWrongStage) {
				return fmt.Errorf("%w: %w", ErrWrongStage, err)
			}

			return err
		}

		err := tx.QueryRow(ctx, `
			INSERT INTO meetings (initiative_id, attempt, form, administrator_user_id, chair_owner_id,
			                      secretary_owner_id, notice_at, voting_starts_at, voting_ends_at)
			SELECT $1::uuid, COALESCE(max(attempt), 0) + 1, $2, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8
			FROM meetings
			WHERE initiative_id = $1::uuid
			RETURNING id::text`,
			initiative.ID, in.Form, in.ByUserID, in.ChairOwnerID, in.SecretaryOwnerID,
			in.NoticeAt, in.VotingStartsAt, in.VotingEndsAt,
		).Scan(&meetingID)
		if isUniqueViolation(err, "meetings_one_active_per_initiative") {
			return ErrActiveMeetingExists
		}
		if err != nil {
			return fmt.Errorf("create meeting: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO ballots (meeting_id, owner_id, weight_num, weight_den, qr_token)
			SELECT $1::uuid, b.owner_id, b.num, b.den, b.token
			FROM unnest($2::uuid[], $3::bigint[], $4::bigint[], $5::text[]) AS b (owner_id, num, den, token)`,
			meetingID, ownerIDs, nums, dens, tokens); err != nil {
			return fmt.Errorf("create ballots: %w", err)
		}

		return s.queue.EnqueueTx(ctx, tx, notify.Job{
			Type:     notify.TypeMeetingCreated,
			DedupKey: notify.TypeMeetingCreated + ":" + meetingID,
			Payload:  map[string]any{"meeting_id": meetingID},
		})
	})
	if err != nil {
		return View{}, err
	}

	return s.Get(ctx, meetingID, in.ByUserID)
}

// officersValid: the chair and the secretary are two different owners of the
// snapshot — they sign the protocol as owners of the house.
func officersValid(owners []registry.SnapshotOwner, chairID, secretaryID string) bool {
	if chairID == secretaryID {
		return false
	}
	chair, secretary := false, false
	for _, o := range owners {
		chair = chair || o.ID == chairID
		secretary = secretary || o.ID == secretaryID
	}

	return chair && secretary
}
