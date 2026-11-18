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
			ID: "step-1", Queue: "orders",
			Input: json.RawMessage("{\"url\":\"https://example.com\"}"), Activity: "http.call"},
		{
			ID: "step-2", Queue: "orders",
			Input:     json.RawMessage("{\"url\":\"https://example.com\"}"),
			DependsOn: []string{"step-1"}, Activity: "http.call"},
		{
			ID: "step-3", Queue: "orders",
			Input:     json.RawMessage("{\"exchange\":\"events\"}"),
			DependsOn: []string{"step-2"}, Activity: "message.send"},
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
			ID: "step-a", Queue: "orders",
			Input: json.RawMessage("{}"), Activity: "http.call"},
		{
			ID: "step-b", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-a"}, Activity: "http.call"},
		{
			ID: "step-c", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-a"}, Activity: "message.send"},
		{
			ID: "step-d", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-b", "step-c"}, Activity: "http.call"},
	}

	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid diamond DAG, got %v", err)
	}
}

func TestValidateSteps_SelfDependency(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID: "step-1", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-1"}, Activity: "http.call"},
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
			ID: "step-1", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-2"}, Activity: "http.call"},
		{
			ID: "step-2", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-1"}, Activity: "http.call"},
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
			ID: "step-1", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-3"}, Activity: "http.call"},
		{
			ID: "step-2", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-1"}, Activity: "http.call"},
		{
			ID: "step-3", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"step-2"}, Activity: "http.call"},
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
			ID: "step-1", Queue: "orders",
			Input:     json.RawMessage("{}"),
			DependsOn: []string{"non-existent-step"}, Activity: "http.call"},
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
			ID: "step-1", Queue: "orders",
			Input: json.RawMessage("{}"), Activity: "http.call"},
		{
			ID: "step-1", Queue: "orders",
			Input: json.RawMessage("{}"), Activity: "http.call"},
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
			ID: "step-1", Queue: "orders",
			Input: json.RawMessage("{}"), Activity: "invalid activity!"},
	}

	err := logic.ValidateSteps(steps)
	if err != errdefs.ErrInvalidActivity {
		t.Fatalf("expected errdefs.ErrInvalidActivity, got %v", err)
	}

	steps[0].Activity = ""
	err = logic.ValidateSteps(steps)
	if err != errdefs.ErrEmptyActivity {
		t.Fatalf("expected errdefs.ErrEmptyActivity, got %v", err)
	}
}

func TestValidateSteps_InvalidConfig(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID: "step-1", Queue: "orders",
			Input: json.RawMessage(`invalid-json`), Activity: "http.call"},
	}

	err := logic.ValidateSteps(steps)
	if err != errdefs.ErrInvalidStepInput {
		t.Fatalf("expected errdefs.ErrInvalidStepInput, got %v", err)
	}

	steps[0].Input = json.RawMessage(`{broken`)
	err = logic.ValidateSteps(steps)
	if err != errdefs.ErrInvalidStepInput {
		t.Fatalf("expected errdefs.ErrInvalidStepInput, got %v", err)
	}
}

func TestValidateSteps_RetryPolicy(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID: "step-1", Queue: "orders",
			Input: json.RawMessage("{}"),
			Retry: &step.RetryPolicy{
				MaxAttempts: 0,
			}, Activity: "http.call"},
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
			ID: "step-1", Queue: "orders",
			Input: json.RawMessage("{}"),
			Compensation: &step.Compensation{Queue: "orders",
				Input: json.RawMessage("{}"), Activity: "invalid activity!"}, Activity: "http.call"},
	}

	err := logic.ValidateSteps(steps)
	if !errors.Is(err, errdefs.ErrInvalidCompensation) {
		t.Fatalf("expected errdefs.ErrInvalidCompensation, got %v", err)
	}

	// Valid compensation
	steps[0].Compensation = &step.Compensation{Queue: "orders",
		Input: json.RawMessage("{\"url\":\"https://example.com/rollback\"}"), Activity: "http.call"}
	if err := logic.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid compensation, got %v", err)
	}
}

