package initiatives

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// Registry reads houses and registry snapshots (the registry module).
type Registry interface {
	House(ctx context.Context, id string) (registry.HouseRef, error)
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
	registry   Registry
	recipients Recipients
	queue      Queue
}

// NewService creates an initiatives service.
func NewService(
	pool *pgxpool.Pool,
	tm *db.TransactionManager,
	templates Templates,
	reg Registry,
	recipients Recipients,
	queue Queue,
) *Service {
	return &Service{pool: pool, tm: tm, templates: templates, registry: reg, recipients: recipients, queue: queue}
}

// Domain errors.
var (
	// ErrNotFound means the initiative does not exist.
	ErrNotFound = errors.New("initiative not found")
	// ErrWrongStage means the transition is not allowed from the current stage.
	ErrWrongStage = errors.New("initiative stage does not allow this action")
	// ErrNotInitiator means only the initiator may do this.
	ErrNotInitiator = errors.New("only the initiator may do this")
	// ErrEmptyTitle means the title is empty or only spaces.
	ErrEmptyTitle = errors.New("initiative title is empty")
	// ErrTooManyInitiatives means the user has reached the daily limit of new initiatives.
	ErrTooManyInitiatives = errors.New("too many initiatives created in the last 24 hours")
)

// MaxInitiativesPerDay limits how many initiatives one user creates in 24 hours.
// Every initiative can start a poll that messages the owners of the house, so the
// limit keeps one account from flooding the neighbours with bot messages.
const MaxInitiativesPerDay = 10

// CreateInput starts a new initiative from a template.
type CreateInput struct {
	HouseID         string
	InitiatorUserID string
	TemplateCode    string
	Title           string
	Description     string
	// Params are the values of the template form (docs/04, решение 14). They are
	// stored as given: the MVP templates have an empty schema, which accepts any object.
	Params map[string]any
}

// CreateFromTemplate creates a draft initiative with the agenda of the template. The
// draft points to the current registry version; the poll start pins the snapshot
// (docs/04, принцип 3 и решение 2).
func (s *Service) CreateFromTemplate(ctx context.Context, in CreateInput) (Initiative, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Initiative{}, ErrEmptyTitle
	}
	description := strings.TrimSpace(in.Description)
	params := in.Params
	if params == nil {
		params = map[string]any{}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return Initiative{}, fmt.Errorf("initiative params: %w", err)
	}

	tpl, err := s.templates.TemplateByCode(ctx, in.TemplateCode)
	if err != nil {
		return Initiative{}, err
	}
	if len(tpl.Items) == 0 {
		return Initiative{}, fmt.Errorf("template %s has no items", in.TemplateCode)
	}

	snap, err := s.registry.CurrentSnapshot(ctx, in.HouseID)
	if err != nil {
		return Initiative{}, err
	}

	var created Initiative
	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var recent int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM initiatives
			WHERE initiator_user_id = $1::uuid AND created_at > now() - interval '24 hours'`,
			in.InitiatorUserID).Scan(&recent); err != nil {
			return fmt.Errorf("count recent initiatives: %w", err)
		}
		if recent >= MaxInitiativesPerDay {
			return ErrTooManyInitiatives
		}

		var descriptionValue any
		if description != "" {
			descriptionValue = description
		}

		// An owner who creates the initiative is both its author and initiator (docs/04, решение 11).
		err := tx.QueryRow(ctx, `
			INSERT INTO initiatives (house_id, template_id, title, description, params, stage,
			                         registry_upload_id, initiator_user_id, author_user_id)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'draft', $6::uuid, $7::uuid, $7::uuid)
			RETURNING id::text`, in.HouseID, tpl.ID, title, descriptionValue, paramsJSON, snap.UploadID, in.InitiatorUserID,
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
	created.Title = title
	created.Description = description
	created.Stage = "draft"
	created.RegistryUploadID = snap.UploadID
	created.RegistryVersion = snap.Version
	created.InitiatorUserID = &in.InitiatorUserID
	created.TemplateID = &tpl.ID
	for _, item := range tpl.Items {
		created.AgendaItems = append(created.AgendaItems, AgendaItem{
			Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
		})
	}

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

	house, err := s.registry.House(ctx, current.HouseID)
	if err != nil {
		return Initiative{}, err
	}
	// The snapshot is taken when the poll starts (решение 2): a draft may wait while
	// the registry is reloaded, and the votes must weigh by the version the owners see.
	snap, err := s.registry.CurrentSnapshot(ctx, current.HouseID)
	if err != nil {
		return Initiative{}, err
	}
	recipients, err := s.recipients.VerifiedOwnerRecipients(ctx, current.HouseID)
	if err != nil {
		return Initiative{}, err
	}
	recipients = invitees(recipients, house, byUserID)

	// Started in the evening, the poll reaches the owners in the morning (решение 29).
	var runAt time.Time
	if until, quiet := notify.QuietHoursEnd(time.Now(), house.Location()); quiet {
		runAt = until
	}

	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE initiatives
			SET stage = 'poll', poll_ends_at = $2, registry_upload_id = $3::uuid, updated_at = now()
			WHERE id = $1::uuid AND stage = 'draft'`, initiativeID, pollEndsAt, snap.UploadID)
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
				RunAt: runAt,
			})
		}

		return s.queue.EnqueueTx(ctx, tx, jobs...)
	})
	if err != nil {
		return Initiative{}, err
	}

	current.Stage = "poll"
	current.PollEndsAt = &pollEndsAt
	current.RegistryUploadID = snap.UploadID
	current.RegistryVersion = snap.Version

	return current, nil
}

// invitees narrows the poll audience. In the demo house every tester confirms
// themselves as an owner, so inviting all owners would deliver one tester's text to
// every member of the jury: there the poll goes to its initiator only (решение 72).
func invitees(all []Recipient, house registry.HouseRef, initiatorID string) []Recipient {
	if !house.IsDemo {
		return all
	}

	own := make([]Recipient, 0, 1)
	for _, r := range all {
		if r.UserID == initiatorID {
			own = append(own, r)
		}
	}

	return own
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

	// Filled by CreateFromTemplate and StartPoll only: the version of the registry
	// snapshot and (on creation) the agenda copied from the template.
	RegistryVersion int
	AgendaItems     []AgendaItem
}

// AgendaItem is one question of the agenda with the majority it needs.
type AgendaItem struct {
	Position     int
	Text         string
	MajorityRule string
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
