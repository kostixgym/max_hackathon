package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionManager runs application operations in a PostgreSQL transaction.
// Services own transaction boundaries and pass the provided pgx.Tx to every
// repository participating in the same atomic operation.
type TransactionManager struct {
	pool *pgxpool.Pool
}

// NewTransactionManager creates a transaction manager backed by the shared pool.
func NewTransactionManager(pool *pgxpool.Pool) *TransactionManager {
	return &TransactionManager{pool: pool}
}

// WithinTransaction commits when fn succeeds and rolls back when fn returns an
// error or panics. The callback error is returned unchanged so callers can use
// errors.Is to map domain errors to HTTP responses.
func (m *TransactionManager) WithinTransaction(
	ctx context.Context,
	fn func(context.Context, pgx.Tx) error,
) (err error) {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}

		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && err == nil {
			err = fmt.Errorf("rollback transaction: %w", rollbackErr)
		}
	}()

	if err = fn(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	committed = true

	return nil
}
