package access

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
)

// Part of the module API used by the poll flow: who votes, who receives mailings
// and how a demo-house resident becomes a confirmed owner (docs/01, «Путь
// пользователя»; docs/04, решение 51).

// OwnerLink is the user's verified link to a premise as an owner.
type OwnerLink struct {
	MembershipID  string
	PremiseID     string
	OwnerID       string
	PremiseNumber string
}

// ErrNoOwnerLink means the user has no verified owner membership.
var ErrNoOwnerLink = errors.New("no verified owner membership")

// VerifiedOwnerIn returns the user's verified owner link in the house.
// A user with several premises in one house gets all of them: each vote weighs
// its own share (docs/01, «Особые случаи»).
func (s *Store) VerifiedOwnerIn(ctx context.Context, userID, houseID string) ([]OwnerLink, error) {
	links, err := s.memberships(ctx, userID)
	if err != nil {
		return nil, err
	}

	premiseIDs := make([]string, 0, len(links))
	for _, m := range links {
		if m.role == "owner" && m.status == "verified" && m.ownerID != nil {
			premiseIDs = append(premiseIDs, m.premiseID)
		}
	}
	if len(premiseIDs) == 0 {
		return nil, ErrNoOwnerLink
	}

	premises, err := s.registry.Premises(ctx, premiseIDs)
	if err != nil {
		return nil, err
	}

	result := make([]OwnerLink, 0, len(premises))
	for _, p := range premises {
		if p.House.ID != houseID {
			continue
		}
		for _, m := range links {
			if m.premiseID == p.ID && m.role == "owner" && m.status == "verified" && m.ownerID != nil {
				result = append(result, OwnerLink{
					MembershipID:  m.id,
					PremiseID:     p.ID,
					OwnerID:       *m.ownerID,
					PremiseNumber: p.Number,
				})
			}
		}
	}
	if len(result) == 0 {
		return nil, ErrNoOwnerLink
	}

	return result, nil
}

// IsVerifiedOwnerIn reports whether the user has at least one verified owner link in the house.
func (s *Store) IsVerifiedOwnerIn(ctx context.Context, userID, houseID string) (bool, error) {
	links, err := s.VerifiedOwnerIn(ctx, userID, houseID)
	if errors.Is(err, ErrNoOwnerLink) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return len(links) > 0, nil
}

