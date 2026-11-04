package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/worker"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type mockExecutionRepository struct {
	claimTaskFunc        func(ctx context.Context, filter execution.ClaimFilter) (*execution.ClaimResult, error)
	heartbeatTaskFunc    func(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, duration time.Duration) (time.Time, time.Duration, error)
	completeTaskFunc     func(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, output json.RawMessage) error
	failTaskFunc         func(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, errMsg string, retryable bool) error
	getStepTaskQueueFunc func(ctx context.Context, stepExecID string) (string, error)
}

func (m *mockExecutionRepository) ClaimTask(ctx context.Context, filter execution.ClaimFilter) (*execution.ClaimResult, error) {
	if m.claimTaskFunc != nil {
		return m.claimTaskFunc(ctx, filter)
	}
	return nil, nil
}

func (m *mockExecutionRepository) HeartbeatTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, duration time.Duration) (time.Time, time.Duration, error) {
	if m.heartbeatTaskFunc != nil {
		return m.heartbeatTaskFunc(ctx, serviceID, stepExecID, workerID, leaseToken, duration)
	}
	return time.Now().Add(duration), duration, nil
}

func (m *mockExecutionRepository) CompleteTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, output json.RawMessage) error {
	if m.completeTaskFunc != nil {
		return m.completeTaskFunc(ctx, serviceID, stepExecID, workerID, leaseToken, output)
	}
	return nil
}

func (m *mockExecutionRepository) FailTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, errMsg string, retryable bool) error {
	if m.failTaskFunc != nil {
		return m.failTaskFunc(ctx, serviceID, stepExecID, workerID, leaseToken, errMsg, retryable)
	}
	return nil
}

func (m *mockExecutionRepository) GetStepTaskQueue(ctx context.Context, stepExecID string) (string, error) {
	if m.getStepTaskQueueFunc != nil {
		return m.getStepTaskQueueFunc(ctx, stepExecID)
	}
	return "legacy", nil
}

func (m *mockExecutionRepository) ClaimPending(ctx context.Context, workerID string, leaseDuration time.Duration) (*step.Step, *execution.StepExecution, error) {
	return nil, nil, nil
}

func (m *mockExecutionRepository) Heartbeat(ctx context.Context, stepExecutionID string, workerID string, leaseDuration time.Duration) error {
	return nil
}

func (m *mockExecutionRepository) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	return nil
}

func (m *mockExecutionRepository) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	return nil
}

func TestQueueNotifier(t *testing.T) {
	notifier := worker.NewQueueNotifier()
	ch1, cancel1 := notifier.Subscribe("orders")
	defer cancel1()
	ch2, cancel2 := notifier.Subscribe("orders")
	defer cancel2()
	chOther, cancelOther := notifier.Subscribe("payments")
	defer cancelOther()

	notifier.Notify("orders")

	select {
	case <-ch1:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected ch1 to receive notification")
	}

	select {
	case <-ch2:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected ch2 to receive notification")
	}

	select {
	case <-chOther:
		t.Fatalf("expected chOther NOT to receive notification")
	default:
	}

	cancel1()
	notifier.Notify("orders")

	select {
	case <-ch2:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected ch2 to receive notification after cancel1")
	}
}

