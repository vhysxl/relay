package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type executionLogic struct {
	repo ExecutionRepository
}

func NewExecutionLogic(repo ExecutionRepository) ExecutionLogic {
	return &executionLogic{
		repo: repo,
	}
}

func (l *executionLogic) ExecuteWorkflow(ctx context.Context, name string, req *ExecuteWorkflowRequest) (*step.WorkflowExecution, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errdefs.ErrInvalidWorkflowName
	}

	var ctxBytes []byte
	if req != nil && len(req.Context) > 0 {
		trimmed := strings.TrimSpace(string(req.Context))
		if trimmed != "" && trimmed != "null" {
			if !json.Valid(req.Context) {
				return nil, errdefs.ErrInvalidWorkflowContext
			}
			ctxBytes = req.Context
		}
	}

	return l.repo.ExecuteWorkflowTx(ctx, name, ctxBytes)
}

func (l *executionLogic) ListExecutions(ctx context.Context, limit, offset int) ([]step.WorkflowExecution, error) {
	limit, offset = util.NormalizePagination(limit, offset, 50, 100)
	return l.repo.ListExecutions(ctx, limit, offset)
}

func (l *executionLogic) GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return nil, errors.New("execution id is required")
	}
	return l.repo.GetExecution(ctx, id)
}
