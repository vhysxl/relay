package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/worker"
	"github.com/vhysxl/relay/internal/workflow"
)

func getTestDB(t *testing.T) *sql.DB {
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

func TestIntegration_Workflow_Steps_And_ClaimNext(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// setup repositories and logic
	stepRepo := step.NewStepRepository(db)
	stepValidator := step.NewStepValidator()
	workflowRepo := workflow.NewWorkflowRepository(db, stepRepo)
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// 1. CREATE WORKFLOW + STEPS
	uniqueWorkflowName := "order-saga-" + uuid.NewString()[:8]
	stepAID := "step-a-" + uuid.NewString()[:8]
	stepBID := "step-b-" + uuid.NewString()[:8]

	wfReq := &workflow.WorkflowRequest{
		Name:    uniqueWorkflowName,
		Version: 1,
		Context: json.RawMessage(`{"environment":"integration-test"}`),
		Steps: []step.Step{
			{
				ID:     stepAID,
				Name:   "Create Order",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"method":"POST","url":"https://api.test/orders"}`),
			},
			{
				ID:        stepBID,
				Name:      "Process Payment",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"method":"POST","url":"https://api.test/payments"}`),
				DependsOn: []string{stepAID},
			},
		},
	}

	wf, err := workflowLogic.CreateWorkflow(ctx, wfReq)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	t.Logf("✓ Workflow created with ID: %s", wf.ID)

	if len(wf.Steps) != 2 {
		t.Fatalf("expected 2 steps in created workflow, got %d", len(wf.Steps))
	}
	t.Logf("✓ Verified 2 steps associated with workflow %s", wf.ID)

	defer func() {
		// cleanup created workflow
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE id = ?", wf.ID)
	}()

	// 2. CREATE WORKFLOW EXECUTION & STEP EXECUTIONS
	wfExecID := uuid.NewString()
	_, err = db.ExecContext(ctx, `
		INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at)
		VALUES (?, ?, 'RUNNING', NOW(), NOW())
	`, wfExecID, wf.ID)
	if err != nil {
		t.Fatalf("failed to insert workflow execution: %v", err)
	}
	t.Logf("✓ WorkflowExecution created: %s (status: RUNNING)", wfExecID)

	stepExecAID := uuid.NewString()
	stepExecBID := uuid.NewString()

	// Step A execution (no dependencies)
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on, status, attempt, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'http', '{"method":"POST"}', NULL, 'PENDING', 0, NOW(), NOW())
	`, stepExecAID, wfExecID, stepAID, "Create Order")
	if err != nil {
		t.Fatalf("failed to insert step execution A: %v", err)
	}

	// Step B execution (depends on Step A)
	dependsOnJSON, _ := json.Marshal([]string{stepAID})
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on, status, attempt, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'http', '{"method":"POST"}', ?, 'PENDING', 0, NOW(), NOW())
	`, stepExecBID, wfExecID, stepBID, "Process Payment", string(dependsOnJSON))
	if err != nil {
		t.Fatalf("failed to insert step execution B: %v", err)
	}
	t.Logf("✓ StepExecutions created: %s (Step A), %s (Step B depends on A)", stepExecAID, stepExecBID)

	// 3. WORKER-1 CLAIMS NEXT TASK (Should get Step A)
	s1, se1, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil {
		t.Fatalf("worker-1 failed to claim next task: %v", err)
	}
	if s1 == nil || se1 == nil {
		t.Fatalf("expected worker-1 to claim a task, got nil")
	}
	if se1.ID != stepExecAID {
		t.Errorf("expected worker-1 to claim step A (%s), but got %s", stepExecAID, se1.ID)
	}
	if se1.Status != step.StatusRunning {
		t.Errorf("expected claimed execution status RUNNING, got %s", se1.Status)
	}
	if se1.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", se1.Attempt)
	}
	if se1.LockedBy == nil || *se1.LockedBy != "worker-1" {
		t.Errorf("expected locked_by worker-1, got %v", se1.LockedBy)
	}
	t.Logf("✓ Worker-1 successfully claimed Step A (%s): attempt=%d, locked_by=%s", se1.ID, se1.Attempt, *se1.LockedBy)

	// 4. WORKER-2 TRIES TO CLAIM NEXT TASK (Should get NOTHING because Step A is RUNNING and Step B is blocked by A)
	s2, se2, err := workerLogic.ClaimNext(ctx, "worker-2")
	if err != nil {
		t.Fatalf("worker-2 encountered error during claim: %v", err)
	}
	if s2 != nil || se2 != nil {
		t.Errorf("expected worker-2 to get nil (dependency not met and Step A locked), but got: %+v", se2)
	}
	t.Logf("✓ Worker-2 correctly received nil task (Step A is locked and Step B depends on A)")

	// 5. SIMULATE STEP A COMPLETION
	_, err = db.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'COMPLETED', locked_by = NULL, locked_until = NULL, updated_at = NOW()
		WHERE id = ?
	`, stepExecAID)
	if err != nil {
		t.Fatalf("failed to complete step A: %v", err)
	}
	t.Logf("✓ Step A marked as COMPLETED")

	// 6. WORKER-2 CLAIMS NEXT TASK NOW (Should get Step B now that Step A is completed)
	s3, se3, err := workerLogic.ClaimNext(ctx, "worker-2")
	if err != nil {
		t.Fatalf("worker-2 failed to claim next task after Step A completed: %v", err)
	}
	if s3 == nil || se3 == nil {
		t.Fatalf("expected worker-2 to claim Step B, got nil")
	}
	if se3.ID != stepExecBID {
		t.Errorf("expected worker-2 to claim Step B (%s), got %s", stepExecBID, se3.ID)
	}
	if se3.Status != step.StatusRunning {
		t.Errorf("expected Step B status RUNNING, got %s", se3.Status)
	}
	if se3.LockedBy == nil || *se3.LockedBy != "worker-2" {
		t.Errorf("expected locked_by worker-2, got %v", se3.LockedBy)
	}
	t.Logf("✓ Worker-2 successfully claimed Step B (%s) after dependency resolved!", se3.ID)

	// 7. TEST COMPENSATING PRIORITY OVER RUNNING WORKFLOW
	// Create another running workflow step
	otherWfExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'RUNNING', NOW(), NOW())", otherWfExecID, wf.ID)
	otherNormalStepExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, `INSERT INTO step_executions (id, workflow_execution_id, step_id, name, type, config, status, is_compensation, created_at, updated_at) VALUES (?, ?, 'step-normal', 'Normal Step', 'http', '{}', 'PENDING', FALSE, NOW(), NOW())`, otherNormalStepExecID, otherWfExecID)

	// Create emergency compensating workflow step
	compWfExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'COMPENSATING', NOW(), NOW())", compWfExecID, wf.ID)
	compStepExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, `INSERT INTO step_executions (id, workflow_execution_id, step_id, name, type, config, status, is_compensation, created_at, updated_at) VALUES (?, ?, 'step-rollback', 'Rollback Payment', 'http', '{}', 'PENDING', TRUE, NOW(), NOW())`, compStepExecID, compWfExecID)

	// ClaimNext should pick the COMPENSATING step first!
	compStep, compExec, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil {
		t.Fatalf("failed to claim task during compensation test: %v", err)
	}
	if compExec == nil || compExec.ID != compStepExecID {
		t.Errorf("expected COMPENSATING task %s to be claimed first, but got %+v", compStepExecID, compExec)
	}
	t.Logf("✓ Emergency priority verified: COMPENSATING task (%s) was claimed before normal RUNNING task!", compStep.ID)
}

func TestIntegration_Heartbeat_ExtendLease(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	stepValidator := step.NewStepValidator()
	workflowRepo := workflow.NewWorkflowRepository(db, stepRepo)
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// 1. Create test workflow and step definition
	uniqueWorkflowName := "heartbeat-wf-" + uuid.NewString()[:8]
	stepID := "step-hb-" + uuid.NewString()[:8]

	wfReq := &workflow.WorkflowRequest{
		Name:    uniqueWorkflowName,
		Version: 1,
		Steps: []step.Step{
			{
				ID:     stepID,
				Name:   "Heartbeat Test Step",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"method":"GET","url":"https://api.test/heartbeat"}`),
			},
		},
	}

	wf, err := workflowLogic.CreateWorkflow(ctx, wfReq)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE id = ?", wf.ID)
	}()

	// 2. Create workflow execution and step execution
	wfExecID := uuid.NewString()
	_, err = db.ExecContext(ctx, `
		INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at)
		VALUES (?, ?, 'RUNNING', NOW(), NOW())
	`, wfExecID, wf.ID)
	if err != nil {
		t.Fatalf("failed to insert workflow execution: %v", err)
	}

	stepExecID := uuid.NewString()
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, status, attempt, created_at, updated_at
		) VALUES (?, ?, ?, 'Heartbeat Test Step', 'http', '{"method":"GET"}', 'PENDING', 0, NOW(), NOW())
	`, stepExecID, wfExecID, stepID)
	if err != nil {
		t.Fatalf("failed to insert step execution: %v", err)
	}

	// 3. Worker-1 claims the task
	_, se, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil {
		t.Fatalf("worker-1 failed to claim: %v", err)
	}
	if se == nil || se.ID != stepExecID {
		t.Fatalf("expected claimed execution %s, got %+v", stepExecID, se)
	}

	// Fetch initial locked_until from DB
	var initialLockedUntil time.Time
	err = db.QueryRowContext(ctx, "SELECT locked_until FROM step_executions WHERE id = ?", stepExecID).Scan(&initialLockedUntil)
	if err != nil {
		t.Fatalf("failed to read initial locked_until: %v", err)
	}
	t.Logf("✓ Claimed task initial locked_until: %s", initialLockedUntil.Format(time.RFC3339))

	// 4. Wait 1 second and send heartbeat from worker-1
	time.Sleep(1 * time.Second)
	err = workerLogic.Heartbeat(ctx, stepExecID, "worker-1")
	if err != nil {
		t.Fatalf("worker-1 heartbeat failed: %v", err)
	}

	var extendedLockedUntil time.Time
	err = db.QueryRowContext(ctx, "SELECT locked_until FROM step_executions WHERE id = ?", stepExecID).Scan(&extendedLockedUntil)
	if err != nil {
		t.Fatalf("failed to read extended locked_until: %v", err)
	}
	t.Logf("✓ Extended locked_until: %s", extendedLockedUntil.Format(time.RFC3339))

	if !extendedLockedUntil.After(initialLockedUntil) {
		t.Fatalf("expected extended locked_until (%s) to be after initial (%s)",
			extendedLockedUntil.Format(time.RFC3339), initialLockedUntil.Format(time.RFC3339))
	}
	t.Logf("✓ Verified locked_until successfully extended by Heartbeat")

	// 5. Worker-2 tries to heartbeat the same execution -> should fail with ErrLockLost
	err = workerLogic.Heartbeat(ctx, stepExecID, "worker-2")
	if !errors.Is(err, step.ErrLockLost) {
		t.Fatalf("expected ErrLockLost for unauthorized worker-2, got %v", err)
	}
	t.Logf("✓ Verified unauthorized worker cannot heartbeat")

	// 6. Complete the step and verify heartbeat fails with ErrLockLost
	_, err = db.ExecContext(ctx, "UPDATE step_executions SET status = 'COMPLETED' WHERE id = ?", stepExecID)
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	err = workerLogic.Heartbeat(ctx, stepExecID, "worker-1")
	if !errors.Is(err, step.ErrLockLost) {
		t.Fatalf("expected ErrLockLost for completed step, got %v", err)
	}
	t.Logf("✓ Verified completed step execution rejects heartbeat")

	// 7. Test interval StartHeartbeat with real database
	stepExecID2 := uuid.NewString()
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, status, attempt, created_at, updated_at
		) VALUES (?, ?, ?, 'Interval Test Step', 'http', '{"method":"GET"}', 'PENDING', 0, NOW(), NOW())
	`, stepExecID2, wfExecID, stepID)
	if err != nil {
		t.Fatalf("failed to insert step execution 2: %v", err)
	}

	_, se2, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se2 == nil || se2.ID != stepExecID2 {
		t.Fatalf("failed to claim step 2: %v, se2=%+v", err, se2)
	}

	var intervalInitialLocked time.Time
	err = db.QueryRowContext(ctx, "SELECT locked_until FROM step_executions WHERE id = ?", stepExecID2).Scan(&intervalInitialLocked)
	if err != nil {
		t.Fatalf("failed to read initial locked_until for step 2: %v", err)
	}

	// Start periodic heartbeat every 1s
	stop, errChan := workerLogic.StartHeartbeat(ctx, stepExecID2, "worker-1", 1*time.Second)
	// Wait 2.2 seconds to allow at least 2 ticks to fire and extend the lease
	time.Sleep(2200 * time.Millisecond)
	stop()

	select {
	case hbErr := <-errChan:
		t.Fatalf("unexpected error from StartHeartbeat: %v", hbErr)
	default:
	}

	var intervalFinalLocked time.Time
	err = db.QueryRowContext(ctx, "SELECT locked_until FROM step_executions WHERE id = ?", stepExecID2).Scan(&intervalFinalLocked)
	if err != nil {
		t.Fatalf("failed to read final locked_until for step 2: %v", err)
	}
	t.Logf("✓ Interval heartbeat: initial=%s, final=%s",
		intervalInitialLocked.Format(time.RFC3339), intervalFinalLocked.Format(time.RFC3339))

	if !intervalFinalLocked.After(intervalInitialLocked) {
		t.Fatalf("expected final locked_until (%s) to be after initial (%s)",
			intervalFinalLocked.Format(time.RFC3339), intervalInitialLocked.Format(time.RFC3339))
	}
	t.Logf("✓ Verified StartHeartbeat ticker continuously kept lease alive in database!")
}

