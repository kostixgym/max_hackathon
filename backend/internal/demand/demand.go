// Package demand is the «Требование в УК» module (docs/04, module 6): the owners'
// demand that the management company holds a general meeting (ст. 45 ч. 6 ЖК,
// путь A). The demand pins the support in м² that reached the 10% threshold and
// starts the 45-day term of the company (Д1 of docs/plan-do-30-09.md).
package demand

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/documents"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/meeting"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
)

// Channels of the meeting the demand asks for.
const (
	ChannelPaper        = "paper"
	ChannelGosuslugiDom = "gosuslugi_dom"
)

// Errors of the demand flow, mapped to API codes by the adapter.
var (
	ErrNotFound           = errors.New("demand not found")
	ErrNotInitiator       = errors.New("only the initiator may do this")
	ErrForbidden          = errors.New("no access to this demand")
	ErrExists             = errors.New("the demand already exists")
	ErrSupportNotReached  = errors.New("support is below 10% of the house area")
	ErrWrongStage         = errors.New("initiative stage does not allow this action")
	ErrAlreadyDelivered   = errors.New("the demand is already delivered")
	ErrInvalidChannel     = errors.New("invalid channel")
	ErrInvalidDeliveredAt = errors.New("delivered_at is in the past")
)

// PollProgress reads the poll support (the poll module).
type PollProgress interface {
	Progress(ctx context.Context, initiativeID string) (poll.Progress, error)
}

// Initiatives reads the initiative and moves its stage in the caller's transaction.
type Initiatives interface {
	Details(ctx context.Context, id string) (initiatives.Initiative, error)
	SetStageTx(ctx context.Context, tx pgx.Tx, id, from, to string, path *string) error
}

// Access answers who acts as the management company (the access module).
type Access interface {
	ManagesAsStaff(ctx context.Context, userID, houseID string, initiatorUserID *string) (bool, error)
}

// Meetings tells whether the initiative already has a meeting (the meeting module).
type Meetings interface {
	ActiveMeeting(ctx context.Context, initiativeID string) (meeting.Ref, bool, error)
}

// Registry reads the house, the organization and the full names of the owners
// (the registry module): the PDF names the initiator in full.
type Registry interface {
	House(ctx context.Context, id string) (registry.HouseRef, error)
	OrgsByIDs(ctx context.Context, ids []string) ([]registry.Org, error)
	Owners(ctx context.Context, ids []string) ([]registry.Owner, error)
}

// Verified links of the user (the access module).
type Verified interface {
	VerifiedOwnerIn(ctx context.Context, userID, houseID string) ([]access.OwnerLink, error)
}

// Documents renders the PDF: a pure function of the documents module.
type Documents func(data documents.DemandData) ([]byte, error)

// Demand is the read model of the demand.
type Demand struct {
	ID           string
	InitiativeID string
	HouseID      string
	Channel      string
	Status       string // draft / delivered
	SupportNum   int64  // поддержка «за», сотые м² точной дробью
	SupportDen   int64
	CreatedAt    time.Time
	DeliveredAt  *time.Time
	UKDueAt      *time.Time
	// Overdue считается при чтении (решение плана): срок 45 дней прошёл, а собрания нет.
	Overdue bool
}

// Queue is the notify module job queue.
type Queue interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, jobs ...notify.Job) error
}

// Service owns the demands table and the transaction boundary of the flow.
type Service struct {
	pool        *pgxpool.Pool
	tm          *db.TransactionManager
	initiatives Initiatives
	polls       PollProgress
	access      Access
	meetings    Meetings
	registry    Registry
	verified    Verified
	documents   Documents
	queue       Queue
}

// NewService creates a demand service.
func NewService(pool *pgxpool.Pool, tm *db.TransactionManager, initiatives Initiatives,
	polls PollProgress, access Access, meetings Meetings, registry Registry,
	verified Verified, documents Documents, queue Queue) *Service {
	return &Service{pool: pool, tm: tm, initiatives: initiatives, polls: polls,
		access: access, meetings: meetings, registry: registry, verified: verified,
		documents: documents, queue: queue}
}

