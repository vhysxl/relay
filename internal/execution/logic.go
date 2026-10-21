package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type executionLogic struct {
	repo      ExecutionRepository
	workflows WorkflowReader
}

func NewExecutionLogic(repo ExecutionRepository, workflows WorkflowReader) ExecutionLogic {
	return &executionLogic{
		repo:      repo,
		workflows: workflows,
	}
}

func (l *executionLogic) ExecuteWorkflow(ctx context.Context, name string, req *ExecuteWorkflowRequest) (*ExecuteWorkflowResponse, bool, error) {
	if req == nil || req.IdempotencyKey == "" {
		return nil, false, errdefs.ErrIdempotencyKeyRequired
	}
	if err := ValidateIdempotencyKey(req.IdempotencyKey); err != nil {
		return nil, false, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, errdefs.ErrInvalidWorkflowName
	}

	var rawInput json.RawMessage
	if req != nil {
		rawInput = req.Context
	}
	normalizedInput, err := normalizeInputContext(rawInput)
	if err != nil {
		return nil, false, err
	}

	wf, err := l.workflows.GetWorkflow(ctx, name)
	if err != nil {
		return nil, false, err
	}

	targetIsExplicitID := (name == wf.ID)

	// Step 2 & 3: Check existing idempotency record before merging context or building snapshot
	existing, err := l.repo.GetExecutionRequest(ctx, wf.Name, req.IdempotencyKey)
	if err != nil && !errors.Is(err, errdefs.ErrExecutionRequestNotFound) {
		return nil, false, err
	}
	if existing != nil {
		receipt, err := evaluateReplay(existing, normalizedInput, targetIsExplicitID, name)
		if err != nil {
			return nil, false, err
		}
		return receipt, true, nil
	}

	// Step 4: Build execution, snapshot steps, and receipt
	merged, err := mergeContext(wf.Context, normalizedInput)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	exec := &WorkflowExecution{
		ID:           util.GenerateUUID(),
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Status:       StatusRunning,
		Context:      merged,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	exec.Steps = snapshotSteps(exec.ID, wf.Steps, merged, now)

	receipt := ExecuteWorkflowResponse{
		ID:         exec.ID,
		WorkflowID: exec.WorkflowID,
		Status:     string(exec.Status),
		Context:    exec.Context,
		TotalSteps: len(exec.Steps),
		CreatedAt:  exec.CreatedAt,
	}
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return nil, false, fmt.Errorf("marshal receipt: %w", err)
	}

	reqRecord := &ExecutionRequestRecord{
		WorkflowName:        wf.Name,
		IdempotencyKey:      req.IdempotencyKey,
		WorkflowExecutionID: exec.ID,
		InputContext:        normalizedInput,
		ResponseReceipt:     receiptBytes,
		CreatedAt:           now,
	}

	// Step 5 & 6: Create execution with idempotency inside a single transaction
	return l.repo.CreateExecutionWithIdempotency(ctx, exec, reqRecord, targetIsExplicitID, name)
}

func (l *executionLogic) ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error) {
	limit, offset = util.NormalizePagination(limit, offset, 50, 100)
	return l.repo.ListExecutions(ctx, limit, offset)
}

func (l *executionLogic) GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return nil, errdefs.ErrEmptyExecutionID
	}
	return l.repo.GetExecution(ctx, id)
}
