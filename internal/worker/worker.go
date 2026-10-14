package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

const (
	DefaultHeartbeatInterval = 10 * time.Second
)

type WorkerLogic interface {
	ClaimNext(ctx context.Context, workerID string) (*step.Step, *execution.StepExecution, error)
	Heartbeat(ctx context.Context, stepExecutionID string, workerID string) error
	StartHeartbeat(ctx context.Context, stepExecutionID string, workerID string, interval time.Duration) (stop func(), errChan <-chan error)
	CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error
	FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error
}


