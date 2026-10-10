package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vhysxl/relay/internal/step"
)

type mockRepository struct {
	createFn         func(ctx context.Context, wf *Workflow) (*Workflow, error)
	getFn            func(ctx context.Context, id string) (*Workflow, error)
	executeFn        func(ctx context.Context, workflowID string, initialContext json.RawMessage) (*step.WorkflowExecution, error)
	listWorkflowsFn  func(ctx context.Context) ([]Workflow, error)
	listExecutionsFn func(ctx context.Context) ([]step.WorkflowExecution, error)
}

func (m *mockRepository) CreateWorkflow(ctx context.Context, wf *Workflow) (*Workflow, error) {
	if m.createFn != nil {
		return m.createFn(ctx, wf)
	}
	return wf, nil
}

func (m *mockRepository) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, ErrWorkflowNotFound
}

func (m *mockRepository) ExecuteWorkflowTx(ctx context.Context, workflowID string, initialContext json.RawMessage) (*step.WorkflowExecution, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, workflowID, initialContext)
	}
	return &step.WorkflowExecution{
		ID:         "exec-123",
		WorkflowID: workflowID,
		Status:     step.StatusRunning,
		Context:    initialContext,
	}, nil
}

func (m *mockRepository) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	if m.listWorkflowsFn != nil {
		return m.listWorkflowsFn(ctx)
	}
	return []Workflow{}, nil
}

func (m *mockRepository) ListExecutions(ctx context.Context) ([]step.WorkflowExecution, error) {
	if m.listExecutionsFn != nil {
		return m.listExecutionsFn(ctx)
	}
	return []step.WorkflowExecution{}, nil
}

func (m *mockRepository) GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error) {
	return &step.WorkflowExecution{ID: executionID, Status: step.StatusRunning}, nil
}


func TestWorkflowLogic_CreateWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())

	// Test empty name
	_, err := logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name: "",
	})
	if err != ErrInvalidWorkflowName {
		t.Fatalf("expected ErrInvalidWorkflowName, got %v", err)
	}

	// Test nil request
	_, err = logic.CreateWorkflow(context.Background(), nil)
	if err != ErrInvalidWorkflowName {
		t.Fatalf("expected ErrInvalidWorkflowName, got %v", err)
	}

	// Test empty steps
	_, err = logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "no-steps-workflow",
		Version: 1,
	})
	if err != ErrWorkflowEmptySteps {
		t.Fatalf("expected ErrWorkflowEmptySteps, got %v", err)
	}

	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"url":"https://api.test/order"}`),
	}

	// Test name too long
	_, err = logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:  strings.Repeat("a", 256),
		Steps: []step.Step{validStep},
	})
	if err != ErrWorkflowNameTooLong {
		t.Fatalf("expected ErrWorkflowNameTooLong, got %v", err)
	}

	// Test negative version
	_, err = logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "test-workflow",
		Version: -1,
		Steps:   []step.Step{validStep},
	})
	if err != ErrInvalidWorkflowVersion {
		t.Fatalf("expected ErrInvalidWorkflowVersion, got %v", err)
	}

	// Test invalid JSON context
	_, err = logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "test-workflow",
		Context: json.RawMessage(`{invalid-json`),
		Steps:   []step.Step{validStep},
	})
	if err != ErrInvalidWorkflowContext {
		t.Fatalf("expected ErrInvalidWorkflowContext, got %v", err)
	}

	// Test non-object JSON context (array or primitive)
	_, err = logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "test-workflow",
		Context: json.RawMessage(`["not", "an", "object"]`),
		Steps:   []step.Step{validStep},
	})
	if err != ErrInvalidWorkflowContext {
		t.Fatalf("expected ErrInvalidWorkflowContext for array context, got %v", err)
	}

	// Test success & defaults
	wf, err := logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "my-workflow",
		Version: 0, // should default to 1
		Context: json.RawMessage(`{"key":"value"}`),
		Steps:   []step.Step{validStep},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.Version != 1 {
		t.Errorf("expected version 1, got %d", wf.Version)
	}
	if wf.Status != WorkflowStatusActive {
		t.Errorf("expected status ACTIVE, got %s", wf.Status)
	}
	if wf.ID == "" {
		t.Errorf("expected non-empty ID")
	}
}

func TestWorkflowLogic_CreateWorkflow_WithSteps(t *testing.T) {
	repo := &mockRepository{}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())

	// Valid steps
	wf, err := logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "order-flow",
		Version: 1,
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
			},
			{
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/payment"}`),
				DependsOn: []string{"step-1"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(wf.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(wf.Steps))
	}
	if wf.Steps[0].WorkflowID != wf.ID || wf.Steps[1].WorkflowID != wf.ID {
		t.Errorf("expected step WorkflowID to be %s", wf.ID)
	}
	if wf.Steps[1].ID == "" {
		t.Errorf("expected step 2 to have auto-generated ID")
	}

	// Cyclic steps
	_, err = logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "cyclic-flow",
		Version: 1,
		Steps: []step.Step{
			{
				ID:        "step-a",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{}`),
				DependsOn: []string{"step-b"},
			},
			{
				ID:        "step-b",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{}`),
				DependsOn: []string{"step-a"},
			},
		},
	})
	if err != step.ErrCyclicDependency {
		t.Fatalf("expected ErrCyclicDependency, got %v", err)
	}
}

