package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"task/queue/pkg/util"
	"time"
)

var (
	ErrInvalidWorkflowName    = errors.New("workflow name is required")
	ErrInvalidWorkflowID      = errors.New("workflow id is required")
	ErrInvalidWorkflowContext = errors.New("workflow context must be valid JSON")
)

type workflowLogic struct {
	repo WorkflowRepository
}

func NewWorkflowLogic(repo WorkflowRepository) WorkflowLogic {
	return &workflowLogic{repo: repo}
}

func (l *workflowLogic) CreateWorkflow(ctx context.Context, req *WorkflowRequest) (*Workflow, error) {
	if req == nil {
		return nil, ErrInvalidWorkflowName
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrInvalidWorkflowName
	}

	version := req.Version
	if version <= 0 {
		version = 1
	}

	var ctxBytes []byte
	if len(req.Context) > 0 {
		trimmed := strings.TrimSpace(string(req.Context))
		if trimmed != "" && trimmed != "null" {
			if !json.Valid(req.Context) {
				return nil, ErrInvalidWorkflowContext
			}
			ctxBytes = req.Context
		}
	}

	id := util.GenerateUUID()

	hash := util.GenerateWorkflowHash(name, version, ctxBytes)
	now := time.Now().UTC()

	wf := &Workflow{
		ID:        id,
		Name:      name,
		Version:   version,
		Status:    WorkflowStatusPending,
		Hash:      hash,
		Context:   ctxBytes,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return l.repo.CreateWorkflow(ctx, wf)
}

func (l *workflowLogic) GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error) {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		return nil, ErrInvalidWorkflowID
	}

	return l.repo.GetWorkflow(ctx, id)
}
