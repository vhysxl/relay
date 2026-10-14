package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

var (
	ErrUnsupportedStepType = errors.New("unsupported step type")
	ErrInvalidStepConfig   = errors.New("invalid step configuration")
)

// StepExecutor defines the contract for executing a single workflow step
type StepExecutor interface {
	Execute(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error)
}

// Registry manages and routes step executors by StepType
type Registry interface {
	Register(stepType step.StepType, executor StepExecutor)
	Get(stepType step.StepType) (StepExecutor, error)
}

type registry struct {
	mu        sync.RWMutex
	executors map[step.StepType]StepExecutor
}

// NewRegistry creates a new empty executor registry
func NewRegistry() Registry {
	return &registry{
		executors: make(map[step.StepType]StepExecutor),
	}
}

// Register adds or replaces a StepExecutor for a given StepType
func (r *registry) Register(stepType step.StepType, executor StepExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[stepType] = executor
}

// Get retrieves the StepExecutor registered for the given StepType
func (r *registry) Get(stepType step.StepType) (StepExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exec, ok := r.executors[stepType]
	if !ok || exec == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedStepType, stepType)
	}
	return exec, nil
}
