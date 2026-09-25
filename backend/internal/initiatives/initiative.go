package initiatives

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
)

// The initiatives module (docs/04, module 4): the life cycle of an idea. Data of
// other modules comes only through their interfaces (principle 8).

// Recipient is one addressee of a house mailing (the access module returns them).
type Recipient struct {
	UserID    string
	MaxUserID int64
	OwnerID   string
}

// Templates reads the rules module catalog.
type Templates interface {
	TemplateByCode(ctx context.Context, code string) (rules.CatalogTemplate, error)
}

// Snapshots reads registry snapshots.
type Snapshots interface {
	CurrentSnapshot(ctx context.Context, houseID string) (registry.Snapshot, error)
}

// Recipients reads the poll audience from the access module.
type Recipients interface {
	VerifiedOwnerRecipients(ctx context.Context, houseID string) ([]Recipient, error)
}

// Queue is the notify module job queue.
type Queue interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, jobs ...notify.Job) error
}

// Service owns the transaction boundary of initiative operations (docs/05:
// «границу транзакции определяет сервис через TransactionManager»).
type Service struct {
	pool       *pgxpool.Pool
	tm         *db.TransactionManager
	templates  Templates
	snapshots  Snapshots
	recipients Recipients
	queue      Queue
}

// NewService creates an initiatives service.
func NewService(
	pool *pgxpool.Pool,
	tm *db.TransactionManager,
	templates Templates,
	snapshots Snapshots,
	recipients Recipients,
	queue Queue,
) *Service {
	return &Service{pool: pool, tm: tm, templates: templates, snapshots: snapshots, recipients: recipients, queue: queue}
}

// Domain errors.
var (
	// ErrNotFound means the initiative does not exist.
	ErrNotFound = errors.New("initiative not found")
	// ErrWrongStage means the transition is not allowed from the current stage.
	ErrWrongStage = errors.New("initiative stage does not allow this action")
	// ErrNotInitiator means only the initiator may do this.
	ErrNotInitiator = errors.New("only the initiator may do this")
)

// CreateInput starts a new initiative from a template.
type CreateInput struct {
	HouseID         string
	InitiatorUserID string
	TemplateCode    string
	Title           string
	Description     string
}

// CreateFromTemplate creates a draft initiative with the agenda of the template
// and pins the current registry snapshot (docs/04, принцип 3).
func (s *Service) CreateFromTemplate(ctx context.Context, in CreateInput) (Initiative, error) {
	tpl, err := s.templates.TemplateByCode(ctx, in.TemplateCode)
	if err != nil {
		return Initiative{}, err
	}
	if len(tpl.Items) == 0 {
		return Initiative{}, fmt.Errorf("template %s has no items", in.TemplateCode)
	}

	snap, err := s.snapshots.CurrentSnapshot(ctx, in.HouseID)
	if err != nil {
		return Initiative{}, err
	}

	var created Initiative
	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var description any
		if in.Description != "" {
			description = in.Description
		}

		// An owner who creates the initiative is both its author and initiator (docs/04, решение 11).
		err := tx.QueryRow(ctx, `
			INSERT INTO initiatives (house_id, template_id, title, description, stage,
			                         registry_upload_id, initiator_user_id, author_user_id)
			VALUES ($1::uuid, $2::uuid, $3, $4, 'draft', $5::uuid, $6::uuid, $6::uuid)
			RETURNING id::text`, in.HouseID, tpl.ID, in.Title, description, snap.UploadID, in.InitiatorUserID,
		).Scan(&created.ID)
		if err != nil {
			return fmt.Errorf("create initiative: %w", err)
		}

		for _, item := range tpl.Items {
			if _, err = tx.Exec(ctx, `
				INSERT INTO agenda_items (initiative_id, position, text, decision_type_id)
				VALUES ($1::uuid, $2, $3, $4::uuid)`,
				created.ID, item.Position, item.Text, item.DecisionTypeID); err != nil {
				return fmt.Errorf("agenda item %d: %w", item.Position, err)
			}
		}

		return nil
	})
	if err != nil {
		return Initiative{}, err
	}

	created.HouseID = in.HouseID
	created.Title = in.Title
	created.Description = in.Description
	created.Stage = "draft"
	created.RegistryUploadID = snap.UploadID
	created.InitiatorUserID = &in.InitiatorUserID
	created.TemplateID = &tpl.ID

	return created, nil
}

