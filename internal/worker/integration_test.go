package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/vhysxl/relay/internal/executor"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/validator"
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

	cleanDB(t, db)

	return db
}

func cleanDB(t *testing.T, db *sql.DB) {
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "DELETE FROM step_executions"); err != nil {
		t.Logf("cleanDB step_executions error: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM workflow_executions"); err != nil {
		t.Logf("cleanDB workflow_executions error: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM step_definitions"); err != nil {
		t.Logf("cleanDB step_definitions error: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM workflow_definitions"); err != nil {
		t.Logf("cleanDB workflow_definitions error: %v", err)
	}
}

func TestIntegration_Workflow_Steps_And_ClaimNext(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// setup repositories and logic
	stepRepo := step.NewStepRepository(db)
	stepValidator := validator.NewValidator()
	workflowRepo := workflow.NewWorkflowRepository(db, stepRepo)
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// 1. CREATE WORKFLOW + STEPS
	uniqueWorkflowName := "test-order-saga-" + uuid.NewString()[:8]
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
	stepValidator := validator.NewValidator()
	workflowRepo := workflow.NewWorkflowRepository(db, stepRepo)
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// 1. Create test workflow and step definition
	uniqueWorkflowName := "test-heartbeat-wf-" + uuid.NewString()[:8]
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
	stepID2 := "step-hb-2-" + uuid.NewString()[:8]
	_, err = db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, status, attempt, created_at, updated_at
		) VALUES (?, ?, ?, 'Interval Test Step', 'http', '{"method":"GET"}', 'PENDING', 0, NOW(), NOW())
	`, stepExecID2, wfExecID, stepID2)
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
	_, err := db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, created_at, updated_at) VALUES (?, ?, 1, NOW(), NOW())", wfID, "test-complete-wf-"+wfID[:8])
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
		(?, ?, 'step-b', 'Step B', 'http', '{}', '["step-a"]', 'PENDING', 1, FALSE, NOW(), NOW())
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
	var expMap, gotMap map[string]interface{}
	_ = json.Unmarshal(outputA, &expMap)
	_ = json.Unmarshal(outputDB, &gotMap)
	if expMap["order_id"] != gotMap["order_id"] {
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
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, created_at, updated_at) VALUES (?, ?, 1, NOW(), NOW())", wfID, "test-fail-retry-wf-"+wfID[:8])

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
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, created_at, updated_at) VALUES (?, ?, 1, NOW(), NOW())", wfID, "test-fail-saga-"+wfID[:8])

	wfExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'RUNNING', NOW(), NOW())", wfExecID, wfID)

	stepChargeID := uuid.NewString()
	stepRefundID := uuid.NewString()
	stepReserveID := uuid.NewString()
	stepCancelReserveID := uuid.NewString()
	stepShipID := uuid.NewString()
	stepCancelShipID := uuid.NewString()

	// Setup:
	// - Step 1 (step-charge): Already COMPLETED successfully. Its compensation (step-refund) is PENDING.
	// - Step 2 (step-reserve): PENDING with max_attempts=1. Will fail. Its compensation (step-cancel-reserve) is PENDING.
	// - Step 3 (step-ship): PENDING. Will never run. Its compensation (step-cancel-ship) is PENDING.
	_, err := db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, status, attempt, max_attempts, is_compensation, created_at, updated_at
		) VALUES 
		(?, ?, 'step-charge', 'Charge Card', 'http', '{}', 'COMPLETED', 1, 1, FALSE, NOW(), NOW()),
		(?, ?, 'step-charge', 'Refund Card', 'http', '{}', 'PENDING', 0, 1, TRUE, DATE_ADD(NOW(), INTERVAL 1 SECOND), DATE_ADD(NOW(), INTERVAL 1 SECOND)),
		(?, ?, 'step-reserve', 'Reserve Inventory', 'http', '{}', 'PENDING', 0, 1, FALSE, NOW(), NOW()),
		(?, ?, 'step-reserve', 'Cancel Inventory Reservation', 'http', '{}', 'PENDING', 0, 1, TRUE, DATE_ADD(NOW(), INTERVAL 1 SECOND), DATE_ADD(NOW(), INTERVAL 1 SECOND)),
		(?, ?, 'step-ship', 'Ship Package', 'http', '{}', 'PENDING', 0, 1, FALSE, DATE_ADD(NOW(), INTERVAL 2 SECOND), DATE_ADD(NOW(), INTERVAL 2 SECOND)),
		(?, ?, 'step-ship', 'Cancel Shipping', 'http', '{}', 'PENDING', 0, 1, TRUE, DATE_ADD(NOW(), INTERVAL 3 SECOND), DATE_ADD(NOW(), INTERVAL 3 SECOND))
	`, stepChargeID, wfExecID, stepRefundID, wfExecID,
		stepReserveID, wfExecID, stepCancelReserveID, wfExecID,
		stepShipID, wfExecID, stepCancelShipID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert steps: %v", err)
	}

	// 1. Worker claims Step 2 (step-reserve)
	_, seReserve, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || seReserve == nil || seReserve.ID != stepReserveID {
		t.Fatalf("failed to claim reserve step: %v, seReserve=%+v", err, seReserve)
	}

	// 2. Fail Step 2 (exhausting retries)
	err = workerLogic.FailStep(ctx, stepReserveID, "worker-1", "inventory out of stock")
	if err != nil {
		t.Fatalf("failed to report step failure: %v", err)
	}

	// 3. Verify Step 2 is FAILED
	var reserveStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepReserveID).Scan(&reserveStatus)
	if reserveStatus != "FAILED" {
		t.Errorf("expected reserve step FAILED, got %s", reserveStatus)
	}

	// 4. Verify Step 3 (unexecuted normal step) is CANCELLED
	var shipStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepShipID).Scan(&shipStatus)
	if shipStatus != "CANCELLED" {
		t.Errorf("expected ship step CANCELLED, got %s", shipStatus)
	}

	// 5. Verify unexecuted step's compensation and failed step's compensation are CANCELLED
	var cancelReserveStatus, cancelShipStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepCancelReserveID).Scan(&cancelReserveStatus)
	if cancelReserveStatus != "CANCELLED" {
		t.Errorf("expected cancel-reserve compensation CANCELLED, got %s", cancelReserveStatus)
	}
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepCancelShipID).Scan(&cancelShipStatus)
	if cancelShipStatus != "CANCELLED" {
		t.Errorf("expected cancel-ship compensation CANCELLED, got %s", cancelShipStatus)
	}

	// 6. Verify workflow transitioned to COMPENSATING because a completed step has compensation
	var wfStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if wfStatus != "COMPENSATING" {
		t.Errorf("expected workflow status COMPENSATING, got %s", wfStatus)
	}
	t.Logf("✓ Workflow transitioned to COMPENSATING only for successful step")

	// 7. Worker-1 claims next task -> Should pick ONLY Step 1's compensation (step-refund)!
	_, seComp, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || seComp == nil || seComp.ID != stepRefundID {
		t.Fatalf("expected ClaimNext to pick refund step %s, got %+v, err=%v", stepRefundID, seComp, err)
	}

	// 9. Complete the refund compensation step
	err = workerLogic.CompleteStep(ctx, stepRefundID, "worker-1", json.RawMessage(`{"refunded":true}`))
	if err != nil {
		t.Fatalf("failed to complete compensation step: %v", err)
	}

	// 10. Verify refund compensation step is COMPENSATED and workflow is COMPENSATED!
	var refundStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepRefundID).Scan(&refundStatus)
	if refundStatus != "COMPENSATED" {
		t.Errorf("expected compensation step COMPENSATED, got %s", refundStatus)
	}

	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if wfStatus != "COMPENSATED" {
		t.Errorf("expected workflow status COMPENSATED, got %s", wfStatus)
	}
	t.Logf("✓ Saga compensation cycle verified end-to-end: workflow %s marked COMPENSATED!", wfExecID)
}

