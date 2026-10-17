package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/executor"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
)

const testLockDuration = 30 * time.Second

type mockExecutionRepository struct {
	claimFunc     func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error)
	heartbeatFunc func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error
	completeFunc  func(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error
	failFunc      func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error
}

func (m *mockExecutionRepository) ExecuteWorkflowTx(ctx context.Context, name string, initialContext json.RawMessage) (*execution.WorkflowExecution, error) {
	return nil, nil
}

func (m *mockExecutionRepository) ListExecutions(ctx context.Context, limit, offset int) ([]execution.WorkflowExecution, error) {
	return nil, nil
}

func (m *mockExecutionRepository) GetExecution(ctx context.Context, executionID string) (*execution.WorkflowExecution, error) {
	return nil, nil
}

func (m *mockExecutionRepository) ClaimPending(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
	if m.claimFunc != nil {
		return m.claimFunc(ctx, workerID, lockDuration)
	}
	return nil, nil, nil
}

func (m *mockExecutionRepository) ReapExpiredSteps(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockExecutionRepository) WakeUpDependentsTx(ctx context.Context, tx *sql.Tx, workflowExecID string, isCompensation bool) error {
	return nil
}

func (m *mockExecutionRepository) Heartbeat(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
	if m.heartbeatFunc != nil {
		return m.heartbeatFunc(ctx, stepExecutionID, workerID, lockDuration)
	}
	return nil
}

func (m *mockExecutionRepository) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, stepExecutionID, workerID, output)
	}
	return nil
}

func (m *mockExecutionRepository) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	if m.failFunc != nil {
		return m.failFunc(ctx, stepExecutionID, workerID, errMsg)
	}
	return nil
}

func TestWorkerLogic_ClaimNext_EmptyWorkerID(t *testing.T) {
	logic := NewWorkerLogic(&mockExecutionRepository{}, testLockDuration)

	_, _, err := logic.ClaimNext(context.Background(), "")
	if err != errdefs.ErrEmptyWorkerID {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}

	_, _, err = logic.ClaimNext(context.Background(), "   ")
	if err != errdefs.ErrEmptyWorkerID {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}
}

