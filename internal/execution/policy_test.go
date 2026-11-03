package execution

import (
	"encoding/json"
	"math"
	"testing"
)

func ticket(id string, status StepStatus, compensation bool, deps ...string) StepExecution {
	return StepExecution{ID: id, StepID: id, Status: status, IsCompensation: compensation, DependsOn: deps}
}

func TestReconcileParallelRollback(t *testing.T) {
	steps := []StepExecution{
		ticket("a", StatusCompleted, false), ticket("b", StatusRunning, false, "a"), ticket("c", StatusFailed, false, "a"),
		ticket("a", StatusWaiting, true, "b", "c"), ticket("b", StatusWaiting, true), ticket("c", StatusWaiting, true),
	}
	status, next := reconcile(StatusRunning, steps)
	if status != StatusCompensating || next[3].Status != StatusWaiting || next[4].Status != StatusWaiting || next[5].Status != StatusCancelled {
		t.Fatalf("unsafe rollback: %s %+v", status, next)
	}
	// The original snapshot must not be mutated by policy evaluation.
	if steps[5].Status != StatusWaiting {
		t.Fatal("policy mutated input")
	}
	next[1].Status = StatusCompleted
	status, next = reconcile(status, next)
	if next[4].Status != StatusPending || next[3].Status != StatusWaiting {
		t.Fatal("leaf compensation not scheduled first")
	}
	next[4].Status = StatusCompensated
	status, next = reconcile(status, next)
	if next[3].Status != StatusPending {
		t.Fatal("parent compensation not released")
	}
	next[3].Status = StatusCompensated
	status, _ = reconcile(status, next)
	if status != StatusCompensated {
		t.Fatalf("got %s", status)
	}
}

func TestReconcileRollbackThroughUncompensatedStep(t *testing.T) {
	steps := []StepExecution{ticket("a", StatusCompleted, false), ticket("b", StatusCompleted, false, "a"),
		ticket("c", StatusCompleted, false, "b"), ticket("failure", StatusFailed, false),
		ticket("a", StatusWaiting, true, "b"), ticket("c", StatusWaiting, true)}
	_, next := reconcile(StatusRunning, steps)
	if next[4].Status != StatusWaiting || next[5].Status != StatusPending {
		t.Fatal("rollback skipped transitive dependency")
	}
}

