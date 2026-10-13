package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

type workflowLogic struct {
	repo      WorkflowRepository
	validator Validator
}

func NewWorkflowLogic(repo WorkflowRepository, v Validator) WorkflowLogic {
	return &workflowLogic{
		repo:      repo,
		validator: v,
	}
}

func (l *workflowLogic) CreateWorkflow(ctx context.Context, req *WorkflowRequest) (*Workflow, error) {
	if req == nil {
		return nil, ErrInvalidWorkflow
	}

	if req.Version <= 0 {
		req.Version = 1
	}

	// Check if a workflow with this name already exists in the database
	existing, err := l.repo.GetWorkflow(ctx, req.Name)
	if err != nil && !errors.Is(err, ErrWorkflowNotFound) {
		return nil, err
	}

	if err == nil && existing != nil {
		// If 1:1 match with existing workflow, return it idempotently
		if l.validator != nil && l.validator.IsWorkflowEqual(existing.Steps, existing.Context, req) {
			return existing, nil
		}
		// If not 1:1 match, auto-increment the version
		req.Version = existing.Version + 1
	}

	id := util.GenerateUUID()
	now := time.Now().UTC()

	steps := make([]step.Step, len(req.Steps))
	for i, s := range req.Steps {
		s.WorkflowID = id
		s.CreatedAt = now
		s.UpdatedAt = now
		steps[i] = s
	}

	wf := &Workflow{
		ID:        id,
		Name:      req.Name,
		Version:   req.Version,
		Context:   req.Context,
		CreatedAt: now,
		UpdatedAt: now,
		Steps:     steps,
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

func (l *workflowLogic) ExecuteWorkflow(ctx context.Context, workflowID string, req *ExecuteWorkflowRequest) (*step.WorkflowExecution, error) {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		return nil, ErrInvalidWorkflowID
	}

	var ctxBytes []byte
	if req != nil && len(req.Context) > 0 {
		trimmed := strings.TrimSpace(string(req.Context))
		if trimmed != "" && trimmed != "null" {
			if !json.Valid(req.Context) {
				return nil, ErrInvalidWorkflowContext
			}
			ctxBytes = req.Context
		}
	}

	if l.repo == nil {
		return nil, errors.New("workflow repository is not configured")
	}

	return l.repo.ExecuteWorkflowTx(ctx, id, ctxBytes)
}

func (l *workflowLogic) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	if l.repo == nil {
		return nil, errors.New("workflow repository is not configured")
	}
	return l.repo.ListWorkflows(ctx)
}

func (l *workflowLogic) ListExecutions(ctx context.Context) ([]step.WorkflowExecution, error) {
	if l.repo == nil {
		return nil, errors.New("workflow repository is not configured")
	}
	return l.repo.ListExecutions(ctx)
}

func (l *workflowLogic) GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return nil, errors.New("execution id is required")
	}
	if l.repo == nil {
		return nil, errors.New("workflow repository is not configured")
	}
	return l.repo.GetExecution(ctx, id)
}

