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

// WorkflowSummary contains only the metadata needed to list workflow definitions.
type WorkflowSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	StepCount int       `json:"step_count"`
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
	GetWorkflow(ctx context.Context, name string) (*Workflow, error)
	ListWorkflows(ctx context.Context, limit, offset int) ([]WorkflowSummary, error)
}

type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow *Workflow) (*Workflow, error)
	GetWorkflow(ctx context.Context, name string) (*Workflow, error)
	ListWorkflows(ctx context.Context, limit, offset int) ([]WorkflowSummary, error)
	// DeleteWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
}
