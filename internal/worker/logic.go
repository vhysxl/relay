package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"sync"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

var (
	ErrEmptyWorkerID        = errors.New("worker id is required to claim step")
	ErrEmptyStepExecutionID = errors.New("step execution id is required for heartbeat")
	ErrInvalidStepOutput    = errors.New("step output must be valid JSON")
)

type workerLogic struct {
	stepRepo step.StepRepository
}

// NewWorkerLogic creates a new WorkerLogic with required step repository
func NewWorkerLogic(stepRepo step.StepRepository) WorkerLogic {
	return &workerLogic{stepRepo: stepRepo}
}

// ClaimNext validates workerID and claims next ready step from repository
func (l *workerLogic) ClaimNext(ctx context.Context, workerID string) (*step.Step, *step.StepExecution, error) {
	// validate worker id
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return nil, nil, ErrEmptyWorkerID
	}

	// delegate claim to step repository with default 30s lease
	if l.stepRepo == nil {
		return nil, nil, errors.New("step repository is not configured")
	}
	return l.stepRepo.ClaimNext(ctx, trimmedWorkerID, step.DefaultLockDuration)
}

// Heartbeat validates parameters and delegates lease extension to step repository
func (l *workerLogic) Heartbeat(ctx context.Context, stepExecutionID string, workerID string) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return ErrEmptyWorkerID
	}

	if l.stepRepo == nil {
		return errors.New("step repository is not configured")
	}

	return l.stepRepo.Heartbeat(ctx, trimmedExecID, trimmedWorkerID, step.DefaultLockDuration)
}

// StartHeartbeat begins a background ticker to periodically extend the step execution lease
// until stop() is called, context is cancelled, or a heartbeat error occurs.
func (l *workerLogic) StartHeartbeat(ctx context.Context, stepExecutionID string, workerID string, interval time.Duration) (func(), <-chan error) {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}

	stopChan := make(chan struct{})
	errChan := make(chan error, 1)
	var once sync.Once

	stop := func() {
		once.Do(func() {
			close(stopChan)
		})
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case <-stopChan:
					return
				case <-ctx.Done():
					return
				default:
				}

				if err := l.Heartbeat(ctx, stepExecutionID, workerID); err != nil {
					select {
					case errChan <- err:
					default:
					}
					return
				}
			}
		}
	}()

	return stop, errChan
}

// CompleteStep validates parameters and delegates completion to step repository
func (l *workerLogic) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return ErrEmptyWorkerID
	}

	if len(output) > 0 {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput != "" && trimmedOutput != "null" && !json.Valid(output) {
			return ErrInvalidStepOutput
		}
	}

	if l.stepRepo == nil {
		return errors.New("step repository is not configured")
	}

	return l.stepRepo.CompleteStep(ctx, trimmedExecID, trimmedWorkerID, output)
}

// FailStep validates parameters and delegates step failure handling to step repository
func (l *workerLogic) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return ErrEmptyWorkerID
	}

	trimmedErr := strings.TrimSpace(errMsg)
	if trimmedErr == "" {
		trimmedErr = "unknown error"
	}

	if l.stepRepo == nil {
		return errors.New("step repository is not configured")
	}

	return l.stepRepo.FailStep(ctx, trimmedExecID, trimmedWorkerID, trimmedErr)
}

