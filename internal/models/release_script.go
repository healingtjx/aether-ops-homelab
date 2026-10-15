package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReleaseScript struct {
	ID               int64
	Name             string
	Description      *string
	Content          string
	DefaultVariables Variables
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func ListReleaseScripts(ctx context.Context, db *pgxpool.Pool) ([]ReleaseScript, error) {
	rows, err := queryRows(ctx, db, sqlListReleaseScripts)
	if err != nil {
		return nil, fmt.Errorf("list release scripts: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ReleaseScript])
}

func GetReleaseScript(ctx context.Context, db *pgxpool.Pool, id int64) (ReleaseScript, error) {
	rows, err := queryRows(ctx, db, sqlGetReleaseScript, id)
	if err != nil {
		return ReleaseScript{}, fmt.Errorf("get release script: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[ReleaseScript])
}

func CreateReleaseScript(ctx context.Context, db *pgxpool.Pool, name string, description *string, content string, defaultVariables Variables) (ReleaseScript, error) {
	var id int64
	err := queryRow(ctx, db, sqlCreateReleaseScript, name, description, content, defaultVariables).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return ReleaseScript{}, fmt.Errorf("已存在同名发布脚本")
		}
		return ReleaseScript{}, fmt.Errorf("create release script: %w", err)
	}
	return GetReleaseScript(ctx, db, id)
}

func UpdateReleaseScript(ctx context.Context, db *pgxpool.Pool, id int64, name string, description *string, content string, defaultVariables Variables) (ReleaseScript, error) {
	_, err := execSQL(ctx, db, sqlUpdateReleaseScript, id, name, description, content, defaultVariables)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return ReleaseScript{}, fmt.Errorf("已存在同名发布脚本")
		}
		return ReleaseScript{}, fmt.Errorf("update release script: %w", err)
	}
	return GetReleaseScript(ctx, db, id)
}

func DeleteReleaseScript(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := execSQL(ctx, db, sqlDeleteReleaseScript, id)
	if err != nil {
		return fmt.Errorf("delete release script: %w", err)
	}
	return nil
}
