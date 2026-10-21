package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type mockRepository struct {
	executeFn        func(ctx context.Context, name string, initialContext json.RawMessage) (*execution.WorkflowExecution, error)
	listExecutionsFn func(ctx context.Context, limit, offset int) ([]execution.WorkflowExecution, error)
	getExecutionFn   func(ctx context.Context, executionID string) (*execution.WorkflowExecution, error)
}

func (m *mockRepository) CreateExecution(ctx context.Context, exec *execution.WorkflowExecution) (*execution.WorkflowExecution, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, exec.WorkflowID, exec.Context)
	}
	return &execution.WorkflowExecution{
		ID:         "exec-123",
		WorkflowID: exec.WorkflowID,
		Status:     execution.StatusRunning,
		Context:    exec.Context,
	}, nil
}

func (m *mockRepository) ListExecutions(ctx context.Context, limit, offset int) ([]execution.WorkflowExecution, error) {
	if m.listExecutionsFn != nil {
		return m.listExecutionsFn(ctx, limit, offset)
	}
	return []execution.WorkflowExecution{}, nil
}

func (m *mockRepository) GetExecution(ctx context.Context, executionID string) (*execution.WorkflowExecution, error) {
	if m.getExecutionFn != nil {
		return m.getExecutionFn(ctx, executionID)
	}
	return nil, errdefs.ErrExecutionNotFound
}

type workflowReader struct{}

func (workflowReader) GetWorkflow(ctx context.Context, name string) (*workflow.Workflow, error) {
	if name == "unknown-wf" {
		return nil, errdefs.ErrWorkflowNotFound
	}
	return &workflow.Workflow{ID: name, Steps: []step.Step{{ID: "step-1", Type: step.StepTypeHTTP, Config: json.RawMessage("{}")}}}, nil
}

func TestExecutionLogic_ExecuteWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	logic := execution.NewExecutionLogic(repo, workflowReader{})

	// Empty name
	_, err := logic.ExecuteWorkflow(context.Background(), "", nil)
	if err != errdefs.ErrInvalidWorkflowName {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowName, got %v", err)
	}

	// Whitespace name
	_, err = logic.ExecuteWorkflow(context.Background(), "   ", nil)
	if err != errdefs.ErrInvalidWorkflowName {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowName, got %v", err)
	}

	// Invalid JSON context
	_, err = logic.ExecuteWorkflow(context.Background(), "wf-1", &execution.ExecuteWorkflowRequest{
		Context: json.RawMessage(`{invalid-json`),
	})
	if err != errdefs.ErrInvalidWorkflowContext {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowContext, got %v", err)
	}
}

