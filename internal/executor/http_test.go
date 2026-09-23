package executor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

func TestHTTPExecutor_Success_GET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if r.Header.Get("X-Custom-Header") != "TestValue" {
			t.Errorf("expected X-Custom-Header=TestValue, got %s", r.Header.Get("X-Custom-Header"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hello world"}`))
	}))
	defer ts.Close()

	exec := NewHTTPExecutor(ts.Client())
	configJSON := []byte(`{
		"method": "GET",
		"url": "` + ts.URL + `",
		"headers": {"X-Custom-Header": "TestValue"}
	}`)

	s := &step.Step{ID: "step-1", Type: step.StepTypeHTTP}
	se := &step.StepExecution{ID: "exec-1", Config: configJSON}

	output, err := exec.Execute(context.Background(), s, se)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if parsed["message"] != "hello world" {
		t.Errorf("expected message='hello world', got %v", parsed["message"])
	}
}

func TestHTTPExecutor_Success_POST_JSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		var bodyMap map[string]any
		_ = json.Unmarshal(body, &bodyMap)
		if bodyMap["order_id"] != "ORD-123" {
			t.Errorf("unexpected body content: %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created","id":"ORD-123"}`))
	}))
	defer ts.Close()

	exec := NewHTTPExecutor(ts.Client())
	configJSON := []byte(`{
		"method": "POST",
		"url": "` + ts.URL + `",
		"body": {"order_id": "ORD-123"}
	}`)

	s := &step.Step{ID: "step-2", Type: step.StepTypeHTTP}
	se := &step.StepExecution{ID: "exec-2", Config: configJSON}

	output, err := exec.Execute(context.Background(), s, se)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if parsed["status"] != "created" || parsed["id"] != "ORD-123" {
		t.Errorf("unexpected output: %v", parsed)
	}
}

func TestHTTPExecutor_Success_PlainText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("raw text response"))
	}))
	defer ts.Close()

	exec := NewHTTPExecutor(ts.Client())
	configJSON := []byte(`{"url": "` + ts.URL + `"}`)

	s := &step.Step{ID: "step-text", Type: step.StepTypeHTTP}
	se := &step.StepExecution{ID: "exec-text", Config: configJSON}

	output, err := exec.Execute(context.Background(), s, se)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if parsed["data"] != "raw text response" {
		t.Errorf("expected data='raw text response', got %v", parsed["data"])
	}
}

func TestHTTPExecutor_HTTP4xxAnd5xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"database unavailable"}`))
	}))
	defer ts.Close()

	exec := NewHTTPExecutor(ts.Client())
	configJSON := []byte(`{"url": "` + ts.URL + `"}`)

	s := &step.Step{ID: "step-err", Type: step.StepTypeHTTP}
	se := &step.StepExecution{ID: "exec-err", Config: configJSON}

	_, err := exec.Execute(context.Background(), s, se)
	if err == nil {
		t.Fatalf("expected error on HTTP 500, got nil")
	}
	if !errors.Is(err, ErrInvalidStepConfig) && err.Error() == "" {
		t.Errorf("expected descriptive error, got %v", err)
	}
}

func TestHTTPExecutor_EmptyURL(t *testing.T) {
	exec := NewHTTPExecutor(nil)
	configJSON := []byte(`{"method":"GET","url":""}`)

	s := &step.Step{ID: "step-empty", Type: step.StepTypeHTTP}
	se := &step.StepExecution{ID: "exec-empty", Config: configJSON}

	_, err := exec.Execute(context.Background(), s, se)
	if !errors.Is(err, ErrEmptyHTTPURL) {
		t.Fatalf("expected ErrEmptyHTTPURL, got %v", err)
	}
}

func TestHTTPExecutor_InvalidConfig(t *testing.T) {
	exec := NewHTTPExecutor(nil)
	s := &step.Step{ID: "step-inv", Type: step.StepTypeHTTP}
	se := &step.StepExecution{ID: "exec-inv", Config: []byte(`invalid-json`)}

	_, err := exec.Execute(context.Background(), s, se)
	if !errors.Is(err, ErrInvalidStepConfig) {
		t.Fatalf("expected ErrInvalidStepConfig, got %v", err)
	}
}

func TestHTTPExecutor_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	exec := NewHTTPExecutor(ts.Client())
	configJSON := []byte(`{"url": "` + ts.URL + `"}`)

	s := &step.Step{ID: "step-to", Type: step.StepTypeHTTP}
	// step timeout of 1 second, but context timeout of 20ms
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	se := &step.StepExecution{ID: "exec-to", Config: configJSON}

	_, err := exec.Execute(ctx, s, se)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}
