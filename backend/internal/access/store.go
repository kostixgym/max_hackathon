// Package access is the «Доступ и роли» module: users, memberships (links between
// a user and a premise) and management company staff.
package access

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound means that the requested house or premise does not exist.
var ErrNotFound = errors.New("not found")

// ErrForbidden means that the user may not view the requested owner directory.
var ErrForbidden = errors.New("forbidden")

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
	House   HouseRef
	Premise PremiseRef
	Owner   *OwnerSummary
}

// HouseRef is the part of a house needed for a user's membership card.
type HouseRef struct {
	ID         string
	InviteSlug string
	Address    string
	Region     string
	IsDemo     bool
}

// PremiseRef is the non-personal part of a premise shown to its member.
type PremiseRef struct {
	ID               string
	Number           string
	Kind             string
	Entrance         *int
	Floor            *int
	DisplayAreaCenti *int64
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

// Store reads and writes access data.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates an access store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// UpsertUser returns the user with the given MAX id, creating it on the first visit.
func (s *Store) UpsertUser(ctx context.Context, maxUserID int64) (User, error) {
	u := User{MaxUserID: maxUserID}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (max_user_id) VALUES ($1)
		ON CONFLICT (max_user_id) DO UPDATE SET max_user_id = EXCLUDED.max_user_id
		RETURNING id::text`, maxUserID,
	).Scan(&u.ID)
	if err != nil {
		return u, fmt.Errorf("upsert user: %w", err)
	}

	return u, nil
}

// MembershipsByUser returns all of the user's non-revoked links together with
// the house, premise and current owner facts required by the mini-app home page.
func (s *Store) MembershipsByUser(ctx context.Context, userID string) ([]MembershipSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id::text, m.role, m.status, m.method,
		       h.id::text, h.invite_slug, h.address, h.region, h.is_demo,
		       p.id::text, p.number, p.kind, p.entrance, p.floor, p.display_area_centi,
		       o.id::text, r.full_name, r.owner_kind,
		       r.share_num, r.share_den, r.weight_num, r.weight_den
		FROM memberships m
		JOIN premises p ON p.id = m.premise_id
		JOIN houses h ON h.id = p.house_id
		LEFT JOIN owners o ON o.id = m.owner_id
		LEFT JOIN registry_uploads ru
		       ON ru.house_id = h.id AND ru.version = h.current_registry_version
		LEFT JOIN owner_records r
		       ON r.owner_id = o.id AND r.registry_upload_id = ru.id
		WHERE m.user_id = $1::uuid AND m.status <> 'revoked'
		ORDER BY h.address, p.number, m.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user memberships: %w", err)
	}
	defer rows.Close()

	result := make([]MembershipSummary, 0)
	for rows.Next() {
		var item MembershipSummary
		var method, ownerID, fullName, ownerKind sql.NullString
		var entrance, floor, displayArea sql.NullInt64
		var shareNum, shareDen, weightNum, weightDen sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.Role, &item.Status, &method,
			&item.House.ID, &item.House.InviteSlug, &item.House.Address, &item.House.Region, &item.House.IsDemo,
			&item.Premise.ID, &item.Premise.Number, &item.Premise.Kind, &entrance, &floor, &displayArea,
			&ownerID, &fullName, &ownerKind, &shareNum, &shareDen, &weightNum, &weightDen,
		); err != nil {
			return nil, fmt.Errorf("scan user membership: %w", err)
		}

		item.Method = nullableString(method)
		item.Premise.Entrance = nullableInt(entrance)
		item.Premise.Floor = nullableInt(floor)
		item.Premise.DisplayAreaCenti = nullableInt64(displayArea)
		if ownerID.Valid && fullName.Valid && ownerKind.Valid && shareNum.Valid && shareDen.Valid && weightNum.Valid && weightDen.Valid {
			item.Owner = &OwnerSummary{
				ID: ownerID.String, PremiseID: item.Premise.ID, PremiseNumber: item.Premise.Number,
				MaskedName: maskOwnerName(fullName.String, ownerKind.String), Kind: ownerKind.String,
				ShareNum: shareNum.Int64, ShareDen: shareDen.Int64,
				WeightNum: weightNum.Int64, WeightDen: weightDen.Int64,
			}
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user memberships: %w", err)
	}

	return result, nil
}

// PremiseOwners returns owners from the current registry version. A pending or
// verified member of the premise needs this list to choose their owner record;
// staff of the house's management organization may also view it.
func (s *Store) PremiseOwners(ctx context.Context, userID, premiseID string) ([]OwnerSummary, error) {
	id, err := parseResourceID(premiseID)
	if err != nil {
		return nil, err
	}
	exists, allowed, err := s.premiseOwnerAccess(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if !allowed {
		return nil, ErrForbidden
	}

	return s.listOwners(ctx, `p.id = $1`, id)
}

// HouseOfficerCandidates returns current-registry owners that can be selected
// as a meeting chair or secretary. Only a verified owner of the house or its
// management-organization staff may view the house-wide masked directory.
func (s *Store) HouseOfficerCandidates(ctx context.Context, userID, houseID string) ([]OwnerSummary, error) {
	id, err := parseResourceID(houseID)
	if err != nil {
		return nil, err
	}
	exists, allowed, err := s.houseOwnerAccess(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if !allowed {
		return nil, ErrForbidden
	}

	return s.listOwners(ctx, `h.id = $1`, id)
}

func (s *Store) premiseOwnerAccess(ctx context.Context, userID string, premiseID pgtype.UUID) (bool, bool, error) {
	var exists, allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM premises p WHERE p.id = $2),
			EXISTS (
				SELECT 1
				FROM memberships m
				WHERE m.user_id = $1::uuid AND m.premise_id = $2
				  AND m.status IN ('pending', 'verified')
				UNION ALL
				SELECT 1
				FROM org_members om
				JOIN houses h ON h.org_id = om.org_id
				JOIN premises p ON p.house_id = h.id
				WHERE om.user_id = $1::uuid AND p.id = $2
			)`, userID, premiseID).Scan(&exists, &allowed)
	if err != nil {
		return false, false, fmt.Errorf("check premise owner access: %w", err)
	}

	return exists, allowed, nil
}

