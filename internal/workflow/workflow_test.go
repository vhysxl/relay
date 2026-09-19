package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockRepository struct {
	createFn func(ctx context.Context, wf *Workflow) (*Workflow, error)
	getFn    func(ctx context.Context, id string) (*Workflow, error)
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

func TestWorkflowLogic_CreateWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	logic := NewWorkflowLogic(repo)

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

	// Test invalid JSON context
	_, err = logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "test-workflow",
		Context: json.RawMessage(`{invalid-json`),
	})
	if err != ErrInvalidWorkflowContext {
		t.Fatalf("expected ErrInvalidWorkflowContext, got %v", err)
	}

	// Test success & defaults
	wf, err := logic.CreateWorkflow(context.Background(), &WorkflowRequest{
		Name:    "my-workflow",
		Version: 0, // should default to 1
		Context: json.RawMessage(`{"key":"value"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.Version != 1 {
		t.Errorf("expected version 1, got %d", wf.Version)
	}
	if wf.Status != WorkflowStatusPending {
		t.Errorf("expected status PENDING, got %s", wf.Status)
	}
	if wf.ID == "" {
		t.Errorf("expected non-empty ID")
	}
	if wf.Hash == "" {
		t.Errorf("expected non-empty Hash")
	}
}

func TestWorkflowLogic_GetWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	logic := NewWorkflowLogic(repo)

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
	logic := NewWorkflowLogic(repo)
	handler := NewWorkflowHandler(logic)

	body, _ := json.Marshal(WorkflowRequest{
		Name:    "order-saga",
		Version: 2,
		Context: json.RawMessage(`{"max_retries":3}`),
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
	if created.Status != WorkflowStatusPending {
		t.Errorf("expected PENDING, got %s", created.Status)
	}
}

func TestWorkflowHandler_GetWorkflow(t *testing.T) {
	testWf := &Workflow{
		ID:      "test-uuid-1234",
		Name:    "order-saga",
		Version: 1,
		Status:  WorkflowStatusPending,
	}

	repo := &mockRepository{
		getFn: func(ctx context.Context, id string) (*Workflow, error) {
			if id == "test-uuid-1234" {
				return testWf, nil
			}
			return nil, ErrWorkflowNotFound
		},
	}
	logic := NewWorkflowLogic(repo)
	handler := NewWorkflowHandler(logic)

	// Test found
	req := httptest.NewRequest(http.MethodGet, "/workflows/test-uuid-1234", nil)
	req.SetPathValue("id", "test-uuid-1234")
	rec := httptest.NewRecorder()

	handler.GetWorkflow(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
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