func TestWorkerLogic_ClaimTask_ValidationAndImmediate(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	logic := worker.NewWorkerLogic(repo, 30*time.Second, 25*time.Second, notifier)

	// Empty worker ID
	_, err := logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{})
	if !errors.Is(err, errdefs.ErrEmptyWorkerID) {
		t.Fatalf("expected ErrEmptyWorkerID, got %v", err)
	}

	// Negative wait timeout
	neg := -5
	_, err = logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{
		WorkerID:      "w-1",
		WaitTimeoutMS: &neg,
	})
	if !errors.Is(err, errdefs.ErrInvalidWaitTimeout) {
		t.Fatalf("expected ErrInvalidWaitTimeout for negative timeout, got %v", err)
	}

	// Timeout exceeding maximum
	tooLong := 30000
	_, err = logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{
		WorkerID:      "w-1",
		WaitTimeoutMS: &tooLong,
	})
	if !errors.Is(err, errdefs.ErrInvalidWaitTimeout) {
		t.Fatalf("expected ErrInvalidWaitTimeout for timeout > max, got %v", err)
	}

	// Immediate claim with 0ms timeout returning no task
	zero := 0
	res, err := logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{
		WorkerID:      "w-1",
		WaitTimeoutMS: &zero,
	})
	if err != nil || res != nil {
		t.Fatalf("expected nil result on immediate empty claim, got res=%v, err=%v", res, err)
	}

	// Immediate claim with task found
	actName := "order.create"
	repo.claimTaskFunc = func(ctx context.Context, filter execution.ClaimFilter) (*execution.ClaimResult, error) {
		return &execution.ClaimResult{
			StepDefinition: &step.Step{ID: "step-1", Type: step.StepTypeActivity, Config: json.RawMessage(`{"amount":100}`)},
			StepExecution: &execution.StepExecution{
				ID:                  "exec-step-1",
				WorkflowExecutionID: "wf-exec-1",
				StepID:              "step-1",
				TaskQueue:           "orders",
				ActivityName:        &actName,
				Attempt:             1,
			},
			LeaseToken:     "test-token",
			LeaseExpiresAt: time.Now().Add(30 * time.Second),
			LeaseDuration:  30 * time.Second,
		}, nil
	}

	res, err = logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{
		WorkerID:      "w-1",
		Queue:         "orders",
		WaitTimeoutMS: &zero,
	})
	if err != nil {
		t.Fatalf("expected successful claim, got err=%v", err)
	}
	if res.TaskID != "exec-step-1" || res.ActivityName != "order.create" {
		t.Fatalf("unexpected claim result: %+v", res)
	}
}

func TestWorkerLogic_ClaimTask_WakeupViaNotifier(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	logic := worker.NewWorkerLogic(repo, 30*time.Second, 25*time.Second, notifier)

	var callCount int
	repo.claimTaskFunc = func(ctx context.Context, filter execution.ClaimFilter) (*execution.ClaimResult, error) {
		callCount++
		if callCount == 1 {
			return nil, nil // First attempt returns nothing
		}
		return &execution.ClaimResult{
			StepDefinition: &step.Step{ID: "step-1", Type: step.StepTypeActivity, Config: json.RawMessage(`{}`)},
			StepExecution: &execution.StepExecution{
				ID:                  "exec-step-1",
				WorkflowExecutionID: "wf-exec-1",
				StepID:              "step-1",
				TaskQueue:           "orders",
				Attempt:             1,
			},
			LeaseToken:     "token-abc",
			LeaseExpiresAt: time.Now().Add(30 * time.Second),
			LeaseDuration:  30 * time.Second,
		}, nil
	}

	waitMS := 5000
	done := make(chan *worker.ClaimTaskResponse)
	go func() {
		res, _ := logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{
			WorkerID:      "w-1",
			Queue:         "orders",
			WaitTimeoutMS: &waitMS,
		})
		done <- res
	}()

	// Give goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)
	// Trigger wake-up notification
	notifier.Notify("orders")

	select {
	case res := <-done:
		if res == nil || res.TaskID != "exec-step-1" {
			t.Fatalf("expected claim result on notification wakeup, got %+v", res)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for task claim after notifier wake-up")
	}
}