// MayViewInitiatives reports whether the user sees the initiatives of the house and
// their poll progress: a verified member, resident or owner (docs/04, решение 19),
// or staff of the house's management organization. The progress holds only sums
// in м², exactly what the company may see (решение 43).
func (s *Store) MayViewInitiatives(ctx context.Context, userID, houseID string) (bool, error) {
	// The house id may come from the URL: a malformed one is no house at all.
	if _, err := resourceID(houseID); err != nil {
		return false, nil
	}
	links, err := s.memberships(ctx, userID)
	if err != nil {
		return false, err
	}

	premiseIDs := make([]string, 0, len(links))
	for _, m := range links {
		if m.status == "verified" {
			premiseIDs = append(premiseIDs, m.premiseID)
		}
	}
	if len(premiseIDs) > 0 {
		premises, err := s.registry.Premises(ctx, premiseIDs)
		if err != nil {
			return false, err
		}
		if slices.ContainsFunc(premises, func(p registry.Premise) bool { return p.House.ID == houseID }) {
			return true, nil
		}
	}

	house, err := s.registry.House(ctx, houseID)
	if errors.Is(err, registry.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return s.isStaff(ctx, userID, house)
}

// Recipient is one addressee of a house mailing.
type Recipient = initiatives.Recipient

// VerifiedOwnerRecipients returns the verified owners of the house with their
// MAX ids: the poll invitation with voting buttons goes exactly to them
// (docs/02, шаг 2). Only owners of the current registry version are asked for,
// so the query reads the links of one house, not of the whole platform.
func (s *Store) VerifiedOwnerRecipients(ctx context.Context, houseID string) ([]Recipient, error) {
	owners, err := s.registry.HouseOwners(ctx, houseID)
	if err != nil {
		return nil, err
	}
	if len(owners) == 0 {
		return []Recipient{}, nil
	}
	ownerIDs := make([]string, 0, len(owners))
	for _, o := range owners {
		ownerIDs = append(ownerIDs, o.ID)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT m.user_id::text, u.max_user_id, m.owner_id::text
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.owner_id = ANY($1::uuid[]) AND m.role = 'owner' AND m.status = 'verified'
		ORDER BY m.created_at`, ownerIDs)
	if err != nil {
		return nil, fmt.Errorf("list owner recipients: %w", err)
	}
	defer rows.Close()

	result := make([]Recipient, 0)
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.UserID, &r.MaxUserID, &r.OwnerID); err != nil {
			return nil, fmt.Errorf("scan owner recipient: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list owner recipients: %w", err)
	}

	return result, nil
}

// ErrNotDemo means the endpoint works only in a demo house.
var ErrNotDemo = errors.New("house is not a demo house")

// ErrOwnerTaken means the owner record is already confirmed for another account
// (инвариант 9): the conflict is for the management company to resolve.
var ErrOwnerTaken = errors.New("owner is already confirmed for another account")
var ErrPhoneNotMatched = errors.New("MAX phone does not match an owner of this premise")

// ConfirmOwnerByPhone promotes only this user's pending guest link, and only if
// the verified MAX contact matches a phone in the current registry snapshot.
func (s *Store) ConfirmOwnerByPhone(ctx context.Context, userID, membershipID, phone string) (OwnerLink, error) {
	if s.hasher == nil {
		return OwnerLink{}, errors.New("phone hasher is not configured")
	}
	phoneHash, err := s.hasher.Phone(phone)
	if err != nil {
		return OwnerLink{}, err
	}
	var premiseID, number string
	err = s.pool.QueryRow(ctx, `
		SELECT p.id::text, p.number
		FROM memberships m JOIN premises p ON p.id = m.premise_id
		WHERE m.id = $1::uuid AND m.user_id = $2::uuid AND m.role = 'guest' AND m.status = 'pending'`, membershipID, userID).Scan(&premiseID, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		return OwnerLink{}, ErrNotFound
	}
	if err != nil {
		return OwnerLink{}, fmt.Errorf("load membership for phone verification: %w", err)
	}
	phoneRegistry, ok := s.registry.(interface {
		OwnerByPhoneHash(context.Context, string, []byte) (registry.Owner, error)
	})
	if !ok {
		return OwnerLink{}, errors.New("registry phone lookup is not configured")
	}
	owner, err := phoneRegistry.OwnerByPhoneHash(ctx, premiseID, phoneHash)
	if errors.Is(err, registry.ErrNotFound) {
		return OwnerLink{}, ErrPhoneNotMatched
	}
	if err != nil {
		return OwnerLink{}, err
	}
	link := OwnerLink{PremiseID: premiseID, OwnerID: owner.ID, PremiseNumber: number}
	err = s.pool.QueryRow(ctx, `
		UPDATE memberships SET owner_id = $3::uuid, role = 'owner', method = 'phone', status = 'verified', verified_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND role = 'guest' AND status = 'pending'
		RETURNING id::text`, membershipID, userID, owner.ID).Scan(&link.MembershipID)
	if err != nil {
		if isUniqueViolation(err) {
			return OwnerLink{}, fmt.Errorf("%w: owner %s", ErrOwnerTaken, owner.ID)
		}
		return OwnerLink{}, fmt.Errorf("confirm owner by phone: %w", err)
	}
	return link, nil
}

// ConfirmDemoOwner links the user to an owner of the premise in the current
// registry version (docs/01, «Как это проверит жюри»: auto-confirmation in the
// demo house). ownerIndex is the 1-based number of the owner in display order:
// a flat with co-owners gives each tester their own record (invariant 9).
// The premise and owners come from the registry module.
func (s *Store) ConfirmDemoOwner(ctx context.Context, userID, houseID, premiseNumber string, ownerIndex int) (OwnerLink, error) {
	premise, err := s.registry.PremiseByNumber(ctx, houseID, premiseNumber)
	if errors.Is(err, registry.ErrNotFound) {
		return OwnerLink{}, ErrNotFound
	}
	if err != nil {
		return OwnerLink{}, err
	}
	if !premise.House.IsDemo {
		return OwnerLink{}, ErrNotDemo
	}

	owners, err := s.registry.PremiseOwners(ctx, premise.ID)
	if err != nil {
		return OwnerLink{}, err
	}
	if len(owners) == 0 {
		return OwnerLink{}, ErrNotFound
	}
	if ownerIndex == 0 {
		ownerIndex = 1 // the default: the first owner in display order
	}
	if ownerIndex < 1 || ownerIndex > len(owners) {
		return OwnerLink{}, ErrNotFound
	}
	owner := owners[ownerIndex-1]

	method := "demo"
	link := OwnerLink{
		PremiseID:     premise.ID,
		OwnerID:       owner.ID,
		PremiseNumber: premise.Number,
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO memberships (user_id, premise_id, owner_id, role, method, status, verified_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'owner', $4, 'verified', now())
		ON CONFLICT (user_id, premise_id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id, role = 'owner', method = EXCLUDED.method,
		    status = 'verified', verified_at = now(), rejection_reason = NULL
		RETURNING id::text`, userID, premise.ID, owner.ID, method,
	).Scan(&link.MembershipID)
	if err != nil {
		// Two users claimed the same owner record: the conflict goes to the
		// management company, as in invariant 9.
		if isUniqueViolation(err) {
			return OwnerLink{}, fmt.Errorf("%w: owner %s", ErrOwnerTaken, owner.ID)
		}

		return OwnerLink{}, fmt.Errorf("confirm demo owner: %w", err)
	}

	return link, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == pgerrUniqueViolation
}

const pgerrUniqueViolation = "23505"
