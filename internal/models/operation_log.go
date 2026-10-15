package models

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OperationLog struct {
	ID                 int64
	ApplicationID      int64
	ApplicationName    string
	ProjectID          int64
	ProjectName        string
	UserID             *int64
	Username           *string
	CIConfigID         *int64
	ReleaseScriptID    *int64
	ReleaseScriptName  *string
	ReleaseBranch      *string
	ModuleName         *string
	ModulePath         *string
	ArtifactID         *int64
	ArtifactImage      *string
	ComposeServiceName *string
	DeployRemoteDir    *string
	OperationType      string
	Status             string
	Message            *string
	StartedAt          time.Time
	FinishedAt         *time.Time
}

type OperationLogDetail struct {
	CIConfigID         int64
	ReleaseScriptID    int64
	ReleaseScriptName  string
	ReleaseBranch      string
	ModuleName         string
	ModulePath         string
	ArtifactID         int64
	ArtifactImage      string
	ComposeServiceName string
	DeployRemoteDir    string
}

func CreateOperationLog(ctx context.Context, db *pgxpool.Pool, applicationID int64, projectID int64, operationType string, userID int64) (OperationLog, error) {
	return CreateOperationLogWithDetail(ctx, db, applicationID, projectID, operationType, userID, OperationLogDetail{})
}

func CreateOperationLogWithDetail(ctx context.Context, db *pgxpool.Pool, applicationID int64, projectID int64, operationType string, userID int64, detail OperationLogDetail) (OperationLog, error) {
	var id int64
	err := queryRow(ctx, db, sqlCreateOperationLog,
		applicationID,
		projectID,
		operationType,
		userID,
		nullableInt64(detail.CIConfigID),
		nullableInt64(detail.ReleaseScriptID),
		nullableString(detail.ReleaseScriptName),
		nullableString(detail.ReleaseBranch),
		nullableString(detail.ModuleName),
		nullableString(detail.ModulePath),
		nullableInt64(detail.ArtifactID),
		nullableString(detail.ArtifactImage),
		nullableString(detail.ComposeServiceName),
		nullableString(detail.DeployRemoteDir),
	).Scan(&id)
	if err != nil {
		return OperationLog{}, fmt.Errorf("create operation log: %w", err)
	}
	return GetOperationLog(ctx, db, id)
}

func nullableInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func nullableString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func UpdateOperationLogStatus(ctx context.Context, db *pgxpool.Pool, id int64, status string, message string) (OperationLog, error) {
	_, err := execSQL(ctx, db, sqlUpdateOperationLogStatus, id, status, message)
	if err != nil {
		return OperationLog{}, fmt.Errorf("update operation log status: %w", err)
	}
	return GetOperationLog(ctx, db, id)
}

func GetOperationLog(ctx context.Context, db *pgxpool.Pool, id int64) (OperationLog, error) {
	rows, err := queryRows(ctx, db, sqlGetOperationLog, id)
	if err != nil {
		return OperationLog{}, fmt.Errorf("get operation log: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[OperationLog])
}

func ListOperationLogs(ctx context.Context, db *pgxpool.Pool, projectID int64, applicationID int64, limit int, offset int) ([]OperationLog, error) {
	sql, args := buildListOperationLogsSQL(projectID, applicationID, limit, offset)
	rows, err := queryRows(ctx, db, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list operation logs: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[OperationLog])
}

func CountOperationLogs(ctx context.Context, db *pgxpool.Pool, projectID int64, applicationID int64) (int, error) {
	var count int
	sql, args := buildCountOperationLogsSQL(projectID, applicationID)
	err := queryRow(ctx, db, sql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count operation logs: %w", err)
	}
	return count, nil
}
