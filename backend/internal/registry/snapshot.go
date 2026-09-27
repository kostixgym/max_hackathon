package registry

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Snapshot is a registry version of a house: the base of all vote weights and
// thresholds. An initiative pins one snapshot and keeps it even after the house
// moves to a new version (docs/04, принцип 3).
type Snapshot struct {
	UploadID       string
	Version        int
	TotalAreaCenti int64
}

// CurrentSnapshot returns the applied registry version the house is on now.
// ErrNotApplied means that the house has no applied version yet.
var ErrNotApplied = errors.New("house has no applied registry version")

// CurrentSnapshot returns the current registry snapshot of the house.
func (s *Store) CurrentSnapshot(ctx context.Context, houseID string) (Snapshot, error) {
	var snap Snapshot
	// Without an applied version the LEFT JOIN gives NULL in all three columns.
	var uploadID *string
	var version *int
	var total *int64
	err := s.pool.QueryRow(ctx, `
		SELECT ru.id::text, ru.version, ru.total_area_centi
		FROM houses h
		LEFT JOIN registry_uploads ru
		     ON ru.house_id = h.id AND ru.version = h.current_registry_version
		WHERE h.id = $1::uuid`, houseID,
	).Scan(&uploadID, &version, &total)
	if errors.Is(err, pgx.ErrNoRows) {
		return snap, ErrNotFound
	}
	if err != nil {
		return snap, fmt.Errorf("current snapshot: %w", err)
	}
	if uploadID == nil || version == nil || total == nil {
		return snap, ErrNotApplied
	}
	snap.UploadID, snap.Version, snap.TotalAreaCenti = *uploadID, *version, *total

	return snap, nil
}

// Snapshot returns the registry version with the given upload id.
func (s *Store) Snapshot(ctx context.Context, uploadID string) (Snapshot, error) {
	snap := Snapshot{UploadID: uploadID}
	err := s.pool.QueryRow(ctx,
		`SELECT version, total_area_centi FROM registry_uploads WHERE id = $1::uuid`, uploadID,
	).Scan(&snap.Version, &snap.TotalAreaCenti)
	if errors.Is(err, pgx.ErrNoRows) {
		return snap, ErrNotFound
	}
	if err != nil {
		return snap, fmt.Errorf("snapshot: %w", err)
	}

	return snap, nil
}

// SnapshotTotal returns the total area of the given snapshot in hundredths of м².
func (s *Store) SnapshotTotal(ctx context.Context, uploadID string) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx,
		`SELECT total_area_centi FROM registry_uploads WHERE id = $1::uuid`, uploadID).Scan(&total)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("snapshot total: %w", err)
	}

	return total, nil
}

// OwnerWeightInSnapshot returns the exact vote weight (hundredths of м² as a
// fraction) of the owner in the given snapshot version. ErrNotFound means the
// owner is absent from that version, so the vote cannot be counted.
func (s *Store) OwnerWeightInSnapshot(ctx context.Context, uploadID, ownerID string) (num, den int64, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT weight_num, weight_den
		FROM owner_records
		WHERE registry_upload_id = $1::uuid AND owner_id = $2::uuid`,
		uploadID, ownerID).Scan(&num, &den)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, fmt.Errorf("owner weight in snapshot: %w", err)
	}

	return num, den, nil
}

// SnapshotOwner is an owner with the facts of one registry version and the entrance
// of the premise: a meeting issues its ballots by the initiative's snapshot
// (решение 2), and the tracker groups them by entrance for the round of the flats.
type SnapshotOwner struct {
	Owner
	Entrance *int
}

// SnapshotOwners returns all owners of the registry version; their weights sum to
// the total area of the version. Order: entrance (premises without one last), then
// numeric flat numbers as numbers ("2" before "10"), then other premises ("Н1"), name.
func (s *Store) SnapshotOwners(ctx context.Context, uploadID string) ([]SnapshotOwner, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id::text, p.id::text, p.number, r.full_name, r.owner_kind,
		       r.share_num, r.share_den, r.weight_num, r.weight_den, p.entrance
		FROM owner_records r
		JOIN owners o ON o.id = r.owner_id
		JOIN premises p ON p.id = o.premise_id
		WHERE r.registry_upload_id = $1::uuid
		ORDER BY p.entrance NULLS LAST, p.number !~ '^[0-9]+$', length(p.number), p.number, r.full_name, o.id`,
		uploadID)
	if err != nil {
		return nil, fmt.Errorf("snapshot owners: %w", err)
	}
	defer rows.Close()

	result := make([]SnapshotOwner, 0)
	for rows.Next() {
		var o SnapshotOwner
		if err := rows.Scan(&o.ID, &o.PremiseID, &o.PremiseNumber, &o.FullName, &o.Kind,
			&o.ShareNum, &o.ShareDen, &o.WeightNum, &o.WeightDen, &o.Entrance); err != nil {
			return nil, fmt.Errorf("scan snapshot owner: %w", err)
		}
		result = append(result, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("snapshot owners: %w", err)
	}

	return result, nil
}

// PremiseByNumber returns the premise of the house with the given number.
func (s *Store) PremiseByNumber(ctx context.Context, houseID, number string) (Premise, error) {
	var p Premise
	err := s.pool.QueryRow(ctx, `
		SELECT p.id::text, p.number, p.kind, p.entrance, p.floor, p.display_area_centi,
		       h.id::text, h.org_id::text, h.invite_slug, h.address, h.region, h.is_demo, h.timezone
		FROM premises p
		JOIN houses h ON h.id = p.house_id
		WHERE p.house_id = $1::uuid AND p.number = $2`, houseID, number,
	).Scan(&p.ID, &p.Number, &p.Kind, &p.Entrance, &p.Floor, &p.DisplayAreaCenti,
		&p.House.ID, &p.House.OrgID, &p.House.InviteSlug, &p.House.Address, &p.House.Region, &p.House.IsDemo,
		&p.House.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("premise by number: %w", err)
	}

	return p, nil
}
