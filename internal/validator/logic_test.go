package validator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/errdefs"
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
	if err != errdefs.ErrSelfDependency {
		t.Fatalf("expected errdefs.ErrSelfDependency, got %v", err)
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
	if err != errdefs.ErrCyclicDependency {
		t.Fatalf("expected errdefs.ErrCyclicDependency, got %v", err)
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
	if err != errdefs.ErrCyclicDependency {
		t.Fatalf("expected errdefs.ErrCyclicDependency, got %v", err)
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
	if err != errdefs.ErrDependencyNotFound {
		t.Fatalf("expected errdefs.ErrDependencyNotFound, got %v", err)
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
	if err != errdefs.ErrDuplicateStepID {
		t.Fatalf("expected errdefs.ErrDuplicateStepID, got %v", err)
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
	if err != errdefs.ErrInvalidStepType {
		t.Fatalf("expected errdefs.ErrInvalidStepType, got %v", err)
	}

	steps[0].Type = ""
	err = logic.ValidateSteps(steps)
	if err != errdefs.ErrEmptyStepType {
		t.Fatalf("expected errdefs.ErrEmptyStepType, got %v", err)
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
	if err != errdefs.ErrInvalidStepConfig {
		t.Fatalf("expected errdefs.ErrInvalidStepConfig, got %v", err)
	}

	steps[0].Config = nil
	err = logic.ValidateSteps(steps)
	if err != errdefs.ErrInvalidStepConfig {
		t.Fatalf("expected errdefs.ErrInvalidStepConfig, got %v", err)
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
	if err != errdefs.ErrInvalidRetryPolicy {
		t.Fatalf("expected errdefs.ErrInvalidRetryPolicy, got %v", err)
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
	if err != errdefs.ErrInvalidCompensation {
		t.Fatalf("expected errdefs.ErrInvalidCompensation, got %v", err)
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
	if err != errdefs.ErrIrreversibleCompensation {
		t.Fatalf("expected errdefs.ErrIrreversibleCompensation, got %v", err)
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
	if err != errdefs.ErrStepTimeoutNegative {
		t.Fatalf("expected errdefs.ErrStepTimeoutNegative, got %v", err)
	}

	// Negative compensation timeout
	steps[0].Timeout = 5
	steps[0].Compensation = &step.Compensation{
		Type:    step.StepTypeHTTP,
		Config:  json.RawMessage(`{}`),
		Timeout: -1,
	}
	err = logic.ValidateSteps(steps)
	if err != errdefs.ErrCompensationTimeoutNegative {
		t.Fatalf("expected errdefs.ErrCompensationTimeoutNegative, got %v", err)
	}
}

func TestValidateAndNormalizeWorkflow_Validation(t *testing.T) {
	v := NewValidator()

	// Nil request
	if err := v.ValidateAndNormalizeWorkflow(nil); err != errdefs.ErrInvalidWorkflow {
		t.Fatalf("expected errdefs.ErrInvalidWorkflow, got %v", err)
	}

	// Empty name
	req := &workflow.WorkflowRequest{Name: "   "}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrInvalidWorkflowName {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowName, got %v", err)
	}

	// Name too long
	req = &workflow.WorkflowRequest{
		Name:  string(make([]byte, 256)),
		Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrWorkflowNameTooLong {
		t.Fatalf("expected errdefs.ErrWorkflowNameTooLong, got %v", err)
	}

	// Negative version
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Version: -1,
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrInvalidWorkflowVersion {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowVersion, got %v", err)
	}

	// Empty steps
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Version: 1,
		Steps:   nil,
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrWorkflowEmptySteps {
		t.Fatalf("expected errdefs.ErrWorkflowEmptySteps, got %v", err)
	}

	// Empty step ID must return ErrEmptyID
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Version: 1,
		Steps:   []step.Step{{ID: "   ", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrEmptyID {
		t.Fatalf("expected errdefs.ErrEmptyID, got %v", err)
	}

	// Invalid context JSON
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Context: json.RawMessage(`invalid`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrInvalidWorkflowContext {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowContext, got %v", err)
	}

	// Non-object context JSON
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Context: json.RawMessage(`[1, 2, 3]`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrInvalidWorkflowContext {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowContext, got %v", err)
	}

	// Success and normalization
	req = &workflow.WorkflowRequest{
		Name:    "  flow-1  ",
		Version: 0,
		Context: json.RawMessage(`{"key": "val"}`),
		Steps: []step.Step{
			{ID: "dep1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)},
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
	if req.Steps[1].ID != "step-1" {
		t.Errorf("expected step ID to be preserved, got %s", req.Steps[1].ID)
	}
	if req.Steps[1].DependsOn[0] != "dep1" {
		t.Errorf("expected trimmed dependsOn, got %s", req.Steps[1].DependsOn[0])
	}

	// Step validation integrated into ValidateAndNormalizeWorkflow
	req = &workflow.WorkflowRequest{
		Name:    "flow-self-dep",
		Version: 1,
		Steps: []step.Step{
			{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`), DependsOn: []string{"s1"}},
		},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrSelfDependency {
		t.Fatalf("expected errdefs.ErrSelfDependency, got %v", err)
	}
}

func TestValidateSteps_Conditions(t *testing.T) {
	v := NewValidatorWithLimits(3, 10)

	t.Run("valid ancestor reference", func(t *testing.T) {
		steps := []step.Step{
			{ID: "a", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)},
			{
				ID:        "b",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{}`),
				DependsOn: []string{"a"},
				When:      json.RawMessage(`{"ref":"steps.a.output.user.tier","op":"eq","value":"diamond"}`),
			},
		}
		if err := v.ValidateSteps(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("non-ancestor reference returns ErrConditionNonAncestor", func(t *testing.T) {
		steps := []step.Step{
			{ID: "a", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)},
			{ID: "b", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)},
			{
				ID:        "c",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{}`),
				DependsOn: []string{"a"}, // depends on a, but references b (not an ancestor)
				When:      json.RawMessage(`{"ref":"steps.b.output.user.tier","op":"eq","value":"diamond"}`),
			},
		}
		if err := v.ValidateSteps(steps); err != errdefs.ErrConditionNonAncestor {
			t.Fatalf("expected ErrConditionNonAncestor, got %v", err)
		}
	})

	t.Run("ambiguous context anchor colliding with step ID returns ErrConditionAmbiguousAnchor", func(t *testing.T) {
		steps := []step.Step{
			{ID: "user", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`)},
			{
				ID:        "b",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{}`),
				DependsOn: []string{"user"},
				When:      json.RawMessage(`{"ref":"user.tier","op":"eq","value":"diamond"}`),
			},
		}
		if err := v.ValidateSteps(steps); err != errdefs.ErrConditionAmbiguousAnchor {
			t.Fatalf("expected ErrConditionAmbiguousAnchor, got %v", err)
		}
	})

	t.Run("condition exceeding max depth returns ErrConditionMaxDepth", func(t *testing.T) {
		steps := []step.Step{
			{
				ID:     "a",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{}`),
				When:   json.RawMessage(`{"all":[{"all":[{"all":[{"ref":"context.flag","op":"exists"}]}]}]}`), // depth 4 > 3
			},
		}
		if err := v.ValidateSteps(steps); err != errdefs.ErrConditionMaxDepth {
			t.Fatalf("expected ErrConditionMaxDepth, got %v", err)
		}
	})

	t.Run("long chain without when avoids ancestor computation overhead", func(t *testing.T) {
		const chainLen = 300
		steps := make([]step.Step, chainLen)
		for i := 0; i < chainLen; i++ {
			s := step.Step{
				ID:     fmt.Sprintf("s%d", i),
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{}`),
			}
			if i > 0 {
				s.DependsOn = []string{fmt.Sprintf("s%d", i-1)}
			}
			steps[i] = s
		}
		if err := v.ValidateSteps(steps); err != nil {
			t.Fatalf("unexpected error for long chain: %v", err)
		}
	})
}

func TestIsWorkflowEqual_WhenCondition(t *testing.T) {
	v := NewValidator()
	existing := []step.Step{
		{
			ID:     "a",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{}`),
			When:   json.RawMessage(`{"ref":"flag","op":"eq","value":"true"}`),
		},
	}

	reqIdentical := &workflow.WorkflowRequest{
		Steps: []step.Step{
			{
				ID:     "a",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{}`),
				When:   json.RawMessage(`{"ref":"flag","op":"eq","value":"true"}`),
			},
		},
	}
	if !v.IsWorkflowEqual(existing, nil, reqIdentical) {
		t.Fatal("expected identical condition to be equal")
	}

	reqChanged := &workflow.WorkflowRequest{
		Steps: []step.Step{
			{
				ID:     "a",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{}`),
				When:   json.RawMessage(`{"ref":"flag","op":"eq","value":"false"}`),
			},
		},
	}
	if v.IsWorkflowEqual(existing, nil, reqChanged) {
		t.Fatal("expected changed condition to not be equal")
	}
}

func TestIsWorkflowEqual_WhenCondition_LargeNumbers(t *testing.T) {
	v := NewValidator()
	existing := []step.Step{
		{
			ID:     "a",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{}`),
			When:   json.RawMessage(`{"ref":"val","op":"eq","value":9007199254740992}`),
		},
	}

	reqChanged := &workflow.WorkflowRequest{
		Steps: []step.Step{
			{
				ID:     "a",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{}`),
				When:   json.RawMessage(`{"ref":"val","op":"eq","value":9007199254740993}`),
			},
		},
	}
	if v.IsWorkflowEqual(existing, nil, reqChanged) {
		t.Fatal("expected changing 9007199254740992 to 9007199254740993 in when to not be equal")
	}
}

func TestValidateSteps_ActivityType_Success(t *testing.T) {
	v := NewValidator()
	steps := []step.Step{
		{
			ID:     "act-1",
			Type:   step.StepTypeActivity,
			Config: json.RawMessage(`{"queue":"user-service","activity":"user.finalizeProfile","input":{"id":123}}`),
		},
	}
	if err := v.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid activity step, got: %v", err)
	}
}

func TestValidateSteps_ActivityType_Validation(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		config  string
		wantErr error
	}{
		{
			name:    "missing queue",
			config:  `{"activity":"user.finalizeProfile"}`,
			wantErr: errdefs.ErrEmptyQueueName,
		},
		{
			name:    "empty queue",
			config:  `{"queue":"","activity":"user.finalizeProfile"}`,
			wantErr: errdefs.ErrEmptyQueueName,
		},
		{
			name:    "invalid queue characters (template not allowed)",
			config:  `{"queue":"user-{{context.service}}","activity":"user.finalizeProfile"}`,
			wantErr: errdefs.ErrInvalidQueueName,
		},
		{
			name:    "queue too long (> 128 chars)",
			config:  fmt.Sprintf(`{"queue":"%s","activity":"user.finalizeProfile"}`, strings.Repeat("a", 129)),
			wantErr: errdefs.ErrInvalidQueueName,
		},
		{
			name:    "missing activity",
			config:  `{"queue":"user-service"}`,
			wantErr: errdefs.ErrEmptyActivityName,
		},
		{
			name:    "empty activity",
			config:  `{"queue":"user-service","activity":"   "}`,
			wantErr: errdefs.ErrEmptyActivityName,
		},
		{
			name:    "invalid activity characters",
			config:  `{"queue":"user-service","activity":"user finalize"}`,
			wantErr: errdefs.ErrInvalidActivityName,
		},
		{
			name:    "activity too long (> 128 chars)",
			config:  fmt.Sprintf(`{"queue":"user-service","activity":"%s"}`, strings.Repeat("b", 129)),
			wantErr: errdefs.ErrInvalidActivityName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := []step.Step{
				{
					ID:     "act-1",
					Type:   step.StepTypeActivity,
					Config: json.RawMessage(tt.config),
				},
			}
			err := v.ValidateSteps(steps)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateSteps_HTTPAndMessageQueueValidation(t *testing.T) {
	v := NewValidator()

	// HTTP with valid optional queue
	steps := []step.Step{
		{
			ID:     "http-1",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{"url":"https://example.com","queue":"custom-http-queue"}`),
		},
	}
	if err := v.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid HTTP step with queue, got %v", err)
	}

	// HTTP with invalid queue
	steps[0].Config = json.RawMessage(`{"url":"https://example.com","queue":"invalid queue name!"}`)
	if err := v.ValidateSteps(steps); !errors.Is(err, errdefs.ErrInvalidQueueName) {
		t.Fatalf("expected ErrInvalidQueueName, got %v", err)
	}

	// Message with valid optional queue
	steps[0].Type = step.StepTypeMessage
	steps[0].Config = json.RawMessage(`{"exchange":"ex","queue":"payment-queue"}`)
	if err := v.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid Message step with queue, got %v", err)
	}
}

func TestValidateSteps_Compensation_Activity(t *testing.T) {
	v := NewValidator()

	// Valid activity compensation
	steps := []step.Step{
		{
			ID:     "s1",
			Type:   step.StepTypeHTTP,
			Config: json.RawMessage(`{"url":"https://example.com"}`),
			Compensation: &step.Compensation{
				Type:   step.StepTypeActivity,
				Config: json.RawMessage(`{"queue":"comp-queue","activity":"rollbackAction"}`),
			},
		},
	}
	if err := v.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid activity compensation, got %v", err)
	}

	// Invalid activity compensation (missing queue)
	steps[0].Compensation.Config = json.RawMessage(`{"activity":"rollbackAction"}`)
	if err := v.ValidateSteps(steps); !errors.Is(err, errdefs.ErrInvalidCompensation) {
		t.Fatalf("expected ErrInvalidCompensation, got %v", err)
	}
}