func TestExecutionLogic_ExecuteWorkflow_Success(t *testing.T) {
	repo := &mockRepository{}
	logic := execution.NewExecutionLogic(repo, workflowReader{})

	exec, err := logic.ExecuteWorkflow(context.Background(), "wf-123", &execution.ExecuteWorkflowRequest{
		Context: json.RawMessage(`{"user_id":100}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec == nil || exec.WorkflowID != "wf-123" {
		t.Fatalf("expected valid execution, got %+v", exec)
	}
}

func TestExecutionLogic_ListExecutions(t *testing.T) {
	expectedSteps := []execution.StepExecution{
		{ID: "se-1", StepID: "s-1", Status: execution.StatusCompleted},
		{ID: "se-2", StepID: "s-2", Status: execution.StatusRunning},
	}
	var capturedLimit, capturedOffset int
	repo := &mockRepository{
		listExecutionsFn: func(ctx context.Context, limit, offset int) ([]execution.WorkflowExecution, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []execution.WorkflowExecution{
				{ID: "exec-1", WorkflowID: "wf-1", Status: execution.StatusRunning, Steps: expectedSteps},
				{ID: "exec-2", WorkflowID: "wf-2", Status: execution.StatusCompleted, Steps: []execution.StepExecution{}},
			}, nil
		},
	}
	logic := execution.NewExecutionLogic(repo, workflowReader{})

	// Test pagination normalization
	execs, err := logic.ListExecutions(context.Background(), 20, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(execs) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(execs))
	}
	if len(execs[0].Steps) != 2 {
		t.Errorf("expected 2 steps in exec-1, got %d", len(execs[0].Steps))
	}
	if capturedLimit != 20 || capturedOffset != 10 {
		t.Errorf("expected limit=20, offset=10; got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}

	// Test default pagination
	_, err = logic.ListExecutions(context.Background(), 0, -5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 || capturedOffset != 0 {
		t.Errorf("expected default limit=50, offset=0; got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}
}

func TestExecutionLogic_GetExecution(t *testing.T) {
	repo := &mockRepository{
		getExecutionFn: func(ctx context.Context, executionID string) (*execution.WorkflowExecution, error) {
			if executionID == "exec-ok" {
				return &execution.WorkflowExecution{ID: "exec-ok", WorkflowID: "wf-1", Status: execution.StatusRunning}, nil
			}
			return nil, errdefs.ErrExecutionNotFound
		},
	}
	logic := execution.NewExecutionLogic(repo, workflowReader{})

	// Empty ID
	_, err := logic.GetExecution(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty execution ID, got nil")
	}

	// Not Found
	_, err = logic.GetExecution(context.Background(), "unknown")
	if err != errdefs.ErrExecutionNotFound {
		t.Fatalf("expected ErrExecutionNotFound, got %v", err)
	}

	// Found
	exec, err := logic.GetExecution(context.Background(), "exec-ok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.ID != "exec-ok" {
		t.Errorf("expected exec-ok, got %s", exec.ID)
	}
}

func TestExecutionHandler_ExecuteWorkflow(t *testing.T) {
	repo := &mockRepository{
		executeFn: func(ctx context.Context, name string, initialContext json.RawMessage) (*execution.WorkflowExecution, error) {
			if name == "unknown-wf" {
				return nil, errdefs.ErrWorkflowNotFound
			}
			return &execution.WorkflowExecution{
				ID:         "exec-999",
				WorkflowID: name,
				Status:     execution.StatusRunning,
				Context:    initialContext,
				CreatedAt:  time.Now().UTC(),
				Steps: []execution.StepExecution{
					{ID: "se-1", StepID: "step-1", Status: execution.StatusPending},
				},
			}, nil
		},
	}
	logic := execution.NewExecutionLogic(repo, workflowReader{})
	handler := execution.NewExecutionHandler(logic, &config.Config{})

	// 1. Trigger with {"context": {"amount": 50000}}
	body := bytes.NewBufferString(`{"context":{"amount":50000}}`)
	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-1/execute", body)
	req.SetPathValue("name", "wf-1")
	rec := httptest.NewRecorder()

	handler.ExecuteWorkflow(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp execution.ExecuteWorkflowResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != "exec-999" || resp.WorkflowID != "wf-1" || resp.TotalSteps != 1 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if string(resp.Context) != `{"amount":50000}` {
		t.Errorf("expected context amount 50000, got %s", string(resp.Context))
	}

	// 2. Trigger with raw json body {"amount": 99000}
	rawBody := bytes.NewBufferString(`{"amount":99000}`)
	reqRaw := httptest.NewRequest(http.MethodPost, "/workflows/wf-1/execute", rawBody)
	reqRaw.SetPathValue("name", "wf-1")
	recRaw := httptest.NewRecorder()

	handler.ExecuteWorkflow(recRaw, reqRaw)
	if recRaw.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for raw body, got %d", recRaw.Code)
	}

	// 3. Trigger with empty body
	reqEmpty := httptest.NewRequest(http.MethodPost, "/workflows/wf-1/execute", nil)
	reqEmpty.SetPathValue("name", "wf-1")
	recEmpty := httptest.NewRecorder()

	handler.ExecuteWorkflow(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for empty body, got %d", recEmpty.Code)
	}

	// 4. Unknown workflow -> 404
	reqUnknown := httptest.NewRequest(http.MethodPost, "/workflows/unknown-wf/execute", nil)
	reqUnknown.SetPathValue("name", "unknown-wf")
	recUnknown := httptest.NewRecorder()

	handler.ExecuteWorkflow(recUnknown, reqUnknown)
	if recUnknown.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for unknown workflow, got %d", recUnknown.Code)
	}

	// 5. Empty workflow name -> 400
	reqEmptyName := httptest.NewRequest(http.MethodPost, "/workflows//execute", nil)
	recEmptyName := httptest.NewRecorder()

	handler.ExecuteWorkflow(recEmptyName, reqEmptyName)
	if recEmptyName.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty workflow name, got %d", recEmptyName.Code)
	}
}

func TestExecutionHandler_ListExecutions(t *testing.T) {
	var capturedLimit, capturedOffset int
	repo := &mockRepository{
		listExecutionsFn: func(ctx context.Context, limit, offset int) ([]execution.WorkflowExecution, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []execution.WorkflowExecution{
				{ID: "exec-1", WorkflowID: "wf-1", Status: execution.StatusRunning},
			}, nil
		},
	}
	logic := execution.NewExecutionLogic(repo, workflowReader{})
	handler := execution.NewExecutionHandler(logic, &config.Config{})

	// With query parameters ?limit=25&offset=50
	req := httptest.NewRequest(http.MethodGet, "/executions?limit=25&offset=50", nil)
	rec := httptest.NewRecorder()
	handler.ListExecutions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if capturedLimit != 25 || capturedOffset != 50 {
		t.Errorf("expected limit=25, offset=50; got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}

	var execs []execution.WorkflowExecution
	if err := json.NewDecoder(rec.Body).Decode(&execs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(execs) != 1 || execs[0].ID != "exec-1" {
		t.Errorf("unexpected executions response: %+v", execs)
	}
}

func TestExecutionHandler_GetExecution(t *testing.T) {
	repo := &mockRepository{
		getExecutionFn: func(ctx context.Context, executionID string) (*execution.WorkflowExecution, error) {
			if executionID == "exec-found" {
				return &execution.WorkflowExecution{ID: "exec-found", WorkflowID: "wf-1", Status: execution.StatusCompleted}, nil
			}
			return nil, errdefs.ErrExecutionNotFound
		},
	}
	logic := execution.NewExecutionLogic(repo, workflowReader{})
	handler := execution.NewExecutionHandler(logic, &config.Config{})

	// 1. Found -> 200 OK
	reqFound := httptest.NewRequest(http.MethodGet, "/executions/exec-found", nil)
	reqFound.SetPathValue("id", "exec-found")
	recFound := httptest.NewRecorder()
	handler.GetExecution(recFound, reqFound)

	if recFound.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recFound.Code)
	}

	// 2. Not found -> 404
	reqNotFound := httptest.NewRequest(http.MethodGet, "/executions/exec-missing", nil)
	reqNotFound.SetPathValue("id", "exec-missing")
	recNotFound := httptest.NewRecorder()
	handler.GetExecution(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recNotFound.Code)
	}

	// 3. Empty ID -> 400
	reqEmptyID := httptest.NewRequest(http.MethodGet, "/executions/", nil)
	recEmptyID := httptest.NewRecorder()
	handler.GetExecution(recEmptyID, reqEmptyID)

	if recEmptyID.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty id, got %d", recEmptyID.Code)
	}
}
