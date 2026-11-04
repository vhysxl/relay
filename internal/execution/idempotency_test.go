package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type memoryIdempotencyRepo struct {
	executions       map[string]*execution.WorkflowExecution
	requests         map[string]*execution.ExecutionRequestRecord
	failStepInsert   bool
	simulateRaceWin  bool
	insertHook       func(exec *execution.WorkflowExecution, rec *execution.ExecutionRequestRecord) error
}

func newMemoryIdempotencyRepo() *memoryIdempotencyRepo {
	return &memoryIdempotencyRepo{
		executions: make(map[string]*execution.WorkflowExecution),
		requests:   make(map[string]*execution.ExecutionRequestRecord),
	}
}

func (m *memoryIdempotencyRepo) requestKey(wfName, key string) string {
	return wfName + ":" + key
}

func (m *memoryIdempotencyRepo) CreateExecutionWithIdempotency(
	ctx context.Context,
	exec *execution.WorkflowExecution,
	record *execution.ExecutionRequestRecord,
	targetIsExplicitID bool,
	targetID string,
) (*execution.ExecuteWorkflowResponse, bool, error) {
	if m.insertHook != nil {
		if err := m.insertHook(exec, record); err != nil {
			return nil, false, err
		}
	}
	if m.failStepInsert {
		return nil, false, errors.New("simulated step insert failure")
	}

	key := m.requestKey(record.WorkflowName, record.IdempotencyKey)
	if existing, exists := m.requests[key]; exists {
		// Replay check
		receipt, err := evaluateReplayInMemory(existing, record.InputContext, targetIsExplicitID, targetID)
		if err != nil {
			return nil, false, err
		}
		return receipt, true, nil
	}

	m.executions[exec.ID] = exec
	m.requests[key] = record

	var receipt execution.ExecuteWorkflowResponse
	_ = json.Unmarshal(record.ResponseReceipt, &receipt)
	return &receipt, false, nil
}

func (m *memoryIdempotencyRepo) GetExecutionRequest(ctx context.Context, workflowName string, idempotencyKey string) (*execution.ExecutionRequestRecord, error) {
	key := m.requestKey(workflowName, idempotencyKey)
	if rec, ok := m.requests[key]; ok {
		return rec, nil
	}
	return nil, errdefs.ErrExecutionRequestNotFound
}

func (m *memoryIdempotencyRepo) CreateExecution(ctx context.Context, exec *execution.WorkflowExecution) (*execution.WorkflowExecution, error) {
	m.executions[exec.ID] = exec
	return exec, nil
}

func (m *memoryIdempotencyRepo) ListExecutions(ctx context.Context, limit, offset int) ([]execution.WorkflowExecution, error) {
	res := make([]execution.WorkflowExecution, 0, len(m.executions))
	for _, e := range m.executions {
		res = append(res, *e)
	}
	return res, nil
}

func (m *memoryIdempotencyRepo) GetExecution(ctx context.Context, executionID string) (*execution.WorkflowExecution, error) {
	if e, ok := m.executions[executionID]; ok {
		return e, nil
	}
	return nil, errdefs.ErrExecutionNotFound
}

func evaluateReplayInMemory(record *execution.ExecutionRequestRecord, input json.RawMessage, targetIsExplicitID bool, targetID string) (*execution.ExecuteWorkflowResponse, error) {
	var receipt execution.ExecuteWorkflowResponse
	if err := json.Unmarshal(record.ResponseReceipt, &receipt); err != nil {
		return nil, err
	}
	if targetIsExplicitID && targetID != receipt.WorkflowID {
		return nil, errdefs.ErrIdempotencyConflict
	}
	return &receipt, nil
}

type multiVersionWorkflowReader struct {
	workflows []*workflow.Workflow
}