func TestIntegration_CompleteStep(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// 1. Create test workflow and execution
	wfID := uuid.NewString()
	_, err := db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, status, hash, created_at, updated_at) VALUES (?, ?, 1, 'ACTIVE', 'dummy-hash', NOW(), NOW())", wfID, "complete-wf-"+wfID[:8])
	if err != nil {
		t.Fatalf("failed to insert workflow: %v", err)
	}

	wfExecID := uuid.NewString()
	_, err = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'RUNNING', NOW(), NOW())", wfExecID, wfID)
	if err != nil {
		t.Fatalf("failed to insert workflow_execution: %v", err)
	}

	// 2. Insert 2 steps where Step B depends on Step A
	stepExecAID := uuid.NewString()
	stepExecBID := uuid.NewString()

	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on, status, max_attempts, is_compensation, created_at, updated_at
		) VALUES 
		(?, ?, 'step-a', 'Step A', 'http', '{}', '[]', 'PENDING', 1, FALSE, NOW(), NOW()),
		(?, ?, 'step-b', 'Step B', 'http', '{}', '["step-a"]', 'PENDING', 1, FALSE, DATE_ADD(NOW(), INTERVAL 1 SECOND), DATE_ADD(NOW(), INTERVAL 1 SECOND))
	`, stepExecAID, wfExecID, stepExecBID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert step_executions: %v", err)
	}

	// 3. Worker-1 claims Step A
	_, se1, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se1 == nil || se1.ID != stepExecAID {
		t.Fatalf("failed to claim step A: %v, se1=%+v", err, se1)
	}

	// 4. Worker-2 tries to Complete Step A -> Should fail with ErrLockLost (unauthorized)
	err = workerLogic.CompleteStep(ctx, stepExecAID, "worker-2", json.RawMessage(`{"res":"hacked"}`))
	if !errors.Is(err, step.ErrLockLost) {
		t.Fatalf("expected ErrLockLost for unauthorized worker-2 completing Step A, got %v", err)
	}

	// 5. Worker-1 completes Step A
	outputA := json.RawMessage(`{"order_id":"ORD-999"}`)
	err = workerLogic.CompleteStep(ctx, stepExecAID, "worker-1", outputA)
	if err != nil {
		t.Fatalf("unexpected error completing Step A: %v", err)
	}

	// Verify Step A in DB
	var (
		statusA  string
		outputDB []byte
		lockedBy sql.NullString
	)
	err = db.QueryRowContext(ctx, "SELECT status, output, locked_by FROM step_executions WHERE id = ?", stepExecAID).Scan(&statusA, &outputDB, &lockedBy)
	if err != nil {
		t.Fatalf("failed to query step A after complete: %v", err)
	}
	if statusA != "COMPLETED" {
		t.Errorf("expected Step A status COMPLETED, got %s", statusA)
	}
	if string(outputDB) != string(outputA) {
		t.Errorf("expected output %s, got %s", string(outputA), string(outputDB))
	}
	if lockedBy.Valid {
		t.Errorf("expected locked_by to be NULL, got %s", lockedBy.String)
	}

	// 6. Workflow execution should still be RUNNING because Step B is pending
	var wfStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if err != nil || wfStatus != "RUNNING" {
		t.Fatalf("expected workflow RUNNING, got %s, err=%v", wfStatus, err)
	}

	// 7. Worker-2 claims Step B now that Step A is COMPLETED
	_, se2, err := workerLogic.ClaimNext(ctx, "worker-2")
	if err != nil || se2 == nil || se2.ID != stepExecBID {
		t.Fatalf("failed to claim step B after A completed: %v, se2=%+v", err, se2)
	}

	// 8. Worker-2 completes Step B
	err = workerLogic.CompleteStep(ctx, stepExecBID, "worker-2", json.RawMessage(`{"status":"success"}`))
	if err != nil {
		t.Fatalf("failed to complete Step B: %v", err)
	}

	// 9. Workflow execution should now be automatically marked COMPLETED!
	err = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if err != nil {
		t.Fatalf("failed to query workflow status: %v", err)
	}
	if wfStatus != "COMPLETED" {
		t.Errorf("expected workflow status COMPLETED, got %s", wfStatus)
	}
	t.Logf("✓ Auto-completion verified: workflow %s marked COMPLETED after all steps completed!", wfExecID)
}

func TestIntegration_FailStep_Retry(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	wfID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, status, hash, created_at, updated_at) VALUES (?, ?, 1, 'ACTIVE', 'dummy-hash', NOW(), NOW())", wfID, "fail-retry-wf-"+wfID[:8])

	wfExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'RUNNING', NOW(), NOW())", wfExecID, wfID)

	stepExecID := uuid.NewString()
	_, err := db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, status, attempt, max_attempts, backoff_seconds, multiplier, max_backoff_seconds, is_compensation, created_at, updated_at
		) VALUES (?, ?, 'step-retry', 'Retry Step', 'http', '{}', 'PENDING', 0, 3, 2, 2.0, 10, FALSE, NOW(), NOW())
	`, stepExecID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert step_execution: %v", err)
	}

	// 1. Claim step (attempt increments to 1)
	_, se, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se == nil || se.ID != stepExecID {
		t.Fatalf("failed to claim step: %v, se=%+v", err, se)
	}
	if se.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", se.Attempt)
	}

	// 2. Fail step with retryable error
	err = workerLogic.FailStep(ctx, stepExecID, "worker-1", "connection timeout")
	if err != nil {
		t.Fatalf("failed to report fail step: %v", err)
	}

	// 3. Verify step state in DB: status PENDING, next_retry_at in future, locked_by NULL
	var (
		status      string
		errMsg      sql.NullString
		lockedBy    sql.NullString
		nextRetryAt sql.NullTime
	)
	err = db.QueryRowContext(ctx, "SELECT status, error_message, locked_by, next_retry_at FROM step_executions WHERE id = ?", stepExecID).Scan(&status, &errMsg, &lockedBy, &nextRetryAt)
	if err != nil {
		t.Fatalf("failed to query step after failure: %v", err)
	}

	if status != "PENDING" {
		t.Errorf("expected status PENDING after retry scheduled, got %s", status)
	}
	if !errMsg.Valid || errMsg.String != "connection timeout" {
		t.Errorf("expected error message 'connection timeout', got %v", errMsg)
	}
	if lockedBy.Valid {
		t.Errorf("expected locked_by NULL, got %s", lockedBy.String)
	}
	if !nextRetryAt.Valid || !nextRetryAt.Time.After(time.Now().UTC()) {
		t.Errorf("expected next_retry_at to be in future, got %v", nextRetryAt)
	}

	// 4. Immediate ClaimNext should NOT claim this task because next_retry_at > NOW()
	_, nextExec, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil {
		t.Fatalf("claim next error: %v", err)
	}
	if nextExec != nil {
		t.Errorf("expected nil claim while backoff active, got %+v", nextExec)
	}
	t.Logf("✓ Retry backoff verified: step rescheduled with next_retry_at in future")
}