func TestWorkerLogic_ClaimTask_OmittedWaitTimeout_CappedByConfig(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	cfg := &config.Config{
		WorkerLongPollTimeout: 50 * time.Millisecond,
	}
	logic := worker.NewWorkerLogic(repo, 30*time.Second, cfg, notifier)

	start := time.Now()
	res, err := logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{
		WorkerID: "w-1",
		Queue:    "orders",
		// WaitTimeoutMS is omitted!
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
	// Should return within ~50ms, NOT 25 seconds!
	if elapsed > 500*time.Millisecond {
		t.Fatalf("claim waited %v; expected to be capped near 50ms", elapsed)
	}
}

func TestWorkerHandler_HTTPProtocol(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	logic := worker.NewWorkerLogic(repo, 30*time.Second, 25*time.Second, notifier)

	cfg := &config.Config{
		WorkerAPIEnabled: true,
		WorkerCredentials: []config.WorkerCredential{
			{
				ID:     "srv-payment",
				Token:  "token-secret-123",
				Queues: []string{"payments"},
			},
		},
		WorkerLongPollTimeout: 25 * time.Second,
	}

	handler := worker.NewWorkerHandler(logic, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /worker/tasks/claim", handler.Claim)
	mux.HandleFunc("POST /worker/tasks/{id}/heartbeat", handler.Heartbeat)
	mux.HandleFunc("POST /worker/tasks/{id}/complete", handler.Complete)
	mux.HandleFunc("POST /worker/tasks/{id}/fail", handler.Fail)

	server := httptest.NewServer(mux)
	defer server.Close()

	// 1. Unauthorized request
	clientUnauth := worker.NewTestWorkerClient(server.URL, "wrong-token", "w1", "payments")
	_, err := clientUnauth.Claim(context.Background(), nil, nil, 0)
	if err == nil {
		t.Fatalf("expected unauthorized error with wrong token")
	}

	// 2. Forbidden queue
	clientForbidden := worker.NewTestWorkerClient(server.URL, "token-secret-123", "w1", "other-queue")
	_, err = clientForbidden.Claim(context.Background(), nil, nil, 0)
	if err == nil {
		t.Fatalf("expected forbidden queue error")
	}

	// 3. Authorized client: Claim -> Heartbeat -> Complete
	client := worker.NewTestWorkerClient(server.URL, "token-secret-123", "w1", "payments")

	repo.claimTaskFunc = func(ctx context.Context, filter execution.ClaimFilter) (*execution.ClaimResult, error) {
		return &execution.ClaimResult{
			StepDefinition: &step.Step{ID: "pay-step", Type: step.StepTypeActivity, Config: json.RawMessage(`{"charge":50}`)},
			StepExecution: &execution.StepExecution{
				ID:                  "step-exec-99",
				WorkflowExecutionID: "wf-exec-99",
				StepID:              "pay-step",
				TaskQueue:           "payments",
				Attempt:             1,
			},
			LeaseToken:     "tok-99",
			LeaseExpiresAt: time.Now().Add(30 * time.Second),
			LeaseDuration:  30 * time.Second,
		}, nil
	}
	repo.getStepTaskQueueFunc = func(ctx context.Context, stepExecID string) (string, error) {
		return "payments", nil
	}

	task, err := client.Claim(context.Background(), []string{"activity"}, nil, 0)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if task.TaskID != "step-exec-99" {
		t.Fatalf("unexpected task id: %s", task.TaskID)
	}

	hb, err := client.Heartbeat(context.Background(), task.TaskID, task.LeaseToken, 30000)
	if err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}
	if hb.LeaseDurationMS != 30000 {
		t.Fatalf("unexpected heartbeat duration: %d", hb.LeaseDurationMS)
	}

	err = client.Complete(context.Background(), task.TaskID, task.LeaseToken, json.RawMessage(`{"status":"paid"}`))
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// 4. Heartbeat after complete returns 409 Conflict (LOCK_LOST)
	repo.heartbeatTaskFunc = func(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, duration time.Duration) (time.Time, time.Duration, error) {
		return time.Time{}, 0, errdefs.ErrLockLost
	}
	_, err = client.Heartbeat(context.Background(), task.TaskID, task.LeaseToken, 30000)
	if err == nil {
		t.Fatalf("expected error from heartbeat after complete")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Fatalf("expected 409 Conflict, got: %v", err)
	}

	// 5. Fail cycle
	err = client.Fail(context.Background(), task.TaskID, task.LeaseToken, "failed test", false)
	if err != nil {
		t.Fatalf("fail step failed: %v", err)
	}
}

func TestWorkerLogic_FailTask_RetryableDefault(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	logic := worker.NewWorkerLogic(repo, 30*time.Second, 25*time.Second, notifier)

	var recordedRetryable bool
	repo.failTaskFunc = func(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, errMsg string, retryable bool) error {
		recordedRetryable = retryable
		return nil
	}

	// 1. Omitted retryable (nil) -> defaults to true
	_, err := logic.FailTask(context.Background(), "srv-1", "step-1", worker.FailRequest{
		WorkerID:   "w-1",
		LeaseToken: "tok-1",
		Error: worker.TaskError{
			Message:   "some error",
			Retryable: nil,
		},
	})
	if err != nil {
		t.Fatalf("unexpected fail task error: %v", err)
	}
	if !recordedRetryable {
		t.Fatalf("expected omitted retryable to default to true, got false")
	}

	// 2. Explicit false -> passes false
	f := false
	_, err = logic.FailTask(context.Background(), "srv-1", "step-1", worker.FailRequest{
		WorkerID:   "w-1",
		LeaseToken: "tok-1",
		Error: worker.TaskError{
			Message:   "some error",
			Retryable: &f,
		},
	})
	if err != nil {
		t.Fatalf("unexpected fail task error: %v", err)
	}
	if recordedRetryable {
		t.Fatalf("expected explicit false retryable to be false, got true")
	}
}

func TestWorkerLogic_ClaimTask_ContractFields(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	cfg := &config.Config{
		DefaultTimeout:           45 * time.Second,
		DefaultHeartBeatInterval: 10 * time.Second,
		WorkerLongPollTimeout:    25 * time.Second,
	}
	logic := worker.NewWorkerLogic(repo, 30*time.Second, 25*time.Second, notifier, cfg)

	deadline := time.Now().Add(60 * time.Second)
	repo.claimTaskFunc = func(ctx context.Context, filter execution.ClaimFilter) (*execution.ClaimResult, error) {
		if filter.DefaultTimeout != 45*time.Second {
			t.Fatalf("expected filter.DefaultTimeout to be 45s, got %v", filter.DefaultTimeout)
		}
		return &execution.ClaimResult{
			StepDefinition: &step.Step{ID: "step-comp", Type: step.StepTypeActivity, Config: json.RawMessage(`{}`)},
			StepExecution: &execution.StepExecution{
				ID:                  "exec-step-comp-1",
				WorkflowExecutionID: "wf-exec-1",
				StepID:              "step-comp",
				TaskQueue:           "orders",
				IsCompensation:      true,
				TimeoutSeconds:      60,
				Attempt:             2,
			},
			LeaseToken:     "test-token-comp",
			LeaseExpiresAt: time.Now().Add(30 * time.Second),
			LeaseDuration:  30 * time.Second,
			Deadline:       deadline,
		}, nil
	}

	zero := 0
	res, err := logic.ClaimTask(context.Background(), "srv-1", worker.ClaimRequest{
		WorkerID:      "w-1",
		Queue:         "orders",
		WaitTimeoutMS: &zero,
	})
	if err != nil {
		t.Fatalf("unexpected claim error: %v", err)
	}
	if res.StepExecutionID != "exec-step-comp-1" || res.TaskID != "exec-step-comp-1" {
		t.Fatalf("expected StepExecutionID and TaskID to be exec-step-comp-1, got %s, %s", res.StepExecutionID, res.TaskID)
	}
	if !res.IsCompensation {
		t.Fatalf("expected IsCompensation to be true")
	}
	if res.TimeoutSeconds != 60 {
		t.Fatalf("expected TimeoutSeconds to be 60, got %d", res.TimeoutSeconds)
	}
	if res.HeartbeatIntervalMS != 10000 {
		t.Fatalf("expected HeartbeatIntervalMS to be 10000, got %d", res.HeartbeatIntervalMS)
	}
	if res.Deadline != deadline.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("expected Deadline to match %s, got %s", deadline.UTC().Format(time.RFC3339Nano), res.Deadline)
	}
}

