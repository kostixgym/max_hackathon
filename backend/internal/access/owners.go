package access

import (
	"context"
	"errors"
	"fmt"
	"slices"

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

// IsVerifiedMemberIn reports whether the user has a verified link (resident or
// owner) to any premise of the house: the audience that sees initiatives and
// their progress (docs/04, решение 19).
func (s *Store) IsVerifiedMemberIn(ctx context.Context, userID, houseID string) (bool, error) {
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
	if len(premiseIDs) == 0 {
		return false, nil
	}

	premises, err := s.registry.Premises(ctx, premiseIDs)
	if err != nil {
		return false, err
	}

	return slices.ContainsFunc(premises, func(p registry.Premise) bool { return p.House.ID == houseID }), nil
}

// Recipient is one addressee of a house mailing.
type Recipient = initiatives.Recipient

// VerifiedOwnerRecipients returns the verified owners of the house with their
// MAX ids: the poll invitation with voting buttons goes exactly to them
// (docs/02, шаг 2).
func (s *Store) VerifiedOwnerRecipients(ctx context.Context, houseID string) ([]Recipient, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.user_id::text, u.max_user_id, m.owner_id::text, m.premise_id::text
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.role = 'owner' AND m.status = 'verified' AND m.owner_id IS NOT NULL
		ORDER BY m.created_at`)
	if err != nil {
		return nil, fmt.Errorf("list owner recipients: %w", err)
	}
	defer rows.Close()

	type row struct {
		userID    string
		maxUserID int64
		ownerID   string
		premiseID string
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.userID, &r.maxUserID, &r.ownerID, &r.premiseID); err != nil {
			return nil, fmt.Errorf("scan owner recipient: %w", err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list owner recipients: %w", err)
	}
	if len(all) == 0 {
		return []Recipient{}, nil
	}

	premiseIDs := make([]string, 0, len(all))
	for _, r := range all {
		premiseIDs = append(premiseIDs, r.premiseID)
	}
	premises, err := s.registry.Premises(ctx, premiseIDs)
	if err != nil {
		return nil, err
	}
	housePremise := make(map[string]bool, len(premises))
	for _, p := range premises {
		housePremise[p.ID] = p.House.ID == houseID
	}

	result := make([]Recipient, 0, len(all))
	for _, r := range all {
		if housePremise[r.premiseID] {
			result = append(result, Recipient{UserID: r.userID, MaxUserID: r.maxUserID, OwnerID: r.ownerID})
		}
	}

	return result, nil
}

// ErrNotDemo means the endpoint works only in a demo house.
var ErrNotDemo = errors.New("house is not a demo house")

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
			return OwnerLink{}, fmt.Errorf("owner %s is already confirmed for another account", owner.ID)
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
