package execution

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

type ExecuteWorkflowRequest struct {
	Context json.RawMessage `json:"context,omitempty"`
}

type ExecuteWorkflowResponse struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	Status     string          `json:"status"`
	Context    json.RawMessage `json:"context,omitempty"`
	TotalSteps int             `json:"total_steps"`
	CreatedAt  time.Time       `json:"created_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type ExecutionLogic interface {
	ExecuteWorkflow(ctx context.Context, name string, req *ExecuteWorkflowRequest) (*step.WorkflowExecution, error)
	ListExecutions(ctx context.Context, limit, offset int) ([]step.WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error)
}

type ExecutionRepository interface {
	ExecuteWorkflowTx(ctx context.Context, name string, initialContext json.RawMessage) (*step.WorkflowExecution, error)
	ListExecutions(ctx context.Context, limit, offset int) ([]step.WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error)
}
