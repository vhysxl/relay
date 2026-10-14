package execution_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

func getExecutionTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "relay:relay@tcp(localhost:3306)/relay_taskqueue?parseTime=true&clientFoundRows=true"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("skipping integration test, cannot open db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skipf("skipping integration test, cannot ping db: %v", err)
	}

	return db
}

func TestRepository_ReapExpiredSteps(t *testing.T) {
	db := getExecutionTestDB(t)
	stepRepo := step.NewStepRepository(db)
	repo := execution.NewExecutionRepository(db, stepRepo)
	ctx := context.Background()

	wfDefID := uuid.NewString()
	wfExecID := uuid.NewString()
	stepExecID := uuid.NewString()

	// Setup workflow definition and execution
	_, err := db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, context) VALUES (?, ?, 1, '{}')", wfDefID, "test-reap-"+wfDefID[:8])
	if err != nil {
		t.Fatalf("failed to insert workflow definition: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM step_executions WHERE workflow_execution_id = ?", wfExecID)
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_executions WHERE id = ?", wfExecID)
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE id = ?", wfDefID)
	}()

	_, err = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, context) VALUES (?, ?, 'RUNNING', '{}')", wfExecID, wfDefID)
	if err != nil {
		t.Fatalf("failed to insert workflow execution: %v", err)
	}

	// 1. Insert an expired RUNNING step (attempt 1 of 3)
	expiredLockedUntil := time.Now().UTC().Add(-10 * time.Minute)
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on, status,
			attempt, max_attempts, locked_by, locked_until, is_compensation, created_at, updated_at
		) VALUES (
			?, ?, 'step-1', 'Test Step', 'http', '{}', '[]', 'RUNNING',
			1, 3, 'crashed-worker', ?, FALSE, NOW(), NOW()
		)
	`, stepExecID, wfExecID, expiredLockedUntil)
	if err != nil {
		t.Fatalf("failed to insert expired step execution: %v", err)
	}

	// 2. Call ReapExpiredSteps
	reaped, err := repo.ReapExpiredSteps(ctx)
	if err != nil {
		t.Fatalf("ReapExpiredSteps error: %v", err)
	}
	if reaped < 1 {
		t.Errorf("expected at least 1 step to be reaped, got %d", reaped)
	}

	// 3. Verify step status was reverted to PENDING and lock cleared
	var (
		status      string
		lockedBy    sql.NullString
		lockedUntil sql.NullTime
	)
	err = db.QueryRowContext(ctx, "SELECT status, locked_by, locked_until FROM step_executions WHERE id = ?", stepExecID).Scan(&status, &lockedBy, &lockedUntil)
	if err != nil {
		t.Fatalf("failed to query step after reap: %v", err)
	}

	if status != string(execution.StatusPending) {
		t.Errorf("expected status %s, got %s", execution.StatusPending, status)
	}
	if lockedBy.Valid {
		t.Errorf("expected locked_by to be NULL, got %s", lockedBy.String)
	}
	if lockedUntil.Valid {
		t.Errorf("expected locked_until to be NULL, got %v", lockedUntil.Time)
	}
}

func TestRepository_WakeUpDependents_And_ClaimPending(t *testing.T) {
	db := getExecutionTestDB(t)
	stepRepo := step.NewStepRepository(db)
	repo := execution.NewExecutionRepository(db, stepRepo)
	ctx := context.Background()

	wfDefID := uuid.NewString()
	wfExecID := uuid.NewString()
	seAID := uuid.NewString()
	seBID := uuid.NewString()

	// Seed definition
	_, err := db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, context) VALUES (?, ?, 1, '{}')", wfDefID, "wakeup-process-"+wfDefID[:8])
	if err != nil {
		t.Fatalf("failed to insert workflow definition: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM step_executions WHERE workflow_execution_id = ?", wfExecID)
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_executions WHERE id = ?", wfExecID)
		_, _ = db.ExecContext(ctx, "DELETE FROM step_definitions WHERE workflow_id = ?", wfDefID)
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE id = ?", wfDefID)
	}()

	_, err = db.ExecContext(ctx, "INSERT INTO step_definitions (id, workflow_id, name, type, config, depends_on) VALUES ('step-A', ?, 'Step A', 'http', '{}', '[]')", wfDefID)
	if err != nil {
		t.Fatalf("failed to insert step A definition: %v", err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO step_definitions (id, workflow_id, name, type, config, depends_on) VALUES ('step-B', ?, 'Step B', 'http', '{}', '[\"step-A\"]')", wfDefID)
	if err != nil {
		t.Fatalf("failed to insert step B definition: %v", err)
	}

	// Seed execution: Step A is COMPLETED, Step B is WAITING on Step A
	_, err = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, context) VALUES (?, ?, 'RUNNING', '{}')", wfExecID, wfDefID)
	if err != nil {
		t.Fatalf("failed to insert workflow execution: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (id, workflow_execution_id, step_id, name, type, config, depends_on, status, is_compensation)
		VALUES (?, ?, 'step-A', 'Step A', 'http', '{}', '[]', 'COMPLETED', FALSE)
	`, seAID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert step execution A: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (id, workflow_execution_id, step_id, name, type, config, depends_on, status, is_compensation)
		VALUES (?, ?, 'step-B', 'Step B', 'http', '{}', '["step-A"]', 'WAITING', FALSE)
	`, seBID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert step execution B: %v", err)
	}

	// 1. Wake up dependent steps in transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	err = repo.WakeUpDependentsTx(ctx, tx, wfExecID, false)
	if err != nil {
		tx.Rollback()
		t.Fatalf("WakeUpDependentsTx error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit tx: %v", err)
	}

	// 2. Verify Step B transitioned from WAITING -> PENDING
	var seBStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", seBID).Scan(&seBStatus)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if seBStatus != string(execution.StatusPending) {
		t.Errorf("expected Step B to be PENDING, got %s", seBStatus)
	}

	// 3. Claim Step B with worker
	claimedStep, claimedExec, err := repo.ClaimPending(ctx, "integration-worker-claim", 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimPending error: %v", err)
	}
	if claimedStep == nil || claimedExec == nil {
		t.Fatalf("expected claimed step and execution, got nil")
	}
	if claimedStep.ID != "step-B" {
		t.Errorf("expected claimed step to be step-B, got %s", claimedStep.ID)
	}
	if claimedExec.Status != execution.StatusRunning {
		t.Errorf("expected status RUNNING, got %s", claimedExec.Status)
	}
}
