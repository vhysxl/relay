package executor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

type dummyExecutor struct{}

func (d *dummyExecutor) Execute(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error) {
	return json.RawMessage(`{"dummy":true}`), nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()

	dummy := &dummyExecutor{}
	reg.Register(step.StepTypeHTTP, dummy)

	// Found
	exec, err := reg.Get(step.StepTypeHTTP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec != dummy {
		t.Errorf("expected dummy executor, got %v", exec)
	}

	// Not found
	_, err = reg.Get(step.StepTypeMessage)
	if !errors.Is(err, ErrUnsupportedStepType) {
		t.Fatalf("expected ErrUnsupportedStepType for unregistered type, got %v", err)
	}
}
