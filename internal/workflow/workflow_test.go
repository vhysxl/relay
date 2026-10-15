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
	createFn        func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error)
	getFn           func(ctx context.Context, name string) (*workflow.Workflow, error)
	listWorkflowsFn func(ctx context.Context, limit, offset int) ([]workflow.Workflow, error)
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

func (m *mockRepository) ListWorkflows(ctx context.Context, limit, offset int) ([]workflow.Workflow, error) {
	if m.listWorkflowsFn != nil {
		return m.listWorkflowsFn(ctx, limit, offset)
	}
	return []workflow.Workflow{}, nil
}

var testConfig = &config.Config{MaxPayloadSize: 5}


func TestWorkflowLogic_CreateWorkflow_Nil(t *testing.T) {
	repo := &mockRepository{}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

	// Test nil request
	_, err := logic.CreateWorkflow(context.Background(), nil)
	if err != errdefs.ErrInvalidWorkflow {
		t.Fatalf("expected errdefs.ErrInvalidWorkflow, got %v", err)
	}
}

func TestWorkflowLogic_CreateWorkflow_Success(t *testing.T) {
	repo := &mockRepository{}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"url":"https://api.test/order"}`),
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
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

	// Valid steps
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:    "order-flow",
		Version: 1,
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
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
	if wf.Steps[1].ID != "step-2" {
		t.Errorf("expected step 2 ID to be step-2, got %s", wf.Steps[1].ID)
	}
}

func TestWorkflowLogic_GetWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

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
	logic := workflow.NewWorkflowLogic(repo, v)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	body, _ := json.Marshal(workflow.WorkflowRequest{
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
	logic := workflow.NewWorkflowLogic(repo, v)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	body, _ := json.Marshal(workflow.WorkflowRequest{
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
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(&mockRepository{}, v)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	body, _ := json.Marshal(workflow.WorkflowRequest{
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

func TestWorkflowHandler_CreateWorkflow_Validation(t *testing.T) {
	repo := &mockRepository{}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v)
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
			{ID: "step-a", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`), DependsOn: []string{"step-b"}},
			{ID: "step-b", Type: step.StepTypeHTTP, Config: json.RawMessage(`{}`), DependsOn: []string{"step-a"}},
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
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			if name == "test-uuid-1234" {
				return testWf, nil
			}
			return nil, errdefs.ErrWorkflowNotFound
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v)
	handler := workflow.NewWorkflowHandler(logic, testConfig)

	// Test found
	req := httptest.NewRequest(http.MethodGet, "/workflows/test-uuid-1234", nil)
	req.SetPathValue("name", "test-uuid-1234")
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
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/payment"}`),
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

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

	// Send identical request
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:    "order-saga",
		Context: json.RawMessage(`{"currency":"IDR"}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/payment"}`),
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
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
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

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

	// Different context
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:    "order-saga",
		Context: json.RawMessage(`{"currency":"USD"}`),
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
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
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
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

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

	// Added step-2
	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name: "order-saga",
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/order"}`),
			},
			{
				ID:        "step-2",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"url":"https://api.test/payment"}`),
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
				Config: json.RawMessage(`{"url":"https://api.test/v1"}`),
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

	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

	wf, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name: "order-saga",
		Steps: []step.Step{
			{
				ID:     "step-1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://api.test/v2"}`),
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

func TestWorkflowLogic_ListWorkflows_Batch(t *testing.T) {
	expectedSteps := []step.Step{
		{ID: "s-1", Name: "Step 1", Type: step.StepTypeHTTP},
		{ID: "s-2", Name: "Step 2", Type: step.StepTypeMessage},
	}
	var capturedLimit, capturedOffset int
	repo := &mockRepository{
		listWorkflowsFn: func(ctx context.Context, limit, offset int) ([]workflow.Workflow, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []workflow.Workflow{
				{ID: "wf-1", Name: "Order Flow", Steps: expectedSteps},
				{ID: "wf-2", Name: "Invoice Flow", Steps: []step.Step{}},
			}, nil
		},
	}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())
	wfs, err := logic.ListWorkflows(context.Background(), 20, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wfs) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(wfs))
	}
	if len(wfs[0].Steps) != 2 {
		t.Errorf("expected 2 steps in wf-1, got %d", len(wfs[0].Steps))
	}
	if capturedLimit != 20 || capturedOffset != 10 {
		t.Errorf("expected limit=20, offset=10, got limit=%d, offset=%d", capturedLimit, capturedOffset)
	}
}

