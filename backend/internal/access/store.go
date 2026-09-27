// Package access is the «Доступ и роли» module: users, memberships (links between
// a user and a premise) and management company staff. It decides who may see what.
// Houses, premises, owners and initiatives belong to other modules and come only
// through their interfaces (docs/04, principle 8).
package access

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/registry"
)

// ErrNotFound means that the requested house or premise does not exist.
var ErrNotFound = errors.New("not found")

// ErrForbidden means that the user may not view the requested owner directory.
var ErrForbidden = errors.New("forbidden")

// Registry reads houses, premises, owners and organizations (the registry module).
type Registry interface {
	House(ctx context.Context, id string) (registry.HouseRef, error)
	Premises(ctx context.Context, ids []string) ([]registry.Premise, error)
	PremiseByNumber(ctx context.Context, houseID, number string) (registry.Premise, error)
	PremiseOwners(ctx context.Context, premiseID string) ([]registry.Owner, error)
	HouseOwners(ctx context.Context, houseID string) ([]registry.Owner, error)
	Owners(ctx context.Context, ids []string) ([]registry.Owner, error)
	OrgsByIDs(ctx context.Context, ids []string) ([]registry.Org, error)
}

// Initiatives answers questions about initiatives (the initiatives module).
type Initiatives interface {
	IsPathBInitiator(ctx context.Context, userID, houseID string) (bool, error)
}

// User is a MAX user known to the service. Name and photo from MAX are not stored.
type User struct {
	ID        string
	MaxUserID int64
}

// MembershipSummary is the read model used to bootstrap the mini-app.
// Owner data always comes from the house's current registry version.
type MembershipSummary struct {
	ID      string
	Role    string
	Status  string
	Method  *string
	Premise registry.Premise
	Owner   *OwnerSummary
}

// OwnerSummary contains only data safe for an authorized directory response.
// FullName is deliberately not exposed: callers only receive MaskedName.
type OwnerSummary struct {
	ID            string
	PremiseID     string
	PremiseNumber string
	MaskedName    string
	Kind          string
	ShareNum      int64
	ShareDen      int64
	WeightNum     int64
	WeightDen     int64
}

// Store reads and writes the tables of the module: users, memberships and org_members.
type Store struct {
	pool        *pgxpool.Pool
	registry    Registry
	initiatives Initiatives
}

// NewStore creates an access store on top of the registry and initiatives modules.
func NewStore(pool *pgxpool.Pool, reg Registry, initiatives Initiatives) *Store {
	return &Store{pool: pool, registry: reg, initiatives: initiatives}
}