func TestIntegration_FailStep_ExhaustedAndCompensation(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	wfID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, status, hash, created_at, updated_at) VALUES (?, ?, 1, 'ACTIVE', 'dummy-hash', NOW(), NOW())", wfID, "fail-saga-wf-"+wfID[:8])

	wfExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'RUNNING', NOW(), NOW())", wfExecID, wfID)

	stepMainID := uuid.NewString()
	stepCompID := uuid.NewString()

	// Insert main step with max_attempts=1 (fails immediately on first error)
	// and compensation step with is_compensation=TRUE
	_, err := db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, status, attempt, max_attempts, is_compensation, created_at, updated_at
		) VALUES 
		(?, ?, 'step-charge', 'Charge Card', 'http', '{}', 'PENDING', 0, 1, FALSE, NOW(), NOW()),
		(?, ?, 'step-refund', 'Refund Card', 'http', '{}', 'PENDING', 0, 1, TRUE, DATE_ADD(NOW(), INTERVAL 1 SECOND), DATE_ADD(NOW(), INTERVAL 1 SECOND))
	`, stepMainID, wfExecID, stepCompID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert steps: %v", err)
	}

	// 1. Claim main step
	_, seMain, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || seMain == nil || seMain.ID != stepMainID {
		t.Fatalf("failed to claim main step: %v, seMain=%+v", err, seMain)
	}

	// 2. Fail main step (exhausting retries)
	err = workerLogic.FailStep(ctx, stepMainID, "worker-1", "card declined permanently")
	if err != nil {
		t.Fatalf("failed to report step failure: %v", err)
	}

	// 3. Verify main step is FAILED
	var mainStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepMainID).Scan(&mainStatus)
	if mainStatus != "FAILED" {
		t.Errorf("expected main step FAILED, got %s", mainStatus)
	}

	// 4. Verify workflow transitioned to COMPENSATING because compensation step exists
	var wfStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if wfStatus != "COMPENSATING" {
		t.Errorf("expected workflow status COMPENSATING, got %s", wfStatus)
	}
	t.Logf("✓ Workflow transitioned to COMPENSATING after non-retryable failure")

	// 5. Worker-1 claims next task -> Should pick the compensation step!
	_, seComp, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || seComp == nil || seComp.ID != stepCompID {
		t.Fatalf("expected ClaimNext to pick compensation step %s, got %+v, err=%v", stepCompID, seComp, err)
	}

	// 6. Complete the compensation step
	err = workerLogic.CompleteStep(ctx, stepCompID, "worker-1", json.RawMessage(`{"refunded":true}`))
	if err != nil {
		t.Fatalf("failed to complete compensation step: %v", err)
	}

	// 7. Verify compensation step is COMPENSATED and workflow is COMPENSATED!
	var compStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepCompID).Scan(&compStatus)
	if compStatus != "COMPENSATED" {
		t.Errorf("expected compensation step COMPENSATED, got %s", compStatus)
	}

	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if wfStatus != "COMPENSATED" {
		t.Errorf("expected workflow status COMPENSATED, got %s", wfStatus)
	}
	t.Logf("✓ Saga compensation cycle verified end-to-end: workflow %s marked COMPENSATED!", wfExecID)
}


