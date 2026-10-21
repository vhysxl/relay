package workflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type workflowLogic struct {
	repo                 WorkflowRepository
	validator            Validator
	concurrentRetryLimit int
}

func NewWorkflowLogic(repo WorkflowRepository, v Validator, concurrentRetryLimit int) WorkflowLogic {
	return &workflowLogic{
		repo:                 repo,
		validator:            v,
		concurrentRetryLimit: concurrentRetryLimit,
	}
}

func (l *workflowLogic) CreateWorkflow(ctx context.Context, req *WorkflowRequest) (*Workflow, error) {
	if err := l.validator.ValidateAndNormalizeWorkflow(req); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < l.concurrentRetryLimit; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Check if a workflow with this name already exists in the database
		existing, err := l.repo.GetWorkflow(ctx, req.Name)
		if err != nil && !errors.Is(err, errdefs.ErrWorkflowNotFound) {
			return nil, err
		}

		if existing != nil {
			// If 1:1 match with existing workflow, return it idempotently
			if l.validator.IsWorkflowEqual(existing.Steps, existing.Context, req) {
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

		created, err := l.repo.CreateWorkflow(ctx, wf)
		if err == nil {
			return created, nil
		}

		// If race condition lost (ErrWorkflowAlreadyExists), retry to fetch the newest workflow
		if errors.Is(err, errdefs.ErrWorkflowAlreadyExists) {
			continue
		}

		return nil, err
	}

	return nil, errdefs.ErrWorkflowAlreadyExists
}

func (l *workflowLogic) GetWorkflow(ctx context.Context, name string) (*Workflow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errdefs.ErrInvalidWorkflowName
	}

	return l.repo.GetWorkflow(ctx, name)
}

func (l *workflowLogic) ListWorkflows(ctx context.Context, limit, offset int) ([]WorkflowSummary, error) {
	limit, offset = util.NormalizePagination(limit, offset, 50, 100)
	return l.repo.ListWorkflows(ctx, limit, offset)
}
