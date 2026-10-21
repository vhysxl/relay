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

func (l *handlerLogic) ExecuteWorkflow(_ context.Context, _ string, req *ExecuteWorkflowRequest) (*WorkflowExecution, error) {
	l.called = true
	l.context = string(req.Context)
	return &WorkflowExecution{}, l.err
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
		{"raw", `{"amount":100}`, 201, `{"amount":100}`}, {"wrapped", `{"context":{"amount":100}}`, 201, `{"amount":100}`},
		{"null context", `{"context":null}`, 201, "null"}, {"malformed", `{`, 400, ""}, {"trailing", `{} {}`, 400, ""},
		{"array", `[]`, 400, ""}, {"scalar", `4`, 400, ""}, {"context array", `{"context":[]}`, 400, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logic := &handlerLogic{}
			h := NewExecutionHandler(logic, &config.Config{MaxPayloadSize: 1})
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			r.SetPathValue("name", "wf")
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
	r.Body = io.NopCloser(brokenBody{})
	w := httptest.NewRecorder()
	h.ExecuteWorkflow(w, r)
	if w.Code != 400 || logic.called {
		t.Fatal("read error reached logic")
	}
}

func TestHandlerErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{errdefs.ErrWorkflowNotFound, 404}, {errdefs.ErrExecutionNotFound, 404}, {errdefs.ErrInvalidWorkflowContext, 400},
		{errdefs.ErrEmptyExecutionID, 400}, {errors.New("secret database details"), 500},
	} {
		logic := &handlerLogic{err: tc.err}
		h := NewExecutionHandler(logic, &config.Config{})
		for _, method := range []func(http.ResponseWriter, *http.Request){h.ExecuteWorkflow, h.GetExecution, h.ListExecutions} {
			r := httptest.NewRequest("POST", "/", nil)
			r.SetPathValue("name", "wf")
			r.SetPathValue("id", "exec")
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
