package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
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
		dsn = "relay:relay@tcp(localhost:3306)/relay_taskqueue?parseTime=true"
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
