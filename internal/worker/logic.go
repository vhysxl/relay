package worker

import (
	"context"
	"errors"
	"strings"

	"github.com/vhysxl/relay/internal/step"
)

var (
	ErrEmptyWorkerID = errors.New("worker id is required to claim step")
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
