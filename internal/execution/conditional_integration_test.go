package execution_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

func conditionalRuntimeDB(t *testing.T, steps []step.Step, contextJSON json.RawMessage) runtimeFixture {
	t.Helper()
	db := getExecutionTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	wfRepo := workflow.NewWorkflowRepository(db, step.NewStepRepository(db))
	now := time.Now().UTC()
	wf, err := wfRepo.CreateWorkflow(ctx, &workflow.Workflow{
		ID:        uuid.NewString(),
		Name:      "test-cond-" + uuid.NewString(),
		Version:   1,
		Context:   contextJSON,
		Steps:     steps,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := execution.NewExecutionRepository(db, &config.Config{DefaultLeaseDuration: time.Minute})
	logic := execution.NewExecutionLogic(repo, wfRepo)
	exec, err := logic.ExecuteWorkflow(ctx, wf.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeFixture{db, repo, logic, exec, ctx}
}

func TestRuntimeConditional_ContextTrue(t *testing.T) {
	s := basicStep("cond-step")
	s.When = json.RawMessage(`{"ref":"user.tier","op":"eq","value":"diamond"}`)

	f := conditionalRuntimeDB(t, []step.Step{s}, json.RawMessage(`{"user":{"tier":"diamond"}}`))
	claimed := f.claim(t, "worker-1")
	if claimed.StepID != "cond-step" || claimed.Status != execution.StatusRunning {
		t.Fatalf("expected running claimed step, got %+v", claimed)
	}
	if claimed.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", claimed.Attempt)
	}

	// Verify condition_evaluation is saved in database
	var evalStr sql.NullString
	err := f.db.QueryRowContext(f.ctx, `SELECT condition_evaluation FROM step_executions WHERE id = ?`, claimed.ID).Scan(&evalStr)
	if err != nil || !evalStr.Valid || len(evalStr.String) == 0 {
		t.Fatalf("expected condition_evaluation saved, err: %v, val: %v", err, evalStr)
	}
}

func TestRuntimeConditional_ContextFalse_SkipsAndUnlocksDependent(t *testing.T) {
	stepA := basicStep("step-a")
	stepA.When = json.RawMessage(`{"ref":"user.tier","op":"eq","value":"diamond"}`)

	stepB := basicStep("step-b")
	stepB.DependsOn = []string{"step-a"}

	f := conditionalRuntimeDB(t, []step.Step{stepA, stepB}, json.RawMessage(`{"user":{"tier":"silver"}}`))

	// Step A condition is false -> ClaimPending skips it and returns nil task
	_, claimed, err := f.repo.ClaimPending(f.ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("unexpected claim error: %v", err)
	}
	if claimed != nil {
		t.Fatalf("expected no task returned for skipped step, got %+v", claimed)
	}

	// Verify Step A is marked SKIPPED with attempt = 0 in database
	var status string
	var attempt int
	var evalStr sql.NullString
	err = f.db.QueryRowContext(f.ctx, `SELECT status, attempt, condition_evaluation FROM step_executions WHERE workflow_execution_id = ? AND step_id = 'step-a'`, f.exec.ID).Scan(&status, &attempt, &evalStr)
	if err != nil {
		t.Fatal(err)
	}
	if status != "SKIPPED" || attempt != 0 || !evalStr.Valid {
		t.Fatalf("expected status=SKIPPED, attempt=0, eval saved; got status=%s, attempt=%d", status, attempt)
	}

	// Next claim should now claim Step B because Step A's SKIPPED status satisfied Step B's dependency!
	claimedB := f.claim(t, "worker-1")
	if claimedB.StepID != "step-b" || claimedB.Status != execution.StatusRunning {
		t.Fatalf("expected Step B running, got %+v", claimedB)
	}
}

func TestRuntimeConditional_StepOutputAnchor(t *testing.T) {
	stepA := basicStep("step-a")
	stepB := basicStep("step-b")
	stepB.DependsOn = []string{"step-a"}
	stepB.When = json.RawMessage(`{"ref":"steps.step-a.output.result.vip","op":"eq","value":true}`)

	f := conditionalRuntimeDB(t, []step.Step{stepA, stepB}, nil)

	// Claim and complete Step A with output
	claimedA := f.claim(t, "worker-a")
	err := f.repo.CompleteStep(f.ctx, claimedA.ID, "worker-a", json.RawMessage(`{"result":{"vip":true}}`))
	if err != nil {
		t.Fatal(err)
	}

	// Claim Step B: condition checks step-a output vip == true -> should pass and run!
	claimedB := f.claim(t, "worker-b")
	if claimedB.StepID != "step-b" || claimedB.Status != execution.StatusRunning {
		t.Fatalf("expected Step B to run after output matched, got %+v", claimedB)
	}
}

func TestRuntimeConditional_ChainedSkipsCompleteWorkflow(t *testing.T) {
	stepA := basicStep("step-a")
	stepA.When = json.RawMessage(`{"ref":"enable_a","op":"eq","value":true}`)

	stepB := basicStep("step-b")
	stepB.DependsOn = []string{"step-a"}
	stepB.When = json.RawMessage(`{"ref":"enable_b","op":"eq","value":true}`)

	f := conditionalRuntimeDB(t, []step.Step{stepA, stepB}, json.RawMessage(`{"enable_a":false,"enable_b":false}`))

	// Claim step-a (evaluates false -> skips)
	_, claimed, err := f.repo.ClaimPending(f.ctx, "worker-1", time.Minute)
	if err != nil || claimed != nil {
		t.Fatalf("expected skip a, got %+v %v", claimed, err)
	}

	// Claim step-b (evaluates false -> skips)
	_, claimed, err = f.repo.ClaimPending(f.ctx, "worker-1", time.Minute)
	if err != nil || claimed != nil {
		t.Fatalf("expected skip b, got %+v %v", claimed, err)
	}

	// Workflow must now be COMPLETED
	if s := f.status(t); s != execution.StatusCompleted {
		t.Fatalf("expected workflow COMPLETED after chained skips, got %s", s)
	}
}

func TestRuntimeConditional_MissingAnchorFailsWithoutExecutionAttempt(t *testing.T) {
	stepA := basicStep("step-a")
	// Anchor missing without exists operator
	stepA.When = json.RawMessage(`{"ref":"steps.missing_step.output.foo","op":"eq","value":"bar"}`)

	f := conditionalRuntimeDB(t, []step.Step{stepA}, nil)

	// Claim: condition evaluation fails -> step marked FAILED, attempt = 0, workflow fails
	_, claimed, err := f.repo.ClaimPending(f.ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed != nil {
		t.Fatalf("expected no task returned on condition failure, got %+v", claimed)
	}

	var status, errMsg string
	var attempt int
	err = f.db.QueryRowContext(f.ctx, `SELECT status, attempt, error_message FROM step_executions WHERE workflow_execution_id = ? AND step_id = 'step-a'`, f.exec.ID).Scan(&status, &attempt, &errMsg)
	if err != nil {
		t.Fatal(err)
	}
	if status != "FAILED" || attempt != 0 {
		t.Fatalf("expected FAILED with attempt=0, got status=%s attempt=%d", status, attempt)
	}

	if s := f.status(t); s != execution.StatusFailed {
		t.Fatalf("expected workflow FAILED, got %s", s)
	}
}

func TestRuntimeConditional_SkippedStepCompensationCancelledAndRollbackSucceeds(t *testing.T) {
	a, b, c := basicStep("a"), basicStep("b"), basicStep("c")
	a.Compensation = &step.Compensation{Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}
	b.Compensation = &step.Compensation{Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}
	b.When = json.RawMessage(`{"ref":"run_b","op":"eq","value":true}`)
	b.DependsOn = []string{"a"}
	c.DependsOn = []string{"b"}

	f := conditionalRuntimeDB(t, []step.Step{a, b, c}, json.RawMessage(`{"run_b":false}`))

	// 1. Claim and complete step A
	clA := f.claim(t, "worker-a")
	if err := f.repo.CompleteStep(f.ctx, clA.ID, "worker-a", nil); err != nil {
		t.Fatal(err)
	}

	// 2. Claim step B: skips because run_b is false
	_, clB, err := f.repo.ClaimPending(f.ctx, "worker-b", time.Minute)
	if err != nil || clB != nil {
		t.Fatalf("expected skip b, got %+v %v", clB, err)
	}

	// 3. Claim step C: fail step C
	clC := f.claim(t, "worker-c")
	if err := f.repo.FailStep(f.ctx, clC.ID, "worker-c", "fatal error"); err != nil {
		t.Fatal(err)
	}

	// Workflow must enter COMPENSATING
	if s := f.status(t); s != execution.StatusCompensating {
		t.Fatalf("expected COMPENSATING, got %s", s)
	}

	// Verify Step B compensation was CANCELLED
	var bCompStatus string
	err = f.db.QueryRowContext(f.ctx, `SELECT status FROM step_executions WHERE workflow_execution_id = ? AND step_id = 'b' AND is_compensation = TRUE`, f.exec.ID).Scan(&bCompStatus)
	if err != nil {
		t.Fatal(err)
	}
	if bCompStatus != "CANCELLED" {
		t.Fatalf("expected step b compensation CANCELLED, got %s", bCompStatus)
	}

	// Claim rollback: Step A compensation must be claimed!
	rollbackA := f.claim(t, "worker-rollback")
	if rollbackA.StepID != "a" || !rollbackA.IsCompensation {
		t.Fatalf("expected step a compensation claimed, got %+v", rollbackA)
	}

	// Complete rollback for step A
	if err := f.repo.CompleteStep(f.ctx, rollbackA.ID, "worker-rollback", nil); err != nil {
		t.Fatal(err)
	}

	// Workflow must now be COMPENSATED
	if s := f.status(t); s != execution.StatusCompensated {
		t.Fatalf("expected workflow COMPENSATED, got %s", s)
	}
}
