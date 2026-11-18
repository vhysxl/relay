package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vhysxl/relay/bootstrap/app"
	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/testutil"
	"github.com/vhysxl/relay/internal/validator"
	"github.com/vhysxl/relay/internal/worker"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/util"
)

func TestGenericWorkflowHTTPRoundTrip(t *testing.T) {
	db := testutil.OpenMySQL(t)
	cfg := config.DefaultConfig()
	cfg.DefaultTimeout = 25 * time.Second
	cfg.WorkerAPIEnabled = true
	cfg.WorkerCredentials = []config.WorkerCredential{{ID: "orders-service", Token: "test-token", Queues: []string{"orders"}}}
	definitions := workflow.NewWorkflowRepository(db, step.NewStepRepository(db))
	executions := execution.NewExecutionRepository(db)
	mux := http.NewServeMux()
	RegisterRoutes(mux, &app.Application{
		Config: cfg, WorkflowLogic: workflow.NewWorkflowLogic(definitions, validator.NewValidator(), 3),
		ExecutionLogic: execution.NewExecutionLogic(executions, definitions, cfg.DefaultTimeout),
		WorkerLogic:    worker.NewWorkerLogic(executions, cfg),
	})
	call := func(path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		method := http.MethodPost
		if body == "" {
			method = http.MethodGet
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Idempotency-Key", "order-1")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
		return res
	}
	call("/workflows", `{"name":"generic-checkout","steps":[
	 {"id":"reserve","queue":"orders","activity":"inventory.reserve","input":{"order_id":"{{context.order_id}}"},
	  "compensation":{"queue":"orders","activity":"inventory.release","input":{"reservation_id":"{{this.output.reservation_id}}"}}},
	 {"id":"ship","queue":"orders","activity":"shipping.dispatch","depends_on":["reserve"]}
	]}`, http.StatusCreated)
	created := call("/workflows/generic-checkout/execute", `{"context":{"order_id":"ORD-1"}}`, http.StatusCreated)
	var receipt execution.ExecuteWorkflowResponse
	if err := json.Unmarshal(created.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	claim := func() worker.ClaimTaskResponse {
		t.Helper()
		res := call("/worker/tasks/claim", `{"worker_id":"w1","queue":"orders","wait_timeout_ms":0}`, http.StatusOK)
		var task worker.ClaimTaskResponse
		if err := json.Unmarshal(res.Body.Bytes(), &task); err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(res.Body.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		for _, removed := range []string{"type", "payload", "task_queue", "activity_name"} {
			if _, ok := fields[removed]; ok {
				t.Fatalf("removed claim field %s", removed)
			}
		}
		if task.TimeoutSeconds != 25 || task.Queue != "orders" {
			t.Fatalf("claim policy: %+v", task)
		}
		return task
	}
	reserve := claim()
	if reserve.Activity != "inventory.reserve" || !util.AreJSONEqual(reserve.Input, json.RawMessage(`{"order_id":"ORD-1"}`)) {
		t.Fatalf("resolved input: %+v", reserve)
	}
	encode := func(value any) string {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	call("/worker/tasks/claim", `{"worker_id":"w1","queue":"payments","wait_timeout_ms":0}`, http.StatusForbidden)
	call("/worker/tasks/"+reserve.TaskID+"/heartbeat", encode(worker.HeartbeatRequest{WorkerID: "w1", LeaseToken: reserve.LeaseToken, LeaseDurationMS: 10000}), http.StatusOK)
	complete := worker.CompleteRequest{WorkerID: "w1", LeaseToken: reserve.LeaseToken, Output: json.RawMessage(`{"reservation_id":"RSV-1"}`)}
	call("/worker/tasks/"+reserve.TaskID+"/complete", encode(complete), http.StatusOK)
	call("/worker/tasks/"+reserve.TaskID+"/complete", encode(complete), http.StatusOK)
	ship := claim()
	retryable := false
	call("/worker/tasks/"+ship.TaskID+"/fail", encode(worker.FailRequest{WorkerID: "w1", LeaseToken: ship.LeaseToken, Error: worker.TaskError{Message: "shipping unavailable", Retryable: &retryable}}), http.StatusOK)
	rollback := claim()
	if !rollback.IsCompensation || rollback.Activity != "inventory.release" || !util.AreJSONEqual(rollback.Input, json.RawMessage(`{"reservation_id":"RSV-1"}`)) {
		t.Fatalf("rollback input: %+v", rollback)
	}
	call("/worker/tasks/"+rollback.TaskID+"/complete", encode(worker.CompleteRequest{WorkerID: "w1", LeaseToken: rollback.LeaseToken}), http.StatusOK)
	detail := call("/executions/"+receipt.ID, "", http.StatusOK)
	var executionDetail execution.WorkflowExecution
	if err := json.Unmarshal(detail.Body.Bytes(), &executionDetail); err != nil {
		t.Fatal(err)
	}
	if executionDetail.Status != execution.StatusCompensated {
		t.Fatalf("workflow status: %s", executionDetail.Status)
	}
	for _, s := range executionDetail.Steps {
		if s.StepID == "reserve" && !strings.Contains(string(s.Input), "{{") {
			t.Fatalf("template overwritten: %s", s.Input)
		}
	}
	call("/worker/tasks/claim", `{"worker_id":"w1","queue":"orders","wait_timeout_ms":0}`, http.StatusNoContent)
}