func TestWorkflowLogic_GetWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())

	_, err := logic.GetWorkflow(context.Background(), "")
	if err != ErrInvalidWorkflowID {
		t.Fatalf("expected ErrInvalidWorkflowID, got %v", err)
	}
}

func TestWorkflowHandler_CreateWorkflow(t *testing.T) {
	repo := &mockRepository{
		createFn: func(ctx context.Context, wf *Workflow) (*Workflow, error) {
			return wf, nil
		},
	}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())
	handler := NewWorkflowHandler(logic)

	body, _ := json.Marshal(WorkflowRequest{
		Name:    "order-saga",
		Version: 2,
		Context: json.RawMessage(`{"max_retries":3}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateWorkflow(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var created Workflow
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.Name != "order-saga" || created.Version != 2 {
		t.Errorf("unexpected created workflow: %+v", created)
	}
	if created.Status != WorkflowStatusActive {
		t.Errorf("expected ACTIVE, got %s", created.Status)
	}
	if len(created.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(created.Steps))
	}
}

func TestWorkflowHandler_CreateWorkflow_InvalidStep(t *testing.T) {
	repo := &mockRepository{}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())
	handler := NewWorkflowHandler(logic)

	body, _ := json.Marshal(WorkflowRequest{
		Name:    "order-saga",
		Version: 1,
		Steps: []step.Step{
			{
				ID:        "step-1",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{}`),
				DependsOn: []string{"step-1"}, // Self dependency
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateWorkflow(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestWorkflowHandler_CreateWorkflow_IrreversibleWithCompensation(t *testing.T) {
	logic := NewWorkflowLogic(&mockRepository{}, step.NewStepValidator())
	handler := NewWorkflowHandler(logic)

	body, _ := json.Marshal(WorkflowRequest{
		Name:    "irreversible-flow",
		Version: 1,
		Steps: []step.Step{
			{
				ID:           "step-1",
				Type:         step.StepTypeHTTP,
				Config:       json.RawMessage(`{}`),
				Irreversible: true,
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{}`),
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateWorkflow(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for irreversible step with compensation, got %d", rec.Code)
	}
}

func TestWorkflowHandler_GetWorkflow(t *testing.T) {
	testWf := &Workflow{
		ID:      "test-uuid-1234",
		Name:    "order-saga",
		Version: 1,
		Status:  WorkflowStatusActive,
		Steps: []step.Step{
			{
				ID:         "step-1",
				WorkflowID: "test-uuid-1234",
				Type:       step.StepTypeHTTP,
				Config:     json.RawMessage(`{}`),
			},
		},
	}

	repo := &mockRepository{
		getFn: func(ctx context.Context, id string) (*Workflow, error) {
			if id == "test-uuid-1234" {
				return testWf, nil
			}
			return nil, ErrWorkflowNotFound
		},
	}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())
	handler := NewWorkflowHandler(logic)

	// Test found
	req := httptest.NewRequest(http.MethodGet, "/workflows/test-uuid-1234", nil)
	req.SetPathValue("id", "test-uuid-1234")
	rec := httptest.NewRecorder()

	handler.GetWorkflow(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var fetched Workflow
	if err := json.NewDecoder(rec.Body).Decode(&fetched); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(fetched.Steps) != 1 || fetched.Steps[0].ID != "step-1" {
		t.Errorf("expected 1 step with id step-1, got %+v", fetched.Steps)
	}

	// Test not found
	reqNotFound := httptest.NewRequest(http.MethodGet, "/workflows/unknown-id", nil)
	reqNotFound.SetPathValue("id", "unknown-id")
	recNotFound := httptest.NewRecorder()

	handler.GetWorkflow(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recNotFound.Code)
	}
}

func TestWorkflowLogic_ExecuteWorkflow_Validation(t *testing.T) {
	called := false
	repo := &mockRepository{
		executeFn: func(ctx context.Context, workflowID string, initialContext json.RawMessage) (*step.WorkflowExecution, error) {
			called = true
			if workflowID != "wf-123" {
				t.Errorf("expected wf-123, got %s", workflowID)
			}
			return &step.WorkflowExecution{
				ID:         "exec-123",
				WorkflowID: workflowID,
				Status:     step.StatusRunning,
				Context:    initialContext,
			}, nil
		},
	}

	logic := NewWorkflowLogic(repo, step.NewStepValidator())

	// Test empty workflowID
	_, err := logic.ExecuteWorkflow(context.Background(), "", nil)
	if err != ErrInvalidWorkflowID {
		t.Fatalf("expected ErrInvalidWorkflowID, got %v", err)
	}

	// Test whitespace workflowID
	_, err = logic.ExecuteWorkflow(context.Background(), "   ", nil)
	if err != ErrInvalidWorkflowID {
		t.Fatalf("expected ErrInvalidWorkflowID for whitespace, got %v", err)
	}

	// Test invalid JSON context
	_, err = logic.ExecuteWorkflow(context.Background(), "wf-123", &ExecuteWorkflowRequest{
		Context: json.RawMessage(`{not-json`),
	})
	if err != ErrInvalidWorkflowContext {
		t.Fatalf("expected ErrInvalidWorkflowContext, got %v", err)
	}

	// Test success with valid context
	exec, err := logic.ExecuteWorkflow(context.Background(), "wf-123", &ExecuteWorkflowRequest{
		Context: json.RawMessage(`{"order_id":"ORD-1"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected executeFn to be called")
	}
	if string(exec.Context) != `{"order_id":"ORD-1"}` {
		t.Errorf("expected context ORD-1, got %s", string(exec.Context))
	}
}

func TestWorkflowHandler_ExecuteWorkflow(t *testing.T) {
	repo := &mockRepository{
		executeFn: func(ctx context.Context, workflowID string, initialContext json.RawMessage) (*step.WorkflowExecution, error) {
			if workflowID == "wf-not-found" {
				return nil, ErrWorkflowNotFound
			}
			if workflowID == "wf-inactive" {
				return nil, ErrWorkflowInactive
			}
			return &step.WorkflowExecution{
				ID:         "exec-999",
				WorkflowID: workflowID,
				Status:     step.StatusRunning,
				Context:    initialContext,
				Steps: []step.StepExecution{
					{ID: "step-exec-1", Status: step.StatusPending},
				},
			}, nil
		},
	}

	logic := NewWorkflowLogic(repo, step.NewStepValidator())
	handler := NewWorkflowHandler(logic)

	// 1. Trigger with {"context": {"amount": 50000}}
	body := bytes.NewBufferString(`{"context":{"amount":50000}}`)
	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-1/execute", body)
	req.SetPathValue("id", "wf-1")
	rec := httptest.NewRecorder()

	handler.ExecuteWorkflow(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp ExecuteWorkflowResponse
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
	reqRaw.SetPathValue("id", "wf-1")
	recRaw := httptest.NewRecorder()

	handler.ExecuteWorkflow(recRaw, reqRaw)
	if recRaw.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for raw body, got %d", recRaw.Code)
	}

	// 3. Trigger with empty body
	reqEmpty := httptest.NewRequest(http.MethodPost, "/workflows/wf-1/execute", nil)
	reqEmpty.SetPathValue("id", "wf-1")
	recEmpty := httptest.NewRecorder()

	handler.ExecuteWorkflow(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for empty body, got %d", recEmpty.Code)
	}

	// 4. Trigger unknown workflow
	reqUnknown := httptest.NewRequest(http.MethodPost, "/workflows/wf-not-found/execute", nil)
	reqUnknown.SetPathValue("id", "wf-not-found")
	recUnknown := httptest.NewRecorder()

	handler.ExecuteWorkflow(recUnknown, reqUnknown)
	if recUnknown.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recUnknown.Code)
	}

	// 5. Trigger inactive workflow
	reqInactive := httptest.NewRequest(http.MethodPost, "/workflows/wf-inactive/execute", nil)
	reqInactive.SetPathValue("id", "wf-inactive")
	recInactive := httptest.NewRecorder()

	handler.ExecuteWorkflow(recInactive, reqInactive)
	if recInactive.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for inactive workflow, got %d", recInactive.Code)
	}
}

func TestWorkflowStatus_Constants(t *testing.T) {
	if WorkflowStatusActive != "ACTIVE" {
		t.Errorf("expected WorkflowStatusActive to be 'ACTIVE', got %q", WorkflowStatusActive)
	}
	if WorkflowStatusInactive != "INACTIVE" {
		t.Errorf("expected WorkflowStatusInactive to be 'INACTIVE', got %q", WorkflowStatusInactive)
	}
}

func TestWorkflowLogic_ListWorkflows_Batch(t *testing.T) {
	expectedSteps := []step.Step{
		{ID: "s-1", Name: "Step 1", Type: step.StepTypeHTTP},
		{ID: "s-2", Name: "Step 2", Type: step.StepTypeMessage},
	}
	repo := &mockRepository{
		listWorkflowsFn: func(ctx context.Context) ([]Workflow, error) {
			return []Workflow{
				{ID: "wf-1", Name: "Order Flow", Steps: expectedSteps},
				{ID: "wf-2", Name: "Invoice Flow", Steps: []step.Step{}},
			}, nil
		},
	}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())
	wfs, err := logic.ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wfs) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(wfs))
	}
	if len(wfs[0].Steps) != 2 {
		t.Errorf("expected 2 steps in wf-1, got %d", len(wfs[0].Steps))
	}
}

func TestWorkflowLogic_ListExecutions_Batch(t *testing.T) {
	expectedSteps := []step.StepExecution{
		{ID: "se-1", StepID: "s-1", Status: step.StatusCompleted},
		{ID: "se-2", StepID: "s-2", Status: step.StatusRunning},
	}
	repo := &mockRepository{
		listExecutionsFn: func(ctx context.Context) ([]step.WorkflowExecution, error) {
			return []step.WorkflowExecution{
				{ID: "exec-1", WorkflowID: "wf-1", Status: step.StatusRunning, Steps: expectedSteps},
				{ID: "exec-2", WorkflowID: "wf-2", Status: step.StatusCompleted, Steps: []step.StepExecution{}},
			}, nil
		},
	}
	logic := NewWorkflowLogic(repo, step.NewStepValidator())
	execs, err := logic.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(execs) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(execs))
	}
	if len(execs[0].Steps) != 2 {
		t.Errorf("expected 2 steps in exec-1, got %d", len(execs[0].Steps))
	}
}

