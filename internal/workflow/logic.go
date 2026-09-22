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

var (
	ErrInvalidWorkflowName    = errors.New("workflow name is required")
	ErrInvalidWorkflowID      = errors.New("workflow id is required")
	ErrInvalidWorkflowContext = errors.New("workflow context must be valid JSON")
)

type workflowLogic struct {
	repo      WorkflowRepository
	stepLogic step.StepLogic
}

func NewWorkflowLogic(repo WorkflowRepository, stepLogic step.StepLogic) WorkflowLogic {
	if stepLogic == nil {
		stepLogic = step.NewStepLogic()
	}
	return &workflowLogic{
		repo:      repo,
		stepLogic: stepLogic,
	}
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
	now := time.Now().UTC()

	steps := make([]step.Step, len(req.Steps))
	copy(steps, req.Steps)

	for i := range steps {
		if strings.TrimSpace(steps[i].ID) == "" {
			steps[i].ID = util.GenerateUUID()
		}
		steps[i].WorkflowID = id
		if steps[i].CreatedAt.IsZero() {
			steps[i].CreatedAt = now
		}
		if steps[i].UpdatedAt.IsZero() {
			steps[i].UpdatedAt = now
		}
	}

	if len(steps) > 0 && l.stepLogic != nil {
		if err := l.stepLogic.ValidateSteps(steps); err != nil {
			return nil, err
		}
	}

	hash := util.GenerateWorkflowHash(name, version, ctxBytes)

	wf := &Workflow{
		ID:        id,
		Name:      name,
		Version:   version,
		Status:    WorkflowStatusPending,
		Hash:      hash,
		Context:   ctxBytes,
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
