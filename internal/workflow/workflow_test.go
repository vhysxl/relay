package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/validator"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type mockRepository struct {
	createFn                     func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error)
	getFn                        func(ctx context.Context, name string) (*workflow.Workflow, error)
	listWorkflowsFn              func(ctx context.Context, limit, offset int) ([]workflow.WorkflowSummary, error)
	listWorkflowVersionsFn       func(ctx context.Context, name string, limit, offset int) ([]workflow.WorkflowSummary, error)
	getWorkflowVersionFn         func(ctx context.Context, name string, version int) (*workflow.Workflow, error)
	getPreviousWorkflowVersionFn func(ctx context.Context, name string, version int) (*workflow.Workflow, error)
}

func (m *mockRepository) CreateWorkflow(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
	if m.createFn != nil {
		return m.createFn(ctx, wf)
	}
	return wf, nil
}

func (m *mockRepository) GetWorkflow(ctx context.Context, name string) (*workflow.Workflow, error) {
	if m.getFn != nil {
		return m.getFn(ctx, name)
	}
	return nil, errdefs.ErrWorkflowNotFound
}

func (m *mockRepository) ListWorkflows(ctx context.Context, limit, offset int) ([]workflow.WorkflowSummary, error) {
	if m.listWorkflowsFn != nil {
		return m.listWorkflowsFn(ctx, limit, offset)
	}
	return []workflow.WorkflowSummary{}, nil
}

func (m *mockRepository) ListWorkflowVersions(ctx context.Context, name string, limit, offset int) ([]workflow.WorkflowSummary, error) {
	if m.listWorkflowVersionsFn != nil {
		return m.listWorkflowVersionsFn(ctx, name, limit, offset)
	}
	return []workflow.WorkflowSummary{}, nil
}

func (m *mockRepository) GetWorkflowVersion(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
	if m.getWorkflowVersionFn != nil {
		return m.getWorkflowVersionFn(ctx, name, version)
	}
	return nil, errdefs.ErrWorkflowNotFound
}

func (m *mockRepository) GetPreviousWorkflowVersion(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
	if m.getPreviousWorkflowVersionFn != nil {
		return m.getPreviousWorkflowVersionFn(ctx, name, version)
	}
	return nil, errdefs.ErrWorkflowNotFound
}

var testConfig = &config.Config{MaxPayloadSize: 5}

func TestWorkflowLogic_CreateWorkflow_Nil(t *testing.T) {
	repo := &mockRepository{}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	// Test nil request
	_, err := logic.CreateWorkflow(context.Background(), nil)
	if err != errdefs.ErrInvalidWorkflow {
		t.Fatalf("expected errdefs.ErrInvalidWorkflow, got %v", err)
	}
}

func TestWorkflowLogic_CreateWorkflow_Success(t *testing.T) {
	repo := &mockRepository{}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
	}

	// Test success & defaults
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
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
	if wf.ID == "" {
		t.Errorf("expected non-empty ID")
	}
}

func TestWorkflowLogic_CreateWorkflow_WithSteps(t *testing.T) {
	repo := &mockRepository{}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	// Valid steps
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:    "order-flow",
		Version: 1,
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"queue":"orders","url":"https://api.test/payment"}`),
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
	if wf.Steps[1].ID != "step-2" {
		t.Errorf("expected step 2 ID to be step-2, got %s", wf.Steps[1].ID)
	}
}

func TestWorkflowLogic_GetWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	_, err := logic.GetWorkflow(context.Background(), "")
	if err != errdefs.ErrInvalidWorkflowName {
		t.Fatalf("expected errdefs.ErrInvalidWorkflowName, got %v", err)
	}
}

func TestWorkflowHandler_CreateWorkflow(t *testing.T) {
	repo := &mockRepository{
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			return wf, nil
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	body, _ := json.Marshal(workflow.WorkflowRequest{
		Name:    "order-saga",
		Version: 2,
		Context: json.RawMessage(`{"max_retries":3}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateWorkflow(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var created workflow.Workflow
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.Name != "order-saga" || created.Version != 2 {
		t.Errorf("unexpected created workflow: %+v", created)
	}
	if len(created.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(created.Steps))
	}
}

