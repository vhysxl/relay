package workflow_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vhysxl/relay/internal/workflow"
)

func TestWorkflowRejectsRemovedStepFields(t *testing.T) {
	handler := workflow.NewWorkflowHandler(&mockWorkflowLogic{}, testConfig)
	for _, body := range []string{
		`{"name":"flow","steps":[{"id":"s","queue":"q","activity":"run","type":"http"}]}`,
		`{"name":"flow","steps":[{"id":"s","queue":"q","activity":"run","config":{}}]}`,
		`{"name":"flow","steps":[{"id":"s","queue":"q","activity":"run","compensation":{"type":"message"}}]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/workflows", strings.NewReader(body))
		res := httptest.NewRecorder()
		handler.CreateWorkflow(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("removed field accepted: %d %s", res.Code, res.Body.String())
		}
	}
	// Arbitrary business input is opaque to the engine's request decoder.
	req := httptest.NewRequest(http.MethodPost, "/workflows", strings.NewReader(`{"name":"flow","steps":[{"id":"s","queue":"q","activity":"run","input":{"type":"http","config":{"url":"example"}}}]}`))
	res := httptest.NewRecorder()
	handler.CreateWorkflow(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("business input rejected: %d %s", res.Code, res.Body.String())
	}
}