func TestWorkerHandler_PayloadLimit(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	logic := worker.NewWorkerLogic(repo, 30*time.Second, 25*time.Second, notifier)

	cfg := &config.Config{
		WorkerAPIEnabled: true,
		WorkerCredentials: []config.WorkerCredential{
			{
				ID:     "srv-1",
				Token:  "valid-token",
				Queues: []string{"test-queue"},
			},
		},
		MaxPayloadSize: 1, // 1 MB
	}
	handler := worker.NewWorkerHandler(logic, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /worker/tasks/claim", handler.Claim)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Send 2MB payload (> 1MB limit)
	largeData := make([]byte, 2*1024*1024)
	req, _ := http.NewRequest("POST", server.URL+"/worker/tasks/claim", bytes.NewReader(largeData))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413 (Payload Too Large), got %d", resp.StatusCode)
	}
}

func TestWorkerHandler_DisabledAPI(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	logic := worker.NewWorkerLogic(repo, 30*time.Second, 25*time.Second, notifier)

	cfg := &config.Config{
		WorkerAPIEnabled: false,
	}
	handler := worker.NewWorkerHandler(logic, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /worker/tasks/claim", handler.Claim)
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/worker/tasks/claim", nil)
	req.Header.Set("Authorization", "Bearer any-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status 403 (Forbidden), got %d", resp.StatusCode)
	}
}

