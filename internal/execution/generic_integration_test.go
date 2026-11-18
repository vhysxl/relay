package execution_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

func TestGenericInputsAndQueueRouting(t *testing.T) {
	values := []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`42`), json.RawMessage(`[1,"two"]`), json.RawMessage(`{"id":"ORD-1"}`)}
	definitions := make([]step.Step, len(values))
	for i, input := range values {
		definitions[i] = basicStep(string(rune('a' + i)))
		definitions[i].Activity = "worker.owns.implementation"
		definitions[i].Input = input
	}
	definitions[0].Queue = "another-service"
	f := runtimeDB(t, definitions)
	for _, definition := range definitions[1:] {
		s := f.claim(t, "worker")
		if s.StepID != definition.ID || s.Activity != definition.Activity || !util.AreJSONEqual(s.Input, util.NormalizeInput(definition.Input)) {
			t.Fatalf("unexpected generic input/routing: %+v", s)
		}
		if err := f.complete(f.ctx, s.ID, "worker", nil); err != nil {
			t.Fatal(err)
		}
	}
	zeroTask, err := f.tryClaim(f.ctx, "worker")
	if err != nil || zeroTask != nil {
		t.Fatalf("other queue leaked: %+v %v", zeroTask, err)
	}
	task, err := f.repo.ClaimTask(f.ctx, execution.ClaimFilter{ServiceID: "runtime-tests", WorkerID: "other-worker", Queue: "another-service", LeaseDuration: 15 * time.Second})
	if err != nil || task == nil || task.StepExecution.StepID != "a" {
		t.Fatalf("queue claim: %+v %v", task, err)
	}
	if string(task.StepExecution.Input) != "{}" {
		t.Fatalf("omitted input: %s", task.StepExecution.Input)
	}
}

func TestGenericRetryKeepsTemplateAndResolvesCurrentContext(t *testing.T) {
	s := basicStep("work")
	s.Activity = "business.execute"
	s.Input = json.RawMessage(`{"value":"{{context.value}}"}`)
	s.Retry = &step.RetryPolicy{MaxAttempts: 2}
	f := conditionalRuntimeDB(t, []step.Step{s}, json.RawMessage(`{"value":"first"}`))
	first := f.claim(t, "worker")
	if !util.AreJSONEqual(first.Input, json.RawMessage(`{"value":"first"}`)) {
		t.Fatalf("first input: %s", first.Input)
	}
	if err := f.fail(f.ctx, first.ID, "worker", "transient"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(f.ctx, `UPDATE workflow_executions SET context=JSON_SET(context,'$.value','second') WHERE id=?`, f.exec.ID); err != nil {
		t.Fatal(err)
	}
	second := f.claim(t, "worker")
	if second.Attempt != 2 || !util.AreJSONEqual(second.Input, json.RawMessage(`{"value":"second"}`)) {
		t.Fatalf("retry input: %+v", second)
	}
	stored, err := f.repo.GetExecution(f.ctx, f.exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !util.AreJSONEqual(stored.Steps[0].Input, s.Input) {
		t.Fatalf("template overwritten: %s", stored.Steps[0].Input)
	}
}

func TestGenericReportsSerializeAndReplayOlderAttempts(t *testing.T) {
	a, b := basicStep("a"), basicStep("b")
	b.Retry = &step.RetryPolicy{MaxAttempts: 2}
	f := runtimeDB(t, []step.Step{a, b})
	first, second := f.claim(t, "worker"), f.claim(t, "worker")
	token := f.token(first.ID, "worker")
	checks := []struct {
		service, worker, token string
		want                   error
	}{
		{"other-service", "worker", token, errdefs.ErrForbiddenTask},
		{"runtime-tests", "other-worker", token, errdefs.ErrLockLost},
		{"runtime-tests", "worker", "invalid", errdefs.ErrInvalidLeaseToken},
	}
	for _, check := range checks {
		if err := f.repo.CompleteTask(f.ctx, check.service, first.ID, check.worker, check.token, nil); !errors.Is(err, check.want) {
			t.Fatalf("ownership error: %v want %v", err, check.want)
		}
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			results <- f.repo.CompleteTask(f.ctx, "runtime-tests", first.ID, "worker", token, json.RawMessage(`{"ok":true}`))
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.repo.CompleteTask(f.ctx, "runtime-tests", first.ID, "worker", token, json.RawMessage(`{"ok":false}`)); !errors.Is(err, errdefs.ErrReportConflict) {
		t.Fatalf("conflicting report: %v", err)
	}
	oldToken := f.token(second.ID, "worker")
	if err := f.fail(f.ctx, second.ID, "worker", "transient"); err != nil {
		t.Fatal(err)
	}
	retry := f.claim(t, "worker")
	if retry.Attempt != 2 {
		t.Fatalf("attempt: %+v", retry)
	}
	if err := f.repo.FailTask(f.ctx, "runtime-tests", second.ID, "worker", oldToken, "transient", true); err != nil {
		t.Fatalf("old fail replay: %v", err)
	}
	if err := f.repo.FailTask(f.ctx, "runtime-tests", second.ID, "worker", oldToken, "different", true); !errors.Is(err, errdefs.ErrReportConflict) {
		t.Fatalf("old fail conflict: %v", err)
	}
	if err := f.complete(f.ctx, retry.ID, "worker", nil); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != execution.StatusCompleted {
		t.Fatal("replay changed active attempt")
	}
}
