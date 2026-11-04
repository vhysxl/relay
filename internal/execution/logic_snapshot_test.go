package execution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type snapshotRepository struct {
	ExecutionRepository
	create func(*WorkflowExecution) error
}

func (r snapshotRepository) CreateExecution(_ context.Context, s *WorkflowExecution) (*WorkflowExecution, error) {
	return s, r.create(s)
}

func (r snapshotRepository) CreateExecutionWithIdempotency(_ context.Context, s *WorkflowExecution, record *ExecutionRequestRecord, _ bool, _ string) (*ExecuteWorkflowResponse, bool, error) {
	if r.create != nil {
		if err := r.create(s); err != nil {
			return nil, false, err
		}
	}
	var receipt ExecuteWorkflowResponse
	_ = json.Unmarshal(record.ResponseReceipt, &receipt)
	return &receipt, false, nil
}

func (r snapshotRepository) GetExecutionRequest(_ context.Context, _ string, _ string) (*ExecutionRequestRecord, error) {
	return nil, errdefs.ErrExecutionRequestNotFound
}

type snapshotReader struct {
	read func(string) (*workflow.Workflow, error)
}

func (r snapshotReader) GetWorkflow(_ context.Context, name string) (*workflow.Workflow, error) {
	return r.read(name)
}

func (r snapshotReader) GetWorkflowVersion(_ context.Context, name string, _ int) (*workflow.Workflow, error) {
	return r.read(name)
}

func TestLogicBuildsIndependentSnapshot(t *testing.T) {
	wf := &workflow.Workflow{ID: "wf-id", Context: json.RawMessage(`{"tier":"diamond","nested":{"base":true}}`), Steps: []step.Step{
		{ID: "user", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"url":"/{{tier}}"}`), Compensation: &step.Compensation{Type: step.StepTypeMessage, Config: json.RawMessage(`{}`), Timeout: 7, Retry: &step.RetryPolicy{MaxAttempts: 4, Backoff: 2, Multiplier: 2}}},
		{ID: "checkout", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`), DependsOn: []string{"user"}},
	}}
	called := false
	logic := NewExecutionLogic(snapshotRepository{create: func(exec *WorkflowExecution) error {
		called = true
		if exec.WorkflowID != "wf-id" || exec.Status != StatusRunning || exec.ID == "" || exec.CreatedAt.IsZero() {
			t.Fatalf("bad execution: %+v", exec)
		}
		if len(exec.Steps) != 3 {
			t.Fatalf("steps: %+v", exec.Steps)
		}
		normal, comp, dependent := exec.Steps[0], exec.Steps[1], exec.Steps[2]
		if normal.Status != StatusPending || normal.MaxAttempts != 1 || normal.Multiplier != 1 || string(normal.Config) != `{"url":"/diamond"}` {
			t.Fatalf("normal: %+v", normal)
		}
		if !comp.IsCompensation || comp.Status != StatusWaiting || comp.Type != "message" || comp.TimeoutSeconds != 7 || comp.MaxAttempts != 4 || len(comp.DependsOn) != 1 || comp.DependsOn[0] != "checkout" {
			t.Fatalf("comp: %+v", comp)
		}
		if dependent.Status != StatusWaiting {
			t.Fatal("dependent not waiting")
		}
		if normal.ID == comp.ID || normal.ID == dependent.ID || normal.WorkflowExecutionID != exec.ID {
			t.Fatal("snapshot identities")
		}
		dependent.DependsOn[0] = "changed"
		var ctx map[string]json.RawMessage
		json.Unmarshal(exec.Context, &ctx)
		if string(ctx["nested"]) != `{"override":true}` {
			t.Fatalf("merge: %s", exec.Context)
		}
		return nil
	}}, snapshotReader{read: func(name string) (*workflow.Workflow, error) {
		if name != "checkout" {
			t.Fatalf("name %q", name)
		}
		return wf, nil
	}})
	_, _, err := logic.ExecuteWorkflow(context.Background(), " checkout ", &ExecuteWorkflowRequest{
		IdempotencyKey: "key-snapshot-1",
		Context:        json.RawMessage(`{"nested":{"override":true}}`),
	})
	if err != nil || !called {
		t.Fatalf("execute: %v", err)
	}
	if wf.Steps[1].DependsOn[0] != "user" || string(wf.Steps[0].Config) != `{"url":"/{{tier}}"}` {
		t.Fatal("mutated workflow definition")
	}
}

func TestLogicExecutionFailures(t *testing.T) {
	dbErr := errors.New("database failure")
	for _, tc := range []struct {
		name              string
		readErr, writeErr error
	}{
		{"missing", errdefs.ErrWorkflowNotFound, nil}, {"read failure", dbErr, nil}, {"write failure", nil, dbErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			logic := NewExecutionLogic(snapshotRepository{create: func(*WorkflowExecution) error { created = true; return tc.writeErr }}, snapshotReader{read: func(string) (*workflow.Workflow, error) { return &workflow.Workflow{ID: "wf"}, tc.readErr }})
			_, _, err := logic.ExecuteWorkflow(context.Background(), "wf", &ExecuteWorkflowRequest{IdempotencyKey: "key-failure-1"})
			want := tc.readErr
			if want == nil {
				want = tc.writeErr
			}
			if !errors.Is(err, want) || (tc.readErr != nil && created) {
				t.Fatalf("error %v created %v", err, created)
			}
		})
	}
}
