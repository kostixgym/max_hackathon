// Package access is the «Доступ и роли» module: users, memberships (links between
// a user and a premise) and management company staff.
package access

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// User is a MAX user known to the service. Name and photo from MAX are not stored.
type User struct {
	ID        string
	MaxUserID int64
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
