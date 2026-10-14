package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

var (
	ErrEmptyWorkerID        = errors.New("worker id is required to claim step")
	ErrEmptyStepExecutionID = errors.New("step execution id is required for heartbeat")
	ErrInvalidStepOutput    = errors.New("step output must be valid JSON")
)

type workerLogic struct {
	execRepo     execution.ExecutionRepository
	lockDuration time.Duration
}

// NewWorkerLogic creates a new WorkerLogic with required execution repository and optional injected config.
func NewWorkerLogic(execRepo execution.ExecutionRepository, cfg ...*config.Config) WorkerLogic {
	leaseDuration := config.DefaultLeaseDuration
	if len(cfg) > 0 && cfg[0] != nil && cfg[0].DefaultLeaseDuration > 0 {
		leaseDuration = cfg[0].DefaultLeaseDuration
	}
	return &workerLogic{
		execRepo:     execRepo,
		lockDuration: leaseDuration,
	}
}

// ClaimNext validates workerID and claims next ready step from repository
func (l *workerLogic) ClaimNext(ctx context.Context, workerID string) (*step.Step, *execution.StepExecution, error) {
	// validate worker id
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return nil, nil, ErrEmptyWorkerID
	}

	// delegate claim to execution repository with configured lease
	if l.execRepo == nil {
		return nil, nil, errors.New("execution repository is not configured")
	}

	leaseDuration := l.lockDuration
	if leaseDuration <= 0 {
		leaseDuration = config.DefaultLeaseDuration
	}
	return l.execRepo.ClaimPending(ctx, trimmedWorkerID, leaseDuration)
}

// Heartbeat validates parameters and delegates lease extension to execution repository
func (l *workerLogic) Heartbeat(ctx context.Context, stepExecutionID string, workerID string) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return ErrEmptyWorkerID
	}

	if l.execRepo == nil {
		return errors.New("execution repository is not configured")
	}

	leaseDuration := l.lockDuration
	if leaseDuration <= 0 {
		leaseDuration = config.DefaultLeaseDuration
	}
	return l.execRepo.Heartbeat(ctx, trimmedExecID, trimmedWorkerID, leaseDuration)
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

// CompleteStep validates parameters and delegates completion to execution repository
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

	if l.execRepo == nil {
		return errors.New("execution repository is not configured")
	}

	return l.execRepo.CompleteStep(ctx, trimmedExecID, trimmedWorkerID, output)
}

// FailStep validates parameters and delegates step failure handling to execution repository
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

	if l.execRepo == nil {
		return errors.New("execution repository is not configured")
	}

	return l.execRepo.FailStep(ctx, trimmedExecID, trimmedWorkerID, trimmedErr)
}