func TestWorkflowHandler_CreateWorkflow_InvalidStep(t *testing.T) {
	repo := &mockRepository{}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	body, _ := json.Marshal(workflow.WorkflowRequest{
		Name:    "order-saga",
		Version: 1,
		Steps: []step.Step{
			{
				ID:        "step-1",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"queue":"orders"}`),
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
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(&mockRepository{}, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	body, _ := json.Marshal(workflow.WorkflowRequest{
		Name:    "irreversible-flow",
		Version: 1,
		Steps: []step.Step{
			{
				ID:           "step-1",
				Type:         step.StepTypeHTTP,
				Config:       json.RawMessage(`{"queue":"orders"}`),
				Irreversible: true,
				Compensation: &step.Compensation{
					Type:   step.StepTypeHTTP,
					Config: json.RawMessage(`{"queue":"orders"}`),
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

func TestWorkflowHandler_CreateWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	// Test empty name
	body, _ := json.Marshal(workflow.WorkflowRequest{
		Name: "",
	})
	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.CreateWorkflow(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty name, got %d", rec.Code)
	}

	// Test cyclic DAG
	body, _ = json.Marshal(workflow.WorkflowRequest{
		Name:    "cyclic-flow",
		Version: 1,
		Steps: []step.Step{
			{ID: "step-a", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders"}`), DependsOn: []string{"step-b"}},
			{ID: "step-b", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders"}`), DependsOn: []string{"step-a"}},
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	handler.CreateWorkflow(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for cyclic DAG, got %d", rec.Code)
	}
}

func TestWorkflowHandler_GetWorkflow(t *testing.T) {
	testWf := &workflow.Workflow{
		ID:      "test-uuid-1234",
		Name:    "order-saga",
		Version: 1,
		Context: json.RawMessage(`{"currency":"IDR"}`),
		Steps: []step.Step{
			{
				ID:         "step-1",
				WorkflowID: "test-uuid-1234",
				Type:       step.StepTypeHTTP,
				Config:     json.RawMessage(`{"queue":"orders","url":"https://example.test","body":{"amount":100}}`),
			},
		},
	}

	repo := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			if name == "order-saga" {
				return testWf, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	// Test found by name
	req := httptest.NewRequest(http.MethodGet, "/workflows/order-saga", nil)
	req.SetPathValue("name", "order-saga")
	rec := httptest.NewRecorder()

	handler.GetWorkflow(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var fetched workflow.Workflow
	if err := json.NewDecoder(rec.Body).Decode(&fetched); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(fetched.Steps) != 1 || fetched.Steps[0].ID != "step-1" {
		t.Errorf("expected 1 step with id step-1, got %+v", fetched.Steps)
	}
	if !bytes.Equal(fetched.Context, testWf.Context) || len(fetched.Steps) != 1 || !bytes.Equal(fetched.Steps[0].Config, testWf.Steps[0].Config) {
		t.Error("detail response must preserve workflow context and step config")
	}

	// Test lookup by ID when no workflow with that name exists -> 404
	reqByID := httptest.NewRequest(http.MethodGet, "/workflows/test-uuid-1234", nil)
	reqByID.SetPathValue("name", "test-uuid-1234")
	recByID := httptest.NewRecorder()

	handler.GetWorkflow(recByID, reqByID)

	if recByID.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for ID lookup without matching name, got %d", recByID.Code)
	}

	// Test not found
	reqNotFound := httptest.NewRequest(http.MethodGet, "/workflows/unknown-id", nil)
	reqNotFound.SetPathValue("name", "unknown-id")
	recNotFound := httptest.NewRecorder()

	handler.GetWorkflow(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recNotFound.Code)
	}

	// Test empty name
	reqEmpty := httptest.NewRequest(http.MethodGet, "/workflows/", nil)
	recEmpty := httptest.NewRecorder()
	handler.GetWorkflow(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty name, got %d", recEmpty.Code)
	}
}

func TestWorkflowLogic_CreateWorkflow_Idempotency(t *testing.T) {
	existingWf := &workflow.Workflow{
		ID:      "existing-uuid-1",
		Name:    "order-saga",
		Version: 1,
		Context: json.RawMessage(`{"currency":"IDR"}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"queue":"orders","url":"https://api.test/payment"}`),
				DependsOn: []string{"step-1"},
			},
		},
	}

	createCalled := false
	repo := &mockRepository{
		getFn: func(ctx context.Context, id string) (*workflow.Workflow, error) {
			if id == "order-saga" {
				return existingWf, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			createCalled = true
			return wf, nil
		},
	}

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	// Send identical request
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:    "order-saga",
		Context: json.RawMessage(`{"currency":"IDR"}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"queue":"orders","url":"https://api.test/payment"}`),
				DependsOn: []string{"step-1"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createCalled {
		t.Errorf("expected repo.CreateWorkflow NOT to be called on idempotent request")
	}
	if wf.ID != "existing-uuid-1" {
		t.Errorf("expected existing ID, got %s", wf.ID)
	}
	if wf.Version != 1 {
		t.Errorf("expected version 1, got %d", wf.Version)
	}
}

func TestWorkflowLogic_CreateWorkflow_AutoIncrementVersion_ContextChanged(t *testing.T) {
	existingWf := &workflow.Workflow{
		ID:      "existing-uuid-1",
		Name:    "order-saga",
		Version: 1,
		Context: json.RawMessage(`{"currency":"IDR"}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
		},
	}

	var createdWf *workflow.Workflow
	repo := &mockRepository{
		getFn: func(ctx context.Context, id string) (*workflow.Workflow, error) {
			if id == "order-saga" {
				return existingWf, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			createdWf = wf
			return wf, nil
		},
	}

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	// Different context
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:    "order-saga",
		Context: json.RawMessage(`{"currency":"USD"}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createdWf == nil {
		t.Fatalf("expected repo.CreateWorkflow to be called")
	}
	if wf.Version != 2 {
		t.Errorf("expected version 2, got %d", wf.Version)
	}
	if wf.ID == "existing-uuid-1" {
		t.Errorf("expected new workflow ID for version 2")
	}
}

func TestWorkflowLogic_CreateWorkflow_AutoIncrementVersion_StepsChanged(t *testing.T) {
	existingWf := &workflow.Workflow{
		ID:      "existing-uuid-1",
		Name:    "order-saga",
		Version: 1,
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
		},
	}

	var createdWf *workflow.Workflow
	repo := &mockRepository{
		getFn: func(ctx context.Context, id string) (*workflow.Workflow, error) {
			if id == "order-saga" {
				return existingWf, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			createdWf = wf
			return wf, nil
		},
	}

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	// Added step-2
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name: "order-saga",
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"queue":"orders","url":"https://api.test/payment"}`),
				DependsOn: []string{"step-1"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createdWf == nil {
		t.Fatalf("expected repo.CreateWorkflow to be called")
	}
	if wf.Version != 2 {
		t.Errorf("expected version 2, got %d", wf.Version)
	}
}

func TestWorkflowLogic_CreateWorkflow_MultiVersionIncrement(t *testing.T) {
	existingWf := &workflow.Workflow{
		ID:      "existing-uuid-3",
		Name:    "order-saga",
		Version: 3,
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/v1"}`),
			},
		},
	}

	var createdWf *workflow.Workflow
	repo := &mockRepository{
		getFn: func(ctx context.Context, id string) (*workflow.Workflow, error) {
			if id == "order-saga" {
				return existingWf, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			createdWf = wf
			return wf, nil
		},
	}

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name: "order-saga",
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/v2"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createdWf == nil {
		t.Fatalf("expected repo.CreateWorkflow to be called")
	}
	if wf.Version != 4 {
		t.Errorf("expected version 4, got %d", wf.Version)
	}
}

