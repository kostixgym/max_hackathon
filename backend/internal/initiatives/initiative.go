package initiatives

import (
	"bytes"
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
	TemplateByID(ctx context.Context, id string) (rules.CatalogTemplate, error)
	DecisionTypes(ctx context.Context, ids []string) ([]rules.DecisionType, error)
}

// Registry reads houses and registry snapshots (the registry module).
type Registry interface {
	House(ctx context.Context, id string) (registry.HouseRef, error)
	CurrentSnapshot(ctx context.Context, houseID string) (registry.Snapshot, error)
	Snapshot(ctx context.Context, uploadID string) (registry.Snapshot, error)
}

// Stages of an initiative (docs/04, решение 48).
const (
	StageDraft     = "draft"
	StagePoll      = "poll"
	StageDemand    = "demand"
	StageMeeting   = "meeting"
	StageCompleted = "completed"
	StageCanceled  = "canceled"
)

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
	// quietHoursOff sends mailings at night too (QUIET_HOURS=false); set once at start.
	quietHoursOff bool
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
	ErrNotInitiator      = errors.New("only the initiator may do this")
	ErrPollStillOpen     = errors.New("support poll is still open")
	ErrPathAlreadyChosen = errors.New("initiative path is already chosen")
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
	// Params are the values of the template form (docs/04, решение 14), a JSON object.
	// They are checked against the schema of the template and stored as given; nil is
	// an empty object.
	Params json.RawMessage
}

// CreateFromTemplate creates a draft initiative with the agenda of the template. The
// draft points to the current registry version; the poll start pins the snapshot
// (docs/04, принцип 3 и решение 2). Params that do not match the template form are a
// *rules.ParamsError.
func (s *Service) CreateFromTemplate(ctx context.Context, in CreateInput) (Initiative, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Initiative{}, ErrEmptyTitle
	}
	description := strings.TrimSpace(in.Description)
	params := in.Params
	if trimmed := bytes.TrimSpace(params); len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		params = json.RawMessage(`{}`)
	}

	tpl, err := s.templates.TemplateByCode(ctx, in.TemplateCode)
	if err != nil {
		return Initiative{}, err
	}
	if len(tpl.Items) == 0 {
		return Initiative{}, fmt.Errorf("template %s has no items", in.TemplateCode)
	}
	if err := tpl.ValidateParams(params); err != nil {
		return Initiative{}, err
	}

	snap, err := s.registry.CurrentSnapshot(ctx, in.HouseID)
	if err != nil {
		return Initiative{}, err
	}

	var created Initiative
	var itemIDs []string
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
			RETURNING id::text, created_at`, in.HouseID, tpl.ID, title, descriptionValue, string(params), snap.UploadID, in.InitiatorUserID,
		).Scan(&created.ID, &created.CreatedAt)
		if err != nil {
			return fmt.Errorf("create initiative: %w", err)
		}

		itemIDs = make([]string, len(tpl.Items))
		for i, item := range tpl.Items {
			if err = tx.QueryRow(ctx, `
				INSERT INTO agenda_items (initiative_id, position, text, decision_type_id)
				VALUES ($1::uuid, $2, $3, $4::uuid)
				RETURNING id::text`,
				created.ID, item.Position, item.Text, item.DecisionTypeID).Scan(&itemIDs[i]); err != nil {
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
	created.Stage = StageDraft
	created.RegistryUploadID = snap.UploadID
	created.RegistryVersion = snap.Version
	created.TotalAreaCenti = snap.TotalAreaCenti
	created.InitiatorUserID = &in.InitiatorUserID
	created.AuthorUserID = &in.InitiatorUserID
	created.TemplateID = &tpl.ID
	created.Template = &TemplateRef{Code: tpl.Code, Name: tpl.Name, Version: tpl.Version}
	created.Params = params
	for i, item := range tpl.Items {
		created.AgendaItems = append(created.AgendaItems, AgendaItem{
			ID: itemIDs[i], Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
			LegalReference: item.LegalReference,
		})
	}

	return created, nil
}

// StartPoll moves the draft to the poll stage and enqueues one invitation job per
// verified owner of the house and the result job for the end of the term — in one
// transaction, so an accepted request always results in mailings (docs/02, шаг 2).
func (s *Service) StartPoll(ctx context.Context, initiativeID, byUserID string, pollEndsAt time.Time) (Initiative, error) {
	current, err := s.Get(ctx, initiativeID)
	if err != nil {
		return Initiative{}, err
	}
	// The same rule shows or hides the button in the card.
	switch current.startPollBlocked(byUserID) {
	case "":
	case ReasonNotInitiator:
		return Initiative{}, ErrNotInitiator
	default:
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
	startedAt := time.Now()
	var runAt time.Time
	if until, quiet := s.quietHoursEnd(startedAt, house.Location()); quiet {
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

		jobs := make([]notify.Job, 0, 2*len(recipients)+1)
		for _, r := range recipients {
			jobs = append(jobs, notify.Job{
				Type:     notify.TypePollInvite,
				DedupKey: fmt.Sprintf("%s:%s:%s", notify.TypePollInvite, initiativeID, r.UserID),
				Payload: map[string]any{
					"initiative_id": initiativeID,
					"max_user_id":   r.MaxUserID,
					// The initiator's message has no «Есть вопрос»: they answer questions.
					"initiator": r.UserID == byUserID,
				},
				RunAt: runAt,
			})
		}

		// The poll closes by its term, and the initiator gets the result with the next
		// step (решения 18, 77). A term at night is reported in the morning.
		finishAt := pollEndsAt
		if until, quiet := s.quietHoursEnd(pollEndsAt, house.Location()); quiet {
			finishAt = until
		}
		jobs = append(jobs, notify.Job{
			Type:     notify.TypePollFinished,
			DedupKey: fmt.Sprintf("%s:%s", notify.TypePollFinished, initiativeID),
			Payload:  map[string]any{"initiative_id": initiativeID},
			RunAt:    finishAt,
		})
		// One reminder halfway through the support poll. The bot checks the owner's
		// current vote before sending, so a vote after scheduling suppresses it.
		remindAt := startedAt.Add(pollEndsAt.Sub(startedAt) / 2)
		if until, quiet := s.quietHoursEnd(remindAt, house.Location()); quiet {
			remindAt = until
		}
		for _, r := range recipients {
			if r.MaxUserID <= 0 {
				continue
			}
			jobs = append(jobs, notify.Job{
				Type:     notify.TypePollReminder,
				DedupKey: fmt.Sprintf("%s:%s:%s", notify.TypePollReminder, initiativeID, r.UserID),
				Payload:  map[string]any{"initiative_id": initiativeID, "user_id": r.UserID, "max_user_id": r.MaxUserID},
				RunAt:    remindAt,
			})
		}

		return s.queue.EnqueueTx(ctx, tx, jobs...)
	})
	if err != nil {
		return Initiative{}, err
	}

	current.Stage = StagePoll
	current.PollEndsAt = &pollEndsAt
	current.RegistryUploadID = snap.UploadID
	current.RegistryVersion = snap.Version
	current.TotalAreaCenti = snap.TotalAreaCenti

	return current, nil
}

// SetQuietHours turns the quiet hours of mailings on or off (config QUIET_HOURS). Call
// it once before serving; they are on by default.
func (s *Service) SetQuietHours(on bool) {
	s.quietHoursOff = !on
}

// quietHoursEnd is notify.QuietHoursEnd unless the quiet hours are turned off.
func (s *Service) quietHoursEnd(t time.Time, loc *time.Location) (time.Time, bool) {
	if s.quietHoursOff {
		return time.Time{}, false
	}

	return notify.QuietHoursEnd(t, loc)
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

// Polling is the part of the initiative the poll module and the bot need.
type Polling struct {
	ID               string
	HouseID          string
	Title            string
	Stage            string
	PollEndsAt       *time.Time
	RegistryUploadID string
	// InitiatorUserID: the initiator's own poll message differs — they answer
	// questions instead of asking them. Nil when the account is deleted.
	InitiatorUserID *string
}

// IsInitiator reports whether the user leads the poll.
func (p Polling) IsInitiator(userID string) bool {
	return p.InitiatorUserID != nil && *p.InitiatorUserID == userID
}

// Polling returns the poll-relevant fields of the initiative.
func (s *Service) Polling(ctx context.Context, id string) (Polling, error) {
	var p Polling
	if !validID(id) {
		return p, ErrNotFound
	}
	var pollEndsAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, house_id::text, title, stage, poll_ends_at, registry_upload_id::text, initiator_user_id::text
		FROM initiatives
		WHERE id = $1::uuid AND hidden_at IS NULL`, id,
	).Scan(&p.ID, &p.HouseID, &p.Title, &p.Stage, &pollEndsAt, &p.RegistryUploadID, &p.InitiatorUserID)
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
	Path             *string // A or B, chosen after the poll (решение 45)
	PollEndsAt       *time.Time
	RegistryUploadID string
	InitiatorUserID  *string
	AuthorUserID     *string
	CreatedAt        time.Time
	// Params are the values of the template form, a JSON object.
	Params json.RawMessage

	// Filled by CreateFromTemplate, StartPoll and Details: the registry snapshot the
	// votes and thresholds are counted by.
	RegistryVersion int
	TotalAreaCenti  int64
	// Filled by CreateFromTemplate and Details: the agenda and the template version.
	AgendaItems []AgendaItem
	Template    *TemplateRef
}

