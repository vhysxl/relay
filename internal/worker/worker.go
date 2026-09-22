package worker

import (
	"context"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

const (
	DefaultHeartbeatInterval = 10 * time.Second
)

type WorkerLogic interface {
	ClaimNext(ctx context.Context, workerID string) (*step.Step, *step.StepExecution, error)
	Heartbeat(ctx context.Context, stepExecutionID string, workerID string) error
	StartHeartbeat(ctx context.Context, stepExecutionID string, workerID string, interval time.Duration) (stop func(), errChan <-chan error)
}

