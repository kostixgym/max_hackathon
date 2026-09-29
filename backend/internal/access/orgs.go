package access

// Staff of management organizations and the demo-staff shortcut (К2 of
// docs/plan-do-30-09.md). The staff role gates the management cabinet: demands,
// meeting creation and ballot processing.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"maxhackathon/backend/internal/registry"
)

// OrgSummary is an organization the user works in, as /me and /orgs show it.
type OrgSummary struct {
	ID   string
	Name string
	Type string // uk / tszh / zhsk
	Role string // admin / operator
}

// IsStaffOf reports whether the user works in the management organization of the
// house. A house without an organization has no staff. It answers «who may see»;
// «who may act on an initiative» is ManagesAsStaff.
func (s *Store) IsStaffOf(ctx context.Context, userID, houseID string) (bool, error) {
	house, ok, err := s.houseByID(ctx, houseID)
	if !ok || err != nil {
		return false, err
	}

	return s.isStaff(ctx, userID, house)
}

// ManagesAsStaff reports whether the user acts as the management company on an
// initiative of the house: receives the demand, creates and runs the meeting, fixes
// the result (path A). Staff of the house's organization may — except in the demo
// house. There every tester joins the one demo organization, so a member of the
// staff acts only on the initiatives they lead (решение 79): otherwise one member of
// the jury would run the meeting of another one's initiative.
func (s *Store) ManagesAsStaff(ctx context.Context, userID, houseID string, initiatorUserID *string) (bool, error) {
	house, ok, err := s.houseByID(ctx, houseID)
	if !ok || err != nil {
		return false, err
	}
	staff, err := s.isStaff(ctx, userID, house)
	if err != nil || !staff {
		return false, err
	}
	if house.IsDemo {
		return initiatorUserID != nil && *initiatorUserID == userID, nil
	}

	return true, nil
}

// StaffRecipients returns the MAX ids of the staff the bot notifies about an
// initiative of the house (a demand delivered, К3). In the demo house it is the
// initiator alone, and only if they joined the demo organization (решение 79): the
// jury must not receive each other's demands.
func (s *Store) StaffRecipients(ctx context.Context, houseID string, initiatorUserID *string) ([]int64, error) {
	ids := make([]int64, 0)
	house, ok, err := s.houseByID(ctx, houseID)
	if !ok || err != nil || house.OrgID == nil {
		return ids, err
	}

	query := `
		SELECT u.max_user_id
		FROM org_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1::uuid
		ORDER BY u.max_user_id`
	args := []any{*house.OrgID}
	if house.IsDemo {
		if initiatorUserID == nil {
			return ids, nil
		}
		query = `
			SELECT u.max_user_id
			FROM org_members m
			JOIN users u ON u.id = m.user_id
			WHERE m.org_id = $1::uuid AND m.user_id = $2::uuid`
		args = append(args, *initiatorUserID)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan staff id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}

	return ids, nil
}

// houseByID reads the house; ok is false for a malformed or unknown id: an id from
// a URL must not reach SQL, and a missing house has no staff.
func (s *Store) houseByID(ctx context.Context, houseID string) (registry.HouseRef, bool, error) {
	id, err := resourceID(houseID)
	if err != nil {
		return registry.HouseRef{}, false, nil
	}
	house, err := s.registry.House(ctx, id)
	if errors.Is(err, registry.ErrNotFound) {
		return registry.HouseRef{}, false, nil
	}
	if err != nil {
		return registry.HouseRef{}, false, err
	}

	return house, true, nil
}

// OrgsByUser returns the organizations the user works in. Membership rows are
// this module's data; the organization details come from the registry module.
func (s *Store) OrgsByUser(ctx context.Context, userID string) ([]OrgSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT org_id::text, role
		FROM org_members
		WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user orgs: %w", err)
	}
	defer rows.Close()

	type link struct {
		orgID string
		role  string
	}
	links := make([]link, 0)
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.orgID, &l.role); err != nil {
			return nil, fmt.Errorf("scan org link: %w", err)
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user orgs: %w", err)
	}
	if len(links) == 0 {
		return []OrgSummary{}, nil
	}

	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.orgID)
	}
	orgs, err := s.registry.OrgsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]registry.Org, len(orgs))
	for _, o := range orgs {
		byID[o.ID] = o
	}
	result := make([]OrgSummary, 0, len(links))
	for _, l := range links {
		if o, ok := byID[l.orgID]; ok {
			result = append(result, OrgSummary{ID: o.ID, Name: o.Name, Type: o.Type, Role: l.role})
		}
	}
	slices.SortFunc(result, func(a, b OrgSummary) int { return strings.Compare(a.Name, b.Name) })

	return result, nil
}

// OrgByUser returns one organization of the user. ErrForbidden means the user
// does not work there: the cabinet of another organization is none of their business.
func (s *Store) OrgByUser(ctx context.Context, userID, orgID string) (OrgSummary, error) {
	orgs, err := s.OrgsByUser(ctx, userID)
	if err != nil {
		return OrgSummary{}, err
	}
	for _, o := range orgs {
		if o.ID == orgID {
			return o, nil
		}
	}

	return OrgSummary{}, ErrForbidden
}

// ErrNoOrg means the demo house has no management organization to join.
var ErrNoOrg = errors.New("the house has no management organization")

// ConfirmDemoStaff makes the user an operator of the management organization of
// the demo house (docs/01, «Как это проверит жюри»): the jury gets the
// management-company view without a real organization. Only a demo house, and
// the operation is idempotent.
func (s *Store) ConfirmDemoStaff(ctx context.Context, userID, houseID string) (OrgSummary, error) {
	allowed, err := s.configuredUK(ctx, userID)
	if err != nil {
		return OrgSummary{}, err
	}
	if !allowed {
		return OrgSummary{}, ErrForbidden
	}
	house, err := s.registry.House(ctx, houseID)
	if errors.Is(err, registry.ErrNotFound) {
		return OrgSummary{}, ErrNotFound
	}
	if err != nil {
		return OrgSummary{}, err
	}
	if !house.IsDemo {
		return OrgSummary{}, ErrNotDemo
	}
	if house.OrgID == nil {
		return OrgSummary{}, ErrNoOrg
	}

	var org OrgSummary
	_, err = s.pool.Exec(ctx, `
		INSERT INTO org_members (user_id, org_id, role) VALUES ($1::uuid, $2::uuid, 'operator')
		ON CONFLICT (user_id, org_id) DO UPDATE SET role = 'operator'`,
		userID, *house.OrgID)
	if err != nil {
		return OrgSummary{}, fmt.Errorf("confirm demo staff: %w", err)
	}

	// The organization summary comes from the registry module.
	orgs, err := s.registry.OrgsByIDs(ctx, []string{*house.OrgID})
	if err != nil {
		return OrgSummary{}, err
	}
	if len(orgs) != 1 {
		return OrgSummary{}, fmt.Errorf("demo org %s not found", *house.OrgID)
	}
	org = OrgSummary{ID: orgs[0].ID, Name: orgs[0].Name, Type: orgs[0].Type, Role: "operator"}

	return org, nil
}
