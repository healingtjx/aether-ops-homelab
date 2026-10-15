package models

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Project struct {
	ID          int64
	Name        string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func ListProjects(ctx context.Context, db *pgxpool.Pool) ([]Project, error) {
	rows, err := queryRows(ctx, db, sqlListProjects)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Project])
}

func GetProject(ctx context.Context, db *pgxpool.Pool, id int64) (Project, error) {
	rows, err := queryRows(ctx, db, sqlGetProject, id)
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[Project])
}

func CreateProject(ctx context.Context, db *pgxpool.Pool, name string, description *string) (Project, error) {
	rows, err := queryRows(ctx, db, sqlCreateProject, name, description)
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[Project])
}

func UpdateProject(ctx context.Context, db *pgxpool.Pool, id int64, name string, description *string) (Project, error) {
	rows, err := queryRows(ctx, db, sqlUpdateProject, id, name, description)
	if err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[Project])
}

func DeleteProject(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := execSQL(ctx, db, sqlDeleteProject, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

func ProjectNameExists(ctx context.Context, db *pgxpool.Pool, name string, excludeID int64) (bool, error) {
	var exists bool
	err := queryRow(ctx, db, sqlProjectNameExists, name, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check project name: %w", err)
	}
	return exists, nil
}