// EnsureUser returns the user with the given MAX id, creating it on the first visit.
// It runs on every API request, so a known user costs one indexed read and no write.
func (s *Store) EnsureUser(ctx context.Context, maxUserID int64) (User, error) {
	u := User{MaxUserID: maxUserID}
	err := s.pool.QueryRow(ctx, `SELECT id::text FROM users WHERE max_user_id = $1`, maxUserID).Scan(&u.ID)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return u, fmt.Errorf("find user: %w", err)
	}

	// The first visit. Parallel first requests of the same user race here: the loser's
	// insert returns no row and reads the winner's user.
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (max_user_id) VALUES ($1)
		ON CONFLICT (max_user_id) DO NOTHING
		RETURNING id::text`, maxUserID,
	).Scan(&u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.pool.QueryRow(ctx, `SELECT id::text FROM users WHERE max_user_id = $1`, maxUserID).Scan(&u.ID)
	}
	if err != nil {
		return u, fmt.Errorf("create user: %w", err)
	}

	return u, nil
}

// MaxUserID returns the MAX id of the user: the bot writes to them by it. ErrNotFound
// means the account is deleted (решение 28).
func (s *Store) MaxUserID(ctx context.Context, userID string) (int64, error) {
	id, err := resourceID(userID)
	if err != nil {
		return 0, err
	}
	var maxUserID int64
	err = s.pool.QueryRow(ctx, `SELECT max_user_id FROM users WHERE id = $1::uuid`, id).Scan(&maxUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("max user id: %w", err)
	}

	return maxUserID, nil
}

// MembershipsByUser returns all of the user's non-revoked links together with
// the premise, its house and the current owner facts required by the mini-app home page.
func (s *Store) MembershipsByUser(ctx context.Context, userID string) ([]MembershipSummary, error) {
	links, err := s.memberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return []MembershipSummary{}, nil
	}

	premiseIDs := make([]string, 0, len(links))
	var ownerIDs []string
	for _, m := range links {
		premiseIDs = append(premiseIDs, m.premiseID)
		if m.ownerID != nil {
			ownerIDs = append(ownerIDs, *m.ownerID)
		}
	}

	premises, err := s.registry.Premises(ctx, premiseIDs)
	if err != nil {
		return nil, err
	}
	premiseByID := make(map[string]registry.Premise, len(premises))
	for _, p := range premises {
		premiseByID[p.ID] = p
	}

	ownerByID := make(map[string]registry.Owner, len(ownerIDs))
	if len(ownerIDs) > 0 {
		owners, err := s.registry.Owners(ctx, ownerIDs)
		if err != nil {
			return nil, err
		}
		for _, o := range owners {
			ownerByID[o.ID] = o
		}
	}

	result := make([]MembershipSummary, 0, len(links))
	for _, m := range links {
		premise, ok := premiseByID[m.premiseID]
		if !ok {
			return nil, fmt.Errorf("premise %s of membership %s is missing in the registry", m.premiseID, m.id)
		}
		item := MembershipSummary{ID: m.id, Role: m.role, Status: m.status, Method: m.method, Premise: premise}
		// An owner who left the current registry version has no facts to show.
		if m.ownerID != nil {
			if o, ok := ownerByID[*m.ownerID]; ok {
				owner := maskOwner(o)
				item.Owner = &owner
			}
		}
		result = append(result, item)
	}
	slices.SortFunc(result, func(a, b MembershipSummary) int {
		return cmp.Or(
			strings.Compare(a.Premise.House.Address, b.Premise.House.Address),
			strings.Compare(a.Premise.Number, b.Premise.Number),
			strings.Compare(a.ID, b.ID),
		)
	})

	return result, nil
}

// PremiseOwners returns owners from the current registry version. The list is
// needed to choose one's owner record, so it is shown only to a member whose link
// to the premise is already confirmed: a verified resident (receipt check), an
// owner (pending or verified) and staff of the house's management organization.
// A guest is refused: anyone can claim any flat as a guest, and otherwise could
// collect surnames and initials of all owners flat by flat (docs/01, «Роли и права»).
func (s *Store) PremiseOwners(ctx context.Context, userID, premiseID string) ([]OwnerSummary, error) {
	id, err := resourceID(premiseID)
	if err != nil {
		return nil, err
	}
	premises, err := s.registry.Premises(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(premises) == 0 {
		return nil, ErrNotFound
	}

	allowed, err := s.mayViewPremiseOwners(ctx, userID, premises[0])
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}

	owners, err := s.registry.PremiseOwners(ctx, id)
	if err != nil {
		return nil, err
	}

	return maskOwners(owners), nil
}

// HouseOfficerCandidates returns current-registry owners that can be selected
// as a meeting chair or secretary. The house-wide directory (masked names, flats,
// shares) is needed only by whoever organizes the meeting: the initiator of an
// active path-B initiative of the house (a verified owner) and the staff of the
// house's management organization (path A). Other owners do not see it.
func (s *Store) HouseOfficerCandidates(ctx context.Context, userID, houseID string) ([]OwnerSummary, error) {
	id, err := resourceID(houseID)
	if err != nil {
		return nil, err
	}
	house, err := s.registry.House(ctx, id)
	if errors.Is(err, registry.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	allowed, err := s.organizesMeeting(ctx, userID, house)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}

	owners, err := s.registry.HouseOwners(ctx, id)
	if err != nil {
		return nil, err
	}

	return maskOwners(owners), nil
}

// membership is a user's link to a premise as the module stores it.
type membership struct {
	id        string
	role      string
	status    string
	method    *string
	premiseID string
	ownerID   *string
}

// memberships returns the user's links that are not revoked.
func (s *Store) memberships(ctx context.Context, userID string) ([]membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, role, status, method, premise_id::text, owner_id::text
		FROM memberships
		WHERE user_id = $1::uuid AND status <> 'revoked'`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user memberships: %w", err)
	}
	defer rows.Close()

	var result []membership
	for rows.Next() {
		var m membership
		if err := rows.Scan(&m.id, &m.role, &m.status, &m.method, &m.premiseID, &m.ownerID); err != nil {
			return nil, fmt.Errorf("scan user membership: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user memberships: %w", err)
	}

	return result, nil
}