func TestWorkflowLogic_ListWorkflows_Summaries(t *testing.T) {
	var capturedLimit, capturedOffset int
	repo := &mockRepository{
		listWorkflowsFn: func(ctx context.Context, limit, offset int) ([]workflow.WorkflowSummary, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []workflow.WorkflowSummary{
				{ID: "wf-1", Name: "Order Flow", StepCount: 2},
				{ID: "wf-2", Name: "Invoice Flow", StepCount: 0},
			}, nil
		},
	}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)
	wfs, err := logic.ListWorkflows(context.Background(), 20, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wfs) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(wfs))
	}
	if wfs[0].StepCount != 2 || wfs[1].StepCount != 0 {
		t.Errorf("unexpected step counts: %+v", wfs)
	}
	if capturedLimit != 20 || capturedOffset != 10 {
		t.Errorf("expected limit=20, offset=10, got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}
}

func TestWorkflowLogic_ListWorkflows_PaginationNormalization(t *testing.T) {
	var capturedLimit, capturedOffset int
	repo := &mockRepository{
		listWorkflowsFn: func(ctx context.Context, limit, offset int) ([]workflow.WorkflowSummary, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []workflow.WorkflowSummary{}, nil
		},
	}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3)

	// Test default when <= 0
	_, err := logic.ListWorkflows(context.Background(), 0, -5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 || capturedOffset != 0 {
		t.Errorf("expected default limit=50, offset=0; got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}

	// Test max clamp
	_, err = logic.ListWorkflows(context.Background(), 500, 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 100 || capturedOffset != 15 {
		t.Errorf("expected clamped limit=100, offset=15; got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}
}

func TestWorkflowHandler_ListWorkflows(t *testing.T) {
	var capturedLimit, capturedOffset int
	repo := &mockRepository{
		listWorkflowsFn: func(ctx context.Context, limit, offset int) ([]workflow.WorkflowSummary, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []workflow.WorkflowSummary{
				{ID: "wf-1", Name: "Order Flow", Version: 1, StepCount: 3},
				{ID: "wf-2", Name: "Empty Flow", Version: 2, StepCount: 0},
			}, nil
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	// 1. With query params ?limit=25&offset=50
	req := httptest.NewRequest(http.MethodGet, "/workflows?limit=25&offset=50", nil)
	rec := httptest.NewRecorder()
	handler.ListWorkflows(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if capturedLimit != 25 || capturedOffset != 50 {
		t.Errorf("expected limit=25, offset=50; got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}

	var wfs []workflow.WorkflowSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &wfs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(wfs) != 2 || wfs[0].Name != "Order Flow" || wfs[0].StepCount != 3 || wfs[1].StepCount != 0 {
		t.Errorf("unexpected workflows response: %+v", wfs)
	}
	var payload []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, summary := range payload {
		for _, field := range []string{"context", "steps", "config"} {
			if _, ok := summary[field]; ok {
				t.Errorf("list response must not include %q", field)
			}
		}
		if _, ok := summary["step_count"]; !ok {
			t.Error("list response must include step_count, including for zero steps")
		}
	}

	// 2. Default without query params
	reqDefault := httptest.NewRequest(http.MethodGet, "/workflows", nil)
	recDefault := httptest.NewRecorder()
	handler.ListWorkflows(recDefault, reqDefault)

	if recDefault.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recDefault.Code)
	}
	if capturedLimit != 50 || capturedOffset != 0 {
		t.Errorf("expected default limit=50, offset=0; got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}
}

func TestWorkflowHandler_ListWorkflows_EmptyAndError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "empty", status: http.StatusOK},
		{name: "database error", err: errors.New("query failed"), status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{
				listWorkflowsFn: func(context.Context, int, int) ([]workflow.WorkflowSummary, error) {
					return nil, tc.err
				},
			}
			h := workflow.NewWorkflowHandler(workflow.NewWorkflowLogic(repo, validator.NewValidator(), 3), testConfig)
			rec := httptest.NewRecorder()
			h.ListWorkflows(rec, httptest.NewRequest(http.MethodGet, "/workflows", nil))
			if rec.Code != tc.status {
				t.Fatalf("expected status %d, got %d", tc.status, rec.Code)
			}
			if tc.err == nil && strings.TrimSpace(rec.Body.String()) != "[]" {
				t.Fatalf("expected empty JSON array, got %s", rec.Body.String())
			}
		})
	}
}

func TestWorkflowLogic_CreateWorkflow_RaceCondition_RetryIdempotent(t *testing.T) {
	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
	}

	concurrentlyCreatedWf := &workflow.Workflow{
		ID:      "race-winner-uuid",
		Name:    "order-race",
		Version: 1,
		Context: json.RawMessage(`{"currency":"IDR"}`),
		Steps:   []step.Step{validStep},
	}

	getCallCount := 0
	createCallCount := 0

	repo := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			getCallCount++
			if getCallCount == 1 {
				// First check: DB still doesn't have it
				return nil, errdefs.ErrWorkflowNotFound
			}
			// Second check: Competitor just committed the identical workflow!
			return concurrentlyCreatedWf, nil
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			createCallCount++
			// First attempt fails due to competitor finishing insert first
			return nil, errdefs.ErrWorkflowAlreadyExists
		},
	}

	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)

	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:    "order-race",
		Context: json.RawMessage(`{"currency":"IDR"}`),
		Steps:   []step.Step{validStep},
	})
	if err != nil {
		t.Fatalf("expected successful recovery via retry, got error: %v", err)
	}

	if wf.ID != "race-winner-uuid" {
		t.Errorf("expected idempotent return of winner workflow, got ID %s", wf.ID)
	}
	if getCallCount != 2 {
		t.Errorf("expected 2 get calls (initial check + retry), got %d", getCallCount)
	}
	if createCallCount != 1 {
		t.Errorf("expected 1 create call, got %d", createCallCount)
	}
}

func TestWorkflowLogic_CreateWorkflow_RaceCondition_RetryAutoIncrement(t *testing.T) {
	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
	}

	differentStep := step.Step{
		ID:     "step-different",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/other"}`),
	}

	concurrentlyCreatedWf := &workflow.Workflow{
		ID:      "race-winner-uuid",
		Name:    "order-race-diff",
		Version: 1,
		Steps:   []step.Step{differentStep},
	}

	getCallCount := 0
	createCallCount := 0
	var finalCreated *workflow.Workflow

	repo := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			getCallCount++
			if getCallCount == 1 {
				return nil, errdefs.ErrWorkflowNotFound
			}
			return concurrentlyCreatedWf, nil
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			createCallCount++
			if createCallCount == 1 {
				return nil, errdefs.ErrWorkflowAlreadyExists
			}
			finalCreated = wf
			return wf, nil
		},
	}

	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)

	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:  "order-race-diff",
		Steps: []step.Step{validStep},
	})
	if err != nil {
		t.Fatalf("expected successful version increment, got error: %v", err)
	}

	if wf.Version != 2 {
		t.Errorf("expected version 2 after retry bump, got %d", wf.Version)
	}
	if finalCreated == nil || finalCreated.Version != 2 {
		t.Errorf("expected final created workflow version 2, got %+v", finalCreated)
	}
}