func TestIntegration_FailStep_FirstStepFails_TransitionsDirectlyToFailed(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	wfID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, created_at, updated_at) VALUES (?, ?, 1, NOW(), NOW())", wfID, "test-fail-immediate-wf-"+wfID[:8])

	wfExecID := uuid.NewString()
	_, _ = db.ExecContext(ctx, "INSERT INTO workflow_executions (id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'RUNNING', NOW(), NOW())", wfExecID, wfID)

	stepMainID := uuid.NewString()
	stepCompID := uuid.NewString()

	// Step 1 fails on first attempt and has a compensation defined
	_, err := db.ExecContext(ctx, `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, status, attempt, max_attempts, is_compensation, created_at, updated_at
		) VALUES 
		(?, ?, 'step-1', 'Step 1', 'http', '{}', 'PENDING', 0, 1, FALSE, NOW(), NOW()),
		(?, ?, 'step-1', 'Comp 1', 'http', '{}', 'PENDING', 0, 1, TRUE, DATE_ADD(NOW(), INTERVAL 1 SECOND), DATE_ADD(NOW(), INTERVAL 1 SECOND))
	`, stepMainID, wfExecID, stepCompID, wfExecID)
	if err != nil {
		t.Fatalf("failed to insert steps: %v", err)
	}

	_, se, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se == nil || se.ID != stepMainID {
		t.Fatalf("failed to claim step 1: %v", err)
	}

	// Fail step 1
	err = workerLogic.FailStep(ctx, stepMainID, "worker-1", "immediate failure")
	if err != nil {
		t.Fatalf("fail step error: %v", err)
	}

	// Since Step 1 never succeeded, its compensation must be CANCELLED and workflow must be FAILED (not COMPENSATING)
	var compStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM step_executions WHERE id = ?", stepCompID).Scan(&compStatus)
	if compStatus != "CANCELLED" {
		t.Errorf("expected compStatus CANCELLED, got %s", compStatus)
	}

	var wfStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExecID).Scan(&wfStatus)
	if wfStatus != "FAILED" {
		t.Errorf("expected workflow status FAILED, got %s", wfStatus)
	}
	t.Logf("✓ Verified workflow with no successful steps transitions directly to FAILED")
}