func TestWorkflowLogic_ListWorkflows_PaginationNormalization(t *testing.T) {
	var capturedLimit, capturedOffset int
	repo := &mockRepository{
		listWorkflowsFn: func(ctx context.Context, limit, offset int) ([]workflow.Workflow, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []workflow.Workflow{}, nil
		},
	}
	logic := workflow.NewWorkflowLogic(repo, validator.NewValidator())

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
		listWorkflowsFn: func(ctx context.Context, limit, offset int) ([]workflow.Workflow, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []workflow.Workflow{
				{ID: "wf-1", Name: "Order Flow", Version: 1},
			}, nil
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v)
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

	var wfs []workflow.Workflow
	if err := json.NewDecoder(rec.Body).Decode(&wfs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(wfs) != 1 || wfs[0].Name != "Order Flow" {
		t.Errorf("unexpected workflows response: %+v", wfs)
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

func TestWorkflowLogic_CreateWorkflow_RaceCondition_RetryIdempotent(t *testing.T) {
	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"url":"https://api.test/order"}`),
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
	logic := workflow.NewWorkflowLogic(repo, v)

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
		Config: json.RawMessage(`{"url":"https://api.test/order"}`),
	}

	differentStep := step.Step{
		ID:     "step-different",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"url":"https://api.test/other"}`),
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
	logic := workflow.NewWorkflowLogic(repo, v)

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

func TestWorkflowLogic_MaxRetriesConfig_EnvAndOptions(t *testing.T) {
	// 1. Default value
	t.Setenv("WORKFLOW_MAX_RETRIES", "")
	if retries := workflow.GetMaxRetries(); retries != workflow.DefaultMaxRetries {
		t.Errorf("expected default retries %d, got %d", workflow.DefaultMaxRetries, retries)
	}

	// 2. Read from ENV
	t.Setenv("WORKFLOW_MAX_RETRIES", "7")
	if retries := workflow.GetMaxRetries(); retries != 7 {
		t.Errorf("expected 7 from ENV, got %d", retries)
	}

	// 3. Fallback when invalid ENV value
	t.Setenv("WORKFLOW_MAX_RETRIES", "invalid_num")
	if retries := workflow.GetMaxRetries(); retries != workflow.DefaultMaxRetries {
		t.Errorf("expected fallback %d for invalid env, got %d", workflow.DefaultMaxRetries, retries)
	}

	// 4. Overridden via WithMaxRetries option
	t.Setenv("WORKFLOW_MAX_RETRIES", "5")
	repo := &mockRepository{
		getFn: func(ctx context.Context, name string) (*workflow.Workflow, error) {
			return nil, errdefs.ErrWorkflowNotFound
		},
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			return nil, errdefs.ErrWorkflowAlreadyExists
		},
	}
	v := validator.NewValidator()
	// Pass option WithMaxRetries(2)
	logic := workflow.NewWorkflowLogic(repo, v, workflow.WithMaxRetries(2))

	validStep := step.Step{
		ID:     "step-1",
		Type:   step.StepTypeHTTP,
		Config: json.RawMessage(`{"url":"https://api.test/order"}`),
	}

	_, err := logic.CreateWorkflow(context.Background(), &workflow.WorkflowRequest{
		Name:  "test-max-retries",
		Steps: []step.Step{validStep},
	})
	if !errors.Is(err, errdefs.ErrWorkflowAlreadyExists) {
		t.Fatalf("expected ErrWorkflowAlreadyExists after exhausting retries, got: %v", err)
	}
}

func TestWorkflowHandler_PayloadSizeGuard(t *testing.T) {
	repo := &mockRepository{
		createFn: func(ctx context.Context, wf *workflow.Workflow) (*workflow.Workflow, error) {
			return wf, nil
		},
	}
	v := validator.NewValidator()
	logic := workflow.NewWorkflowLogic(repo, v)

	// Injected config with 1MB payload limit
	cfg := &config.Config{MaxPayloadSize: 1}
	handler := workflow.NewWorkflowHandler(logic, cfg)

	validReq := workflow.WorkflowRequest{
		Name: "small-wf",
		Steps: []step.Step{
			{
				ID:     "s1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://test.local"}`),
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

	// 2. Exceeds limit -> 413 Payload Too Large
	bigReq := workflow.WorkflowRequest{
		Name:    "big-wf",
		Context: json.RawMessage(`{"payload":"` + strings.Repeat("x", 1024*1024+100) + `"}`),
		Steps: []step.Step{
			{
				ID:     "s1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"url":"https://test.local"}`),
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
}



