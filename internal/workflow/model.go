package workflow

import (
	"bytes"
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

type ChangeKind string

const (
	ChangeKindAdded    ChangeKind = "added"
	ChangeKindRemoved  ChangeKind = "removed"
	ChangeKindModified ChangeKind = "modified"
)

type WorkflowChange struct {
	Path   string     `json:"path"`
	Kind   ChangeKind `json:"kind"`
	Before any        `json:"-"`
	After  any        `json:"-"`
}

func (c WorkflowChange) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"path": c.Path,
		"kind": c.Kind,
	}
	if c.Kind == ChangeKindRemoved || c.Kind == ChangeKindModified {
		m["before"] = c.Before
	}
	if c.Kind == ChangeKindAdded || c.Kind == ChangeKindModified {
		m["after"] = c.After
	}
	return json.Marshal(m)
}

func (c *WorkflowChange) UnmarshalJSON(data []byte) error {
	var raw struct {
		Path   string          `json:"path"`
		Kind   ChangeKind      `json:"kind"`
		Before json.RawMessage `json:"before"`
		After  json.RawMessage `json:"after"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Path = raw.Path
	c.Kind = raw.Kind
	if len(raw.Before) > 0 {
		var val any
		dec := json.NewDecoder(bytes.NewReader(raw.Before))
		dec.UseNumber()
		if err := dec.Decode(&val); err == nil {
			c.Before = val
		}
	}
	if len(raw.After) > 0 {
		var val any
		dec := json.NewDecoder(bytes.NewReader(raw.After))
		dec.UseNumber()
		if err := dec.Decode(&val); err == nil {
			c.After = val
		}
	}
	return nil
}

type WorkflowVersionChangesResponse struct {
	FromVersion *int             `json:"from_version"`
	ToVersion   int              `json:"to_version"`
	Changes     []WorkflowChange `json:"changes"`
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
	ListWorkflowVersions(ctx context.Context, name string, limit, offset int) ([]WorkflowSummary, error)
	GetWorkflowVersion(ctx context.Context, name string, version int) (*Workflow, error)
	GetWorkflowVersionChanges(ctx context.Context, name string, version int) (*WorkflowVersionChangesResponse, error)
}

type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow *Workflow) (*Workflow, error)
	GetWorkflow(ctx context.Context, name string) (*Workflow, error)
	ListWorkflows(ctx context.Context, limit, offset int) ([]WorkflowSummary, error)
	ListWorkflowVersions(ctx context.Context, name string, limit, offset int) ([]WorkflowSummary, error)
	GetWorkflowVersion(ctx context.Context, name string, version int) (*Workflow, error)
	GetPreviousWorkflowVersion(ctx context.Context, name string, version int) (*Workflow, error)
	// DeleteWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
}
