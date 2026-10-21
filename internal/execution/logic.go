package execution

import (
	"context"
	"strings"
	"time"

	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type executionLogic struct {
	repo      ExecutionRepository
	workflows WorkflowReader
}

func NewExecutionLogic(repo ExecutionRepository, workflows WorkflowReader) ExecutionLogic {
	return &executionLogic{
		repo:      repo,
		workflows: workflows,
	}
}

func (l *executionLogic) ExecuteWorkflow(ctx context.Context, name string, req *ExecuteWorkflowRequest) (*WorkflowExecution, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errdefs.ErrInvalidWorkflowName
	}

	var input []byte
	if req != nil {
		input = req.Context
	}
	if _, err := contextObject(input); err != nil {
		return nil, err
	}
	wf, err := l.workflows.GetWorkflow(ctx, name)
	if err != nil {
		return nil, err
	}
	merged, err := mergeContext(wf.Context, input)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	exec := &WorkflowExecution{ID: util.GenerateUUID(), WorkflowID: wf.ID,
		Status: StatusRunning, Context: merged, CreatedAt: now, UpdatedAt: now}
	exec.Steps = snapshotSteps(exec.ID, wf.Steps, merged, now)
	return l.repo.CreateExecution(ctx, exec)
}

func (l *executionLogic) ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error) {
	limit, offset = util.NormalizePagination(limit, offset, 50, 100)
	return l.repo.ListExecutions(ctx, limit, offset)
}

func (l *executionLogic) GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return nil, errdefs.ErrEmptyExecutionID
	}
	return l.repo.GetExecution(ctx, id)
}
