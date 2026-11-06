package transaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type contextKey struct{}

type Manager struct {
	pool databasePool
}

func New(pool databasePool) *Manager {
	return &Manager{pool: pool}
}

func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if transactionFromContext(ctx) != nil {
		return fn(ctx)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txContext := context.WithValue(ctx, contextKey{}, tx)

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(ctx)
			panic(recovered)
		}
	}()

	if err := fn(txContext); err != nil {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (m *Manager) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return m.executor(ctx).Exec(ctx, sql, args...)
}

func (m *Manager) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.executor(ctx).QueryRow(ctx, sql, args...)
}

func (m *Manager) executor(ctx context.Context) dbExecutor {
	if tx := transactionFromContext(ctx); tx != nil {
		return tx
	}
	return m.pool
}

func transactionFromContext(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(contextKey{}).(pgx.Tx)
	return tx
}
