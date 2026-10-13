package workflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

type WorkflowRequest struct {
	Name    string          `json:"name"`
	Version int             `json:"version"`
	Context json.RawMessage `json:"context,omitempty"`
	Steps   []step.Step     `json:"steps"`
}

type Workflow struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Version   int             `json:"version"`
	Context   json.RawMessage `json:"context,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Steps     []step.Step     `json:"steps"`
}

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

type Validator interface {
	ValidateAndNormalizeWorkflow(workflowReq *WorkflowRequest) error
	ValidateSteps(steps []step.Step) error
	IsWorkflowEqual(existingSteps []step.Step, existingContext json.RawMessage, req *WorkflowRequest) bool
}

type WorkflowLogic interface {
	CreateWorkflow(ctx context.Context, req *WorkflowRequest) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	ListWorkflows(ctx context.Context) ([]Workflow, error)
	ExecuteWorkflow(ctx context.Context, workflowID string, req *ExecuteWorkflowRequest) (*step.WorkflowExecution, error)
	ListExecutions(ctx context.Context) ([]step.WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error)
}

type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow *Workflow) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	ListWorkflows(ctx context.Context) ([]Workflow, error)
	ExecuteWorkflowTx(ctx context.Context, workflowID string, initialContext json.RawMessage) (*step.WorkflowExecution, error)
	ListExecutions(ctx context.Context) ([]step.WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error)
	// DeleteWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
}
