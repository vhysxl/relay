package execution_test

import (
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

func TestRuntimeRechecksLegacyPendingCompensation(t *testing.T) {
	a, b, c := basicStep("a"), basicStep("b"), basicStep("c")
	a.Compensation = &step.Compensation{Type: step.StepTypeHTTP, Config: a.Config}
	b.Compensation = &step.Compensation{Type: step.StepTypeHTTP, Config: b.Config}
	b.DependsOn, c.DependsOn = []string{"a"}, []string{"a"}
	f := runtimeDB(t, []step.Step{a, b, c})
	first := f.claim(t, "worker-a")
	if err := f.repo.CompleteStep(f.ctx, first.ID, "worker-a", nil); err != nil {
		t.Fatal(err)
	}
	left, right := f.claim(t, "worker-b"), f.claim(t, "worker-c")
	workerB, workerC := "worker-b", "worker-c"
	if left.StepID == "c" {
		left, right = right, left
		workerB, workerC = workerC, workerB
	}
	if err := f.repo.FailStep(f.ctx, right.ID, workerC, "failure"); err != nil {
		t.Fatal(err)
	}
	// Simulate an execution persisted before the runtime refactor.
	if _, err := f.db.ExecContext(f.ctx, `UPDATE step_executions SET status='PENDING' WHERE workflow_execution_id=? AND step_id='a' AND is_compensation=TRUE`, f.exec.ID); err != nil {
		t.Fatal(err)
	}
	_, unsafe, err := f.repo.ClaimPending(f.ctx, "rollback", time.Minute)
	if err != nil || unsafe != nil {
		t.Fatalf("legacy parent rolled back before child: %+v %v", unsafe, err)
	}
	exec, err := f.repo.GetExecution(f.ctx, f.exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range exec.Steps {
		if s.IsCompensation && s.Status != execution.StatusWaiting {
			t.Fatalf("legacy reconciliation not committed: %+v", s)
		}
	}
	if err := f.repo.CompleteStep(f.ctx, left.ID, workerB, nil); err != nil {
		t.Fatal(err)
	}
	next := f.claim(t, "rollback")
	if !next.IsCompensation || next.StepID != "b" {
		t.Fatalf("wrong rollback order: %+v", next)
	}
}
