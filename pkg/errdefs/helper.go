package errdefs

import "errors"

// IsValidationError checks whether an error is a client-side validation error.
func IsValidationError(err error) bool {
	switch {
	case errors.Is(err, ErrInvalidWorkflow),
		errors.Is(err, ErrInvalidWorkflowName),
		errors.Is(err, ErrWorkflowNameTooLong),
		errors.Is(err, ErrInvalidWorkflowVersion),
		errors.Is(err, ErrWorkflowEmptySteps),
		errors.Is(err, ErrInvalidWorkflowContext),
		errors.Is(err, ErrEmptyStepType),
		errors.Is(err, ErrEmptyID),
		errors.Is(err, ErrInvalidStepType),
		errors.Is(err, ErrInvalidStepConfig),
		errors.Is(err, ErrDuplicateStepID),
		errors.Is(err, ErrDependencyNotFound),
		errors.Is(err, ErrSelfDependency),
		errors.Is(err, ErrCyclicDependency),
		errors.Is(err, ErrInvalidRetryPolicy),
		errors.Is(err, ErrInvalidCompensation),
		errors.Is(err, ErrIrreversibleCompensation),
		errors.Is(err, ErrStepTimeoutNegative),
		errors.Is(err, ErrCompensationTimeoutNegative),
		errors.Is(err, ErrInvalidCondition),
		errors.Is(err, ErrConditionMaxDepth),
		errors.Is(err, ErrConditionMaxNodes),
		errors.Is(err, ErrConditionNonAncestor),
		errors.Is(err, ErrConditionAmbiguousAnchor):
		return true
	default:
		return false
	}
}
