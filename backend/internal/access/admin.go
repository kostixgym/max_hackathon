package access

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/registry"
)

type ManagedUser struct {
	UserID    string
	MaxUserID int64
}

var ErrInvalidRole = errors.New("invalid organization role")

// IsSystemAdmin checks the persisted global administrator role.
func (s *Store) IsSystemAdmin(ctx context.Context, userID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM system_admins WHERE user_id=$1::uuid)`, userID).Scan(&ok)
	return ok, err
}

func (s *Store) IsSystemAdminByMaxID(ctx context.Context, maxID int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM system_admins a JOIN users u ON u.id=a.user_id WHERE u.max_user_id=$1)`, maxID).Scan(&ok)
	return ok, err
}

// SearchKnownUsers returns registered accounts by MAX id for staff assignment.
func (s *Store) SearchKnownUsers(ctx context.Context, query string) ([]ManagedUser, error) {
	query = strings.TrimSpace(query)
	if query == "" || strings.Trim(query, "0123456789") != "" {
		return []ManagedUser{}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, max_user_id FROM users WHERE max_user_id::text LIKE $1 ORDER BY max_user_id LIMIT 50`, "%"+query+"%")
	if err != nil {
		return nil, fmt.Errorf("search known users: %w", err)
	}
	defer rows.Close()
	users := make([]ManagedUser, 0)
	for rows.Next() {
		var u ManagedUser
		if err := rows.Scan(&u.UserID, &u.MaxUserID); err != nil {
			return nil, fmt.Errorf("scan known user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search known users: %w", err)
	}
	return users, nil
}

func (s *Store) AllOrganizations(ctx context.Context) ([]registry.Org, error) {
	reader, ok := s.registry.(interface {
		AllOrgs(context.Context) ([]registry.Org, error)
	})
	if !ok {
		return nil, errors.New("organization listing is unavailable")
	}
	return reader.AllOrgs(ctx)
}

// SetOrgStaff grants or revokes an organization role for an already registered user.
func (s *Store) SetOrgStaff(ctx context.Context, maxUserID int64, orgID, role string, grant bool) error {
	if role != "admin" && role != "operator" {
		return ErrInvalidRole
	}
	if _, err := resourceID(orgID); err != nil {
		return ErrNotFound
	}
	orgs, err := s.registry.OrgsByIDs(ctx, []string{orgID})
	if err != nil {
		return fmt.Errorf("find staff organization: %w", err)
	}
	if len(orgs) == 0 {
		return ErrNotFound
	}
	var userID string
	if err := s.pool.QueryRow(ctx, `SELECT id::text FROM users WHERE max_user_id=$1`, maxUserID).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("find staff user: %w", err)
	}
	if grant {
		_, err := s.pool.Exec(ctx, `INSERT INTO org_members(user_id,org_id,role) VALUES($1::uuid,$2::uuid,$3) ON CONFLICT(user_id,org_id) DO UPDATE SET role=EXCLUDED.role`, userID, orgID, role)
		if err != nil {
			return fmt.Errorf("grant organization role: %w", err)
		}
		return nil
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM org_members WHERE user_id=$1::uuid AND org_id=$2::uuid`, userID, orgID)
	if err != nil {
		return fmt.Errorf("revoke organization role: %w", err)
	}
	return nil
}
