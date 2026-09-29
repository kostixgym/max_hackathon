package access

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ConfigureUK synchronizes the demo organization's staff with UK_MAX_USER_IDS.
// When the variable is present, only listed MAX accounts keep access. The
// transaction makes removing an ID revoke its rights on the next app restart.
func (s *Store) ConfigureUK(ctx context.Context, orgID string, ids []int64, configured bool) error {
	if !configured {
		return nil
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("configure UK: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM org_members
		WHERE org_id = $1::uuid
		  AND user_id NOT IN (SELECT id FROM users WHERE max_user_id = ANY($2::bigint[]))`, orgID, ids); err != nil {
		return fmt.Errorf("configure UK: revoke removed ids: %w", err)
	}
	if len(ids) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (max_user_id)
			SELECT unnest($1::bigint[])
			ON CONFLICT (max_user_id) DO NOTHING`, ids); err != nil {
			return fmt.Errorf("configure UK: create users: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO org_members (user_id, org_id, role)
			SELECT id, $2::uuid, 'operator' FROM users WHERE max_user_id = ANY($1::bigint[])
			ON CONFLICT (user_id, org_id) DO NOTHING`, ids, orgID); err != nil {
			return fmt.Errorf("configure UK: grant ids: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("configure UK: commit: %w", err)
	}
	s.ukIDsConfigured = true
	s.ukIDs = make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		s.ukIDs[id] = struct{}{}
	}
	return nil
}

func (s *Store) configuredUK(ctx context.Context, userID string) (bool, error) {
	if !s.ukIDsConfigured {
		return true, nil
	}
	var maxID int64
	if err := s.pool.QueryRow(ctx, `SELECT max_user_id FROM users WHERE id = $1::uuid`, userID).Scan(&maxID); err != nil {
		return false, fmt.Errorf("check configured UK user: %w", err)
	}
	_, ok := s.ukIDs[maxID]
	return ok, nil
}
