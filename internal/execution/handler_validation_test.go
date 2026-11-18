package execution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type handlerLogic struct {
	ExecutionLogic
	called  bool
	err     error
	context string
}

func (l *handlerLogic) ExecuteWorkflow(_ context.Context, _ string, req *ExecuteWorkflowRequest) (*ExecuteWorkflowResponse, bool, error) {
	l.called = true
	l.context = string(req.Context)
	return &ExecuteWorkflowResponse{}, false, l.err
}
func (l *handlerLogic) GetExecution(context.Context, string) (*WorkflowExecution, error) {
	l.called = true
	return &WorkflowExecution{}, l.err
}
func (l *handlerLogic) ListExecutions(context.Context, int, int) ([]WorkflowExecution, error) {
	l.called = true
	return nil, l.err
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("read failure") }
func (brokenBody) Close() error             { return nil }

func TestHandlerBodyValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body  string
		status      int
		wantContext string
	}{
		{"empty", "", 201, ""}, {"whitespace", " \n ", 201, ""}, {"null", "null", 201, ""},
		{"raw", `{"amount":100}`, 400, ""}, {"wrapped", `{"context":{"amount":100}}`, 201, `{"amount":100}`},
		{"null context", `{"context":null}`, 201, "null"}, {"malformed", `{`, 400, ""}, {"trailing", `{} {}`, 400, ""},
		{"array", `[]`, 400, ""}, {"scalar", `4`, 400, ""}, {"context array", `{"context":[]}`, 400, ""},
		{"raw with workflow_version", `{"workflow_version":1,"amount":6000000}`, 400, ""},
		{"version without context", `{"workflow_version":2}`, 201, ""},
		{"zero version", `{"workflow_version":0}`, 400, ""},
		{"null version", `{"workflow_version":null}`, 400, ""},
		{"fraction version", `{"workflow_version":1.5}`, 400, ""},
		{"unknown with context", `{"context":{},"amount":1}`, 400, ""},
		{"wrapped with workflow_version", `{"context":{"amount":100},"workflow_version":2}`, 201, `{"amount":100}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logic := &handlerLogic{}
			h := NewExecutionHandler(logic, &config.Config{MaxPayloadSize: 1})
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			r.SetPathValue("name", "wf")
			r.Header.Set("Idempotency-Key", "valid-key-123")
			w := httptest.NewRecorder()
			h.ExecuteWorkflow(w, r)
			if w.Code != tc.status || logic.called != (tc.status == 201) || logic.context != tc.wantContext {
				t.Fatalf("status %d called %v context %q body %s", w.Code, logic.called, logic.context, w.Body.String())
			}
		})
	}
	for _, unknownLength := range []bool{false, true} {
		logic := &handlerLogic{}
		h := NewExecutionHandler(logic, &config.Config{MaxPayloadSize: 1})
		r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat(" ", 1024*1024+1)))
		r.SetPathValue("name", "wf")
		r.Header.Set("Idempotency-Key", "valid-key-123")
		if unknownLength {
			r.ContentLength = -1
		}
		w := httptest.NewRecorder()
		h.ExecuteWorkflow(w, r)
		if w.Code != 413 || logic.called {
			t.Fatal("oversize request reached logic")
		}
	}
	logic := &handlerLogic{}
	h := NewExecutionHandler(logic, &config.Config{})
	r := httptest.NewRequest("POST", "/", nil)
	r.SetPathValue("name", "wf")
	r.Header.Set("Idempotency-Key", "valid-key-123")
	r.Body = io.NopCloser(brokenBody{})
	w := httptest.NewRecorder()
	h.ExecuteWorkflow(w, r)
	if w.Code != 400 || logic.called {
		t.Fatal("read error reached logic")
	}
}

func TestHandlerIdempotencyHeaderValidation(t *testing.T) {
	tests := []struct {
		name       string
		headers    []string
		wantStatus int
	}{
		{"missing header", nil, 400},
		{"empty header", []string{""}, 400},
		{"header with whitespace", []string{"key with space"}, 400},
		{"header with tab", []string{"key\twith\ttab"}, 400},
		{"header with newline", []string{"key\nwith\nnewline"}, 400},
		{"repeated headers", []string{"key-1", "key-2"}, 400},
		{"key too long", []string{strings.Repeat("a", 129)}, 400},
		{"valid key max length", []string{strings.Repeat("a", 128)}, 201},
		{"valid standard key", []string{"ord-123:checkout"}, 201},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logic := &handlerLogic{}
			h := NewExecutionHandler(logic, &config.Config{})
			r := httptest.NewRequest("POST", "/", nil)
			r.SetPathValue("name", "wf")
			for _, v := range tt.headers {
				r.Header.Add("Idempotency-Key", v)
			}
			w := httptest.NewRecorder()
			h.ExecuteWorkflow(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandlerErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{errdefs.ErrWorkflowNotFound, 404}, {errdefs.ErrExecutionNotFound, 404}, {errdefs.ErrInvalidWorkflowContext, 400},
		{errdefs.ErrEmptyExecutionID, 400}, {errdefs.ErrIdempotencyKeyRequired, 400}, {errdefs.ErrInvalidIdempotencyKey, 400},
		{errdefs.ErrIdempotencyConflict, 409}, {errors.New("secret database details"), 500},
	} {
		logic := &handlerLogic{err: tc.err}
		h := NewExecutionHandler(logic, &config.Config{})
		for _, method := range []func(http.ResponseWriter, *http.Request){h.ExecuteWorkflow, h.GetExecution, h.ListExecutions} {
			r := httptest.NewRequest("POST", "/", nil)
			r.SetPathValue("name", "wf")
			r.SetPathValue("id", "exec")
			r.Header.Set("Idempotency-Key", "valid-key-123")
			w := httptest.NewRecorder()
			method(w, r)
			if w.Code != tc.want {
				t.Fatalf("%v: got %d", tc.err, w.Code)
			}
			if tc.want == 500 && strings.Contains(w.Body.String(), "secret") {
				t.Fatal("database details leaked")
			}
		}
	}
}
