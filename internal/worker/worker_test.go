package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

type mockStepRepository struct {
	claimFunc     func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *step.StepExecution, error)
	heartbeatFunc func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error
	completeFunc  func(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error
	failFunc      func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error
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

func (m *mockStepRepository) Heartbeat(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
	if m.heartbeatFunc != nil {
		return m.heartbeatFunc(ctx, stepExecutionID, workerID, lockDuration)
	}
	return nil
}

func (m *mockStepRepository) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, stepExecutionID, workerID, output)
	}
	return nil
}

func (m *mockStepRepository) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	if m.failFunc != nil {
		return m.failFunc(ctx, stepExecutionID, workerID, errMsg)
	}
	return nil
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

func TestWorkerLogic_Heartbeat_Validation(t *testing.T) {
	logic := NewWorkerLogic(&mockStepRepository{})

	// Empty stepExecutionID
	err := logic.Heartbeat(context.Background(), "", "worker-1")
	if !errors.Is(err, ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID, got %v", err)
	}
	err = logic.Heartbeat(context.Background(), "   ", "worker-1")
	if !errors.Is(err, ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID for whitespace, got %v", err)
	}

	// Empty workerID
	err = logic.Heartbeat(context.Background(), "exec-1", "")
	if !errors.Is(err, ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}
	err = logic.Heartbeat(context.Background(), "exec-1", "   ")
	if !errors.Is(err, ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}

	// Nil repo
	nilLogic := NewWorkerLogic(nil)
	err = nilLogic.Heartbeat(context.Background(), "exec-1", "worker-1")
	if err == nil {
		t.Fatalf("expected error when repo is nil, got nil")
	}
}

func TestWorkerLogic_Heartbeat_Success(t *testing.T) {
	called := false
	repo := &mockStepRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			called = true
			if stepExecutionID != "exec-123" {
				t.Errorf("expected exec-123, got %s", stepExecutionID)
			}
			if workerID != "worker-1" {
				t.Errorf("expected worker-1, got %s", workerID)
			}
			if lockDuration != step.DefaultLockDuration {
				t.Errorf("expected lockDuration %v, got %v", step.DefaultLockDuration, lockDuration)
			}
			return nil
		},
	}

	logic := NewWorkerLogic(repo)
	err := logic.Heartbeat(context.Background(), "  exec-123  ", "  worker-1  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected repo Heartbeat to be called")
	}
}

func TestWorkerLogic_Heartbeat_LockLost(t *testing.T) {
	repo := &mockStepRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			return step.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo)
	err := logic.Heartbeat(context.Background(), "exec-123", "worker-1")
	if !errors.Is(err, step.ErrLockLost) {
		t.Fatalf("expected ErrLockLost, got %v", err)
	}
}

func TestWorkerLogic_StartHeartbeat_TicksAndStops(t *testing.T) {
	var count int32
	repo := &mockStepRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	}

	logic := NewWorkerLogic(repo)
	// Start with fast 15ms interval
	stop, errChan := logic.StartHeartbeat(context.Background(), "exec-1", "worker-1", 15*time.Millisecond)

	// Allow several ticks to fire
	time.Sleep(50 * time.Millisecond)
	stop()
	// Allow in-flight tick to exit cleanly
	time.Sleep(20 * time.Millisecond)

	// Ensure no error on errChan
	select {
	case err := <-errChan:
		t.Fatalf("unexpected error from errChan: %v", err)
	default:
	}

	currentCount := atomic.LoadInt32(&count)
	if currentCount < 2 {
		t.Errorf("expected at least 2 heartbeat ticks, got %d", currentCount)
	}

	// Wait a bit more and verify no more ticks fire after stop
	time.Sleep(45 * time.Millisecond)
	afterStopCount := atomic.LoadInt32(&count)
	if afterStopCount != currentCount {
		t.Errorf("expected heartbeat to stop firing, count changed from %d to %d", currentCount, afterStopCount)
	}
}

func TestWorkerLogic_StartHeartbeat_StopsOnError(t *testing.T) {
	repo := &mockStepRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			return step.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo)
	stop, errChan := logic.StartHeartbeat(context.Background(), "exec-1", "worker-1", 10*time.Millisecond)
	defer stop()

	select {
	case err := <-errChan:
		if !errors.Is(err, step.ErrLockLost) {
			t.Fatalf("expected step.ErrLockLost, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for error on errChan")
	}
}

func TestWorkerLogic_StartHeartbeat_ContextCancelled(t *testing.T) {
	repo := &mockStepRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			return nil
		},
	}

	logic := NewWorkerLogic(repo)
	ctx, cancel := context.WithCancel(context.Background())

	stop, _ := logic.StartHeartbeat(ctx, "exec-1", "worker-1", 10*time.Millisecond)
	defer stop()

	// Cancel context immediately
	cancel()
	time.Sleep(30 * time.Millisecond)
}

