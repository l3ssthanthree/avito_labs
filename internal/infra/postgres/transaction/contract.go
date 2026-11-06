package transaction

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type dbExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type databasePool interface {
	dbExecutor
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}
