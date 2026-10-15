package models

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DockerDeployRun struct {
	ID             int64
	MappingID      int64
	OperationLogID *int64
	ArtifactID     *int64
	ArtifactImage  string
	Status         string
	Message        *string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

type DockerDeployRunLog struct {
	ID        int64
	RunID     int64
	LineNo    int
	Content   string
	CreatedAt time.Time
}

func CreateDockerDeployRun(ctx context.Context, db *pgxpool.Pool, mappingID int64, operationLogID int64, artifactID int64, artifactImage string) (DockerDeployRun, error) {
	rows, err := queryRows(ctx, db, sqlCreateDockerDeployRun, mappingID, nullableInt64(operationLogID), nullableInt64(artifactID), artifactImage)
	if err != nil {
		return DockerDeployRun{}, fmt.Errorf("create docker deploy run: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[DockerDeployRun])
}

func UpdateDockerDeployRunStatus(ctx context.Context, db *pgxpool.Pool, id int64, status string, message string) (DockerDeployRun, error) {
	rows, err := queryRows(ctx, db, sqlUpdateDockerDeployRunStatus, id, status, message)
	if err != nil {
		return DockerDeployRun{}, fmt.Errorf("update docker deploy run status: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[DockerDeployRun])
}

func CreateDockerDeployRunLog(ctx context.Context, db *pgxpool.Pool, runID int64, lineNo int, content string) (DockerDeployRunLog, error) {
	rows, err := queryRows(ctx, db, sqlCreateDockerDeployRunLog, runID, lineNo, content)
	if err != nil {
		return DockerDeployRunLog{}, fmt.Errorf("create docker deploy run log: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[DockerDeployRunLog])
}

func GetLatestDockerDeployRun(ctx context.Context, db *pgxpool.Pool, mappingID int64) (DockerDeployRun, error) {
	rows, err := queryRows(ctx, db, sqlGetLatestDockerDeployRun, mappingID)
	if err != nil {
		return DockerDeployRun{}, fmt.Errorf("get latest docker deploy run: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[DockerDeployRun])
}

func GetDockerDeployRun(ctx context.Context, db *pgxpool.Pool, mappingID int64, runID int64) (DockerDeployRun, error) {
	rows, err := queryRows(ctx, db, sqlGetDockerDeployRun, runID, mappingID)
	if err != nil {
		return DockerDeployRun{}, fmt.Errorf("get docker deploy run: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[DockerDeployRun])
}

func ListDockerDeployRunLogs(ctx context.Context, db *pgxpool.Pool, runID int64) ([]DockerDeployRunLog, error) {
	rows, err := queryRows(ctx, db, sqlListDockerDeployRunLogs, runID)
	if err != nil {
		return nil, fmt.Errorf("list docker deploy run logs: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[DockerDeployRunLog])
}