func TestWorkerLogic_ClaimNext_Success(t *testing.T) {
	expectedStep := &step.Step{
		ID:   "step-1",
		Type: step.StepTypeHTTP,
	}
	expectedExec := &execution.StepExecution{
		ID:      "exec-999",
		StepID:  "step-1",
		Status:  execution.StatusRunning,
		Attempt: 1,
	}

	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			if workerID != "worker-1" {
				t.Errorf("expected workerID 'worker-1', got %s", workerID)
			}
			if lockDuration != 30*time.Second {
				t.Errorf("expected lockDuration %v, got %v", 30*time.Second, lockDuration)
			}
			return expectedStep, expectedExec, nil
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	s, se, err := logic.ClaimNext(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil || s.ID != "step-1" {
		t.Errorf("expected step-1, got %+v", s)
	}
	if se == nil || se.ID != "exec-999" || se.Status != execution.StatusRunning {
		t.Errorf("expected exec-999 in RUNNING state, got %+v", se)
	}
}

func TestWorkerLogic_ClaimNext_NoTaskAvailable(t *testing.T) {
	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			return nil, nil, nil
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	s, se, err := logic.ClaimNext(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != nil || se != nil {
		t.Fatalf("expected nil step and exec when queue empty, got s=%+v, se=%+v", s, se)
	}
}

func TestWorkerLogic_Heartbeat_Validation(t *testing.T) {
	logic := NewWorkerLogic(&mockExecutionRepository{}, testLockDuration)

	// Empty stepExecutionID
	err := logic.Heartbeat(context.Background(), "", "worker-1")
	if !errors.Is(err, errdefs.ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID, got %v", err)
	}
	err = logic.Heartbeat(context.Background(), "   ", "worker-1")
	if !errors.Is(err, errdefs.ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID for whitespace, got %v", err)
	}

	// Empty workerID
	err = logic.Heartbeat(context.Background(), "exec-1", "")
	if !errors.Is(err, errdefs.ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}
	err = logic.Heartbeat(context.Background(), "exec-1", "   ")
	if !errors.Is(err, errdefs.ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}
}

func TestWorkerLogic_Heartbeat_Success(t *testing.T) {
	called := false
	repo := &mockExecutionRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			called = true
			if stepExecutionID != "exec-123" {
				t.Errorf("expected exec-123, got %s", stepExecutionID)
			}
			if workerID != "worker-1" {
				t.Errorf("expected worker-1, got %s", workerID)
			}
			if lockDuration != 30*time.Second {
				t.Errorf("expected lockDuration %v, got %v", 30*time.Second, lockDuration)
			}
			return nil
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	err := logic.Heartbeat(context.Background(), "  exec-123  ", "  worker-1  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected repo Heartbeat to be called")
	}
}

func TestWorkerLogic_Heartbeat_LockLost(t *testing.T) {
	repo := &mockExecutionRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			return errdefs.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	err := logic.Heartbeat(context.Background(), "exec-123", "worker-1")
	if !errors.Is(err, errdefs.ErrLockLost) {
		t.Fatalf("expected ErrLockLost, got %v", err)
	}
}

func TestWorkerLogic_StartHeartbeat_TicksAndStops(t *testing.T) {
	var count int32
	repo := &mockExecutionRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
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
	repo := &mockExecutionRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			return errdefs.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	stop, errChan := logic.StartHeartbeat(context.Background(), "exec-1", "worker-1", 10*time.Millisecond)
	defer stop()

	select {
	case err := <-errChan:
		if !errors.Is(err, errdefs.ErrLockLost) {
			t.Fatalf("expected errdefs.ErrLockLost, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for error on errChan")
	}
}

func TestWorkerLogic_StartHeartbeat_ContextCancelled(t *testing.T) {
	repo := &mockExecutionRepository{
		heartbeatFunc: func(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
			return nil
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	ctx, cancel := context.WithCancel(context.Background())

	stop, _ := logic.StartHeartbeat(ctx, "exec-1", "worker-1", 10*time.Millisecond)
	defer stop()

	// Cancel context immediately
	cancel()
	time.Sleep(30 * time.Millisecond)
}

func TestWorkerLogic_CompleteStep_Validation(t *testing.T) {
	logic := NewWorkerLogic(&mockExecutionRepository{}, testLockDuration)

	// Empty stepExecutionID
	err := logic.CompleteStep(context.Background(), "", "worker-1", json.RawMessage("{}"))
	if !errors.Is(err, errdefs.ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID, got %v", err)
	}
	err = logic.CompleteStep(context.Background(), "   ", "worker-1", json.RawMessage("{}"))
	if !errors.Is(err, errdefs.ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID for whitespace, got %v", err)
	}

	// Empty workerID
	err = logic.CompleteStep(context.Background(), "exec-1", "", json.RawMessage("{}"))
	if !errors.Is(err, errdefs.ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}
	err = logic.CompleteStep(context.Background(), "exec-1", "   ", json.RawMessage("{}"))
	if !errors.Is(err, errdefs.ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}

	// Invalid JSON output
	err = logic.CompleteStep(context.Background(), "exec-1", "worker-1", json.RawMessage("{invalid-json"))
	if !errors.Is(err, errdefs.ErrInvalidStepOutput) {
		t.Fatalf("expected ErrInvalidStepOutput, got %v", err)
	}
}

func TestWorkerLogic_CompleteStep_Success(t *testing.T) {
	called := false
	repo := &mockExecutionRepository{
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

	logic := NewWorkerLogic(repo, testLockDuration)
	err := logic.CompleteStep(context.Background(), "  exec-123  ", "  worker-1  ", json.RawMessage(`{"status":"ok"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected completeFunc to be called")
	}
}

func TestWorkerLogic_CompleteStep_LockLost(t *testing.T) {
	repo := &mockExecutionRepository{
		completeFunc: func(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
			return errdefs.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	err := logic.CompleteStep(context.Background(), "exec-123", "worker-1", json.RawMessage("{}"))
	if !errors.Is(err, errdefs.ErrLockLost) {
		t.Fatalf("expected ErrLockLost, got %v", err)
	}
}

func TestWorkerLogic_FailStep_Validation(t *testing.T) {
	logic := NewWorkerLogic(&mockExecutionRepository{}, testLockDuration)

	// Empty stepExecutionID
	err := logic.FailStep(context.Background(), "", "worker-1", "some error")
	if !errors.Is(err, errdefs.ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID, got %v", err)
	}
	err = logic.FailStep(context.Background(), "   ", "worker-1", "some error")
	if !errors.Is(err, errdefs.ErrEmptyStepExecutionID) {
		t.Fatalf("expected ErrEmptyStepExecutionID for whitespace, got %v", err)
	}

	// Empty workerID
	err = logic.FailStep(context.Background(), "exec-1", "", "some error")
	if !errors.Is(err, errdefs.ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}
	err = logic.FailStep(context.Background(), "exec-1", "   ", "some error")
	if !errors.Is(err, errdefs.ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID for whitespace, got %v", err)
	}
}

func TestWorkerLogic_FailStep_Success(t *testing.T) {
	called := false
	repo := &mockExecutionRepository{
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

	logic := NewWorkerLogic(repo, testLockDuration)
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
	repo := &mockExecutionRepository{
		failFunc: func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
			return errdefs.ErrLockLost
		},
	}

	logic := NewWorkerLogic(repo, testLockDuration)
	err := logic.FailStep(context.Background(), "exec-123", "worker-1", "error")
	if !errors.Is(err, errdefs.ErrLockLost) {
		t.Fatalf("expected ErrLockLost, got %v", err)
	}
}

type mockExecutor struct {
	executeFn func(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error)
}

func (m *mockExecutor) Execute(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, s, exec)
	}
	return json.RawMessage(`{"status":"ok"}`), nil
}

func TestWorkerPool_ProcessOne_Success(t *testing.T) {
	completed := false
	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			return &step.Step{
					ID:   "step-1",
					Type: step.StepTypeHTTP,
				}, &execution.StepExecution{
					ID:     "exec-1",
					StepID: "step-1",
					Type:   string(step.StepTypeHTTP),
				}, nil
		},
		completeFunc: func(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
			completed = true
			if stepExecutionID != "exec-1" {
				t.Errorf("expected exec-1, got %s", stepExecutionID)
			}
			if workerID != "test-worker-1" {
				t.Errorf("expected test-worker-1, got %s", workerID)
			}
			if string(output) != `{"result":"success"}` {
				t.Errorf("unexpected output: %s", string(output))
			}
			return nil
		},
	}

	reg := executor.NewRegistry()
	reg.Register(step.StepTypeHTTP, &mockExecutor{
		executeFn: func(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error) {
			return json.RawMessage(`{"result":"success"}`), nil
		},
	})

	logic := NewWorkerLogic(repo, testLockDuration)
	pool := NewWorkerPool(logic, reg, WithBaseID("test-worker"))

	processed, err := pool.ProcessOne(context.Background(), "test-worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processed {
		t.Fatalf("expected task to be processed")
	}
	if !completed {
		t.Fatalf("expected task to be marked completed")
	}
}

func TestWorkerPool_ProcessOne_Failure(t *testing.T) {
	failed := false
	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			return &step.Step{
					ID:   "step-1",
					Type: step.StepTypeHTTP,
				}, &execution.StepExecution{
					ID:   "exec-1",
					Type: string(step.StepTypeHTTP),
				}, nil
		},
		failFunc: func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
			failed = true
			if errMsg != "network error: connection refused" {
				t.Errorf("unexpected errMsg: %s", errMsg)
			}
			return nil
		},
	}

	reg := executor.NewRegistry()
	reg.Register(step.StepTypeHTTP, &mockExecutor{
		executeFn: func(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error) {
			return nil, errors.New("network error: connection refused")
		},
	})

	pool := NewWorkerPool(NewWorkerLogic(repo, testLockDuration), reg)
	processed, err := pool.ProcessOne(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processed || !failed {
		t.Fatalf("expected task to be processed and failed")
	}
}

func TestWorkerPool_ProcessOne_PanicRecovery(t *testing.T) {
	failed := false
	var capturedErr string
	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			return &step.Step{
					ID:   "step-panic",
					Type: step.StepTypeHTTP,
				}, &execution.StepExecution{
					ID:   "exec-panic",
					Type: string(step.StepTypeHTTP),
				}, nil
		},
		failFunc: func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
			failed = true
			capturedErr = errMsg
			return nil
		},
	}

	reg := executor.NewRegistry()
	reg.Register(step.StepTypeHTTP, &mockExecutor{
		executeFn: func(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error) {
			panic("unexpected nil pointer in plugin")
		},
	})

	pool := NewWorkerPool(NewWorkerLogic(repo, testLockDuration), reg)
	processed, err := pool.ProcessOne(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processed || !failed {
		t.Fatalf("expected task to be processed and failed")
	}
	if capturedErr != "panic in step execution: unexpected nil pointer in plugin" {
		t.Errorf("unexpected panic capture message: %s", capturedErr)
	}
}

func TestWorkerPool_ProcessOne_UnsupportedType(t *testing.T) {
	failed := false
	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			return &step.Step{
					ID:   "step-kafka",
					Type: step.StepType("kafka"),
				}, &execution.StepExecution{
					ID:   "exec-kafka",
					Type: "kafka",
				}, nil
		},
		failFunc: func(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
			failed = true
			return nil
		},
	}

	// Registry is empty (kafka is not registered)
	reg := executor.NewRegistry()
	pool := NewWorkerPool(NewWorkerLogic(repo, testLockDuration), reg)

	processed, err := pool.ProcessOne(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processed || !failed {
		t.Fatalf("expected task to be processed and failed for unsupported type")
	}
}

func TestWorkerPool_Start_Fixed3Concurrency(t *testing.T) {
	var (
		worker1Called int32
		worker2Called int32
		worker3Called int32
	)

	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			switch workerID {
			case "test-fixed-1":
				atomic.AddInt32(&worker1Called, 1)
			case "test-fixed-2":
				atomic.AddInt32(&worker2Called, 1)
			case "test-fixed-3":
				atomic.AddInt32(&worker3Called, 1)
			}
			return nil, nil, nil // Queue empty
		},
	}

	reg := executor.NewRegistry()
	pool := NewWorkerPool(
		NewWorkerLogic(repo, testLockDuration),
		reg,
		WithBaseID("test-fixed"),
		WithWorkerCount(3),
		WithPollInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = pool.Start(ctx)
	}()

	// Allow workers to poll several times
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Verify all 3 fixed workers actively polled
	w1 := atomic.LoadInt32(&worker1Called)
	w2 := atomic.LoadInt32(&worker2Called)
	w3 := atomic.LoadInt32(&worker3Called)

	if w1 == 0 || w2 == 0 || w3 == 0 {
		t.Errorf("expected all 3 workers to poll, got w1=%d, w2=%d, w3=%d", w1, w2, w3)
	}
}

func TestWorkerPool_Start_GracefulShutdown(t *testing.T) {
	repo := &mockExecutionRepository{
		claimFunc: func(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
			return nil, nil, nil
		},
	}

	pool := NewWorkerPool(
		NewWorkerLogic(repo, testLockDuration),
		executor.NewRegistry(),
		WithWorkerCount(3),
		WithPollInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- pool.Start(ctx)
	}()

	// Cancel context and verify pool exits cleanly
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("worker pool failed to gracefully shut down within timeout")
	}
}



