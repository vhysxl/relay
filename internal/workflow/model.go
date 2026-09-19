package workflow

import (
	"context"
	"time"
)

type WorkflowStatus string

const (
	WorkflowStatusPending            WorkflowStatus = " PENDING"
	WorkflowStatusRunning            WorkflowStatus = "RUNNING"
	WorkflowStatusCompensating       WorkflowStatus = "COMPENSATING"
	WorkflowStatusSuccess            WorkflowStatus = "SUCCESS"
	WorkflowStatusFailed             WorkflowStatus = "FAILED"
	WorkflowStatusCompensated        WorkflowStatus = "COMPENSATED"
	WorkflowStatusCompensationFailed WorkflowStatus = "COMPENSATION_FAILED"
)

type WorkflowRequest struct {
	Name    string `json:"string"`
	Version int    `json:"version"`
	// Steps []
}

type Workflow struct {
	ID        string
	Name      string
	Version   int
	Status    WorkflowStatus
	Hash      string
	Context   []byte
	CreatedAt time.Time
	UpdatedAt time.Time
	// Steps
}

type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow *Workflow) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	// DeleteWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
}
