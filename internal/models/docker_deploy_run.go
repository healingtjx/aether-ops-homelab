package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDockerDeployRunning = errors.New("该映射正在发布中；若任务已停止或卡死，请重置状态后重试")

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
	tx, err := db.Begin(ctx)
	if err != nil {
		return DockerDeployRun{}, err
	}
	defer tx.Rollback(ctx)
	run, err := createDockerDeployRun(ctx, tx, mappingID, operationLogID, artifactID, artifactImage)
	if err != nil {
		return DockerDeployRun{}, err
	}
	return run, tx.Commit(ctx)
}

func createDockerDeployRun(ctx context.Context, tx pgx.Tx, mappingID, operationLogID, artifactID int64, artifactImage string) (DockerDeployRun, error) {
	var status *string
	if err := tx.QueryRow(ctx, `SELECT last_deploy_status FROM docker_deploy_mappings WHERE id=$1 FOR UPDATE`, mappingID).Scan(&status); err != nil {
		return DockerDeployRun{}, err
	}
	if status != nil && *status == "running" {
		return DockerDeployRun{}, ErrDockerDeployRunning
	}
	if _, err := tx.Exec(ctx, sqlUpdateDockerDeployStatus, mappingID, "running", ""); err != nil {
		return DockerDeployRun{}, err
	}
	rows, err := queryRows(ctx, tx, sqlCreateDockerDeployRun, mappingID, nullableInt64(operationLogID), nullableInt64(artifactID), artifactImage)
	if err != nil {
		return DockerDeployRun{}, fmt.Errorf("create docker deploy run: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[DockerDeployRun])
}

// FinishDockerDeployment and ResetDockerDeployment lock the mapping in the same
// order as creation. A reset run can never overwrite a later deployment.
func FinishDockerDeployment(ctx context.Context, db *pgxpool.Pool, mappingID, runID int64, status, message string) error {
	return changeDockerDeployment(ctx, db, mappingID, runID, status, message)
}

func ResetDockerDeployment(ctx context.Context, db *pgxpool.Pool, mappingID int64) error {
	return changeDockerDeployment(ctx, db, mappingID, 0, "failed", "手动重置发布状态，允许重新发布")
}

func changeDockerDeployment(ctx context.Context, db *pgxpool.Pool, mappingID, runID int64, status, message string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := changeDockerDeploymentTx(ctx, tx, mappingID, runID, status, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func changeDockerDeploymentTx(ctx context.Context, tx pgx.Tx, mappingID, runID int64, status, message string) error {
	var current *string
	if err := tx.QueryRow(ctx, `SELECT last_deploy_status FROM docker_deploy_mappings WHERE id=$1 FOR UPDATE`, mappingID).Scan(&current); err != nil {
		return err
	}
	if current == nil || *current != "running" {
		return nil
	}
	var finished bool
	err := tx.QueryRow(ctx, `WITH finished AS (
 UPDATE docker_deploy_runs SET status=$3, message=$4, finished_at=NOW()
 WHERE mapping_id=$1 AND status='running' AND ($2=0 OR id=$2)
 RETURNING operation_log_id
), logged AS (
 UPDATE operation_logs SET status=$3, message=$4, finished_at=NOW()
 WHERE id IN (SELECT operation_log_id FROM finished)
)
SELECT EXISTS(SELECT 1 FROM finished)`, mappingID, runID, status, message).Scan(&finished)
	if err != nil {
		return err
	}
	// Only the latest run owns the mapping. Manual reset also supports legacy
	// stuck mappings without a corresponding run record.
	if runID != 0 {
		if !finished {
			return nil
		}
		var owns bool
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(id)=$2, FALSE)
FROM docker_deploy_runs WHERE mapping_id=$1`, mappingID, runID).Scan(&owns); err != nil {
			return err
		}
		if !owns {
			return nil
		}
	}
	query := sqlUpdateDockerDeployStatus
	if status == "success" {
		query = sqlUpdateDockerDeployStatusSuccess
	}
	_, err = tx.Exec(ctx, query, mappingID, status, message)
	return err
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
