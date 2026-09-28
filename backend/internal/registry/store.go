// Package registry is the «Реестр домов» module: houses, premises, owners and
// registry versions. Other modules read this data only through this package.
package registry

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/platform/db"
)

// ErrNotFound is returned when a house does not exist.
var ErrNotFound = errors.New("not found")

// ErrVersionNotApplied is returned when a registry version that is not applied
// is about to become the current version of a house.
var ErrVersionNotApplied = errors.New("registry version is not applied")

// Store reads and writes registry data.
type Store struct {
	pool *pgxpool.Pool
	tx   *db.TransactionManager
}

// NewStore creates a registry store. Multi-row operations run through the shared
// TransactionManager (docs/05, «Правило атомарности»).
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, tx: db.NewTransactionManager(pool)}
}

// HouseSummary is what a user sees about a house before any verification:
// the invite link is public, so the summary contains no personal data.
type HouseSummary struct {
	ID              string
	InviteSlug      string
	Address         string
	Region          string
	Locality        string
	Street          string
	HouseNumber     string
	Building        string
	Structure       string
	FIASID          string
	IsDemo          bool
	PremisesCount   int
	RegistryVersion *int
	// TotalAreaCenti is the total area of the current registry version:
	// the base for 10% / quorum / 2/3. Nil when the house has no registry yet.
	TotalAreaCenti *int64
}

// HouseBySlug finds a house by its invite slug.
func (s *Store) HouseBySlug(ctx context.Context, slug string) (HouseSummary, error) {
	var h HouseSummary
	err := s.pool.QueryRow(ctx, `
		SELECT h.id::text, h.invite_slug, h.address, h.region,
		       COALESCE(h.locality, ''), COALESCE(h.street, ''), COALESCE(h.house_number, ''),
		       COALESCE(h.building, ''), COALESCE(h.structure, ''), COALESCE(h.fias_id, ''), h.is_demo,
		       (SELECT count(*) FROM premises p WHERE p.house_id = h.id),
		       h.current_registry_version, ru.total_area_centi
		FROM houses h
		LEFT JOIN registry_uploads ru
		       ON ru.house_id = h.id AND ru.version = h.current_registry_version
		WHERE h.invite_slug = $1`, slug,
	).Scan(&h.ID, &h.InviteSlug, &h.Address, &h.Region, &h.Locality, &h.Street,
		&h.HouseNumber, &h.Building, &h.Structure, &h.FIASID, &h.IsDemo,
		&h.PremisesCount, &h.RegistryVersion, &h.TotalAreaCenti)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, ErrNotFound
	}
	if err != nil {
		return h, fmt.Errorf("house by slug: %w", err)
	}

	return h, nil
}

// SearchHouses finds public house cards by address. Results contain no registry
// or personal data and are limited to houses with an imported premises registry.
func (s *Store) SearchHouses(ctx context.Context, query string) ([]HouseSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT h.id::text, h.invite_slug, h.address, h.region,
		       COALESCE(h.locality, ''), COALESCE(h.street, ''), COALESCE(h.house_number, ''),
		       COALESCE(h.building, ''), COALESCE(h.structure, ''), COALESCE(h.fias_id, ''), h.is_demo,
		       (SELECT count(*) FROM premises p WHERE p.house_id = h.id),
		       h.current_registry_version, ru.total_area_centi
		FROM houses h
		LEFT JOIN registry_uploads ru
		       ON ru.house_id = h.id AND ru.version = h.current_registry_version
		WHERE h.address ILIKE '%' || $1 || '%'
		   OR h.region ILIKE '%' || $1 || '%'
		   OR h.locality ILIKE '%' || $1 || '%'
		   OR h.street ILIKE '%' || $1 || '%'
		   OR h.house_number ILIKE '%' || $1 || '%'
		   OR h.building ILIKE '%' || $1 || '%'
		   OR h.structure ILIKE '%' || $1 || '%'
		ORDER BY CASE WHEN h.address ILIKE $1 || '%' THEN 0 ELSE 1 END,
		         h.is_demo DESC, h.address
		LIMIT 20`, query)
	if err != nil {
		return nil, fmt.Errorf("search houses: %w", err)
	}
	defer rows.Close()
	result := make([]HouseSummary, 0)
	for rows.Next() {
		var h HouseSummary
		if err := rows.Scan(&h.ID, &h.InviteSlug, &h.Address, &h.Region, &h.Locality, &h.Street,
			&h.HouseNumber, &h.Building, &h.Structure, &h.FIASID, &h.IsDemo,
			&h.PremisesCount, &h.RegistryVersion, &h.TotalAreaCenti); err != nil {
			return nil, fmt.Errorf("scan house search result: %w", err)
		}
		result = append(result, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search houses: %w", err)
	}
	return result, nil
}

// applyVersion marks a registry version as applied and makes it the current
// version of the house. This is the only place where current_registry_version
// changes, so «current is always an applied version» holds by construction.
func applyVersion(ctx context.Context, tx pgx.Tx, houseID string, version int) error {
	tag, err := tx.Exec(ctx, `
		UPDATE registry_uploads SET status = 'applied'
		WHERE house_id = $1 AND version = $2 AND status = 'preview'`, houseID, version)
	if err != nil {
		return fmt.Errorf("apply registry version: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("apply registry version %d: %w", version, ErrVersionNotApplied)
	}

	return setCurrentVersion(ctx, tx, houseID, version)
}

// setCurrentVersion points the house to an already applied version.
// A preview version is refused: the composite FK only checks that the version exists.
func setCurrentVersion(ctx context.Context, tx pgx.Tx, houseID string, version int) error {
	tag, err := tx.Exec(ctx, `
		UPDATE houses SET current_registry_version = $2
		WHERE id = $1
		  AND EXISTS (SELECT 1 FROM registry_uploads
		              WHERE house_id = $1 AND version = $2 AND status = 'applied')`, houseID, version)
	if err != nil {
		return fmt.Errorf("set current registry version: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("set current registry version %d: %w", version, ErrVersionNotApplied)
	}

	return nil
}