// TemplateRef is the template version an initiative was created from.
type TemplateRef struct {
	Code    string
	Name    string
	Version int
}

// AgendaItem is one question of the agenda with the majority it needs. ID is the
// agenda item row: meeting ballots record their decisions by it.
type AgendaItem struct {
	ID             string
	Position       int
	Text           string
	MajorityRule   string
	LegalReference string
}

// IsLedBy reports whether the user leads the initiative: its initiator or the author
// of the idea (docs/04, решение 11).
func (in Initiative) IsLedBy(userID string) bool {
	return (in.InitiatorUserID != nil && *in.InitiatorUserID == userID) ||
		(in.AuthorUserID != nil && *in.AuthorUserID == userID)
}

// Get returns the initiative by id. Hidden initiatives are ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (Initiative, error) {
	var in Initiative
	if !validID(id) {
		return in, ErrNotFound
	}
	var description *string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, house_id::text, template_id::text, title, description, stage, path,
		       poll_ends_at, registry_upload_id::text, initiator_user_id::text, author_user_id::text,
		       created_at, params
		FROM initiatives
		WHERE id = $1::uuid AND hidden_at IS NULL`, id,
	).Scan(&in.ID, &in.HouseID, &in.TemplateID, &in.Title, &description, &in.Stage, &in.Path,
		&in.PollEndsAt, &in.RegistryUploadID, &in.InitiatorUserID, &in.AuthorUserID,
		&in.CreatedAt, &in.Params)
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
