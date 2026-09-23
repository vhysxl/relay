package workflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

type WorkflowStatus string

const (
	WorkflowStatusPending            WorkflowStatus = "PENDING"
	WorkflowStatusRunning            WorkflowStatus = "RUNNING"
	WorkflowStatusCompensating       WorkflowStatus = "COMPENSATING"
	WorkflowStatusCompleted          WorkflowStatus = "COMPLETED"
	WorkflowStatusSuccess                           = WorkflowStatusCompleted
	WorkflowStatusFailed             WorkflowStatus = "FAILED"
	WorkflowStatusCompensated        WorkflowStatus = "COMPENSATED"
	WorkflowStatusCompensationFailed WorkflowStatus = "COMPENSATION_FAILED"
	WorkflowStatusCancelled          WorkflowStatus = "CANCELLED"
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
	Status    WorkflowStatus  `json:"status"`
	Hash      string          `json:"hash"`
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

type WorkflowLogic interface {
	CreateWorkflow(ctx context.Context, req *WorkflowRequest) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	ExecuteWorkflow(ctx context.Context, workflowID string, req *ExecuteWorkflowRequest) (*step.WorkflowExecution, error)
}

type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow *Workflow) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	ExecuteWorkflowTx(ctx context.Context, workflowID string, initialContext json.RawMessage) (*step.WorkflowExecution, error)
	// DeleteWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
}