func TestIntegration_EndToEnd_TriggerClaimComplete(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	stepValidator := validator.NewValidator()
	wfRepo := workflow.NewWorkflowRepository(db, stepRepo)
	wfLogic := workflow.NewWorkflowLogic(wfRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// 1. Create a workflow definition with 1 step
	wfName := "e2e-wf-" + uuid.NewString()[:8]
	wf, err := wfLogic.CreateWorkflow(ctx, &workflow.WorkflowRequest{
		Name:    wfName,
		Version: 1,
		Context: json.RawMessage(`{"default_key":"default_val"}`),
		Steps: []step.Step{
			{
				ID:      "step-first",
				Name:    "First Step",
				Type:    step.StepTypeHTTP,
				Config:  json.RawMessage(`{"url":"https://api.example.com/data","method":"GET"}`),
				Timeout: 15,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// 2. Trigger workflow with custom initial context
	customContext := json.RawMessage(`{"order_id":"ORD-456","amount":75000}`)
	wfExec, err := wfLogic.ExecuteWorkflow(ctx, wf.ID, &workflow.ExecuteWorkflowRequest{
		Context: customContext,
	})
	if err != nil {
		t.Fatalf("failed to trigger workflow: %v", err)
	}

	if wfExec.Status != step.StatusRunning {
		t.Errorf("expected workflow status RUNNING, got %s", wfExec.Status)
	}
	if string(wfExec.Context) != string(customContext) {
		t.Errorf("expected context %s, got %s", string(customContext), string(wfExec.Context))
	}
	if len(wfExec.Steps) != 1 {
		t.Fatalf("expected 1 step execution, got %d", len(wfExec.Steps))
	}

	// 3. Worker claims the step
	claimedStep, claimedExec, err := workerLogic.ClaimNext(ctx, "worker-e2e")
	if err != nil || claimedExec == nil {
		t.Fatalf("failed to claim step from triggered workflow: %v, claimedExec=%+v", err, claimedExec)
	}
	if claimedStep.ID != "step-first" {
		t.Errorf("expected claimed step-first, got %s", claimedStep.ID)
	}

	// 4. Worker completes the step
	err = workerLogic.CompleteStep(ctx, claimedExec.ID, "worker-e2e", json.RawMessage(`{"status":"200 OK"}`))
	if err != nil {
		t.Fatalf("failed to complete step: %v", err)
	}

	// 5. Verify workflow execution is now COMPLETED in DB
	var dbWfStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExec.ID).Scan(&dbWfStatus)
	if err != nil {
		t.Fatalf("failed to query workflow execution: %v", err)
	}
	if dbWfStatus != "COMPLETED" {
		t.Errorf("expected workflow execution COMPLETED, got %s", dbWfStatus)
	}
	t.Logf("✓ Full End-to-End Cycle (Create -> Trigger -> Claim -> Complete) verified!")
}

type integrationMockExecutor struct{}

func (e *integrationMockExecutor) Execute(ctx context.Context, s *step.Step, exec *step.StepExecution) (json.RawMessage, error) {
	return json.RawMessage(`{"processed":true}`), nil
}

func TestIntegration_WorkerPool_Fixed3_ParallelProcessing(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stepRepo := step.NewStepRepository(db)
	stepValidator := validator.NewValidator()
	wfRepo := workflow.NewWorkflowRepository(db, stepRepo)
	wfLogic := workflow.NewWorkflowLogic(wfRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// 1. Create a workflow definition with 3 parallel tasks (no dependencies)
	wfName := "test-parallel-pool-wf-" + uuid.NewString()[:8]
	wf, err := wfLogic.CreateWorkflow(ctx, &workflow.WorkflowRequest{
		Name:    wfName,
		Version: 1,
		Steps: []step.Step{
			{
				ID:     "task-1",
				Name:   "Task 1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/1"}`),
			},
			{
				ID:     "task-2",
				Name:   "Task 2",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/2"}`),
			},
			{
				ID:     "task-3",
				Name:   "Task 3",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/3"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// 2. Setup registry with mock HTTP executor
	reg := executor.NewRegistry()
	reg.Register(step.StepTypeHTTP, &integrationMockExecutor{})

	// 3. Start WorkerPool with fixed 3 workers
	pool := worker.NewWorkerPool(
		workerLogic,
		reg,
		worker.WithBaseID("pool-worker"),
		worker.WithWorkerCount(3),
		worker.WithPollInterval(20*time.Millisecond),
	)

	poolDone := make(chan error, 1)
	go func() {
		poolDone <- pool.Start(ctx)
	}()

	// 4. Trigger workflow execution
	wfExec, err := wfLogic.ExecuteWorkflow(ctx, wf.ID, &workflow.ExecuteWorkflowRequest{
		Context: json.RawMessage(`{"batch_id":"batch-123"}`),
	})
	if err != nil {
		t.Fatalf("failed to execute workflow: %v", err)
	}

	// 5. Poll for workflow completion (up to 10 seconds)
	deadline := time.Now().Add(10 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		var status string
		_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExec.ID).Scan(&status)
		if status == "COMPLETED" {
			completed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !completed {
		t.Fatalf("timed out waiting for 3 parallel workers to complete workflow")
	}

	// 6. Cancel pool and verify clean shutdown
	cancel()
	select {
	case err := <-poolDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("unexpected pool shutdown error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for pool to shut down")
	}

	t.Logf("✓ Fixed 3 WorkerPool successfully processed 3 parallel tasks and completed workflow %s!", wfExec.ID)
}

func TestIntegration_DAG_Mirror_Rollback_ParallelBranches(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	stepValidator := validator.NewValidator()
	wfRepo := workflow.NewWorkflowRepository(db, stepRepo)
	wfLogic := workflow.NewWorkflowLogic(wfRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// Workflow: Step A -> (Step B & Step C in parallel) -> Step D
	// All steps have compensation defined.
	stepAID := "step-a-" + uuid.NewString()[:8]
	stepBID := "step-b-" + uuid.NewString()[:8]
	stepCID := "step-c-" + uuid.NewString()[:8]
	stepDID := "step-d-" + uuid.NewString()[:8]

	wfName := "dag-mirror-wf-" + uuid.NewString()[:8]
	wf, err := wfLogic.CreateWorkflow(ctx, &workflow.WorkflowRequest{
		Name:    wfName,
		Version: 1,
		Steps: []step.Step{
			{
				ID:     stepAID,
				Name:   "Step A (Order)",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/a"}`),
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"url":"https://api.test/a-rollback"}`),
				},
			},
			{
				ID:        stepBID,
				Name:      "Step B (Payment)",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/b"}`),
				DependsOn: []string{stepAID},
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"url":"https://api.test/b-rollback"}`),
				},
			},
			{
				ID:        stepCID,
				Name:      "Step C (Stock)",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/c"}`),
				DependsOn: []string{stepAID},
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"url":"https://api.test/c-rollback"}`),
				},
			},
			{
				ID:        stepDID,
				Name:      "Step D (Ship)",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/d"}`),
				DependsOn: []string{stepBID, stepCID},
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"url":"https://api.test/d-rollback"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	wfExec, err := wfLogic.ExecuteWorkflow(ctx, wf.ID, nil)
	if err != nil {
		t.Fatalf("failed to execute workflow: %v", err)
	}

	// 1. Claim and complete Step A
	_, execA, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || execA == nil || execA.StepID != stepAID {
		t.Fatalf("expected to claim Step A, got %v, err=%v", execA, err)
	}
	if err := workerLogic.CompleteStep(ctx, execA.ID, "worker-1", json.RawMessage(`{"status":"ok"}`)); err != nil {
		t.Fatalf("failed to complete Step A: %v", err)
	}

	// 2. Claim and complete Step B & Step C in parallel
	_, execB, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || execB == nil {
		t.Fatalf("failed to claim parallel step: %v", err)
	}
	_, execC, err := workerLogic.ClaimNext(ctx, "worker-2")
	if err != nil || execC == nil {
		t.Fatalf("failed to claim parallel step: %v", err)
	}
	if err := workerLogic.CompleteStep(ctx, execB.ID, "worker-1", json.RawMessage(`{"status":"ok"}`)); err != nil {
		t.Fatalf("failed to complete step: %v", err)
	}
	if err := workerLogic.CompleteStep(ctx, execC.ID, "worker-2", json.RawMessage(`{"status":"ok"}`)); err != nil {
		t.Fatalf("failed to complete step: %v", err)
	}

	// 3. Claim Step D and fail it (exhausting attempts)
	_, execD, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || execD == nil || execD.StepID != stepDID {
		t.Fatalf("expected to claim Step D, got %v, err=%v", execD, err)
	}
	if err := workerLogic.FailStep(ctx, execD.ID, "worker-1", "shipping provider down"); err != nil {
		t.Fatalf("failed to fail Step D: %v", err)
	}

	// 4. Verify workflow transitioned to COMPENSATING
	var wfStatus string
	if err := db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExec.ID).Scan(&wfStatus); err != nil {
		t.Fatalf("failed to check workflow status: %v", err)
	}
	if wfStatus != "COMPENSATING" {
		t.Fatalf("expected workflow status COMPENSATING, got %s", wfStatus)
	}

	// 5. Verify DAG Mirror Rollback:
	// Step D failed, so Step D's compensation was CANCELLED (only completed steps are compensated).
	// Steps B and C both completed, and their downstream dependency (D) is now CANCELLED.
	// Therefore, Step B and Step C's compensations are immediately UNBLOCKED and claimed in parallel!
	_, comp1, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || comp1 == nil || !comp1.IsCompensation {
		t.Fatalf("expected to claim parallel compensation (B or C), got %v, err=%v", comp1, err)
	}
	_, comp2, err := workerLogic.ClaimNext(ctx, "worker-2")
	if err != nil || comp2 == nil || !comp2.IsCompensation {
		t.Fatalf("expected to claim parallel compensation (B or C), got %v, err=%v", comp2, err)
	}

	claimedIDs := map[string]bool{comp1.StepID: true, comp2.StepID: true}
	if !claimedIDs[stepBID] || !claimedIDs[stepCID] {
		t.Fatalf("expected compensations for B and C, got %s and %s", comp1.StepID, comp2.StepID)
	}

	// While B and C's compensations are running, Step A's compensation must remain BLOCKED!
	_, blockedA, err := workerLogic.ClaimNext(ctx, "worker-3")
	if err != nil {
		t.Fatalf("unexpected error claiming while B and C are compensating: %v", err)
	}
	if blockedA != nil {
		t.Fatalf("Step A's compensation must be blocked until both B and C finish, but got %s", blockedA.StepID)
	}

	// Worker 1 and Worker 2 complete B and C compensations
	if err := workerLogic.CompleteStep(ctx, comp1.ID, "worker-1", json.RawMessage(`{"rollback":"done"}`)); err != nil {
		t.Fatalf("failed to complete compensation 1: %v", err)
	}
	if err := workerLogic.CompleteStep(ctx, comp2.ID, "worker-2", json.RawMessage(`{"rollback":"done"}`)); err != nil {
		t.Fatalf("failed to complete compensation 2: %v", err)
	}

	// 6. Finally, Step A's compensation is unblocked and claimed!
	_, compA, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || compA == nil || compA.StepID != stepAID || !compA.IsCompensation {
		t.Fatalf("expected to claim compensation A, got %v, err=%v", compA, err)
	}
	if err := workerLogic.CompleteStep(ctx, compA.ID, "worker-1", json.RawMessage(`{"rollback":"a-done"}`)); err != nil {
		t.Fatalf("failed to complete A compensation: %v", err)
	}

	// 7. Verify workflow execution is now COMPENSATED
	if err := db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExec.ID).Scan(&wfStatus); err != nil {
		t.Fatalf("failed to check final workflow status: %v", err)
	}
	if wfStatus != "COMPENSATED" {
		t.Fatalf("expected final workflow status COMPENSATED, got %s", wfStatus)
	}

	t.Logf("✓ Verified DAG Mirror Rollback: (B & C in parallel) -> A completed in exact reverse topological order!")
}

func TestIntegration_DAG_Rollback_ParallelRunningStepNotCancelled(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	stepValidator := validator.NewValidator()
	wfRepo := workflow.NewWorkflowRepository(db, stepRepo)
	wfLogic := workflow.NewWorkflowLogic(wfRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	// Step A -> (Step B & Step C in parallel)
	stepAID := "step-a-" + uuid.NewString()[:8]
	stepBID := "step-b-" + uuid.NewString()[:8]
	stepCID := "step-c-" + uuid.NewString()[:8]

	wfName := "test-parallel-protect-wf-" + uuid.NewString()[:8]
	wf, err := wfLogic.CreateWorkflow(ctx, &workflow.WorkflowRequest{
		Name:    wfName,
		Version: 1,
		Steps: []step.Step{
			{
				ID:     stepAID,
				Name:   "Step A",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/a"}`),
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"url":"https://api.test/a-rollback"}`),
				},
			},
			{
				ID:        stepBID,
				Name:      "Step B (Slow payment)",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/b"}`),
				DependsOn: []string{stepAID},
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"url":"https://api.test/b-rollback"}`),
				},
			},
			{
				ID:        stepCID,
				Name:      "Step C (Fast fail)",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/c"}`),
				DependsOn: []string{stepAID},
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"url":"https://api.test/c-rollback"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	wfExec, err := wfLogic.ExecuteWorkflow(ctx, wf.ID, nil)
	if err != nil {
		t.Fatalf("failed to execute workflow: %v", err)
	}

	// 1. Complete Step A
	_, execA, _ := workerLogic.ClaimNext(ctx, "worker-1")
	_ = workerLogic.CompleteStep(ctx, execA.ID, "worker-1", json.RawMessage(`{}`))

	// 2. Claim parallel steps: one keeps running, one fails
	_, exec1, _ := workerLogic.ClaimNext(ctx, "worker-1")
	_, exec2, _ := workerLogic.ClaimNext(ctx, "worker-2")
	if exec1 == nil || exec2 == nil {
		t.Fatalf("expected both parallel steps to be claimed, got exec1=%v, exec2=%v", exec1, exec2)
	}

	// Identify which is the slow step (to keep running) and which is to fail
	var slowExec, failExec *step.StepExecution
	var slowWorker, failWorker string
	if exec1.StepID == stepCID {
		failExec = exec1
		failWorker = "worker-1"
		slowExec = exec2
		slowWorker = "worker-2"
	} else {
		failExec = exec2
		failWorker = "worker-2"
		slowExec = exec1
		slowWorker = "worker-1"
	}

	// 3. Fail the fast step
	if err := workerLogic.FailStep(ctx, failExec.ID, failWorker, "immediate error"); err != nil {
		t.Fatalf("failed to fail step: %v", err)
	}

	// 4. CRITICAL CHECK: slowExec is still RUNNING. Verify its compensation ticket was NOT cancelled!
	var slowCompStatus string
	err = db.QueryRowContext(ctx, `
		SELECT status FROM step_executions
		WHERE workflow_execution_id = ? AND step_id = ? AND is_compensation = TRUE
	`, wfExec.ID, slowExec.StepID).Scan(&slowCompStatus)
	if err != nil {
		t.Fatalf("failed to check slow comp status: %v", err)
	}
	if slowCompStatus != "PENDING" {
		t.Fatalf("CRITICAL: Sibling step %s was RUNNING, but its compensation ticket was set to %s instead of PENDING!", slowExec.StepID, slowCompStatus)
	}
	t.Logf("✓ Verified sibling step %s compensation ticket was protected and remained PENDING while RUNNING!", slowExec.StepID)

	// 5. Worker now finishes the slow step successfully
	if err := workerLogic.CompleteStep(ctx, slowExec.ID, slowWorker, json.RawMessage(`{"status":"ok"}`)); err != nil {
		t.Fatalf("failed to complete slow step: %v", err)
	}

	// 6. Worker claims slow step's compensation!
	_, compSlow, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || compSlow == nil || compSlow.StepID != slowExec.StepID || !compSlow.IsCompensation {
		t.Fatalf("expected to claim compensation for slow step, got %v, err=%v", compSlow, err)
	}
	if err := workerLogic.CompleteStep(ctx, compSlow.ID, "worker-1", json.RawMessage(`{"refund":"done"}`)); err != nil {
		t.Fatalf("failed to complete slow step comp: %v", err)
	}

	// 7. Worker claims Step A's compensation
	_, compA, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || compA == nil || compA.StepID != stepAID || !compA.IsCompensation {
		t.Fatalf("expected to claim compensation A, got %v, err=%v", compA, err)
	}
	if err := workerLogic.CompleteStep(ctx, compA.ID, "worker-1", json.RawMessage(`{"cancel":"done"}`)); err != nil {
		t.Fatalf("failed to complete comp A: %v", err)
	}

	// 8. Workflow reaches COMPENSATED
	var finalStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExec.ID).Scan(&finalStatus)
	if finalStatus != "COMPENSATED" {
		t.Fatalf("expected final status COMPENSATED, got %s", finalStatus)
	}
	t.Logf("✓ Sibling parallel running step successfully protected and compensated end-to-end!")
}

func TestIntegration_PayloadPassing_StepOutputChaining(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	stepValidator := validator.NewValidator()
	wfRepo := workflow.NewWorkflowRepository(db, stepRepo)
	wfLogic := workflow.NewWorkflowLogic(wfRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	wfID := uuid.NewString()
	step1ID := "step-auth"
	step2ID := "step-fetch-profile"

	// 1. Create 2-step workflow where step 2 depends on step 1 and references its outputs
	wfReq := &workflow.WorkflowRequest{
		Name:    "test-payload-chaining-" + wfID[:8],
		Version: 1,
		Steps: []step.Step{
			{
				ID:     step1ID,
				Name:   "Authenticate User",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"method":"POST","url":"https://auth.internal/login"}`),
			},
			{
				ID:        step2ID,
				Name:      "Fetch Profile",
				Type:      step.StepTypeHTTP,
				DependsOn: []string{step1ID},
				Config: json.RawMessage(`{
					"method": "GET",
					"url": "https://api.internal/users/{{steps.step-auth.output.user_id}}",
					"headers": {
						"Authorization": "Bearer {{steps.step-auth.output.token}}"
					},
					"body": {
						"user_id": "{{step-auth.user_id}}",
						"first_item": "{{step-auth.output.items.0.id}}",
						"full_meta": "{{step-auth.meta}}"
					}
				}`),
			},
		},
	}

	wf, err := wfLogic.CreateWorkflow(ctx, wfReq)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE id = ?", wf.ID)
	}()

	// 2. Trigger workflow execution
	wfExec, err := wfRepo.ExecuteWorkflowTx(ctx, wf.ID, nil)
	if err != nil {
		t.Fatalf("failed to trigger workflow: %v", err)
	}

	// 3. Worker-1 claims Step 1
	s1, se1, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se1 == nil || se1.StepID != step1ID {
		t.Fatalf("expected to claim step 1, got %v, err=%v", se1, err)
	}
	_ = s1

	// 4. Complete Step 1 with structured JSON output including array and nested object
	step1Output := json.RawMessage(`{
		"user_id": "usr_7781",
		"token": "jwt_secret_token_abc",
		"items": [
			{"id": "item-alpha"},
			{"id": "item-beta"}
		],
		"meta": {
			"role": "admin",
			"tier": 2
		}
	}`)
	err = workerLogic.CompleteStep(ctx, se1.ID, "worker-1", step1Output)
	if err != nil {
		t.Fatalf("failed to complete step 1: %v", err)
	}

	// 5. Worker-1 claims Step 2 (now unblocked) -> JIT interpolation should resolve placeholders!
	s2, se2, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se2 == nil || se2.StepID != step2ID {
		t.Fatalf("expected to claim step 2, got %v, err=%v", se2, err)
	}

	// Verify resolved config in memory and database
	configStr := string(s2.Config)
	t.Logf("Resolved Step 2 Config: %s", configStr)

	// Check URL:
	expectedURL := "https://api.internal/users/usr_7781"
	if !strings.Contains(configStr, expectedURL) {
		t.Errorf("expected URL %s in config, got %s", expectedURL, configStr)
	}

	// Check Header:
	expectedHeader := `"Authorization":"Bearer jwt_secret_token_abc"`
	if !strings.Contains(configStr, expectedHeader) && !strings.Contains(configStr, `"Authorization": "Bearer jwt_secret_token_abc"`) {
		t.Errorf("expected header %s in config, got %s", expectedHeader, configStr)
	}

	// Check body user_id alias:
	expectedBodyUserID := `"user_id":"usr_7781"`
	if !strings.Contains(configStr, expectedBodyUserID) && !strings.Contains(configStr, `"user_id": "usr_7781"`) {
		t.Errorf("expected user_id %s in config, got %s", expectedBodyUserID, configStr)
	}

	// Check array index lookup:
	expectedFirstItem := `"first_item":"item-alpha"`
	if !strings.Contains(configStr, expectedFirstItem) && !strings.Contains(configStr, `"first_item": "item-alpha"`) {
		t.Errorf("expected first_item %s in config, got %s", expectedFirstItem, configStr)
	}

	// Check full object substitution:
	expectedMeta := `{"role":"admin","tier":2}`
	if !strings.Contains(configStr, expectedMeta) {
		t.Errorf("expected meta object %s in config, got %s", expectedMeta, configStr)
	}

	// Also verify step_executions.config in MySQL was updated with resolved config
	var dbConfigStr string
	err = db.QueryRowContext(ctx, "SELECT config FROM step_executions WHERE id = ?", se2.ID).Scan(&dbConfigStr)
	if err != nil {
		t.Fatalf("failed to read step 2 config from DB: %v", err)
	}
	if !strings.Contains(dbConfigStr, expectedURL) {
		t.Errorf("expected DB config to be updated with resolved URL, got %s", dbConfigStr)
	}

	// Complete step 2
	err = workerLogic.CompleteStep(ctx, se2.ID, "worker-1", json.RawMessage(`{"status":"profile_fetched"}`))
	if err != nil {
		t.Fatalf("failed to complete step 2: %v", err)
	}

	// Verify workflow completes
	var wfStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExec.ID).Scan(&wfStatus)
	if wfStatus != "COMPLETED" {
		t.Errorf("expected workflow status COMPLETED, got %s", wfStatus)
	}
	t.Logf("✓ Forward Payload Passing (Step Output Chaining) verified end-to-end!")
}

func TestIntegration_PayloadPassing_CompensationOriginalOutput(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	ctx := context.Background()

	stepRepo := step.NewStepRepository(db)
	stepValidator := validator.NewValidator()
	wfRepo := workflow.NewWorkflowRepository(db, stepRepo)
	wfLogic := workflow.NewWorkflowLogic(wfRepo, stepValidator)
	workerLogic := worker.NewWorkerLogic(stepRepo)

	wfID := uuid.NewString()
	step1ID := "step-charge"
	step2ID := "step-deliver"

	// 1. Workflow where Step 1 has compensation that accesses Step 1's outputs via {{original.key}} and {{original.output.key}}
	wfReq := &workflow.WorkflowRequest{
		Name:    "test-comp-payload-" + wfID[:8],
		Version: 1,
		Steps: []step.Step{
			{
				ID:     step1ID,
				Name:   "Charge Card",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"method":"POST","url":"https://payment.internal/charge"}`),
				Compensation: &step.Compensation{
					Type: step.StepTypeHTTP,
					Config: json.RawMessage(`{
						"method": "POST",
						"url": "https://payment.internal/refund/{{original.charge_id}}",
						"body": {
							"charge_id": "{{original.output.charge_id}}",
							"amount": "{{original.amount}}",
							"ref": "{{steps.step-charge.output.charge_id}}"
						}
					}`),
				},
			},
			{
				ID:        step2ID,
				Name:      "Deliver Goods",
				Type:      step.StepTypeHTTP,
				DependsOn: []string{step1ID},
				Config:    json.RawMessage(`{"method":"POST","url":"https://delivery.internal/ship"}`),
			},
		},
	}

	wf, err := wfLogic.CreateWorkflow(ctx, wfReq)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE id = ?", wf.ID)
	}()

	// 2. Trigger workflow
	wfExec, err := wfRepo.ExecuteWorkflowTx(ctx, wf.ID, nil)
	if err != nil {
		t.Fatalf("failed to trigger workflow: %v", err)
	}

	// 3. Claim and complete Step 1 with charge output
	_, se1, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se1 == nil || se1.StepID != step1ID {
		t.Fatalf("expected to claim step 1, got %v, err=%v", se1, err)
	}

	step1Output := json.RawMessage(`{
		"charge_id": "ch_990182",
		"amount": 350000,
		"currency": "IDR"
	}`)
	err = workerLogic.CompleteStep(ctx, se1.ID, "worker-1", step1Output)
	if err != nil {
		t.Fatalf("failed to complete step 1: %v", err)
	}

	// 4. Claim Step 2 and fail it (exhausting attempts)
	_, se2, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || se2 == nil || se2.StepID != step2ID {
		t.Fatalf("expected to claim step 2, got %v, err=%v", se2, err)
	}

	err = workerLogic.FailStep(ctx, se2.ID, "worker-1", "delivery failed: out of coverage")
	if err != nil {
		t.Fatalf("failed to fail step 2: %v", err)
	}

	// 5. Claim compensation step -> JIT should resolve {{original.charge_id}} and {{original.amount}}!
	compStep, compExec, err := workerLogic.ClaimNext(ctx, "worker-1")
	if err != nil || compExec == nil || !compExec.IsCompensation {
		t.Fatalf("expected to claim compensation step, got %v, err=%v", compExec, err)
	}

	compConfigStr := string(compStep.Config)
	t.Logf("Resolved Compensation Config: %s", compConfigStr)

	expectedRefundURL := "https://payment.internal/refund/ch_990182"
	if !strings.Contains(compConfigStr, expectedRefundURL) {
		t.Errorf("expected URL %s in compensation config, got %s", expectedRefundURL, compConfigStr)
	}

	if !strings.Contains(compConfigStr, `"charge_id":"ch_990182"`) && !strings.Contains(compConfigStr, `"charge_id": "ch_990182"`) {
		t.Errorf("expected charge_id in compensation body, got %s", compConfigStr)
	}

	if !strings.Contains(compConfigStr, `"amount":"350000"`) && !strings.Contains(compConfigStr, `"amount": "350000"`) && !strings.Contains(compConfigStr, `"amount": 350000`) {
		t.Errorf("expected amount in compensation body, got %s", compConfigStr)
	}

	if !strings.Contains(compConfigStr, `"ref":"ch_990182"`) && !strings.Contains(compConfigStr, `"ref": "ch_990182"`) {
		t.Errorf("expected ref in compensation body, got %s", compConfigStr)
	}

	// Complete compensation step
	err = workerLogic.CompleteStep(ctx, compExec.ID, "worker-1", json.RawMessage(`{"refund_id":"ref_112"}`))
	if err != nil {
		t.Fatalf("failed to complete compensation: %v", err)
	}

	// Verify workflow reaches COMPENSATED
	var wfStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM workflow_executions WHERE id = ?", wfExec.ID).Scan(&wfStatus)
	if wfStatus != "COMPENSATED" {
		t.Errorf("expected workflow status COMPENSATED, got %s", wfStatus)
	}
	t.Logf("✓ Rollback Payload Passing (Original Step Output Access) verified end-to-end!")
}