func TestWorkerLogic_CompleteStep_Validation(t *testing.T) {
	logic := NewWorkerLogic(&mockStepRepository{})

	// Empty stepExecutionID
	err := logic.CompleteStep(context.Background(), "", "worker-1", json.RawMessage("{}"))
	if !errors.Is(err, ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID, got %v", err)
	}
	err = logic.CompleteStep(context.Background(), "   ", "worker-1", json.RawMessage("{}"))
	if !errors.Is(err, ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID for whitespace, got %v", err)
	}

	// Empty workerID
	err = logic.CompleteStep(context.Background(), "exec-1", "", json.RawMessage("{}"))
	if !errors.Is(err, ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}
	err = logic.CompleteStep(context.Background(), "exec-1", "   ", json.RawMessage("{}"))
	if !errors.Is(err, ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}

	// Invalid JSON output
	err = logic.CompleteStep(context.Background(), "exec-1", "worker-1", json.RawMessage("{invalid-json"))
	if !errors.Is(err, ErrInvalidStepOutput) {
		t.Fatalf("expected ErrInvalidStepOutput, got %v", err)
	}

	// Nil repository
	nilLogic := NewWorkerLogic(nil)
	err = nilLogic.CompleteStep(context.Background(), "exec-1", "worker-1", json.RawMessage("{}"))
	if err == nil {
		t.Fatalf("expected error for nil repository, got nil")
	}
}

func TestWorkerLogic_CompleteStep_Success(t *testing.T) {
	called := false
	repo := &mockStepRepository{
		completeFunc: func(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
			called = true
			if stepExecutionID != "exec-123" {
				t.Errorf("expected exec-123, got %s", stepExecutionID)
			}
			if workerID != "worker-1" {
				t.Errorf("expected worker-1, got %s", workerID)
			}
			if string(output) != `{"status":"ok"}` {
				t.Errorf("unexpected output: %s", string(output))
			}
			return nil
		},
	}

	logic := NewWorkerLogic(repo)
	err := logic.CompleteStep(context.Background(), "  exec-123  ", "  worker-1  ", json.RawMessage(`{"status":"ok"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected completeFunc to be called")
	}
}

func TestWorkerLogic_CompleteStep_LockLost(t *testing.T) {
	repo := &mockStepRepository{
		completeFunc: func(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
			return step.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo)
	err := logic.CompleteStep(context.Background(), "exec-123", "worker-1", json.RawMessage("{}"))
	if !errors.Is(err, step.ErrLockLost) {
		t.Fatalf("expected ErrLockLost, got %v", err)
	}
}

func TestWorkerLogic_FailStep_Validation(t *testing.T) {
	logic := NewWorkerLogic(&mockStepRepository{})

	// Empty stepExecutionID
	err := logic.FailStep(context.Background(), "", "worker-1", "some error")
	if !errors.Is(err, ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID, got %v", err)
	}
	err = logic.FailStep(context.Background(), "   ", "worker-1", "some error")
	if !errors.Is(err, ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID for whitespace, got %v", err)
	}

	// Empty workerID
	err = logic.FailStep(context.Background(), "exec-1", "", "some error")
	if !errors.Is(err, ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}
	err = logic.FailStep(context.Background(), "exec-1", "   ", "some error")
	if !errors.Is(err, ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}

	// Nil repository
	nilLogic := NewWorkerLogic(nil)
	err = nilLogic.FailStep(context.Background(), "exec-1", "worker-1", "some error")
	if err == nil {
		t.Fatalf("expected error for nil repository, got nil")
	}
}

func TestWorkerLogic_FailStep_Success(t *testing.T) {
	called := false
	repo := &mockStepRepository{
		failFunc: func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
			called = true
			if stepExecutionID != "exec-123" {
				t.Errorf("expected exec-123, got %s", stepExecutionID)
			}
			if workerID != "worker-1" {
				t.Errorf("expected worker-1, got %s", workerID)
			}
			if errMsg != "step timed out" {
				t.Errorf("expected 'step timed out', got %s", errMsg)
			}
			return nil
		},
	}

	logic := NewWorkerLogic(repo)
	err := logic.FailStep(context.Background(), "  exec-123  ", "  worker-1  ", "step timed out")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected failFunc to be called")
	}

	// Empty error message defaults to "unknown error"
	repo.failFunc = func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
		if errMsg != "unknown error" {
			t.Errorf("expected 'unknown error', got %s", errMsg)
		}
		return nil
	}
	_ = logic.FailStep(context.Background(), "exec-123", "worker-1", "   ")
}

func TestWorkerLogic_FailStep_LockLost(t *testing.T) {
	repo := &mockStepRepository{
		failFunc: func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
			return step.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo)
	err := logic.FailStep(context.Background(), "exec-123", "worker-1", "error")
	if !errors.Is(err, step.ErrLockLost) {
		t.Fatalf("expected ErrLockLost, got %v", err)
	}
}