func TestWorkflowLogic_ConcurrentRetryLimit(t *testing.T) {
	attempts := 0
	repo := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			return nil, errdefs.ErrWorkflowNotFound
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			attempts++
			return nil, errdefs.ErrWorkflowAlreadyExists
		},
	}
	v := validator.NewValidator()
	retryLimit := 2
	logic := workflow.NewWorkflowLogic(repo, v, retryLimit)

	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"queue":"orders","url":"https://api.test/order"}`),
	}

	_, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:  "test-max-retries",
		Steps: []step.Step{validStep},
	})
	if !errors.Is(err, errdefs.ErrWorkflowAlreadyExists) {
		t.Fatalf("expected ErrWorkflowAlreadyExists after exhausting retries, got: %v", err)
	}
	if attempts != retryLimit {
		t.Fatalf("expected %d retry attempts, got %d", retryLimit, attempts)
	}
}

func TestWorkflowHandler_PayloadSizeGuard(t *testing.T) {
	repo := &mockRepository{
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			return wf, nil
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)

	// Injected config with 1MB payload limit
	cfg := &config.Config{MaxPayloadSize: 1}
	handler := workflow.NewWorkflowHandler(logic, cfg)

	validReq := workflow.WorkflowRequest{
		Name: "small-wf",
		Steps: []step.Step{
			{
				ID:     "s1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://test.local"}`),
			},
		},
	}
	smallBody, _ := json.Marshal(validReq)

	// 1. Within limit -> 201 Created
	reqOK := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(smallBody))
	recOK := httptest.NewRecorder()
	handler.CreateWorkflow(recOK, reqOK)
	if recOK.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for small payload, got %d: %s", recOK.Code, recOK.Body.String())
	}

	// 2. Exceeds limit -> 413 Payload Too Large (known Content-Length)
	bigReq := workflow.WorkflowRequest{
		Name:    "big-wf",
		Context: json.RawMessage(`{"payload":"` + strings.Repeat("x", 1024*1024+100) + `"}`),
		Steps: []step.Step{
			{
				ID:     "s1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://test.local"}`),
			},
		},
	}
	bigBody, _ := json.Marshal(bigReq)

	reqTooLarge := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(bigBody))
	recTooLarge := httptest.NewRecorder()
	handler.CreateWorkflow(recTooLarge, reqTooLarge)
	if recTooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 StatusRequestEntityTooLarge for oversized payload, got %d: %s", recTooLarge.Code, recTooLarge.Body.String())
	}

	// 3. Exceeds limit with chunked transfer (unknown Content-Length) -> 413
	reqChunked := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(bigBody))
	reqChunked.ContentLength = -1
	recChunked := httptest.NewRecorder()
	handler.CreateWorkflow(recChunked, reqChunked)
	if recChunked.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 StatusRequestEntityTooLarge for oversized chunked payload, got %d: %s", recChunked.Code, recChunked.Body.String())
	}

	// 4. Valid JSON followed by whitespace padding that exceeds the limit -> 413
	padLen := 1024*1024 + 100
	paddedBody := append(smallBody, bytes.Repeat([]byte(" "), padLen)...)
	reqPadded := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(paddedBody))
	reqPadded.ContentLength = -1
	recPadded := httptest.NewRecorder()
	handler.CreateWorkflow(recPadded, reqPadded)
	if recPadded.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 StatusRequestEntityTooLarge for chunked payload with oversized whitespace padding, got %d: %s", recPadded.Code, recPadded.Body.String())
	}
}

func TestWorkflowHandler_CreateWorkflow_InternalServerError(t *testing.T) {
	dbErr := errors.New("database connection refused")
	repo := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			return nil, dbErr
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	body, _ := json.Marshal(workflow.WorkflowRequest{
		Name:    "test-db-err",
		Version: 1,
		Steps: []step.Step{
			{
				ID:     "s1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","url":"https://test.local"}`),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateWorkflow(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 for database error, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWorkflowHandler_GetWorkflow_InternalServerError(t *testing.T) {
	dbErr := errors.New("database disk full")
	repo := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			return nil, dbErr
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	req := httptest.NewRequest(http.MethodGet, "/workflows/test-db-err", nil)
	req.SetPathValue("name", "test-db-err")
	rec := httptest.NewRecorder()

	handler.GetWorkflow(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 for database error, got %d: %s", rec.Code, rec.Body.String())
	}
}

type mockWorkflowLogic struct {
	createFn            func(ctx context.Context, req *workflow.WorkflowRequest) (*workflow.Workflow, error)
	getFn               func(ctx context.Context, name string) (*workflow.Workflow, error)
	listFn              func(ctx context.Context, limit, offset int) ([]workflow.WorkflowSummary, error)
	listVersionsFn      func(ctx context.Context, name string, limit, offset int) ([]workflow.WorkflowSummary, error)
	getVersionFn        func(ctx context.Context, name string, version int) (*workflow.Workflow, error)
	getVersionChangesFn func(ctx context.Context, name string, version int) (*workflow.WorkflowVersionChangesResponse, error)
}

func (m *mockWorkflowLogic) CreateWorkflow(ctx context.Context, req *workflow.WorkflowRequest) (*workflow.Workflow, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return &workflow.Workflow{ID: "wf-id", Name: req.Name}, nil
}

func (m *mockWorkflowLogic) GetWorkflow(ctx context.Context, name string) (*workflow.Workflow, error) {
	if m.getFn != nil {
		return m.getFn(ctx, name)
	}
	return nil, errdefs.ErrWorkflowNotFound
}

func (m *mockWorkflowLogic) ListWorkflows(ctx context.Context, limit, offset int) ([]workflow.WorkflowSummary, error) {
	if m.listFn != nil {
		return m.listFn(ctx, limit, offset)
	}
	return []workflow.WorkflowSummary{}, nil
}

func (m *mockWorkflowLogic) ListWorkflowVersions(ctx context.Context, name string, limit, offset int) ([]workflow.WorkflowSummary, error) {
	if m.listVersionsFn != nil {
		return m.listVersionsFn(ctx, name, limit, offset)
	}
	return []workflow.WorkflowSummary{}, nil
}

func (m *mockWorkflowLogic) GetWorkflowVersion(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
	if m.getVersionFn != nil {
		return m.getVersionFn(ctx, name, version)
	}
	return nil, errdefs.ErrWorkflowNotFound
}

func (m *mockWorkflowLogic) GetWorkflowVersionChanges(ctx context.Context, name string, version int) (*workflow.WorkflowVersionChangesResponse, error) {
	if m.getVersionChangesFn != nil {
		return m.getVersionChangesFn(ctx, name, version)
	}
	return nil, errdefs.ErrWorkflowNotFound
}

func TestWorkflowHandler_CreateWorkflow_BodyValidation(t *testing.T) {
	validJSON := `{"name":"order-wf","steps":[{"id":"s1","type":"http","config":{"url":"https://test.local"}}]}`

	t.Run("valid body and trailing whitespace accepted", func(t *testing.T) {
		logicCalled := false
		mockLogic := &mockWorkflowLogic{
			createFn: func(ctx context.Context, req *workflow.WorkflowRequest) (*workflow.Workflow, error) {
				logicCalled = true
				return &workflow.Workflow{ID: "wf-1", Name: req.Name}, nil
			},
		}
		handler := workflow.NewWorkflowHandler(mockLogic, testConfig)

		req := httptest.NewRequest(http.MethodPost, "/workflows", strings.NewReader(validJSON+"  \r\n\t  \n"))
		rec := httptest.NewRecorder()
		handler.CreateWorkflow(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created for valid body with trailing whitespace, got %d: %s", rec.Code, rec.Body.String())
		}
		if !logicCalled {
			t.Fatal("expected logic to be called for valid body")
		}
	})

	invalidBodies := []struct {
		name string
		body string
	}{
		{"empty body", ""},
		{"whitespace only", "   \n\t\r  "},
		{"truncated json", `{"name":"order-wf"`},
		{"trailing garbage", validJSON + " garbage"},
		{"second json object", validJSON + `{"name":"second"}`},
		{"second json primitive number", validJSON + ` 123`},
		{"second json primitive string", validJSON + ` "extra"`},
	}

	for _, tc := range invalidBodies {
		t.Run(tc.name, func(t *testing.T) {
			logicCalled := false
			mockLogic := &mockWorkflowLogic{
				createFn: func(ctx context.Context, req *workflow.WorkflowRequest) (*workflow.Workflow, error) {
					logicCalled = true
					return &workflow.Workflow{}, nil
				},
			}
			handler := workflow.NewWorkflowHandler(mockLogic, testConfig)

			req := httptest.NewRequest(http.MethodPost, "/workflows", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			handler.CreateWorkflow(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request for %s, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
			if logicCalled {
				t.Fatalf("logic must NOT be called when body is invalid for %s", tc.name)
			}
		})
	}
}

func TestWorkflowLogic_LookupByNameOnly_LatestVersionAndSameID(t *testing.T) {
	v := validator.NewValidator()

	// 1. Name lookup retrieves latest version
	wfV1 := &workflow.Workflow{ID: "uuid-v1", Name: "payment-flow", Version: 1}
	wfV2 := &workflow.Workflow{ID: "uuid-v2", Name: "payment-flow", Version: 2}
	workflows := []*workflow.Workflow{wfV1, wfV2}

	repoMulti := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			var latest *workflow.Workflow
			for _, wf := range workflows {
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
		},
	}
	logicMulti := workflow.NewWorkflowLogic(repoMulti, v, 3)

	latest, err := logicMulti.GetWorkflow(context.Background(), "payment-flow")
	if err != nil {
		t.Fatalf("unexpected error fetching latest workflow: %v", err)
	}
	if latest.Version != 2 || latest.ID != "uuid-v2" {
		t.Fatalf("expected version 2 (uuid-v2), got version %d (%s)", latest.Version, latest.ID)
	}

	// 2. ID lookup without matching name returns not found
	_, err = logicMulti.GetWorkflow(context.Background(), "uuid-v1")
	if !errors.Is(err, errdefs.ErrWorkflowNotFound) {
		t.Fatalf("expected ErrWorkflowNotFound when querying by ID with no matching name, got %v", err)
	}

	// 3. Name matching another workflow's ID returns workflow by name; create does not return other workflow
	wfA := &workflow.Workflow{ID: "uuid-shared", Name: "wf-alpha", Version: 1}
	wfB := &workflow.Workflow{ID: "uuid-beta", Name: "uuid-shared", Version: 1}

	repoColl := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			if name == "uuid-shared" {
				return wfB, nil
			}
			if name == "wf-alpha" {
				return wfA, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
	}
	logicColl := workflow.NewWorkflowLogic(repoColl, v, 3)

	res, err := logicColl.GetWorkflow(context.Background(), "uuid-shared")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != wfB.ID || res.Name != "uuid-shared" {
		t.Fatalf("expected workflow B (ID %s, Name %s), got ID %s, Name %s", wfB.ID, wfB.Name, res.ID, res.Name)
	}
}

func TestWorkflowHandler_ListWorkflowVersions(t *testing.T) {
	summaries := []workflow.WorkflowSummary{
		{ID: "wf-v2", Name: "order-flow", Version: 2, StepCount: 3},
		{ID: "wf-v1", Name: "order-flow", Version: 1, StepCount: 2},
	}

	mockLogic := &mockWorkflowLogic{
		listVersionsFn: func(ctx context.Context, name string, limit, offset int) ([]workflow.WorkflowSummary, error) {
			if name == "order-flow" {
				return summaries, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
	}
	handler := workflow.NewWorkflowHandler(mockLogic, testConfig)

	// 1. Success
	req := httptest.NewRequest(http.MethodGet, "/workflows/order-flow/versions?limit=50&offset=0", nil)
	req.SetPathValue("name", "order-flow")
	rec := httptest.NewRecorder()
	handler.ListWorkflowVersions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var res []workflow.WorkflowSummary
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if len(res) != 2 || res[0].Version != 2 || res[1].Version != 1 {
		t.Fatalf("unexpected versions: %+v", res)
	}

	// 2. Not found
	reqNF := httptest.NewRequest(http.MethodGet, "/workflows/unknown/versions", nil)
	reqNF.SetPathValue("name", "unknown")
	recNF := httptest.NewRecorder()
	handler.ListWorkflowVersions(recNF, reqNF)
	if recNF.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", recNF.Code)
	}

	// 3. Empty name
	reqEmpty := httptest.NewRequest(http.MethodGet, "/workflows//versions", nil)
	reqEmpty.SetPathValue("name", "")
	recEmpty := httptest.NewRecorder()
	handler.ListWorkflowVersions(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", recEmpty.Code)
	}
}

func TestWorkflowHandler_GetWorkflowVersion(t *testing.T) {
	mockWf := &workflow.Workflow{ID: "wf-v2", Name: "order-flow", Version: 2}

	mockLogic := &mockWorkflowLogic{
		getVersionFn: func(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
			if name == "order-flow" && version == 2 {
				return mockWf, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
	}
	handler := workflow.NewWorkflowHandler(mockLogic, testConfig)

	// 1. Success
	req := httptest.NewRequest(http.MethodGet, "/workflows/order-flow/versions/2", nil)
	req.SetPathValue("name", "order-flow")
	req.SetPathValue("version", "2")
	rec := httptest.NewRecorder()
	handler.GetWorkflowVersion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Version not found -> 404
	reqNF := httptest.NewRequest(http.MethodGet, "/workflows/order-flow/versions/99", nil)
	reqNF.SetPathValue("name", "order-flow")
	reqNF.SetPathValue("version", "99")
	recNF := httptest.NewRecorder()
	handler.GetWorkflowVersion(recNF, reqNF)
	if recNF.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing version, got %d", recNF.Code)
	}

	// 3. Invalid version formats -> 400
	for _, badVer := range []string{"0", "-1", "abc", "2147483648", "1.5"} {
		reqBad := httptest.NewRequest(http.MethodGet, "/workflows/order-flow/versions/"+badVer, nil)
		reqBad.SetPathValue("name", "order-flow")
		reqBad.SetPathValue("version", badVer)
		recBad := httptest.NewRecorder()
		handler.GetWorkflowVersion(recBad, reqBad)
		if recBad.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for bad version %q, got %d", badVer, recBad.Code)
		}
	}
}

func TestWorkflowHandler_GetWorkflowVersionChanges(t *testing.T) {
	prevVer := 1
	changesResp := &workflow.WorkflowVersionChangesResponse{
		FromVersion: &prevVer,
		ToVersion:   2,
		Changes: []workflow.WorkflowChange{
			{Path: "/context/domain", Kind: workflow.ChangeKindModified, Before: "old", After: "new"},
		},
	}

	mockLogic := &mockWorkflowLogic{
		getVersionChangesFn: func(ctx context.Context, name string, version int) (*workflow.WorkflowVersionChangesResponse, error) {
			if name == "order-flow" && version == 2 {
				return changesResp, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
	}
	handler := workflow.NewWorkflowHandler(mockLogic, testConfig)

	// 1. Success
	req := httptest.NewRequest(http.MethodGet, "/workflows/order-flow/versions/2/changes", nil)
	req.SetPathValue("name", "order-flow")
	req.SetPathValue("version", "2")
	rec := httptest.NewRecorder()
	handler.GetWorkflowVersionChanges(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var res workflow.WorkflowVersionChangesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.FromVersion == nil || *res.FromVersion != 1 || res.ToVersion != 2 || len(res.Changes) != 1 {
		t.Fatalf("unexpected changes response: %+v", res)
	}

	// 2. Version not found -> 404
	reqNF := httptest.NewRequest(http.MethodGet, "/workflows/order-flow/versions/99/changes", nil)
	reqNF.SetPathValue("name", "order-flow")
	reqNF.SetPathValue("version", "99")
	recNF := httptest.NewRecorder()
	handler.GetWorkflowVersionChanges(recNF, reqNF)
	if recNF.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing version, got %d", recNF.Code)
	}

	// 3. Bad version -> 400
	reqBad := httptest.NewRequest(http.MethodGet, "/workflows/order-flow/versions/bad/changes", nil)
	reqBad.SetPathValue("name", "order-flow")
	reqBad.SetPathValue("version", "bad")
	recBad := httptest.NewRecorder()
	handler.GetWorkflowVersionChanges(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad version string, got %d", recBad.Code)
	}
}

func TestWorkflowLogic_VersionHistoryAndChanges(t *testing.T) {
	wfV1 := &workflow.Workflow{
		ID:      "id-1",
		Name:    "order-flow",
		Version: 1,
		Context: json.RawMessage(`{"region":"US"}`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders","url":"https://v1"}`)}},
	}
	wfV3 := &workflow.Workflow{
		ID:      "id-3",
		Name:    "order-flow",
		Version: 3, // Notice gap: v1 then v3
		Context: json.RawMessage(`{"region":"EU"}`),
		Steps:   []step.Step{{ID: "s1", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders","url":"https://v3"}`)}},
	}

	repo := &mockRepository{
		listWorkflowVersionsFn: func(ctx context.Context, name string, limit, offset int) ([]workflow.WorkflowSummary, error) {
			if name != "order-flow" {
				return nil, errdefs.ErrWorkflowNotFound
			}
			return []workflow.WorkflowSummary{
				{ID: "id-3", Name: "order-flow", Version: 3, StepCount: 1},
				{ID: "id-1", Name: "order-flow", Version: 1, StepCount: 1},
			}, nil
		},
		getWorkflowVersionFn: func(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
			if name != "order-flow" {
				return nil, errdefs.ErrWorkflowNotFound
			}
			if version == 1 {
				return wfV1, nil
			}
			if version == 3 {
				return wfV3, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
		getPreviousWorkflowVersionFn: func(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
			if name != "order-flow" {
				return nil, errdefs.ErrWorkflowNotFound
			}
			if version == 3 {
				// Previous available version before 3 is 1
				return wfV1, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
	}

	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v, 3)

	// 1. List versions
	versions, err := logic.ListWorkflowVersions(context.Background(), "order-flow", 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 2 || versions[0].Version != 3 || versions[1].Version != 1 {
		t.Fatalf("unexpected versions: %+v", versions)
	}

	// 2. First version changes (v1): from_version must be null, changes empty
	changesV1, err := logic.GetWorkflowVersionChanges(context.Background(), "order-flow", 1)
	if err != nil {
		t.Fatalf("unexpected error fetching v1 changes: %v", err)
	}
	if changesV1.FromVersion != nil {
		t.Fatalf("expected FromVersion to be nil for version 1, got %v", *changesV1.FromVersion)
	}
	if changesV1.ToVersion != 1 || len(changesV1.Changes) != 0 {
		t.Fatalf("expected 0 changes for version 1, got %+v", changesV1)
	}

	// 3. Version with gap (v3 compared against v1)
	changesV3, err := logic.GetWorkflowVersionChanges(context.Background(), "order-flow", 3)
	if err != nil {
		t.Fatalf("unexpected error fetching v3 changes: %v", err)
	}
	if changesV3.FromVersion == nil || *changesV3.FromVersion != 1 {
		t.Fatalf("expected FromVersion 1, got %v", changesV3.FromVersion)
	}
	if changesV3.ToVersion != 3 {
		t.Fatalf("expected ToVersion 3, got %d", changesV3.ToVersion)
	}
	if len(changesV3.Changes) != 2 {
		t.Fatalf("expected 2 changes (region and config/url), got %d: %+v", len(changesV3.Changes), changesV3.Changes)
	}
}
