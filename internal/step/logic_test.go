package step

import (
	"encoding/json"
	"testing"
)

func TestValidateSteps_Empty(t *testing.T) {
	logic := NewStepValidator()
	if err := logic.ValidateSteps(nil); err != nil {
		t.Fatalf("expected nil for empty steps, got %v", err)
	}
	if err := logic.ValidateSteps([]Step{}); err != nil {
		t.Fatalf("expected nil for empty steps, got %v", err)
	}
}

func TestValidateSteps_LinearDAG(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:     "step-1",
			Type:   StepTypeHTTP,
			Config: json.RawMessage(`{"url":"https://example.com"}`),
		},
		{
			ID:        "step-2",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{"url":"https://example.com"}`),
			DependsOn: []string{"step-1"},
		},
		{
			ID:        "step-3",
			Type:      StepTypeMessage,
			Config:    json.RawMessage(`{"exchange":"events"}`),
			DependsOn: []string{"step-2"},
		},
	}

	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid linear DAG, got %v", err)
	}
}

func TestValidateSteps_BranchingDAG(t *testing.T) {
	logic := NewStepValidator()
	// Diamond: A -> B, A -> C, B+C -> D
	steps := []Step{
		{
			ID:     "step-a",
			Type:   StepTypeHTTP,
			Config: json.RawMessage(`{}`),
		},
		{
			ID:        "step-b",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-a"},
		},
		{
			ID:        "step-c",
			Type:      StepTypeMessage,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-a"},
		},
		{
			ID:        "step-d",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-b", "step-c"},
		},
	}

	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid diamond DAG, got %v", err)
	}
}

func TestValidateSteps_SelfDependency(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:        "step-1",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-1"},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrSelfDependency {
		t.Fatalf("expected ErrSelfDependency, got %v", err)
	}
}

func TestValidateSteps_CyclicDependency_Direct(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:        "step-1",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-2"},
		},
		{
			ID:        "step-2",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-1"},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrCyclicDependency {
		t.Fatalf("expected ErrCyclicDependency, got %v", err)
	}
}

func TestValidateSteps_CyclicDependency_Indirect(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:        "step-1",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-3"},
		},
		{
			ID:        "step-2",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-1"},
		},
		{
			ID:        "step-3",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-2"},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrCyclicDependency {
		t.Fatalf("expected ErrCyclicDependency, got %v", err)
	}
}

func TestValidateSteps_MissingDependency(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:        "step-1",
			Type:      StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"non-existent-step"},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrDependencyNotFound {
		t.Fatalf("expected ErrDependencyNotFound, got %v", err)
	}
}

func TestValidateSteps_DuplicateID(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:     "step-1",
			Type:   StepTypeHTTP,
			Config: json.RawMessage(`{}`),
		},
		{
			ID:     "step-1",
			Type:   StepTypeHTTP,
			Config: json.RawMessage(`{}`),
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrDuplicateStepID {
		t.Fatalf("expected ErrDuplicateStepID, got %v", err)
	}
}

func TestValidateSteps_InvalidType(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:     "step-1",
			Type:   "unknown_type",
			Config: json.RawMessage(`{}`),
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrInvalidStepType {
		t.Fatalf("expected ErrInvalidStepType, got %v", err)
	}

	steps[0].Type = ""
	err = logic.ValidateSteps(steps)
	if err != ErrEmptyStepType {
		t.Fatalf("expected ErrEmptyStepType, got %v", err)
	}
}

func TestValidateSteps_InvalidConfig(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:     "step-1",
			Type:   StepTypeHTTP,
			Config: json.RawMessage(`invalid-json`),
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrInvalidStepConfig {
		t.Fatalf("expected ErrInvalidStepConfig, got %v", err)
	}

	steps[0].Config = nil
	err = logic.ValidateSteps(steps)
	if err != ErrInvalidStepConfig {
		t.Fatalf("expected ErrInvalidStepConfig, got %v", err)
	}
}

func TestValidateSteps_RetryPolicy(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:     "step-1",
			Type:   StepTypeHTTP,
			Config: json.RawMessage(`{}`),
			Retry: &RetryPolicy{
				MaxAttempts: 0,
			},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrInvalidRetryPolicy {
		t.Fatalf("expected ErrInvalidRetryPolicy, got %v", err)
	}

	// Valid retry
	steps[0].Retry = &RetryPolicy{
		MaxAttempts: 3,
		Backoff:     2,
		Multiplier:  2.0,
		MaxBackoff:  10,
	}
	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid retry policy, got %v", err)
	}
}

func TestValidateSteps_Compensation(t *testing.T) {
	logic := NewStepValidator()
	steps := []Step{
		{
			ID:     "step-1",
			Type:   StepTypeHTTP,
			Config: json.RawMessage(`{}`),
			Compensation: &Compensation{
				Type:   "invalid",
				Config: json.RawMessage(`{}`),
			},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrInvalidCompensation {
		t.Fatalf("expected ErrInvalidCompensation, got %v", err)
	}

	// Valid compensation
	steps[0].Compensation = &Compensation{
		Type:   StepTypeHTTP,
		Config: json.RawMessage(`{"url":"https://example.com/rollback"}`),
	}
	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid compensation, got %v", err)
	}
}
