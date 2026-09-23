package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

var (
	ErrEmptyHTTPURL = errors.New("http step url cannot be empty")
)

// HTTPClient allows plugging in custom or mock HTTP clients
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type httpExecutor struct {
	client HTTPClient
}

// NewHTTPExecutor creates a StepExecutor for StepTypeHTTP
func NewHTTPExecutor(client HTTPClient) StepExecutor {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return &httpExecutor{client: client}
}

// Execute performs the HTTP request specified in the step configuration
func (e *httpExecutor) Execute(ctx context.Context, s *step.Step, exec *step.StepExecution) (json.RawMessage, error) {
	// extract config from execution ticket, fallback to step definition
	configRaw := exec.Config
	if len(configRaw) == 0 && s != nil {
		configRaw = s.Config
	}
	if len(configRaw) == 0 {
		return nil, ErrInvalidStepConfig
	}

	var cfg step.HTTPConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return nil, fmt.Errorf("%w: failed to parse HTTPConfig: %v", ErrInvalidStepConfig, err)
	}

	urlStr := strings.TrimSpace(cfg.URL)
	if urlStr == "" {
		return nil, ErrEmptyHTTPURL
	}

	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodGet
	}

	// resolve step timeout if specified
	timeoutSeconds := 0
	if exec.TimeoutSeconds > 0 {
		timeoutSeconds = exec.TimeoutSeconds
	} else if s != nil && s.Timeout > 0 {
		timeoutSeconds = s.Timeout
	}

	reqCtx := ctx
	if timeoutSeconds > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
	}

	// prepare body reader
	var bodyReader io.Reader
	if len(cfg.Body) > 0 {
		bodyReader = bytes.NewReader(cfg.Body)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, urlStr, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	// set custom headers
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	if len(cfg.Body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// return error on HTTP 4xx and 5xx so worker retries can trigger
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http step failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// format output as JSON
	if len(respBody) == 0 {
		return json.RawMessage("{}"), nil
	}

	if json.Valid(respBody) {
		return json.RawMessage(respBody), nil
	}

	// wrap non-JSON responses into structured JSON
	wrapped, err := json.Marshal(map[string]any{
		"status_code": resp.StatusCode,
		"data":        string(respBody),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode non-json response: %w", err)
	}

	return json.RawMessage(wrapped), nil
}
