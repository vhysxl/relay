package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

type Repository interface {
	ClaimPending(context.Context, string, time.Duration) (*step.Step, *execution.StepExecution, error)
	Heartbeat(context.Context, string, string, time.Duration) error
	CompleteStep(context.Context, string, string, json.RawMessage) error
	FailStep(context.Context, string, string, string) error
}

// WorkerLogic defines business logic operations for task claiming, heartbeat renewal, and completion.
type WorkerLogic interface {
	ClaimNext(ctx context.Context, workerID string) (*step.Step, *execution.StepExecution, error)
	Heartbeat(ctx context.Context, stepExecutionID string, workerID string) error
	StartHeartbeat(ctx context.Context, stepExecutionID string, workerID string, interval time.Duration) (stop func(), errChan <-chan error)
	CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error
	FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error
}

// WorkerPool defines the contract for running background worker pools and processing individual tasks.
type WorkerPool interface {
	Start(ctx context.Context) error
	ProcessOne(ctx context.Context, workerID string) (bool, error)
}
