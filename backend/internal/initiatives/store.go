// Package initiatives is the «Инициативы» module: the life cycle of an idea from a
// draft to the outcome of the meeting. Stage 1 fills it in; for now it tells the
// access module who organizes a meeting.
package initiatives

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store reads and writes initiatives.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates an initiatives store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// IsPathBInitiator reports whether the user is the initiator of an active path-B
// initiative of the house: on path B the initiator administers the meeting
// (docs/04, «Кто что может»). A hidden initiative does not count.
func (s *Store) IsPathBInitiator(ctx context.Context, userID, houseID string) (bool, error) {
	var initiator bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM initiatives
			WHERE house_id = $2::uuid AND initiator_user_id = $1::uuid
			  AND path = 'B' AND stage IN ('poll', 'demand', 'meeting')
			  AND hidden_at IS NULL)`, userID, houseID).Scan(&initiator)
	if err != nil {
		return false, fmt.Errorf("check path-B initiator: %w", err)
	}

	return initiator, nil
}
