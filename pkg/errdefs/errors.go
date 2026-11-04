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
	ErrInvalidStepType             = errors.New("invalid step type, must be http, message, or activity")
	ErrInvalidStepConfig           = errors.New("step config must be valid non-empty JSON")
	ErrInvalidQueueName            = errors.New("queue name is invalid: must match [A-Za-z0-9._-]+ and be 1-128 characters")
	ErrInvalidActivityName         = errors.New("activity name is invalid: must match [A-Za-z0-9._-]+ and be 1-128 characters")
	ErrEmptyQueueName              = errors.New("queue name cannot be empty for activity step")
	ErrEmptyActivityName           = errors.New("activity name cannot be empty for activity step")
	ErrDuplicateStepID             = errors.New("duplicate step id in workflow")
	ErrDependencyNotFound          = errors.New("dependency step not found in workflow")
	ErrSelfDependency              = errors.New("step cannot depend on itself")
	ErrCyclicDependency            = errors.New("cyclic dependency detected in workflow steps")
	ErrInvalidRetryPolicy          = errors.New("invalid retry policy")
	ErrInvalidCompensation         = errors.New("invalid compensation configuration")
	ErrIrreversibleCompensation    = errors.New("irreversible step cannot define compensation")
	ErrStepTimeoutNegative         = errors.New("step timeout cannot be negative")
	ErrCompensationTimeoutNegative = errors.New("compensation timeout cannot be negative")
	ErrInvalidCondition            = errors.New("invalid step condition")
	ErrConditionMaxDepth           = errors.New("condition exceeds maximum depth")
	ErrConditionMaxNodes           = errors.New("condition exceeds maximum nodes")
	ErrConditionNonAncestor        = errors.New("condition references non-ancestor step output")
	ErrConditionAmbiguousAnchor    = errors.New("condition context anchor collides with step id")

	// Step execution & runtime
	ErrStepNotFound = errors.New("step not found")
	ErrLockLost     = errors.New("heartbeat failed: step execution lock lost or not running")

	// Workflow Execution
	ErrExecutionNotFound        = errors.New("execution not found")
	ErrEmptyExecutionID         = errors.New("execution id is required")
	ErrIdempotencyKeyRequired   = errors.New("idempotency key is required")
	ErrInvalidIdempotencyKey    = errors.New("idempotency key is invalid")
	ErrIdempotencyConflict      = errors.New("idempotency key conflict: request payload or version mismatch")
	ErrExecutionRequestNotFound = errors.New("execution request not found")

	// Worker & task claiming
	ErrEmptyWorkerID        = errors.New("worker id is required to claim step")
	ErrEmptyStepExecutionID = errors.New("step execution id is required for heartbeat")
	ErrInvalidStepOutput    = errors.New("step output must be valid JSON")
	ErrUnauthorized         = errors.New("unauthorized: missing or invalid credentials")
	ErrForbiddenQueue       = errors.New("forbidden: service is not authorized for queue")
	ErrForbiddenTask        = errors.New("forbidden: task does not belong to service")
	ErrInvalidLeaseToken    = errors.New("invalid or expired lease token")
	ErrTaskNotRunning       = errors.New("task is not in running state")
	ErrReportConflict       = errors.New("conflict: conflicting report received for attempt")
	ErrWorkerAPIDisabled    = errors.New("worker API is disabled")
	ErrInvalidWaitTimeout   = errors.New("wait_timeout_ms is invalid")

	// Config / startup
	ErrInvalidEnvValue = errors.New("invalid environment variable value")
)
