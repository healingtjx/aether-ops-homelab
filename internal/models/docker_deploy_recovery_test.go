package models

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestDockerDeploymentRecovery(t *testing.T) {
	dsn := os.Getenv("AETHER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AETHER_TEST_DATABASE_URL for PostgreSQL recovery checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `
 CREATE TEMP TABLE docker_deploy_mappings (
 id bigint PRIMARY KEY, last_deploy_status text, last_deploy_message text,
 last_deployed_at timestamptz, updated_at timestamptz
 ) ON COMMIT DROP;
 CREATE TEMP TABLE operation_logs (
 id bigint PRIMARY KEY, status text, message text, finished_at timestamptz
 ) ON COMMIT DROP;
 CREATE TEMP TABLE docker_deploy_runs (
 id bigserial PRIMARY KEY, mapping_id bigint, operation_log_id bigint,
 artifact_id bigint, artifact_image text, status text DEFAULT 'running', message text,
 started_at timestamptz DEFAULT NOW(), finished_at timestamptz
 ) ON COMMIT DROP;
 INSERT INTO docker_deploy_mappings (id) VALUES (1), (2);
 INSERT INTO operation_logs (id, status) VALUES (10, 'running'), (20, 'running');`)
	if err != nil {
		t.Fatal(err)
	}
	run, err := createDockerDeployRun(ctx, tx, 1, 10, 100, "image:old")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createDockerDeployRun(ctx, tx, 1, 20, 100, "image:duplicate"); !errors.Is(err, ErrDockerDeployRunning) {
		t.Fatalf("duplicate start: %v", err)
	}
	check := func(table, column string, id int64, want string) {
		t.Helper()
		var got string
		if err := tx.QueryRow(ctx, "SELECT "+column+" FROM "+table+" WHERE id=$1", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s.%s[%d]=%q, want %q", table, column, id, got, want)
		}
	}
	if err := changeDockerDeploymentTx(ctx, tx, 1, 0, "failed", "manual reset"); err != nil {
		t.Fatal(err)
	}
	check("docker_deploy_mappings", "last_deploy_status", 1, "failed")
	check("docker_deploy_runs", "status", run.ID, "failed")
	check("operation_logs", "status", 10, "failed")
	next, err := createDockerDeployRun(ctx, tx, 1, 20, 100, "image:new")
	if err != nil {
		t.Fatal(err)
	}
	if err := changeDockerDeploymentTx(ctx, tx, 1, run.ID, "success", "late old result"); err != nil {
		t.Fatal(err)
	}
	check("docker_deploy_mappings", "last_deploy_status", 1, "running")
	check("docker_deploy_runs", "status", run.ID, "failed")
	check("operation_logs", "status", 10, "failed")
	if err := changeDockerDeploymentTx(ctx, tx, 1, next.ID, "success", "done"); err != nil {
		t.Fatal(err)
	}
	check("docker_deploy_mappings", "last_deploy_status", 1, "success")
	check("operation_logs", "status", 20, "success")
	// Reset on a finished mapping is a no-op.
	if err := changeDockerDeploymentTx(ctx, tx, 1, 0, "failed", "reset"); err != nil {
		t.Fatal(err)
	}
	check("docker_deploy_mappings", "last_deploy_status", 1, "success")
	// Older installations can have a stuck mapping with no run at all.
	if _, err := tx.Exec(ctx, `UPDATE docker_deploy_mappings SET last_deploy_status='running' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if err := changeDockerDeploymentTx(ctx, tx, 2, 0, "failed", "legacy reset"); err != nil {
		t.Fatal(err)
	}
	check("docker_deploy_mappings", "last_deploy_status", 2, "failed")
	if err := changeDockerDeploymentTx(ctx, tx, 999, 0, "failed", "missing"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing mapping: %v", err)
	}
}
