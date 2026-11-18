package execution_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/testutil"
)

func getExecutionTestDB(t *testing.T) *sql.DB { return testutil.OpenMySQL(t) }

func TestRepository_ReapExpiredSteps(t *testing.T) {
	db := getExecutionTestDB(t)
	repo := execution.NewExecutionRepository(db, nil)
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
		INSERT INTO step_executions (task_queue,
			id, workflow_execution_id, step_id, name, type, config, depends_on, status,
			attempt, max_attempts, locked_by, locked_until, is_compensation, created_at, updated_at
		) VALUES ('orders',
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

func TestRepository_WakeUpDependents_And_ClaimTask(t *testing.T) {
	db := getExecutionTestDB(t)
	repo := execution.NewExecutionRepository(db, nil)
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
		INSERT INTO step_executions (task_queue, id, workflow_execution_id, step_id, name, type, config, depends_on, status, is_compensation)
		VALUES ('orders', ?, ?, 'step-A', 'Step A', 'http', '{}', '[]', 'COMPLETED', FALSE)
	`, seAID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert step execution A: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (task_queue, id, workflow_execution_id, step_id, name, type, config, depends_on, status, is_compensation)
		VALUES ('orders', ?, ?, 'step-B', 'Step B', 'http', '{}', '["step-A"]', 'WAITING', FALSE)
	`, seBID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert step execution B: %v", err)
	}

	// Complete the original through the public lifecycle operation.
	_, err = db.ExecContext(ctx, "UPDATE step_executions SET status='PENDING' WHERE id=?", seAID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimTask(ctx, execution.ClaimFilter{ServiceID: "test-service", WorkerID: "worker-a", Queue: "orders", LeaseDuration: 30 * time.Second})
	if err != nil || first == nil {
		t.Fatalf("claim first: %v %v", first, err)
	}
	err = repo.CompleteTask(ctx, "test-service", seAID, "worker-a", first.LeaseToken, nil)
	if err != nil {
		t.Fatal(err)
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
	claimed, err := repo.ClaimTask(ctx, execution.ClaimFilter{ServiceID: "test-service", WorkerID: "integration-worker-claim", Queue: "orders", LeaseDuration: 30 * time.Second})
	if err != nil {
		t.Fatalf("ClaimTask error: %v", err)
	}
	if claimed == nil || claimed.StepExecution == nil {
		t.Fatalf("expected claimed step and execution, got nil")
	}
	if claimed.StepExecution.StepID != "step-B" {
		t.Errorf("expected claimed step to be step-B, got %s", claimed.StepExecution.StepID)
	}
	if claimed.StepExecution.Status != execution.StatusRunning {
		t.Errorf("expected status RUNNING, got %s", claimed.StepExecution.Status)
	}
}

func TestRepository_ReapExpiredSteps_ExhaustedAttempts(t *testing.T) {
	db := getExecutionTestDB(t)
	repo := execution.NewExecutionRepository(db, nil)
	ctx := context.Background()

	wfDefID := uuid.NewString()
	wfExecID := uuid.NewString()
	stepExec1 := uuid.NewString()
	stepExec2 := uuid.NewString()

	_, err := db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, context) VALUES (?, ?, 1, '{}')", wfDefID, "test-exhaust-"+wfDefID[:8])
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

	expiredTime := time.Now().UTC().Add(-10 * time.Minute)
	// Step 1: RUNNING, attempt = 1, max_attempts = 1, expired
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (task_queue,
			id, workflow_execution_id, step_id, name, type, config, depends_on, status,
			attempt, max_attempts, locked_by, locked_until, is_compensation, created_at, updated_at
		) VALUES ('orders',
			?, ?, 'step-1', 'Step 1', 'http', '{}', '[]', 'RUNNING',
			1, 1, 'crashed-worker-1', ?, FALSE, NOW(), NOW()
		)
	`, stepExec1, wfExecID, expiredTime)
	if err != nil {
		t.Fatalf("failed to insert step 1: %v", err)
	}

	// Step 2: PENDING downstream step
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (task_queue,
			id, workflow_execution_id, step_id, name, type, config, depends_on, status,
			attempt, max_attempts, is_compensation, created_at, updated_at
		) VALUES ('orders',
			?, ?, 'step-2', 'Step 2', 'http', '{}', '["step-1"]', 'PENDING',
			0, 1, FALSE, NOW(), NOW()
		)
	`, stepExec2, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert step 2: %v", err)
	}

	// Execute ReapExpiredSteps
	reaped, err := repo.ReapExpiredSteps(ctx)
	if err != nil {
		t.Fatalf("ReapExpiredSteps error: %v", err)
	}
	if reaped < 1 {
		t.Errorf("expected at least 1 reaped step, got %d", reaped)
	}

	// Verify Step 1 is FAILED with error message and locks cleared
	var (
		status1      string
		errMsg1      sql.NullString
		lockedBy1    sql.NullString
		lockedUntil1 sql.NullTime
	)
	err = db.QueryRowContext(ctx, "SELECT status, error_message, locked_by, locked_until FROM step_executions WHERE id = ?", stepExec1).Scan(&status1, &errMsg1, &lockedBy1, &lockedUntil1)
	if err != nil {
		t.Fatalf("failed to query step 1: %v", err)
	}
	if status1 != string(execution.StatusFailed) {
		t.Errorf("expected step 1 to be FAILED, got %s", status1)
	}
	if !errMsg1.Valid || errMsg1.String != "lease expired: max attempts reached" {
		t.Errorf("expected error message 'lease expired: max attempts reached', got %v", errMsg1)
	}
	if lockedBy1.Valid || lockedUntil1.Valid {
		t.Errorf("expected locks to be cleared on step 1")
	}

	// Verify Step 2 is CANCELLED
	var status2 string
	err = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepExec2).Scan(&status2)
	if err != nil {
		t.Fatalf("failed to query step 2: %v", err)
	}
	if status2 != string(execution.StatusCancelled) {
		t.Errorf("expected step 2 to be CANCELLED, got %s", status2)
	}

	// Verify workflow execution is FAILED
	var wfStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if err != nil {
		t.Fatalf("failed to query workflow status: %v", err)
	}
	if wfStatus != string(execution.StatusFailed) {
		t.Errorf("expected workflow status FAILED, got %s", wfStatus)
	}
}