func TestWorkerLogic_HeartbeatDurationCap(t *testing.T) {
	repo := &mockExecutionRepository{}
	notifier := worker.NewQueueNotifier()
	backendLease := 30 * time.Second
	logic := worker.NewWorkerLogic(repo, backendLease, 25*time.Second, notifier)

	var receivedDuration time.Duration
	repo.heartbeatTaskFunc = func(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, duration time.Duration) (time.Time, time.Duration, error) {
		receivedDuration = duration
		return time.Now().Add(duration), duration, nil
	}

	// 1. Worker requests 1 hour (> backend 30s) -> capped at backend lease (30s)
	res, err := logic.HeartbeatTask(context.Background(), "srv-1", "step-1", worker.HeartbeatRequest{
		WorkerID:        "w-1",
		LeaseToken:      "tok-1",
		LeaseDurationMS: 3600000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedDuration != backendLease {
		t.Fatalf("expected repo to receive backendLease (30s), got %v", receivedDuration)
	}
	if res.LeaseDurationMS != 30000 {
		t.Fatalf("expected response LeaseDurationMS to be 30000, got %d", res.LeaseDurationMS)
	}

	// 2. Worker requests 10 seconds (< backend 30s) -> allowed to be 10s
	res, err = logic.HeartbeatTask(context.Background(), "srv-1", "step-1", worker.HeartbeatRequest{
		WorkerID:        "w-1",
		LeaseToken:      "tok-1",
		LeaseDurationMS: 10000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedDuration != 10*time.Second {
		t.Fatalf("expected repo to receive 10s, got %v", receivedDuration)
	}
	if res.LeaseDurationMS != 10000 {
		t.Fatalf("expected response LeaseDurationMS to be 10000, got %d", res.LeaseDurationMS)
	}

	// 3. Worker requests 0 -> defaults to backend 30s
	res, err = logic.HeartbeatTask(context.Background(), "srv-1", "step-1", worker.HeartbeatRequest{
		WorkerID:        "w-1",
		LeaseToken:      "tok-1",
		LeaseDurationMS: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedDuration != backendLease {
		t.Fatalf("expected repo to receive backendLease (30s), got %v", receivedDuration)
	}
}

