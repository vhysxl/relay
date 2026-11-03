package execution_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type runtimeFixture struct {
	db    *sql.DB
	repo  execution.Repository
	logic execution.ExecutionLogic
	exec  *execution.WorkflowExecution
	ctx   context.Context
}

func runtimeDB(t *testing.T, steps []step.Step) runtimeFixture {
	t.Helper()
	db := getExecutionTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	wfRepo := workflow.NewWorkflowRepository(db, step.NewStepRepository(db))
	now := time.Now().UTC()
	wf, err := wfRepo.CreateWorkflow(ctx, &workflow.Workflow{ID: uuid.NewString(), Name: "test-checkout-" + uuid.NewString(), Version: 1, Steps: steps, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	repo := execution.NewExecutionRepository(db, &config.Config{DefaultLeaseDuration: time.Minute})
	logic := execution.NewExecutionLogic(repo, wfRepo)
	resp, _, err := logic.ExecuteWorkflow(ctx, wf.Name, &execution.ExecuteWorkflowRequest{
		IdempotencyKey: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := repo.GetExecution(ctx, resp.ID)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeFixture{db, repo, logic, exec, ctx}
}

func basicStep(id string) step.Step {
	return step.Step{ID: id, Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}
}
func (f runtimeFixture) claim(t *testing.T, worker string) *execution.StepExecution {
	t.Helper()
	_, s, err := f.repo.ClaimPending(f.ctx, worker, time.Minute)
	if err != nil || s == nil {
		t.Fatalf("claim: %+v %v", s, err)
	}
	return s
}
func (f runtimeFixture) status(t *testing.T) execution.WorkflowStatus {
	t.Helper()
	s, err := f.repo.GetExecution(f.ctx, f.exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s.Status
}

func TestRuntimeCreationRollback(t *testing.T) {
	f := runtimeDB(t, []step.Step{basicStep("a")})
	now := time.Now().UTC()
	id := uuid.NewString()
	_, err := f.repo.CreateExecution(f.ctx, &execution.WorkflowExecution{ID: id, WorkflowID: f.exec.WorkflowID, Status: execution.StatusRunning, Context: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now, Steps: []execution.StepExecution{
		{ID: uuid.NewString(), StepID: "valid", Type: "http", Config: json.RawMessage(`{}`), Status: execution.StatusPending, MaxAttempts: 1, Multiplier: 1, CreatedAt: now, UpdatedAt: now},
		{ID: uuid.NewString(), StepID: "invalid", Type: "http", Config: json.RawMessage(`{invalid`), Status: execution.StatusPending, MaxAttempts: 1, Multiplier: 1, CreatedAt: now, UpdatedAt: now},
	}})
	if err == nil {
		t.Fatal("invalid insert unexpectedly succeeded")
	}
	var count int
	if err := f.db.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM workflow_executions WHERE id=?`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial workflow persisted: %d %v", count, err)
	}
	if err := f.db.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM step_executions WHERE workflow_execution_id=?`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial steps persisted: %d %v", count, err)
	}
}

func TestRuntimeConcurrentClaimsAndOutputs(t *testing.T) {
	const total = 12
	var definitions []step.Step
	for i := range total {
		definitions = append(definitions, basicStep(fmt.Sprintf("s%d", i)))
	}
	f := runtimeDB(t, definitions)
	start := make(chan struct{})
	results := make(chan string, total)
	failures := make(chan error, total)
	var wg sync.WaitGroup
	for i := range total {
		wg.Go(func() {
			<-start
			worker := fmt.Sprintf("server-regression-%d", i)
			var s *execution.StepExecution
			for s == nil {
				var err error
				_, s, err = f.repo.ClaimPending(f.ctx, worker, time.Minute)
				if err != nil {
					failures <- err
					return
				}
				if f.ctx.Err() != nil {
					failures <- f.ctx.Err()
					return
				}
				if s == nil {
					time.Sleep(time.Millisecond)
				}
			}
			results <- s.ID
			if err := f.repo.CompleteStep(f.ctx, s.ID, worker, json.RawMessage(`{"value":9007199254740993}`)); err != nil {
				failures <- err
			}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := map[string]bool{}
	for id := range results {
		if seen[id] {
			t.Errorf("double claim %s", id)
		}
		seen[id] = true
	}
	if len(seen) != total {
		t.Fatalf("claimed %d of %d", len(seen), total)
	}
	if f.status(t) != execution.StatusCompleted {
		t.Fatal("workflow not completed")
	}
	exec, err := f.repo.GetExecution(f.ctx, f.exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ctx struct {
		Steps map[string]json.RawMessage `json:"steps"`
	}
	if err := json.Unmarshal(exec.Context, &ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Steps) != total {
		t.Fatalf("lost output: %s", exec.Context)
	}
	for _, s := range exec.Steps {
		if s.Attempt != 1 || s.Status != execution.StatusCompleted {
			t.Fatalf("invalid claim: %+v", s)
		}
	}
}

func TestRuntimeExpiredLeaseAndRetry(t *testing.T) {
	s := basicStep("a")
	s.Retry = &step.RetryPolicy{MaxAttempts: 2}
	f := runtimeDB(t, []step.Step{s})
	claimed := f.claim(t, "old-worker")
	if _, err := f.db.ExecContext(f.ctx, `UPDATE step_executions SET locked_until=DATE_SUB(NOW(),INTERVAL 1 SECOND) WHERE id=?`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{
		func() error { return f.repo.Heartbeat(f.ctx, claimed.ID, "old-worker", time.Minute) },
		func() error { return f.repo.CompleteStep(f.ctx, claimed.ID, "old-worker", nil) },
		func() error { return f.repo.FailStep(f.ctx, claimed.ID, "old-worker", "late") },
	} {
		if err := action(); !errors.Is(err, errdefs.ErrLockLost) {
			t.Fatalf("expired owner accepted: %v", err)
		}
	}
	n, err := f.repo.ReapExpiredSteps(f.ctx)
	if err != nil || n != 1 {
		t.Fatalf("reap: %d %v", n, err)
	}
	next := f.claim(t, "new-worker")
	if next.Attempt != 2 {
		t.Fatal("attempt not incremented")
	}
	if err := f.repo.CompleteStep(f.ctx, claimed.ID, "old-worker", nil); !errors.Is(err, errdefs.ErrLockLost) {
		t.Fatal("old owner acknowledged retry")
	}
	if err := f.repo.CompleteStep(f.ctx, next.ID, "new-worker", nil); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != execution.StatusCompleted {
		t.Fatal("retry did not finish")
	}
}

func TestRuntimeReaperRacesWithLiveOwner(t *testing.T) {
	f := runtimeDB(t, []step.Step{basicStep("a")})
	s := f.claim(t, "worker")
	start := make(chan struct{})
	failures := make(chan error, 3)
	var wg sync.WaitGroup
	wg.Go(func() { <-start; failures <- f.repo.Heartbeat(f.ctx, s.ID, "worker", time.Minute) })
	wg.Go(func() { <-start; _, err := f.repo.ReapExpiredSteps(f.ctx); failures <- err })
	wg.Go(func() { <-start; failures <- f.repo.CompleteStep(f.ctx, s.ID, "worker", nil) })
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil && !errors.Is(err, errdefs.ErrLockLost) {
			t.Error(err)
		}
	}
	if f.status(t) != execution.StatusCompleted {
		t.Fatal("live owner lost work")
	}
}

func TestRuntimeReaperRacesWithExpiredOwner(t *testing.T) {
	f := runtimeDB(t, []step.Step{basicStep("a")})
	s := f.claim(t, "worker")
	if _, err := f.db.ExecContext(f.ctx, `UPDATE step_executions SET locked_until=DATE_SUB(NOW(),INTERVAL 1 SECOND) WHERE id=?`, s.ID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Go(func() { <-start; results <- f.repo.Heartbeat(f.ctx, s.ID, "worker", time.Minute) })
	wg.Go(func() { <-start; results <- f.repo.CompleteStep(f.ctx, s.ID, "worker", nil) })
	var count int64
	var reapErr error
	wg.Go(func() { <-start; count, reapErr = f.repo.ReapExpiredSteps(f.ctx) })
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if !errors.Is(err, errdefs.ErrLockLost) {
			t.Fatalf("expired owner won race: %v", err)
		}
	}
	if reapErr != nil || count != 1 || f.status(t) != execution.StatusFailed {
		t.Fatalf("expired recovery: %d %v", count, reapErr)
	}
}

func TestRuntimeSnapshotAndCompensationFailure(t *testing.T) {
	a, b, c := basicStep("a"), basicStep("b"), basicStep("c")
	c.DependsOn = []string{"a", "b"}
	for _, s := range []*step.Step{&a, &b} {
		s.Compensation = &step.Compensation{Type: step.StepTypeMessage, Config: json.RawMessage(`{"payload":"{{original.output}}"}`), Timeout: 9, Retry: &step.RetryPolicy{MaxAttempts: 1}}
	}
	f := runtimeDB(t, []step.Step{a, b, c})
	// Definition changes must not overwrite the persisted execution snapshot.
	if _, err := f.db.ExecContext(f.ctx, `UPDATE step_definitions SET timeout=99, type='http' WHERE workflow_id=?`, f.exec.WorkflowID); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		s := f.claim(t, "worker")
		var err error
		if s.StepID == "c" {
			err = f.repo.FailStep(f.ctx, s.ID, "worker", "checkout failed")
		} else {
			err = f.repo.CompleteStep(f.ctx, s.ID, "worker", json.RawMessage(`{"charged":true}`))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	first, second := f.claim(t, "rollback-1"), f.claim(t, "rollback-2")
	for _, s := range []*execution.StepExecution{first, second} {
		if !s.IsCompensation || s.Type != "message" || s.TimeoutSeconds != 9 || s.MaxAttempts != 1 {
			t.Fatalf("definition overwrote snapshot: %+v", s)
		}
		var config struct {
			Payload struct {
				Charged bool `json:"charged"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(s.Config, &config); err != nil || !config.Payload.Charged {
			t.Fatalf("original output missing: %s %v", s.Config, err)
		}
	}
	if err := f.repo.FailStep(f.ctx, first.ID, "rollback-1", "refund failed"); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.CompleteStep(f.ctx, second.ID, "rollback-2", nil); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != execution.StatusCompensationFailed {
		t.Fatal("compensation failure overwritten by sibling completion")
	}
}

func TestRuntimeRecoveryContinuesAfterWorkflowError(t *testing.T) {
	s := basicStep("a")
	s.Retry = &step.RetryPolicy{MaxAttempts: 2}
	f := runtimeDB(t, []step.Step{s})
	otherResp, _, err := f.logic.ExecuteWorkflow(f.ctx, f.exec.WorkflowName, &execution.ExecuteWorkflowRequest{
		IdempotencyKey: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.repo.GetExecution(f.ctx, otherResp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(f.ctx, `UPDATE step_executions SET status='RUNNING',attempt=1,locked_by='crashed',locked_until=DATE_SUB(NOW(),INTERVAL 1 SECOND)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(f.ctx, `UPDATE step_executions SET depends_on='42' WHERE workflow_execution_id=?`, f.exec.ID); err != nil {
		t.Fatal(err)
	}
	n, err := f.repo.ReapExpiredSteps(f.ctx)
	if err == nil || n != 1 {
		t.Fatalf("recovery did not isolate workflow error: %d %v", n, err)
	}
	good, err := f.repo.GetExecution(f.ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if good.Steps[0].Status != execution.StatusPending {
		t.Fatal("healthy workflow not recovered")
	}
}
