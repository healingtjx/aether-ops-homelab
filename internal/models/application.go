package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Application struct {
	ID              int64
	ProjectID       int64
	ProjectName     string
	Name            string
	Description     *string
	GitURL          string
	DefaultBranch   string
	LastPulledAt    *time.Time
	LastPullStatus  *string
	LastPullMessage *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func ListApplications(ctx context.Context, db *pgxpool.Pool) ([]Application, error) {
	rows, err := queryRows(ctx, db, sqlListApplications)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Application])
}

func GetApplication(ctx context.Context, db *pgxpool.Pool, id int64) (Application, error) {
	rows, err := queryRows(ctx, db, sqlGetApplication, id)
	if err != nil {
		return Application{}, fmt.Errorf("get application: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[Application])
}

func CreateApplication(ctx context.Context, db *pgxpool.Pool, projectID int64, name string, description *string, gitURL string, defaultBranch string) (Application, error) {
	var id int64
	err := queryRow(ctx, db, sqlCreateApplication, projectID, name, description, gitURL, defaultBranch).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return Application{}, fmt.Errorf("该项目中已存在同名应用")
		}
		return Application{}, fmt.Errorf("create application: %w", err)
	}

	return GetApplication(ctx, db, id)
}

func UpdateApplication(ctx context.Context, db *pgxpool.Pool, id int64, projectID int64, name string, description *string, gitURL string, defaultBranch string) (Application, error) {
	rows, err := queryRows(ctx, db, sqlUpdateApplication, id, projectID, name, description, gitURL, defaultBranch)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return Application{}, fmt.Errorf("该项目中已存在同名应用")
		}
		return Application{}, fmt.Errorf("update application: %w", err)
	}
	defer rows.Close()
	return GetApplication(ctx, db, id)
}

func DeleteApplication(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := execSQL(ctx, db, sqlDeleteApplication, id)
	if err != nil {
		return fmt.Errorf("delete application: %w", err)
	}
	return nil
}

func UpdateApplicationBranch(ctx context.Context, db *pgxpool.Pool, id int64, branch string) (Application, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	_, err := execSQL(ctx, db, sqlUpdateApplicationBranch, id, branch)
	if err != nil {
		return Application{}, fmt.Errorf("update application branch: %w", err)
	}
	return GetApplication(ctx, db, id)
}

func UpdatePullStatus(ctx context.Context, db *pgxpool.Pool, id int64, status string, message string) (Application, error) {
	if status == "success" {
		_, err := execSQL(ctx, db, sqlUpdateApplicationPullStatusSuccess, id, status, message)
		if err != nil {
			return Application{}, fmt.Errorf("update pull status: %w", err)
		}
	} else {
		_, err := execSQL(ctx, db, sqlUpdateApplicationPullStatus, id, status, message)
		if err != nil {
			return Application{}, fmt.Errorf("update pull status: %w", err)
		}
	}
	return GetApplication(ctx, db, id)
}