func (r *multiVersionWorkflowReader) GetWorkflow(ctx context.Context, name string) (*workflow.Workflow, error) {
	// Search strictly by workflow name, returning latest version
	var latest *workflow.Workflow
	for _, wf := range r.workflows {
		if wf.Name == name {
			if latest == nil || wf.Version > latest.Version {
				latest = wf
			}
		}
	}
	if latest != nil {
		return latest, nil
	}
	return nil, errdefs.ErrWorkflowNotFound
}

func (r *multiVersionWorkflowReader) GetWorkflowVersion(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
	for _, wf := range r.workflows {
		if wf.Name == name && wf.Version == version {
			return wf, nil
		}
	}
	return nil, errdefs.ErrWorkflowNotFound
}

func TestIdempotency_HeaderAndKeyValidation(t *testing.T) {
	repo := newMemoryIdempotencyRepo()
	reader := &multiVersionWorkflowReader{
		workflows: []*workflow.Workflow{
			{ID: "wf-id-1", Name: "wf-test", Version: 1, Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}}},
		},
	}
	logic := execution.NewExecutionLogic(repo, reader)
	handler := execution.NewExecutionHandler(logic, &config.Config{})

	tests := []struct {
		name       string
		headers    []string
		wantStatus int
	}{
		{"missing header", nil, 400},
		{"empty string header", []string{""}, 400},
		{"whitespace in header", []string{"idemp key"}, 400},
		{"newline in header", []string{"key\n1"}, 400},
		{"tab in header", []string{"key\t1"}, 400},
		{"repeated header", []string{"key-1", "key-2"}, 400},
		{"over 128 bytes", []string{strings.Repeat("a", 129)}, 400},
		{"valid boundary 1 byte", []string{"x"}, 201},
		{"valid boundary 128 bytes", []string{strings.Repeat("b", 128)}, 201},
		{"valid printable ascii symbols", []string{"ord:123.45-ABC_xyz~!"}, 201},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/workflows/wf-test/execute", strings.NewReader(`{}`))
			r.SetPathValue("name", "wf-test")
			for _, v := range tt.headers {
				r.Header.Add("Idempotency-Key", v)
			}
			w := httptest.NewRecorder()
			handler.ExecuteWorkflow(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestIdempotency_ContextCompatibilityAndNormalization(t *testing.T) {
	repo := newMemoryIdempotencyRepo()
	reader := &multiVersionWorkflowReader{
		workflows: []*workflow.Workflow{
			{
				ID:      "wf-v1",
				Name:    "checkout",
				Version: 1,
				Context: json.RawMessage(`{"currency":"USD"}`),
				Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}},
			},
		},
	}
	logic := execution.NewExecutionLogic(repo, reader)
	handler := execution.NewExecutionHandler(logic, &config.Config{})

	// 1. First execution with wrapped context
	r1 := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"context":{"order_id":"ORD-1","items":["apple"]}}`))
	r1.SetPathValue("name", "checkout")
	r1.Header.Set("Idempotency-Key", "key-ctx-1")
	w1 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w1.Code, w1.Body.String())
	}
	if w1.Header().Get("Idempotency-Replayed") != "false" {
		t.Errorf("expected Idempotency-Replayed: false, got %s", w1.Header().Get("Idempotency-Replayed"))
	}
	var resp1 execution.ExecuteWorkflowResponse
	_ = json.Unmarshal(w1.Body.Bytes(), &resp1)

	// 2. Replay with raw context (equivalent)
	r2 := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"items":["apple"],"order_id":"ORD-1"}`))
	r2.SetPathValue("name", "checkout")
	r2.Header.Set("Idempotency-Key", "key-ctx-1")
	w2 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w2, r2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201 on replay, got %d: %s", w2.Code, w2.Body.String())
	}
	if w2.Header().Get("Idempotency-Replayed") != "true" {
		t.Errorf("expected Idempotency-Replayed: true, got %s", w2.Header().Get("Idempotency-Replayed"))
	}
	var resp2 execution.ExecuteWorkflowResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp1.ID != resp2.ID {
		t.Errorf("expected same execution ID, got %s vs %s", resp1.ID, resp2.ID)
	}

	// 3. Replay with different input -> 409 Conflict
	r3 := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"context":{"order_id":"ORD-DIFFERENT"}}`))
	r3.SetPathValue("name", "checkout")
	r3.Header.Set("Idempotency-Key", "key-ctx-1")
	w3 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w3, r3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on payload mismatch, got %d: %s", w3.Code, w3.Body.String())
	}

	// 4. Large number distinction vs same key
	r4 := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"val":9007199254740992}`))
	r4.SetPathValue("name", "checkout")
	r4.Header.Set("Idempotency-Key", "key-large-num")
	w4 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w4, r4)
	if w4.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w4.Code)
	}

	// 4b. Mutate integer above 2^53 with same key -> 409 Conflict
	r4b := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"val":9007199254740993}`))
	r4b.SetPathValue("name", "checkout")
	r4b.Header.Set("Idempotency-Key", "key-large-num")
	w4b := httptest.NewRecorder()
	handler.ExecuteWorkflow(w4b, r4b)
	if w4b.Code != http.StatusConflict {
		t.Fatalf("expected 409 for large number mismatch, got %d: %s", w4b.Code, w4b.Body.String())
	}

	// 5. Number vs string distinction
	r5 := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"amount":42}`))
	r5.SetPathValue("name", "checkout")
	r5.Header.Set("Idempotency-Key", "key-type-check")
	w5 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w5, r5)
	if w5.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w5.Code)
	}

	r5b := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"amount":"42"}`))
	r5b.SetPathValue("name", "checkout")
	r5b.Header.Set("Idempotency-Key", "key-type-check")
	w5b := httptest.NewRecorder()
	handler.ExecuteWorkflow(w5b, r5b)
	if w5b.Code != http.StatusConflict {
		t.Fatalf("expected 409 for number vs string type mismatch, got %d", w5b.Code)
	}

	// 6. Empty body, null, and {} are equivalent
	r6 := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", nil)
	r6.SetPathValue("name", "checkout")
	r6.Header.Set("Idempotency-Key", "key-empty-check")
	w6 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w6, r6)
	if w6.Code != http.StatusCreated {
		t.Fatalf("expected 201 for empty body, got %d", w6.Code)
	}

	r6b := httptest.NewRequest(http.MethodPost, "/workflows/checkout/execute", strings.NewReader(`{"context":null}`))
	r6b.SetPathValue("name", "checkout")
	r6b.Header.Set("Idempotency-Key", "key-empty-check")
	w6b := httptest.NewRecorder()
	handler.ExecuteWorkflow(w6b, r6b)
	if w6b.Code != http.StatusCreated || w6b.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("expected 201 replay for null context, got status %d replayed %s", w6b.Code, w6b.Header().Get("Idempotency-Replayed"))
	}
}

func TestIdempotency_ReceiptImmutabilityAcrossVersionsAndStatusChanges(t *testing.T) {
	repo := newMemoryIdempotencyRepo()
	wfV1 := &workflow.Workflow{
		ID:      "wf-v1",
		Name:    "order-flow",
		Version: 1,
		Context: json.RawMessage(`{"region":"US"}`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}},
	}
	wfV2 := &workflow.Workflow{
		ID:      "wf-v2",
		Name:    "order-flow",
		Version: 2,
		Context: json.RawMessage(`{"region":"EU","v2":true}`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}, {ID: "s2", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}},
	}
	reader := &multiVersionWorkflowReader{
		workflows: []*workflow.Workflow{wfV1},
	}
	logic := execution.NewExecutionLogic(repo, reader)
	handler := execution.NewExecutionHandler(logic, &config.Config{})

	// 1. Initial execution created on v1
	r1 := httptest.NewRequest(http.MethodPost, "/workflows/order-flow/execute", strings.NewReader(`{"user":"alice"}`))
	r1.SetPathValue("name", "order-flow")
	r1.Header.Set("Idempotency-Key", "key-ver-1")
	w1 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w1.Code)
	}

	var resp1 execution.ExecuteWorkflowResponse
	_ = json.Unmarshal(w1.Body.Bytes(), &resp1)
	if resp1.WorkflowID != "wf-v1" {
		t.Fatalf("expected v1 ID, got %s", resp1.WorkflowID)
	}

	// 2. Mutate execution status in DB to COMPLETED
	exec := repo.executions[resp1.ID]
	exec.Status = execution.StatusCompleted
	exec.Context = json.RawMessage(`{"user":"alice","region":"US","runtime_output":"done"}`)

	// 3. Upgrade workflow to v2
	reader.workflows = append(reader.workflows, wfV2)

	// 4. Replay via workflow name "order-flow"
	r2 := httptest.NewRequest(http.MethodPost, "/workflows/order-flow/execute", strings.NewReader(`{"user":"alice"}`))
	r2.SetPathValue("name", "order-flow")
	r2.Header.Set("Idempotency-Key", "key-ver-1")
	w2 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w2, r2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201 on replay, got %d", w2.Code)
	}
	if w2.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("expected Idempotency-Replayed: true")
	}

	var resp2 execution.ExecuteWorkflowResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	// Receipt is IMMUTABLE: status remains RUNNING from original receipt, workflow_id is wf-v1, not wf-v2
	if resp2.WorkflowID != "wf-v1" {
		t.Errorf("expected immutable receipt workflow_id wf-v1, got %s", resp2.WorkflowID)
	}
	if resp2.Status != "RUNNING" {
		t.Errorf("expected immutable receipt status RUNNING, got %s", resp2.Status)
	}
	if strings.Contains(string(resp2.Context), "runtime_output") {
		t.Errorf("receipt should not leak runtime outputs: %s", string(resp2.Context))
	}

	// 5. Attempt execute targeting explicit version ID wf-v1 or wf-v2 without matching workflow name -> 404 Not Found
	r3 := httptest.NewRequest(http.MethodPost, "/workflows/wf-v1/execute", strings.NewReader(`{"user":"alice"}`))
	r3.SetPathValue("name", "wf-v1")
	r3.Header.Set("Idempotency-Key", "key-ver-1")
	w3 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w3, r3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when executing by ID without matching name, got %d: %s", w3.Code, w3.Body.String())
	}

	r4 := httptest.NewRequest(http.MethodPost, "/workflows/wf-v2/execute", strings.NewReader(`{"user":"alice"}`))
	r4.SetPathValue("name", "wf-v2")
	r4.Header.Set("Idempotency-Key", "key-ver-1")
	w4 := httptest.NewRecorder()
	handler.ExecuteWorkflow(w4, r4)
	if w4.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when executing by ID without matching name, got %d: %s", w4.Code, w4.Body.String())
	}
}

func TestIdempotency_AtomicRollbackOnStepFailure(t *testing.T) {
	repo := newMemoryIdempotencyRepo()
	repo.failStepInsert = true

	reader := &multiVersionWorkflowReader{
		workflows: []*workflow.Workflow{
			{ID: "wf-1", Name: "wf", Version: 1, Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}}},
		},
	}
	logic := execution.NewExecutionLogic(repo, reader)

	// Attempt creation with failing step insert
	_, _, err := logic.ExecuteWorkflow(context.Background(), "wf", &execution.ExecuteWorkflowRequest{
		IdempotencyKey: "key-atomic-fail",
		Context:        json.RawMessage(`{"test":true}`),
	})
	if err == nil {
		t.Fatal("expected failure on step insert, got nil")
	}

	// Key should NOT be consumed in repo
	if len(repo.requests) != 0 {
		t.Fatalf("idempotency record should not be stored on transaction rollback, got %d", len(repo.requests))
	}
	if len(repo.executions) != 0 {
		t.Fatalf("execution should not be stored on rollback, got %d", len(repo.executions))
	}

	// Retry after database recovery succeeds
	repo.failStepInsert = false
	resp, replayed, err := logic.ExecuteWorkflow(context.Background(), "wf", &execution.ExecuteWorkflowRequest{
		IdempotencyKey: "key-atomic-fail",
		Context:        json.RawMessage(`{"test":true}`),
	})
	if err != nil {
		t.Fatalf("expected retry after rollback to succeed, got %v", err)
	}
	if replayed {
		t.Fatalf("expected replayed=false for fresh successful creation after rollback")
	}
	if resp == nil || resp.WorkflowID != "wf-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestIdempotency_SameKeyDifferentWorkflows(t *testing.T) {
	repo := newMemoryIdempotencyRepo()
	reader := &multiVersionWorkflowReader{
		workflows: []*workflow.Workflow{
			{ID: "id-alpha", Name: "wf-alpha", Version: 1, Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}}},
			{ID: "id-beta", Name: "wf-beta", Version: 1, Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}}},
		},
	}
	logic := execution.NewExecutionLogic(repo, reader)

	// Same key on workflow alpha
	respA, replayedA, err := logic.ExecuteWorkflow(context.Background(), "wf-alpha", &execution.ExecuteWorkflowRequest{
		IdempotencyKey: "shared-key-1",
		Context:        json.RawMessage(`{"client":"foo"}`),
	})
	if err != nil || replayedA {
		t.Fatalf("failed to create alpha execution: %v", err)
	}

	// Same key on workflow beta -> should succeed and NOT conflict because scope is workflow_name
	respB, replayedB, err := logic.ExecuteWorkflow(context.Background(), "wf-beta", &execution.ExecuteWorkflowRequest{
		IdempotencyKey: "shared-key-1",
		Context:        json.RawMessage(`{"client":"foo"}`),
	})
	if err != nil || replayedB {
		t.Fatalf("failed to create beta execution with same key: %v", err)
	}

	if respA.ID == respB.ID {
		t.Fatalf("expected distinct executions across workflows for same key")
	}
}

func TestIdempotency_ExplicitWorkflowVersion(t *testing.T) {
	repo := newMemoryIdempotencyRepo()
	reader := &multiVersionWorkflowReader{
		workflows: []*workflow.Workflow{
			{ID: "wf-id-v1", Name: "order-flow", Version: 1, Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}}},
			{ID: "wf-id-v2", Name: "order-flow", Version: 2, Steps: []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}}},
		},
	}
	logic := execution.NewExecutionLogic(repo, reader)

	// Execute with explicit version 1
	resp1, replayed1, err := logic.ExecuteWorkflow(context.Background(), "order-flow", &execution.ExecuteWorkflowRequest{
		IdempotencyKey:  "key-v1",
		WorkflowVersion: 1,
		Context:         json.RawMessage(`{"user":"alice"}`),
	})
	if err != nil || replayed1 {
		t.Fatalf("expected create success, got err=%v replayed=%v", err, replayed1)
	}
	if resp1.WorkflowID != "wf-id-v1" {
		t.Fatalf("expected workflow id wf-id-v1, got %s", resp1.WorkflowID)
	}

	// Replay with explicit version 1 -> succeeds
	resp1Replay, replayed2, err := logic.ExecuteWorkflow(context.Background(), "order-flow", &execution.ExecuteWorkflowRequest{
		IdempotencyKey:  "key-v1",
		WorkflowVersion: 1,
		Context:         json.RawMessage(`{"user":"alice"}`),
	})
	if err != nil || !replayed2 {
		t.Fatalf("expected replay success, got err=%v replayed=%v", err, replayed2)
	}
	if resp1Replay.ID != resp1.ID {
		t.Fatalf("expected same execution ID")
	}

	// Replay with version 2 -> conflict!
	_, _, err = logic.ExecuteWorkflow(context.Background(), "order-flow", &execution.ExecuteWorkflowRequest{
		IdempotencyKey:  "key-v1",
		WorkflowVersion: 2,
		Context:         json.RawMessage(`{"user":"alice"}`),
	})
	if !errors.Is(err, errdefs.ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict when replaying with different version, got %v", err)
	}
}
