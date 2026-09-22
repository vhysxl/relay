package worker

import (
	"context"

	"github.com/vhysxl/relay/internal/step"
)

type WorkerLogic interface {
	ClaimNext(ctx context.Context, workerID string) (*step.Step, *step.StepExecution, error)
}