// PDF builds the demand document. Access rules are the same as Get.
func (s *Service) PDF(ctx context.Context, demandID, viewerID string) ([]byte, error) {
	d, err := s.Get(ctx, demandID, viewerID)
	if err != nil {
		return nil, err
	}

	in, err := s.initiatives.Details(ctx, d.InitiativeID)
	if err != nil {
		return nil, err
	}
	house, err := s.registry.House(ctx, in.HouseID)
	if err != nil {
		return nil, err
	}
	var orgName string
	if house.OrgID != nil {
		if orgs, err := s.registry.OrgsByIDs(ctx, []string{*house.OrgID}); err == nil && len(orgs) == 1 {
			orgName = orgs[0].Name
		}
	}

	progress, err := s.polls.Progress(ctx, d.InitiativeID)
	if err != nil {
		return nil, err
	}

	// Полное ФИО инициатора — из реестра снимка (для документа; в приложении — маска).
	initiatorName, premise := "инициатор", "—"
	if in.InitiatorUserID != nil {
		if links, err := s.verified.VerifiedOwnerIn(ctx, *in.InitiatorUserID, in.HouseID); err == nil && len(links) > 0 {
			ownerIDs := make([]string, 0, len(links))
			for _, l := range links {
				ownerIDs = append(ownerIDs, l.OwnerID)
			}
			if owners, err := s.registry.Owners(ctx, ownerIDs); err == nil && len(owners) > 0 {
				initiatorName = owners[0].FullName
				premise = owners[0].PremiseNumber
			}
		}
	}

	thresholds := progress.Thresholds()
	data := documents.DemandData{
		OrgName:       orgName,
		HouseAddress:  house.Address,
		InitiatorName: initiatorName,
		PremiseNumber: premise,
		Title:         in.Title,
		Agenda:        agendaTexts(in.AgendaItems),
		SupportM2:     registry.FormatM2(progress.ForM2()),
		ThresholdM2:   registry.FormatM2(thresholds.Demand),
		Percent:       documents.PercentString(progress.ForM2(), progress.TotalM2()),
		Channel:       channelText(d.Channel),
		CreatedAt:     time.Now().Format("02.01.2006"),
	}

	return s.documents(data)
}

func agendaTexts(items []initiatives.AgendaItem) []string {
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}

	return texts
}

func channelText(channel string) string {
	if channel == ChannelGosuslugiDom {
		return "Госуслуги.Дом"
	}

	return "бумажные бюллетени"
}

// Create checks the 10% support and records the demand, moving the initiative
// poll → demand (путь A) in one transaction.
func (s *Service) Create(ctx context.Context, initiativeID, byUserID, channel string) (Demand, error) {
	if channel != ChannelPaper && channel != ChannelGosuslugiDom {
		return Demand{}, fmt.Errorf("%w: %q", ErrInvalidChannel, channel)
	}

	in, err := s.initiatives.Details(ctx, initiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		return Demand{}, ErrNotFound
	}
	if err != nil {
		return Demand{}, err
	}
	if !in.IsLedBy(byUserID) {
		return Demand{}, ErrNotInitiator
	}

	// Повторное требование — «уже есть», независимо от стадии: так контракт
	// отвечает и на «стадия ушла дальше», и на гонку двух одновременных созданий.
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM demands WHERE initiative_id = $1::uuid)`, initiativeID).Scan(&exists); err != nil {
		return Demand{}, fmt.Errorf("check demand exists: %w", err)
	}
	if exists {
		return Demand{}, ErrExists
	}

	if in.Stage != initiatives.StagePoll {
		return Demand{}, fmt.Errorf("%w: %s", ErrWrongStage, in.Stage)
	}

	progress, err := s.polls.Progress(ctx, initiativeID)
	if err != nil {
		return Demand{}, err
	}
	if !progress.DemandReached() {
		return Demand{}, ErrSupportNotReached
	}

	pathA := "A"

	var d Demand
	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Гонка двух создателей требования: SetStageTx требует стадию poll,
		// проигравший получает ErrExists, а не сырую ошибку стадии.
		err := s.initiatives.SetStageTx(ctx, tx, initiativeID, initiatives.StagePoll, initiatives.StageDemand, &pathA)
		if errors.Is(err, initiatives.ErrWrongStage) {
			return ErrExists
		}
		if err != nil {
			return err
		}

		return tx.QueryRow(ctx, `
			INSERT INTO demands (initiative_id, house_id, channel, support_weight_num, support_weight_den, created_by_user_id)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::uuid)
			RETURNING id::text, status, created_at`,
			initiativeID, in.HouseID, channel, progress.ForNum, progress.ForDen, byUserID,
		).Scan(&d.ID, &d.Status, &d.CreatedAt)
	})
	if err != nil {
		return Demand{}, err
	}

	d.InitiativeID = initiativeID
	d.HouseID = in.HouseID
	d.Channel = channel
	d.SupportNum, d.SupportDen = progress.ForNum, progress.ForDen

	return d, nil
}

// Get returns the demand with the overdue flag. It is visible to the initiator
// and to the staff that manages the initiative (docs/04, решение 79).
func (s *Service) Get(ctx context.Context, demandID, viewerID string) (Demand, error) {
	d, err := s.load(ctx, demandID)
	if err != nil {
		return d, err
	}

	in, err := s.initiatives.Details(ctx, d.InitiativeID)
	if err != nil {
		return d, err
	}

	allowed := in.IsLedBy(viewerID)
	if !allowed {
		staff, err := s.access.ManagesAsStaff(ctx, viewerID, in.HouseID, in.InitiatorUserID)
		if err != nil {
			return d, err
		}
		allowed = staff
	}
	if !allowed {
		return d, ErrForbidden
	}

	s.fillOverdue(ctx, &d)

	return d, nil
}

// MarkDelivered records that the demand was handed to the company: the 45-day
// term starts (ст. 45 ч. 6 ЖК). Initiator only; once.
func (s *Service) MarkDelivered(ctx context.Context, demandID, byUserID string, deliveredAt time.Time) (Demand, error) {
	d, err := s.load(ctx, demandID)
	if err != nil {
		return d, err
	}

	in, err := s.initiatives.Details(ctx, d.InitiativeID)
	if err != nil {
		return d, err
	}
	if !in.IsLedBy(byUserID) {
		return d, ErrNotInitiator
	}
	if d.Status == "delivered" {
		return d, ErrAlreadyDelivered
	}

	if deliveredAt.IsZero() {
		deliveredAt = time.Now()
	}
	due := deliveredAt.Add(45 * 24 * time.Hour)

	// Требование и джоба уведомления — одной транзакцией: принято значит разослано.
	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE demands
			SET status = 'delivered', delivered_at = $2, uk_due_at = $3, updated_at = now()
			WHERE id = $1::uuid AND status = 'draft'`,
			demandID, deliveredAt, due)
		if err != nil {
			return fmt.Errorf("mark delivered: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrAlreadyDelivered
		}

		return s.queue.EnqueueTx(ctx, tx, notify.Job{
			Type:     notify.TypeDemandDelivered,
			DedupKey: notify.TypeDemandDelivered + ":" + demandID,
			Payload:  map[string]any{"demand_id": demandID},
		})
	})
	if err != nil {
		return d, err
	}

	return s.load(ctx, demandID)
}