// StartPoll moves the draft to the poll stage and enqueues one invitation job per
// verified owner of the house — in one transaction, so an accepted request always
// results in mailings (docs/02, шаг 2).
func (s *Service) StartPoll(ctx context.Context, initiativeID, byUserID string, pollEndsAt time.Time) (Initiative, error) {
	current, err := s.Get(ctx, initiativeID)
	if err != nil {
		return Initiative{}, err
	}
	if current.InitiatorUserID == nil || *current.InitiatorUserID != byUserID {
		return Initiative{}, ErrNotInitiator
	}
	if current.Stage != "draft" {
		return Initiative{}, fmt.Errorf("%w: %s", ErrWrongStage, current.Stage)
	}
	if pollEndsAt.IsZero() {
		pollEndsAt = time.Now().Add(7 * 24 * time.Hour) // решение 18: 7 дней по умолчанию
	}

	recipients, err := s.recipients.VerifiedOwnerRecipients(ctx, current.HouseID)
	if err != nil {
		return Initiative{}, err
	}

	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE initiatives
			SET stage = 'poll', poll_ends_at = $2, updated_at = now()
			WHERE id = $1::uuid AND stage = 'draft'`, initiativeID, pollEndsAt)
		if err != nil {
			return fmt.Errorf("start poll: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("%w: draft", ErrWrongStage)
		}

		jobs := make([]notify.Job, 0, len(recipients))
		for _, r := range recipients {
			jobs = append(jobs, notify.Job{
				Type:     notify.TypePollInvite,
				DedupKey: fmt.Sprintf("%s:%s:%s", notify.TypePollInvite, initiativeID, r.UserID),
				Payload: map[string]any{
					"initiative_id": initiativeID,
					"max_user_id":   r.MaxUserID,
				},
			})
		}

		return s.queue.EnqueueTx(ctx, tx, jobs...)
	})
	if err != nil {
		return Initiative{}, err
	}

	current.Stage = "poll"
	current.PollEndsAt = &pollEndsAt

	return current, nil
}

// Polling is the part of the initiative the poll module needs.
type Polling struct {
	ID               string
	HouseID          string
	Title            string
	Stage            string
	PollEndsAt       *time.Time
	RegistryUploadID string
}

// Polling returns the poll-relevant fields of the initiative.
func (s *Service) Polling(ctx context.Context, id string) (Polling, error) {
	var p Polling
	if !validID(id) {
		return p, ErrNotFound
	}
	var pollEndsAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, house_id::text, title, stage, poll_ends_at, registry_upload_id::text
		FROM initiatives
		WHERE id = $1::uuid AND hidden_at IS NULL`, id,
	).Scan(&p.ID, &p.HouseID, &p.Title, &p.Stage, &pollEndsAt, &p.RegistryUploadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("initiative polling: %w", err)
	}
	p.PollEndsAt = pollEndsAt

	return p, nil
}

// Initiative is the full read model.
type Initiative struct {
	ID               string
	HouseID          string
	TemplateID       *string
	Title            string
	Description      string
	Stage            string
	PollEndsAt       *time.Time
	RegistryUploadID string
	InitiatorUserID  *string
}

// Get returns the initiative by id. Hidden initiatives are ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (Initiative, error) {
	var in Initiative
	if !validID(id) {
		return in, ErrNotFound
	}
	var description *string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, house_id::text, template_id::text, title, description, stage,
		       poll_ends_at, registry_upload_id::text, initiator_user_id::text
		FROM initiatives
		WHERE id = $1::uuid AND hidden_at IS NULL`, id,
	).Scan(&in.ID, &in.HouseID, &in.TemplateID, &in.Title, &description, &in.Stage,
		&in.PollEndsAt, &in.RegistryUploadID, &in.InitiatorUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return in, ErrNotFound
	}
	if err != nil {
		return in, fmt.Errorf("get initiative: %w", err)
	}
	if description != nil {
		in.Description = *description
	}

	return in, nil
}

// validID reports whether id can be an initiative id. Ids come from URLs and button
// payloads: a malformed one means «not found» without a query.
func validID(id string) bool {
	var u pgtype.UUID

	return u.Scan(id) == nil && u.Valid
}