func TestValidateSteps_IrreversibleWithCompensation(t *testing.T) {
	logic := NewValidator()
	steps := []step.Step{
		{
			ID: "step-1", Queue: "orders",
			Input:        json.RawMessage("{\"url\":\"https://example.com\"}"),
			Irreversible: true,
			Compensation: &step.Compensation{Queue: "orders",
				Input: json.RawMessage("{\"url\":\"https://example.com/rollback\"}"), Activity: "http.call"}, Activity: "http.call"},
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
			ID: "step-1", Queue: "orders",
			Input:   json.RawMessage("{}"),
			Timeout: -5, Activity: "http.call"},
	}

	err := logic.ValidateSteps(steps)
	if err != errdefs.ErrStepTimeoutNegative {
		t.Fatalf("expected errdefs.ErrStepTimeoutNegative, got %v", err)
	}

	// Negative compensation timeout
	steps[0].Timeout = 5
	steps[0].Compensation = &step.Compensation{Queue: "orders",
		Input:   json.RawMessage("{}"),
		Timeout: -1, Activity: "http.call"}
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
		Steps: []step.Step{{ID: "s1", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrWorkflowNameTooLong {
		t.Fatalf("expected errdefs.ErrWorkflowNameTooLong, got %v", err)
	}

	// Negative version
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Version: -1,
		Steps:   []step.Step{{ID: "s1", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"}},
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
		Steps:   []step.Step{{ID: "   ", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrEmptyID {
		t.Fatalf("expected errdefs.ErrEmptyID, got %v", err)
	}

	// Invalid context JSON
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Context: json.RawMessage(`invalid`),
		Steps:   []step.Step{{ID: "s1", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"}},
	}
	if err := v.ValidateAndNormalizeWorkflow(req); err != errdefs.ErrInvalidWorkflowContext {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowContext, got %v", err)
	}

	// Non-object context JSON
	req = &workflow.WorkflowRequest{
		Name:    "valid-name",
		Context: json.RawMessage(`[1, 2, 3]`),
		Steps:   []step.Step{{ID: "s1", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"}},
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
			{ID: "dep1", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"},
			{ID: "step-1", Queue: "orders", Input: json.RawMessage("{}"), DependsOn: []string{" dep1 "}, Activity: "http.call"},
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
			{ID: "s1", Queue: "orders", Input: json.RawMessage("{}"), DependsOn: []string{"s1"}, Activity: "http.call"},
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
			{ID: "a", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"},
			{
				ID: "b", Queue: "orders",
				Input:     json.RawMessage("{}"),
				DependsOn: []string{"a"},
				When:      json.RawMessage(`{"ref":"steps.a.output.user.tier","op":"eq","value":"diamond"}`), Activity: "http.call"},
		}
		if err := v.ValidateSteps(steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("non-ancestor reference returns ErrConditionNonAncestor", func(t *testing.T) {
		steps := []step.Step{
			{ID: "a", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"},
			{ID: "b", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"},
			{
				ID: "c", Queue: "orders",
				Input:     json.RawMessage("{}"),
				DependsOn: []string{"a"}, // depends on a, but references b (not an ancestor)
				When:      json.RawMessage(`{"ref":"steps.b.output.user.tier","op":"eq","value":"diamond"}`), Activity: "http.call"},
		}
		if err := v.ValidateSteps(steps); err != errdefs.ErrConditionNonAncestor {
			t.Fatalf("expected ErrConditionNonAncestor, got %v", err)
		}
	})

	t.Run("ambiguous context anchor colliding with step ID returns ErrConditionAmbiguousAnchor", func(t *testing.T) {
		steps := []step.Step{
			{ID: "user", Queue: "orders", Input: json.RawMessage("{}"), Activity: "http.call"},
			{
				ID: "b", Queue: "orders",
				Input:     json.RawMessage("{}"),
				DependsOn: []string{"user"},
				When:      json.RawMessage(`{"ref":"user.tier","op":"eq","value":"diamond"}`), Activity: "http.call"},
		}
		if err := v.ValidateSteps(steps); err != errdefs.ErrConditionAmbiguousAnchor {
			t.Fatalf("expected ErrConditionAmbiguousAnchor, got %v", err)
		}
	})

	t.Run("condition exceeding max depth returns ErrConditionMaxDepth", func(t *testing.T) {
		steps := []step.Step{
			{
				ID: "a", Queue: "orders",
				Input:    json.RawMessage("{}"),
				When:     json.RawMessage(`{"all":[{"all":[{"all":[{"ref":"context.flag","op":"exists"}]}]}]}`), // depth 4 > 3
				Activity: "http.call"},
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
				ID: fmt.Sprintf("s%d", i), Queue: "orders",
				Input: json.RawMessage("{}"), Activity: "http.call"}
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
			ID: "a", Queue: "orders",
			Input: json.RawMessage("{}"),
			When:  json.RawMessage(`{"ref":"flag","op":"eq","value":"true"}`), Activity: "http.call"},
	}

	reqIdentical := &workflow.WorkflowRequest{
		Steps: []step.Step{
			{
				ID: "a", Queue: "orders",
				Input: json.RawMessage("{}"),
				When:  json.RawMessage(`{"ref":"flag","op":"eq","value":"true"}`), Activity: "http.call"},
		},
	}
	if !v.IsWorkflowEqual(existing, nil, reqIdentical) {
		t.Fatal("expected identical condition to be equal")
	}

	reqChanged := &workflow.WorkflowRequest{
		Steps: []step.Step{
			{
				ID: "a", Queue: "orders",
				Input: json.RawMessage("{}"),
				When:  json.RawMessage(`{"ref":"flag","op":"eq","value":"false"}`), Activity: "http.call"},
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
			ID: "a", Queue: "orders",
			Input: json.RawMessage("{}"),
			When:  json.RawMessage(`{"ref":"val","op":"eq","value":9007199254740992}`), Activity: "http.call"},
	}

	reqChanged := &workflow.WorkflowRequest{
		Steps: []step.Step{
			{
				ID: "a", Queue: "orders",
				Input: json.RawMessage("{}"),
				When:  json.RawMessage(`{"ref":"val","op":"eq","value":9007199254740993}`), Activity: "http.call"},
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
			ID: "act-1", Queue: "user-service",
			Input: json.RawMessage("{\"id\":123}"), Activity: "user.finalizeProfile"},
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
			wantErr: errdefs.ErrEmptyActivity,
		},
		{
			name:    "empty activity",
			config:  `{"queue":"user-service","activity":"   "}`,
			wantErr: errdefs.ErrEmptyActivity,
		},
		{
			name:    "invalid activity characters",
			config:  `{"queue":"user-service","activity":"user finalize"}`,
			wantErr: errdefs.ErrInvalidActivity,
		},
		{
			name:    "activity too long (> 128 chars)",
			config:  fmt.Sprintf(`{"queue":"user-service","activity":"%s"}`, strings.Repeat("b", 129)),
			wantErr: errdefs.ErrInvalidActivity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := []step.Step{
				{
					ID: "act-1"},
			}
			if err := json.Unmarshal([]byte(tt.config), &steps[0]); err != nil {
				t.Fatal(err)
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
			ID: "http-1", Queue: "custom-http-queue",
			Input: json.RawMessage("{\"url\":\"https://example.com\"}"), Activity: "http.call"},
	}
	if err := v.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid HTTP step with queue, got %v", err)
	}

	// HTTP with invalid queue
	steps[0].Queue = "invalid queue name!"
	if err := v.ValidateSteps(steps); !errors.Is(err, errdefs.ErrInvalidQueueName) {
		t.Fatalf("expected ErrInvalidQueueName, got %v", err)
	}

	// Message with valid optional queue
	steps[0].Activity = "message.send"
	steps[0].Queue = "payment-queue"
	if err := v.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid Message step with queue, got %v", err)
	}
}

func TestValidateSteps_Compensation_Activity(t *testing.T) {
	v := NewValidator()

	// Valid activity compensation
	steps := []step.Step{
		{
			ID: "s1", Queue: "orders",
			Input: json.RawMessage("{\"url\":\"https://example.com\"}"),
			Compensation: &step.Compensation{Queue: "comp-queue",
				Input: json.RawMessage("{}"), Activity: "rollbackAction"}, Activity: "http.call"},
	}
	if err := v.ValidateSteps(steps); err != nil {
		t.Fatalf("expected valid activity compensation, got %v", err)
	}

	// Invalid activity compensation (missing queue)
	steps[0].Compensation.Queue = ""
	if err := v.ValidateSteps(steps); !errors.Is(err, errdefs.ErrInvalidCompensation) {
		t.Fatalf("expected ErrInvalidCompensation, got %v", err)
	}
}
