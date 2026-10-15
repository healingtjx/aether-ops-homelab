package models

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type sqlRunner interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func queryRows(ctx context.Context, runner sqlRunner, sql string, args ...any) (pgx.Rows, error) {
	return runner.Query(ctx, sql, args...)
}

func queryRow(ctx context.Context, runner sqlRunner, sql string, args ...any) pgx.Row {
	return runner.QueryRow(ctx, sql, args...)
}

func execSQL(ctx context.Context, runner sqlRunner, sql string, args ...any) (pgconn.CommandTag, error) {
	return runner.Exec(ctx, sql, args...)
}