// mayViewPremiseOwners reports whether the user's link to the premise is confirmed
// (a verified resident or owner, or an owner waiting for the company's check) or the
// user is staff of the house's management organization.
func (s *Store) mayViewPremiseOwners(ctx context.Context, userID string, premise registry.Premise) (bool, error) {
	var linked bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM memberships
			WHERE user_id = $1::uuid AND premise_id = $2::uuid
			  AND ((role IN ('resident', 'owner') AND status = 'verified')
			       OR (role = 'owner' AND status = 'pending')))`, userID, premise.ID).Scan(&linked)
	if err != nil {
		return false, fmt.Errorf("check premise link: %w", err)
	}
	if linked {
		return true, nil
	}

	return s.isStaff(ctx, userID, premise.House)
}

// isStaff reports whether the user works in the management organization of the house.
func (s *Store) isStaff(ctx context.Context, userID string, house registry.HouseRef) (bool, error) {
	if house.OrgID == nil {
		return false, nil
	}

	var staff bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM org_members WHERE user_id = $1::uuid AND org_id = $2::uuid)`,
		userID, *house.OrgID).Scan(&staff)
	if err != nil {
		return false, fmt.Errorf("check staff: %w", err)
	}

	return staff, nil
}

// organizesMeeting reports whether the user organizes a meeting of the house: staff
// of its management organization (path A) or the initiator of an active path-B
// initiative who is a verified owner in the house.
func (s *Store) organizesMeeting(ctx context.Context, userID string, house registry.HouseRef) (bool, error) {
	staff, err := s.isStaff(ctx, userID, house)
	if err != nil {
		return false, err
	}
	if staff {
		return true, nil
	}

	initiator, err := s.initiatives.IsPathBInitiator(ctx, userID, house.ID)
	if err != nil {
		return false, err
	}
	if !initiator {
		return false, nil
	}

	return s.verifiedOwnerIn(ctx, userID, house.ID)
}

// verifiedOwnerIn reports whether the user is a verified owner of a premise of the house.
func (s *Store) verifiedOwnerIn(ctx context.Context, userID, houseID string) (bool, error) {
	links, err := s.memberships(ctx, userID)
	if err != nil {
		return false, err
	}
	var premiseIDs []string
	for _, m := range links {
		if m.role == "owner" && m.status == "verified" {
			premiseIDs = append(premiseIDs, m.premiseID)
		}
	}
	if len(premiseIDs) == 0 {
		return false, nil
	}

	premises, err := s.registry.Premises(ctx, premiseIDs)
	if err != nil {
		return false, err
	}

	return slices.ContainsFunc(premises, func(p registry.Premise) bool { return p.House.ID == houseID }), nil
}

// resourceID validates an id that comes from the URL: a malformed id means
// «not found» without a query.
func resourceID(value string) (string, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return "", ErrNotFound
	}

	return id.String(), nil
}

func maskOwners(owners []registry.Owner) []OwnerSummary {
	result := make([]OwnerSummary, 0, len(owners))
	for _, o := range owners {
		result = append(result, maskOwner(o))
	}

	return result
}

func maskOwner(o registry.Owner) OwnerSummary {
	return OwnerSummary{
		ID: o.ID, PremiseID: o.PremiseID, PremiseNumber: o.PremiseNumber,
		MaskedName: registry.MaskName(o.FullName, o.Kind), Kind: o.Kind,
		ShareNum: o.ShareNum, ShareDen: o.ShareDen,
		WeightNum: o.WeightNum, WeightDen: o.WeightDen,
	}
}
