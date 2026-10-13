package validator

import (
	"encoding/json"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

type validationError struct {
	s string
}

func (e *validationError) Error() string {
	return e.s
}

func (e *validationError) ValidationError() bool {
	return true
}

func newValidationError(s string) error {
	return &validationError{s: s}
}

var (
	// Workflow
	ErrInvalidWorkflow        = newValidationError("workflow is invalid")
	ErrInvalidWorkflowName    = newValidationError("workflow name is required")
	ErrWorkflowNameTooLong    = newValidationError("workflow name cannot exceed 255 characters")
	ErrInvalidWorkflowVersion = newValidationError("workflow version cannot be negative")
	ErrWorkflowEmptySteps     = newValidationError("workflow must have at least one step")
	ErrInvalidWorkflowID      = newValidationError("workflow id is required")
	ErrInvalidWorkflowContext = newValidationError("workflow context must be a valid JSON object")

	// Step
	ErrEmptyStepType               = newValidationError("step type is required")
	ErrEmptyID                     = newValidationError("step id cannot be empty")
	ErrInvalidStepType             = newValidationError("invalid step type, must be http or message")
	ErrInvalidStepConfig           = newValidationError("step config must be valid non-empty JSON")
	ErrDuplicateStepID             = newValidationError("duplicate step id in workflow")
	ErrDependencyNotFound          = newValidationError("dependency step not found in workflow")
	ErrSelfDependency              = newValidationError("step cannot depend on itself")
	ErrCyclicDependency            = newValidationError("cyclic dependency detected in workflow steps")
	ErrInvalidRetryPolicy          = newValidationError("invalid retry policy")
	ErrInvalidCompensation         = newValidationError("invalid compensation configuration")
	ErrIrreversibleCompensation    = newValidationError("irreversible step cannot define compensation")
	ErrStepTimeoutNegative         = newValidationError("step timeout cannot be negative")
	ErrCompensationTimeoutNegative = newValidationError("compensation timeout cannot be negative")
)

type Validator interface {
	ValidateAndNormalizeWorkflow(workflowReq *workflow.WorkflowRequest) error
	ValidateSteps(steps []step.Step) error
	IsWorkflowEqual(existingSteps []step.Step, existingContext json.RawMessage, req *workflow.WorkflowRequest) bool
}