func TestReconcileTerminalAndRetryPolicies(t *testing.T) {
	cases := []struct {
		name   string
		status WorkflowStatus
		steps  []StepExecution
		want   WorkflowStatus
	}{
		{"success", StatusRunning, []StepExecution{ticket("a", StatusCompleted, false), ticket("a", StatusWaiting, true)}, StatusCompleted},
		{"first failure", StatusRunning, []StepExecution{ticket("a", StatusFailed, false), ticket("a", StatusWaiting, true)}, StatusFailed},
		{"wait sibling", StatusRunning, []StepExecution{ticket("a", StatusFailed, false), ticket("b", StatusRunning, false)}, StatusCompensating},
		{"last sibling done", StatusCompensating, []StepExecution{ticket("a", StatusFailed, false), ticket("b", StatusCompleted, false)}, StatusFailed},
		{"compensation failure", StatusCompensationFailed, []StepExecution{ticket("a", StatusCompleted, false), ticket("a", StatusCompensated, true)}, StatusCompensationFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := reconcile(tc.status, tc.steps)
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
	s := StepExecution{Attempt: 2, MaxAttempts: 3, BackoffSeconds: 2, Multiplier: 3}
	if retryDelay(s) != 6 || !mayRetry(StatusRunning, s) || mayRetry(StatusCompensating, s) {
		t.Fatal("normal retry policy")
	}
	s.IsCompensation = true
	if !mayRetry(StatusCompensating, s) || mayRetry(StatusCompensationFailed, s) {
		t.Fatal("compensation retry policy")
	}
	s.Attempt = 100000
	s.Multiplier = math.MaxFloat64
	if retryDelay(s) != maximumBackoffSeconds {
		t.Fatal("backoff overflow")
	}
	s.MaxBackoffSeconds = 30
	if retryDelay(s) != 30 {
		t.Fatal("backoff cap")
	}
}

func TestContextPreservesNumbersAndAliases(t *testing.T) {
	raw, err := mergeContext(json.RawMessage(`{"id":9007199254740993,"nested":{"a":1}}`), json.RawMessage(`{"nested":{"b":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err = accumulateOutput(raw, ticket("payment", StatusRunning, false), json.RawMessage(`{"id":9007199254740995}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err = accumulateOutput(raw, ticket("payment", StatusRunning, true), json.RawMessage(`{"refund":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if string(obj["id"]) != "9007199254740993" || string(obj["nested"]) != `{"b":2}` {
		t.Fatalf("context changed: %s", raw)
	}
	resolved, err := resolveConfig(json.RawMessage(`{"body":"{{payment.compensation.output}}","original":"{{original.output}}"}`), raw, json.RawMessage(`{"id":123}`), true)
	if err != nil || !json.Valid(resolved) {
		t.Fatalf("resolve: %s %v", resolved, err)
	}
	for _, invalid := range []string{`[]`, `42`, `"text"`, `{invalid`} {
		if _, err := contextObject(json.RawMessage(invalid)); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
}

func TestReconcileSkippedSteps(t *testing.T) {
	// a -> b (skipped) -> c (waiting)
	steps := []StepExecution{
		ticket("a", StatusCompleted, false),
		ticket("b", StatusSkipped, false, "a"),
		ticket("c", StatusWaiting, false, "b"),
		ticket("a", StatusWaiting, true),
		ticket("b", StatusWaiting, true),
	}

	status, next := reconcile(StatusRunning, steps)
	if status != StatusRunning {
		t.Fatalf("expected StatusRunning, got %s", status)
	}
	// Step c must be PENDING since b is SKIPPED
	if next[2].Status != StatusPending {
		t.Fatalf("expected step c to be PENDING, got %s", next[2].Status)
	}
	// Step b compensation must be CANCELLED
	if next[4].Status != StatusCancelled {
		t.Fatalf("expected step b compensation to be CANCELLED, got %s", next[4].Status)
	}

	// Complete step c
	next[2].Status = StatusCompleted
	status, finalSteps := reconcile(status, next)
	// Workflow must be COMPLETED because all normal steps are COMPLETED or SKIPPED
	if status != StatusCompleted {
		t.Fatalf("expected workflow StatusCompleted, got %s", status)
	}
	// Step a compensation must also be CANCELLED
	if finalSteps[3].Status != StatusCancelled {
		t.Fatalf("expected step a compensation to be CANCELLED upon completion, got %s", finalSteps[3].Status)
	}

	// Test rollback through skipped step:
	// a (completed) -> b (skipped) -> c (failed)
	// Rollback should cancel b's comp, and compensate a
	rollbackSteps := []StepExecution{
		ticket("a", StatusCompleted, false),
		ticket("b", StatusSkipped, false, "a"),
		ticket("c", StatusFailed, false, "b"),
		ticket("a", StatusWaiting, true, "b"),
		ticket("b", StatusWaiting, true),
	}
	status, rollNext := reconcile(StatusRunning, rollbackSteps)
	if status != StatusCompensating {
		t.Fatalf("expected StatusCompensating, got %s", status)
	}
	if rollNext[4].Status != StatusCancelled {
		t.Fatalf("expected step b compensation to be CANCELLED, got %s", rollNext[4].Status)
	}
	// Step a compensation should be PENDING (since b's compensation is CANCELLED and passed through)
	if rollNext[3].Status != StatusPending {
		t.Fatalf("expected step a compensation to be PENDING, got %s", rollNext[3].Status)
	}
}

