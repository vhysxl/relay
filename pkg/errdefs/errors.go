package errdefs

import "errors"

var (
	// Workflow definitions and requests
	ErrInvalidWorkflow        = errors.New("workflow is invalid")
	ErrInvalidWorkflowName    = errors.New("workflow name is required")
	ErrWorkflowNameTooLong    = errors.New("workflow name cannot exceed 255 characters")
	ErrInvalidWorkflowVersion = errors.New("workflow version cannot be negative")
	ErrWorkflowEmptySteps     = errors.New("workflow must have at least one step")
	ErrInvalidWorkflowContext = errors.New("workflow context must be a valid JSON object")
	ErrWorkflowNotFound       = errors.New("workflow not found")
	ErrWorkflowAlreadyExists  = errors.New("workflow with this name and version already exists")
	ErrPayloadTooLarge        = errors.New("request payload exceeds maximum allowed size")

	// Step definitions and DAG validation
	ErrEmptyStepType               = errors.New("step type is required")
	ErrEmptyID                     = errors.New("step id cannot be empty")
	ErrInvalidStepType             = errors.New("invalid step type, must be http or message")
	ErrInvalidStepConfig           = errors.New("step config must be valid non-empty JSON")
	ErrDuplicateStepID             = errors.New("duplicate step id in workflow")
	ErrDependencyNotFound          = errors.New("dependency step not found in workflow")
	ErrSelfDependency              = errors.New("step cannot depend on itself")
	ErrCyclicDependency            = errors.New("cyclic dependency detected in workflow steps")
	ErrInvalidRetryPolicy          = errors.New("invalid retry policy")
	ErrInvalidCompensation         = errors.New("invalid compensation configuration")
	ErrIrreversibleCompensation    = errors.New("irreversible step cannot define compensation")
	ErrStepTimeoutNegative         = errors.New("step timeout cannot be negative")
	ErrCompensationTimeoutNegative = errors.New("compensation timeout cannot be negative")

	// Step execution & runtime
	ErrStepNotFound = errors.New("step not found")
	ErrLockLost     = errors.New("heartbeat failed: step execution lock lost or not running")

	// Workflow Execution
	ErrExecutionNotFound = errors.New("execution not found")
	ErrEmptyExecutionID  = errors.New("execution id is required")

	// Worker & task claiming
	ErrEmptyWorkerID        = errors.New("worker id is required to claim step")
	ErrEmptyStepExecutionID = errors.New("step execution id is required for heartbeat")
	ErrInvalidStepOutput    = errors.New("step output must be valid JSON")

	// Config / startup
	ErrInvalidEnvValue = errors.New("invalid environment variable value")
)
