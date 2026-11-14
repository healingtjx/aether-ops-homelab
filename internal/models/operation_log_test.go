package models

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// All fixtures live in temporary tables on one connection and are rolled back;
// this test never reads or changes the database's application tables.
func TestDeleteOperationLogRecovery(t *testing.T) {
	dsn := os.Getenv("AETHER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AETHER_TEST_DATABASE_URL to run PostgreSQL recovery checks")
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
CREATE TEMP TABLE ci_configs (
 id bigint PRIMARY KEY, last_run_status text, last_run_message text, updated_at timestamptz
) ON COMMIT DROP;
CREATE TEMP TABLE operation_logs (
 id bigint PRIMARY KEY, ci_config_id bigint, operation_type text, status text
) ON COMMIT DROP;`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		kind       string
		logStatus  string
		ciStatus   string
		newer      bool
		wantStatus string
	}{
		{"stuck build", "artifact_build", "running", "running", false, "failed"},
		{"stuck CI", "deploy", "running", "running", false, "failed"},
		{"historical run", "artifact_build", "running", "running", true, "running"},
		{"completed log", "artifact_build", "success", "running", false, "running"},
		{"already finished config", "artifact_build", "running", "success", false, "success"},
		{"artifact deployment is separate", "artifact_deploy", "running", "running", false, "running"},
		{"pull is separate", "pull", "running", "running", false, "running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tx.Exec(ctx, `TRUNCATE operation_logs, ci_configs`); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ci_configs (id, last_run_status) VALUES (1, $1), (2, 'running')`, tc.ciStatus); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO operation_logs VALUES (10, 1, $1, $2), (30, 2, 'artifact_build', 'running')`, tc.kind, tc.logStatus); err != nil {
				t.Fatal(err)
			}
			if tc.newer {
				if _, err := tx.Exec(ctx, `INSERT INTO operation_logs VALUES (20, 1, 'artifact_build', 'running')`); err != nil {
					t.Fatal(err)
				}
			}
			var deletedID int64
			if err := tx.QueryRow(ctx, sqlDeleteOperationLog, int64(10)).Scan(&deletedID); err != nil {
				t.Fatal(err)
			}
			var status, otherStatus string
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT last_run_status, EXISTS(SELECT 1 FROM operation_logs WHERE id=10) FROM ci_configs WHERE id=1`).Scan(&status, &exists); err != nil {
				t.Fatal(err)
			}
			if deletedID != 10 || exists || status != tc.wantStatus {
				t.Fatalf("deleted=%d exists=%v status=%q, want status=%q", deletedID, exists, status, tc.wantStatus)
			}
			if err := tx.QueryRow(ctx, `SELECT last_run_status FROM ci_configs WHERE id=2`).Scan(&otherStatus); err != nil || otherStatus != "running" {
				t.Fatalf("unrelated config changed: status=%q err=%v", otherStatus, err)
			}
		})
	}
	var id int64
	if err := tx.QueryRow(ctx, sqlDeleteOperationLog, int64(999)).Scan(&id); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing log: got %v, want ErrNoRows", err)
	}
}
