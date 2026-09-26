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
// house. A house without an organization has no staff.
func (s *Store) IsStaffOf(ctx context.Context, userID, houseID string) (bool, error) {
	house, err := s.registry.House(ctx, houseID)
	if errors.Is(err, registry.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return s.isStaff(ctx, userID, house)
}

// StaffMaxUserIDs returns the MAX ids of the staff of the organization: the bot
// notifies them when a demand arrives (К3).
func (s *Store) StaffMaxUserIDs(ctx context.Context, orgID string) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.max_user_id
		FROM org_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1::uuid
		ORDER BY u.max_user_id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0)
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
