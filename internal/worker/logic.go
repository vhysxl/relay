package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type workerLogic struct {
	execRepo      execution.ExecutionRepository
	leaseDuration time.Duration
}

// NewWorkerLogic creates a new WorkerLogic with required execution repository and lease duration.
func NewWorkerLogic(execRepo execution.ExecutionRepository, leaseDuration time.Duration) WorkerLogic {
	return &workerLogic{
		execRepo:      execRepo,
		leaseDuration: leaseDuration,
	}
}

// ClaimNext validates workerID and claims next ready step from repository
func (l *workerLogic) ClaimNext(ctx context.Context, workerID string) (*step.Step, *execution.StepExecution, error) {
	// validate worker id
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return nil, nil, errdefs.ErrEmptyWorkerID
	}

	return l.execRepo.ClaimPending(ctx, trimmedWorkerID, l.leaseDuration)
}

// Heartbeat validates parameters and delegates lease extension to execution repository
func (l *workerLogic) Heartbeat(ctx context.Context, stepExecutionID string, workerID string) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return errdefs.ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return errdefs.ErrEmptyWorkerID
	}

	return l.execRepo.Heartbeat(ctx, trimmedExecID, trimmedWorkerID, l.leaseDuration)
}

// StartHeartbeat begins a background ticker to periodically extend the step execution lease
// until stop() is called, context is cancelled, or a heartbeat error occurs.
func (l *workerLogic) StartHeartbeat(ctx context.Context, stepExecutionID string, workerID string, interval time.Duration) (func(), <-chan error) {
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

// CompleteStep validates parameters and delegates completion to execution repository
func (l *workerLogic) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return errdefs.ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return errdefs.ErrEmptyWorkerID
	}

	if len(output) > 0 {
		trimmedOutput := bytes.TrimSpace(output)
		if len(trimmedOutput) > 0 && !bytes.Equal(trimmedOutput, []byte("null")) && !json.Valid(output) {
			return errdefs.ErrInvalidStepOutput
		}
	}

	return l.execRepo.CompleteStep(ctx, trimmedExecID, trimmedWorkerID, output)
}

// FailStep validates parameters and delegates step failure handling to execution repository
func (l *workerLogic) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return errdefs.ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return errdefs.ErrEmptyWorkerID
	}

	trimmedErr := strings.TrimSpace(errMsg)
	if trimmedErr == "" {
		trimmedErr = "unknown error"
	}

	return l.execRepo.FailStep(ctx, trimmedExecID, trimmedWorkerID, trimmedErr)
}
