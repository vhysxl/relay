package worker_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/worker"
)

func TestClaimRejectsRemovedFieldsAndTrailingJSON(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WorkerAPIEnabled = true
	cfg.WorkerCredentials = []config.WorkerCredential{{ID: "service", Token: "test-token", Queues: []string{"orders"}}}
	logic := worker.NewWorkerLogic(&mockExecutionRepository{}, cfg)
	handler := worker.NewWorkerHandler(logic, cfg)
	for _, body := range []string{
		`{"worker_id":"w","queue":"orders","executor_types":["activity"]}`,
		`{"worker_id":"w","queue":"orders","activities":["inventory.reserve"]}`,
		`{"worker_id":"w","queue":"orders"} {}`,
		`{"worker_id":"w","queue":""}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/worker/tasks/claim", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		res := httptest.NewRecorder()
		handler.Claim(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("body %s: %d %s", body, res.Code, res.Body.String())
		}
	}
}
