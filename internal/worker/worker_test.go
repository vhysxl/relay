package worker

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

type mockStepRepository struct {
	claimFunc func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *step.StepExecution, error)
}

func (m *mockStepRepository) CreateStepsTx(ctx context.Context, tx *sql.Tx, workflowID string, steps []step.Step) error {
	return nil
}

func (m *mockStepRepository) GetStepsByWorkflowID(ctx context.Context, workflowID string) ([]step.Step, error) {
	return nil, nil
}

func (m *mockStepRepository) GetStepByID(ctx context.Context, workflowID string, stepID string) (*step.Step, error) {
	return nil, nil
}

func (m *mockStepRepository) ClaimNext(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *step.StepExecution, error) {
	if m.claimFunc != nil {
		return m.claimFunc(ctx, workerID, lockDuration)
	}
	return nil, nil, nil
}

func TestWorkerLogic_ClaimNext_EmptyWorkerID(t *testing.T) {
	logic := NewWorkerLogic(&mockStepRepository{})

	_, _, err := logic.ClaimNext(context.Background(), "")
	if err != ErrEmptyWorkerID {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}

	_, _, err = logic.ClaimNext(context.Background(), "   ")
	if err != ErrEmptyWorkerID {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}
}

func TestWorkerLogic_ClaimNext_NilRepo(t *testing.T) {
	logic := NewWorkerLogic(nil)

	_, _, err := logic.ClaimNext(context.Background(), "worker-1")
	if err == nil {
		t.Fatalf("expected error when repo is nil, got nil")
	}
}

func TestWorkerLogic_ClaimNext_Success(t *testing.T) {
	expectedStep := &step.Step{
		ID:   "step-1",
		Type: step.StepTypeHTTP,
	}
	expectedExec := &step.StepExecution{
		ID:      "exec-999",
		StepID:  "step-1",
		Status:  step.StatusRunning,
		Attempt: 1,
	}

	repo := &mockStepRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *step.StepExecution, error) {
			if workerID != "worker-1" {
				t.Errorf("expected workerID 'worker-1', got %s", workerID)
			}
			if lockDuration != step.DefaultLockDuration {
				t.Errorf("expected lockDuration %v, got %v", step.DefaultLockDuration, lockDuration)
			}
			return expectedStep, expectedExec, nil
		},
	}

	logic := NewWorkerLogic(repo)
	s, se, err := logic.ClaimNext(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil || s.ID != "step-1" {
		t.Errorf("expected step-1, got %+v", s)
	}
	if se == nil || se.ID != "exec-999" || se.Status != step.StatusRunning {
		t.Errorf("expected exec-999 in RUNNING state, got %+v", se)
	}
}

func TestWorkerLogic_ClaimNext_NoTaskAvailable(t *testing.T) {
	repo := &mockStepRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *step.StepExecution, error) {
			return nil, nil, nil
		},
	}

	logic := NewWorkerLogic(repo)
	s, se, err := logic.ClaimNext(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != nil || se != nil {
		t.Fatalf("expected nil step and exec when queue empty, got s=%+v, se=%+v", s, se)
	}
}
