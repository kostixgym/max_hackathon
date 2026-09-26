package registry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Read API for other modules (docs/04, principle 8): they get houses, premises and
// owners through these methods and never query the registry tables themselves.
// Ids are UUIDs; ids that come from users are validated by the caller.

// HouseRef is a house as other modules see it. OrgID is the management organization
// whose staff works with the house; nil for a house without one.
type HouseRef struct {
	ID         string
	OrgID      *string
	InviteSlug string
	Address    string
	Region     string
	IsDemo     bool
	// Timezone is the IANA zone of the house: quiet hours and deadlines are local.
	Timezone string
}

// moscow is used when the zone of a house is unknown: the schema default, fixed so
// that it works without the time zone database.
var moscow = time.FixedZone("MSK", 3*60*60)

// Location returns the time zone of the house (Europe/Moscow when it is unknown).
func (h HouseRef) Location() *time.Location {
	if h.Timezone != "" {
		if loc, err := time.LoadLocation(h.Timezone); err == nil {
			return loc
		}
	}

	return moscow
}

// Premise is a premise (stable between registry versions) with its house.
type Premise struct {
	ID       string
	Number   string
	Kind     string
	Entrance *int
	Floor    *int
	// DisplayAreaCenti is the area in the current registry version, for display only (decision 71).
	DisplayAreaCenti *int64
	House            HouseRef
}

// Owner is an owner (stable between versions) with the facts of the current registry
// version of the house. FullName is personal data: the caller decides what to show.
type Owner struct {
	ID            string
	PremiseID     string
	PremiseNumber string
	FullName      string
	Kind          string
	ShareNum      int64
	ShareDen      int64
	WeightNum     int64
	WeightDen     int64
}

// House returns a house by id.
func (s *Store) House(ctx context.Context, id string) (HouseRef, error) {
	var h HouseRef
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, org_id::text, invite_slug, address, region, is_demo, timezone
		FROM houses
		WHERE id = $1::uuid`, id,
	).Scan(&h.ID, &h.OrgID, &h.InviteSlug, &h.Address, &h.Region, &h.IsDemo, &h.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, ErrNotFound
	}
	if err != nil {
		return h, fmt.Errorf("house by id: %w", err)
	}

	return h, nil
}

// Premises returns the premises with the given ids together with their houses.
// Unknown ids are skipped.
func (s *Store) Premises(ctx context.Context, ids []string) ([]Premise, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id::text, p.number, p.kind, p.entrance, p.floor, p.display_area_centi,
		       h.id::text, h.org_id::text, h.invite_slug, h.address, h.region, h.is_demo, h.timezone
		FROM premises p
		JOIN houses h ON h.id = p.house_id
		WHERE p.id = ANY($1::uuid[])
		ORDER BY p.id`, ids)
	if err != nil {
		return nil, fmt.Errorf("premises by ids: %w", err)
	}
	defer rows.Close()

	result := make([]Premise, 0, len(ids))
	for rows.Next() {
		var p Premise
		if err := rows.Scan(&p.ID, &p.Number, &p.Kind, &p.Entrance, &p.Floor, &p.DisplayAreaCenti,
			&p.House.ID, &p.House.OrgID, &p.House.InviteSlug, &p.House.Address, &p.House.Region, &p.House.IsDemo,
			&p.House.Timezone,
		); err != nil {
			return nil, fmt.Errorf("scan premise: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("premises by ids: %w", err)
	}

	return result, nil
}

// currentOwners selects owners with the facts of the current registry version of
// their house. An owner who is not in the current version is not selected.
const currentOwners = `
	SELECT o.id::text, p.id::text, p.number, r.full_name, r.owner_kind,
	       r.share_num, r.share_den, r.weight_num, r.weight_den
	FROM owners o
	JOIN premises p ON p.id = o.premise_id
	JOIN houses h ON h.id = p.house_id
	JOIN registry_uploads ru
	     ON ru.house_id = h.id AND ru.version = h.current_registry_version
	JOIN owner_records r
	     ON r.owner_id = o.id AND r.registry_upload_id = ru.id`

// PremiseOwners returns the current owners of a premise.
func (s *Store) PremiseOwners(ctx context.Context, premiseID string) ([]Owner, error) {
	return s.owners(ctx, currentOwners+`
		WHERE p.id = $1::uuid
		ORDER BY r.full_name, o.id`, premiseID)
}

// HouseOwners returns the current owners of all premises of a house.
func (s *Store) HouseOwners(ctx context.Context, houseID string) ([]Owner, error) {
	return s.owners(ctx, currentOwners+`
		WHERE h.id = $1::uuid
		ORDER BY p.number, r.full_name, o.id`, houseID)
}

// Owners returns the given owners with the facts of the current registry version.
// Owners who are not in the current version are skipped.
func (s *Store) Owners(ctx context.Context, ids []string) ([]Owner, error) {
	return s.owners(ctx, currentOwners+`
		WHERE o.id = ANY($1::uuid[])
		ORDER BY o.id`, ids)
}

func (s *Store) owners(ctx context.Context, query string, arg any) ([]Owner, error) {
	rows, err := s.pool.Query(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("list owners: %w", err)
	}
	defer rows.Close()

	result := make([]Owner, 0)
	for rows.Next() {
		var o Owner
		if err := rows.Scan(&o.ID, &o.PremiseID, &o.PremiseNumber, &o.FullName, &o.Kind,
			&o.ShareNum, &o.ShareDen, &o.WeightNum, &o.WeightDen); err != nil {
			return nil, fmt.Errorf("scan owner: %w", err)
		}
		result = append(result, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list owners: %w", err)
	}

	return result, nil
}