// ListByHouses returns the demands of the houses, newest first: the cabinet of
// the management organization (К2, /orgs/{orgID}/demands).
func (s *Service) ListByHouses(ctx context.Context, houseIDs []string) ([]Demand, error) {
	if len(houseIDs) == 0 {
		return []Demand{}, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, initiative_id::text, house_id::text, channel, status,
		       support_weight_num, support_weight_den, created_at, delivered_at, uk_due_at
		FROM demands
		WHERE house_id = ANY($1::uuid[])
		ORDER BY created_at DESC`, houseIDs)
	if err != nil {
		return nil, fmt.Errorf("list demands: %w", err)
	}
	defer rows.Close()

	result := make([]Demand, 0)
	for rows.Next() {
		var d Demand
		if err := rows.Scan(&d.ID, &d.InitiativeID, &d.HouseID, &d.Channel, &d.Status,
			&d.SupportNum, &d.SupportDen, &d.CreatedAt, &d.DeliveredAt, &d.UKDueAt); err != nil {
			return nil, fmt.Errorf("scan demand: %w", err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list demands: %w", err)
	}

	return result, nil
}

func (s *Service) load(ctx context.Context, demandID string) (Demand, error) {
	var d Demand
	var deliveredAt, dueAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, initiative_id::text, house_id::text, channel, status,
		       support_weight_num, support_weight_den, created_at, delivered_at, uk_due_at
		FROM demands
		WHERE id = $1::uuid`, demandID,
	).Scan(&d.ID, &d.InitiativeID, &d.HouseID, &d.Channel, &d.Status,
		&d.SupportNum, &d.SupportDen, &d.CreatedAt, &deliveredAt, &dueAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, fmt.Errorf("load demand: %w", err)
	}
	d.DeliveredAt, d.UKDueAt = deliveredAt, dueAt

	return d, nil
}

// fillOverdue: the 45-day term passed and the initiative has no meeting yet.
func (s *Service) fillOverdue(ctx context.Context, d *Demand) {
	if d.Status != "delivered" || d.UKDueAt == nil || time.Now().Before(*d.UKDueAt) {
		return
	}

	_, hasMeeting, err := s.meetings.ActiveMeeting(ctx, d.InitiativeID)
	if err != nil {
		return // не смогли узнать — не помечаем
	}
	d.Overdue = !hasMeeting
}