func (s *Store) houseOwnerAccess(ctx context.Context, userID string, houseID pgtype.UUID) (bool, bool, error) {
	var exists, allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM houses h WHERE h.id = $2),
			EXISTS (
				SELECT 1
				FROM memberships m
				JOIN premises p ON p.id = m.premise_id
				WHERE m.user_id = $1::uuid AND p.house_id = $2
				  AND m.role = 'owner' AND m.status = 'verified'
				UNION ALL
				SELECT 1
				FROM org_members om
				JOIN houses h ON h.org_id = om.org_id
				WHERE om.user_id = $1::uuid AND h.id = $2
			)`, userID, houseID).Scan(&exists, &allowed)
	if err != nil {
		return false, false, fmt.Errorf("check house owner access: %w", err)
	}

	return exists, allowed, nil
}

func (s *Store) listOwners(ctx context.Context, where string, id pgtype.UUID) ([]OwnerSummary, error) {
	query := `
		SELECT o.id::text, p.id::text, p.number, r.full_name, r.owner_kind,
		       r.share_num, r.share_den, r.weight_num, r.weight_den
		FROM owners o
		JOIN premises p ON p.id = o.premise_id
		JOIN houses h ON h.id = p.house_id
		JOIN registry_uploads ru
		     ON ru.house_id = h.id AND ru.version = h.current_registry_version
		JOIN owner_records r
		     ON r.owner_id = o.id AND r.registry_upload_id = ru.id
		WHERE ` + where + `
		ORDER BY p.number, r.full_name, o.id`
	rows, err := s.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("list owners: %w", err)
	}
	defer rows.Close()

	result := make([]OwnerSummary, 0)
	for rows.Next() {
		var item OwnerSummary
		var fullName string
		if err := rows.Scan(&item.ID, &item.PremiseID, &item.PremiseNumber, &fullName, &item.Kind,
			&item.ShareNum, &item.ShareDen, &item.WeightNum, &item.WeightDen); err != nil {
			return nil, fmt.Errorf("scan owner: %w", err)
		}
		item.MaskedName = maskOwnerName(fullName, item.Kind)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list owners: %w", err)
	}

	return result, nil
}

func parseResourceID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return pgtype.UUID{}, ErrNotFound
	}

	return id, nil
}

func maskOwnerName(fullName, kind string) string {
	if kind != "person" {
		return fullName
	}
	parts := strings.Fields(fullName)
	if len(parts) < 2 {
		return fullName
	}
	masked := []string{parts[0]}
	for _, part := range parts[1:] {
		r, _ := utf8.DecodeRuneInString(part)
		if r != utf8.RuneError {
			masked = append(masked, string(r)+".")
		}
	}

	return strings.Join(masked, " ")
}

func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	value := v.String

	return &value
}

func nullableInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	value := int(v.Int64)

	return &value
}

func nullableInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64

	return &value
}
