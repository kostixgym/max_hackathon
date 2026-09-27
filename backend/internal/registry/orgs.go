package registry

import (
	"context"
	"fmt"
)

// Org is a management organization as other modules see it.
type Org struct {
	ID   string
	Name string
	Type string // uk / tszh / zhsk
}

// OrgsByIDs returns the organizations with the given ids. Unknown ids are skipped.
func (s *Store) OrgsByIDs(ctx context.Context, ids []string) ([]Org, error) {
	if len(ids) == 0 {
		return []Org{}, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, type
		FROM organizations
		WHERE id = ANY($1::uuid[])
		ORDER BY name`, ids)
	if err != nil {
		return nil, fmt.Errorf("orgs by ids: %w", err)
	}
	defer rows.Close()

	orgs := make([]Org, 0, len(ids))
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Type); err != nil {
			return nil, fmt.Errorf("scan org: %w", err)
		}
		orgs = append(orgs, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orgs by ids: %w", err)
	}

	return orgs, nil
}

// HousesByOrg returns the houses of the management organization, for the staff
// cabinet (/orgs/{orgID}/houses).
func (s *Store) HousesByOrg(ctx context.Context, orgID string) ([]HouseRef, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, org_id::text, invite_slug, address, region, is_demo
		FROM houses
		WHERE org_id = $1::uuid
		ORDER BY address`, orgID)
	if err != nil {
		return nil, fmt.Errorf("houses by org: %w", err)
	}
	defer rows.Close()

	houses := make([]HouseRef, 0)
	for rows.Next() {
		var h HouseRef
		if err := rows.Scan(&h.ID, &h.OrgID, &h.InviteSlug, &h.Address, &h.Region, &h.IsDemo); err != nil {
			return nil, fmt.Errorf("scan house of org: %w", err)
		}
		houses = append(houses, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("houses by org: %w", err)
	}

	return houses, nil
}
