package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TestWorkerClient simulates an external worker connecting via the HTTP worker protocol.
type TestWorkerClient struct {
	BaseURL    string
	Token      string
	WorkerID   string
	Queue      string
	HTTPClient *http.Client
}

// NewTestWorkerClient creates a new TestWorkerClient for testing worker interactions.
func NewTestWorkerClient(baseURL, token, workerID, queue string) *TestWorkerClient {
	return &TestWorkerClient{
		BaseURL:    baseURL,
		Token:      token,
		WorkerID:   workerID,
		Queue:      queue,
		HTTPClient: &http.Client{Timeout: 35 * time.Second},
	}
}

func (c *TestWorkerClient) Claim(ctx context.Context, waitTimeoutMS int) (*ClaimTaskResponse, error) {
	reqBody := ClaimRequest{
		WorkerID:      c.WorkerID,
		Queue:         c.Queue,
		WaitTimeoutMS: &waitTimeoutMS,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/worker/tasks/claim", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("claim failed HTTP %d: %s", resp.StatusCode, string(b))
	}

	var res ClaimTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *TestWorkerClient) Heartbeat(ctx context.Context, taskID, leaseToken string, durationMS int64) (*HeartbeatResponse, error) {
	reqBody := HeartbeatRequest{
		WorkerID:        c.WorkerID,
		LeaseToken:      leaseToken,
		LeaseDurationMS: durationMS,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/worker/tasks/%s/heartbeat", c.BaseURL, taskID), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("heartbeat failed HTTP %d: %s", resp.StatusCode, string(b))
	}

	var res HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *TestWorkerClient) Complete(ctx context.Context, taskID, leaseToken string, output json.RawMessage) error {
	reqBody := CompleteRequest{
		WorkerID:   c.WorkerID,
		LeaseToken: leaseToken,
		Output:     output,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/worker/tasks/%s/complete", c.BaseURL, taskID), bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete failed HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *TestWorkerClient) Fail(ctx context.Context, taskID, leaseToken, errMsg string, retryable bool) error {
	reqBody := FailRequest{
		WorkerID:   c.WorkerID,
		LeaseToken: leaseToken,
		Error: TaskError{
			Message:   errMsg,
			Retryable: &retryable,
		},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/worker/tasks/%s/fail", c.BaseURL, taskID), bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fail failed HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
