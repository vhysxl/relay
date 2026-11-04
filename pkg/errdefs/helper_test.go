package errdefs_test

import (
	"errors"
	"testing"

	"github.com/vhysxl/relay/pkg/errdefs"
)

func TestIsValidationError(t *testing.T) {
	validationErrors := []error{
		errdefs.ErrInvalidWorkflow,
		errdefs.ErrInvalidWorkflowName,
		errdefs.ErrWorkflowNameTooLong,
		errdefs.ErrInvalidWorkflowVersion,
		errdefs.ErrWorkflowEmptySteps,
		errdefs.ErrInvalidWorkflowContext,
		errdefs.ErrEmptyStepType,
		errdefs.ErrEmptyID,
		errdefs.ErrInvalidStepType,
		errdefs.ErrInvalidStepConfig,
		errdefs.ErrDuplicateStepID,
		errdefs.ErrDependencyNotFound,
		errdefs.ErrSelfDependency,
		errdefs.ErrCyclicDependency,
		errdefs.ErrInvalidRetryPolicy,
		errdefs.ErrInvalidCompensation,
		errdefs.ErrIrreversibleCompensation,
		errdefs.ErrStepTimeoutNegative,
		errdefs.ErrCompensationTimeoutNegative,
		errdefs.ErrInvalidQueueName,
		errdefs.ErrInvalidActivityName,
		errdefs.ErrEmptyQueueName,
		errdefs.ErrEmptyActivityName,
		errdefs.ErrInvalidWaitTimeout,
	}

	for _, err := range validationErrors {
		if !errdefs.IsValidationError(err) {
			t.Errorf("expected %v to be recognized as validation error", err)
		}
	}

	nonValidationErrors := []error{
		errdefs.ErrWorkflowNotFound,
		errdefs.ErrWorkflowAlreadyExists,
		errdefs.ErrPayloadTooLarge,
		errdefs.ErrStepNotFound,
		errdefs.ErrLockLost,
		errdefs.ErrExecutionNotFound,
		errdefs.ErrInvalidEnvValue,
		errdefs.ErrUnauthorized,
		errdefs.ErrForbiddenQueue,
		errdefs.ErrForbiddenTask,
		errdefs.ErrInvalidLeaseToken,
		errdefs.ErrTaskNotRunning,
		errdefs.ErrReportConflict,
		errdefs.ErrWorkerAPIDisabled,
		errors.New("db error"),
		nil,
	}

	for _, err := range nonValidationErrors {
		if errdefs.IsValidationError(err) {
			t.Errorf("expected %v to NOT be recognized as validation error", err)
		}
	}
}
