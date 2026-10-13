package validator

import (
	"encoding/json"
	"testing"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

func TestValidateSteps_Empty(t *testing.T) {
	logic := NewValidator()
	if err := logic.ValidateSteps(nil); err != nil {
		t.Fatalf("expected nil for empty steps, got %v", err)
	}
	if err := logic.ValidateSteps([]step.Step{}); err != nil {
		t.Fatalf("expected nil for empty steps, got %v", err)
	}
}

func TestValidateSteps_LinearDAG(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:     "step-1",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{"url":"https://example.com"}`),
		},
		{
			ID:        "step-2",
			Type:      step.StepTypeHTTP,
			Config:    json.RawMessage(`{"url":"https://example.com"}`),
			DependsOn: []string{"step-1"},
		},
		{
			ID:        "step-3",
			Type:      step.StepTypeMessage,
			Config:    json.RawMessage(`{"exchange":"events"}`),
			DependsOn: []string{"step-2"},
		},
	}

	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid linear DAG, got %v", err)
	}
}

func TestValidateSteps_BranchingDAG(t *testing.T) {
	logic := NewValidator()
	// Diamond: A -> B, A -> C, B+C -> D
	steps := []step.Step{
		{
			ID:     "step-a",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{}`),
		},
		{
			ID:        "step-b",
			Type:      step.StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-a"},
		},
		{
			ID:        "step-c",
			Type:      step.StepTypeMessage,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-a"},
		},
		{
			ID:        "step-d",
			Type:      step.StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-b", "step-c"},
		},
	}

	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid diamond DAG, got %v", err)
	}
}

func TestValidateSteps_SelfDependency(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:        "step-1",
			Type:      step.StepTypeHTTP,
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
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:        "step-1",
			Type:      step.StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-2"},
		},
		{
			ID:        "step-2",
			Type:      step.StepTypeHTTP,
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
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:        "step-1",
			Type:      step.StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-3"},
		},
		{
			ID:        "step-2",
			Type:      step.StepTypeHTTP,
			Config:    json.RawMessage(`{}`),
			DependsOn: []string{"step-1"},
		},
		{
			ID:        "step-3",
			Type:      step.StepTypeHTTP,
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
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:        "step-1",
			Type:      step.StepTypeHTTP,
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
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:     "step-1",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{}`),
		},
		{
			ID:     "step-1",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{}`),
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrDuplicateStepID {
		t.Fatalf("expected ErrDuplicateStepID, got %v", err)
	}
}

func TestValidateSteps_InvalidType(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
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
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:     "step-1",
			Type:   step.StepTypeHTTP,
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
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:     "step-1",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{}`),
			Retry: &step.RetryPolicy{
				MaxAttempts: 0,
			},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrInvalidRetryPolicy {
		t.Fatalf("expected ErrInvalidRetryPolicy, got %v", err)
	}

	// Valid retry
	steps[0].Retry = &step.RetryPolicy{
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
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:     "step-1",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{}`),
			Compensation: &step.Compensation{
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
	steps[0].Compensation = &step.Compensation{
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"url":"https://example.com/rollback"}`),
	}
	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid compensation, got %v", err)
	}
}

func TestValidateSteps_IrreversibleWithCompensation(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:           "step-1",
			Type:         step.StepTypeHTTP,
			Config:       json.RawMessage(`{"url":"https://example.com"}`),
			Irreversible: true,
			Compensation: &step.Compensation{
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://example.com/rollback"}`),
			},
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrIrreversibleCompensation {
		t.Fatalf("expected ErrIrreversibleCompensation, got %v", err)
	}

	// Valid irreversible step (without compensation)
	steps[0].Compensation = nil
	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid irreversible step without compensation, got %v", err)
	}
}

func TestValidateSteps_NegativeTimeout(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID:      "step-1",
			Type:    step.StepTypeHTTP,
			Config:  json.RawMessage(`{}`),
			Timeout: -5,
		},
	}

	err := logic.ValidateSteps(steps)
	if err != ErrStepTimeoutNegative {
		t.Fatalf("expected ErrStepTimeoutNegative, got %v", err)
	}

	// Negative compensation timeout
	steps[0].Timeout = 5
	steps[0].Compensation = &step.Compensation{
		Type:    step.StepTypeHTTP,
		Config:  json.RawMessage(`{}`),
		Timeout: -1,
	}
	err = logic.ValidateSteps(steps)
	if err != ErrCompensationTimeoutNegative {
		t.Fatalf("expected ErrCompensationTimeoutNegative, got %v", err)
	}
}

func TestValidateAndNormalizeWorkflow_Validation(t *testing.T) {
	v := NewValidator()

	// Nil request
	if err := v.ValidateAndNormalizeWorkflow(nil); err != ErrInvalidWorkflowName {
		t.Fatalf("expected ErrInvalidWorkflowName, got %v", err)
	}

	// Empty name
	req := &workflow.WorkflowRequest{Name: "   "}
	if err := v.ValidateAndNormalizeWorkflow(req); err != ErrInvalidWorkflowName {
		t.Fatalf("expected ErrInvalidWorkflowName, got %v", err)
	}

	// Name too long
	req = &workflow.WorkflowRequest{
		Name:  string(make([]byte, 256)),
		Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != ErrWorkflowNameTooLong {
		t.Fatalf("expected ErrWorkflowNameTooLong, got %v", err)
	}

	// Negative version
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Version: -1,
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != ErrInvalidWorkflowVersion {
		t.Fatalf("expected ErrInvalidWorkflowVersion, got %v", err)
	}

	// Empty steps
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Version: 1,
		Steps:   nil,
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != ErrWorkflowEmptySteps {
		t.Fatalf("expected ErrWorkflowEmptySteps, got %v", err)
	}

	// Empty step ID must return ErrEmptyID
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Version: 1,
		Steps:   []step.Step{{ID: "   ", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != ErrEmptyID {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}

	// Invalid context JSON
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Context: json.RawMessage(`invalid`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != ErrInvalidWorkflowContext {
		t.Fatalf("expected ErrInvalidWorkflowContext, got %v", err)
	}

	// Non-object context JSON
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Context: json.RawMessage(`[1, 2, 3]`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != ErrInvalidWorkflowContext {
		t.Fatalf("expected ErrInvalidWorkflowContext, got %v", err)
	}

	// Success and normalization
	req = &workflow.WorkflowRequest{
		Name:    "  flow-1  ",
		Version: 0,
		Context: json.RawMessage(`{"key": "val"}`),
		Steps: []step.Step{
			{ID: "step-1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`), DependsOn: []string{" dep1 "}},
		},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Name != "flow-1" {
		t.Errorf("expected trimmed name, got %s", req.Name)
	}
	if req.Version != 1 {
		t.Errorf("expected default version 1, got %d", req.Version)
	}
	if req.Steps[0].ID != "step-1" {
		t.Errorf("expected step ID to be preserved, got %s", req.Steps[0].ID)
	}
	if req.Steps[0].DependsOn[0] != "dep1" {
		t.Errorf("expected trimmed dependsOn, got %s", req.Steps[0].DependsOn[0])
	}
}

