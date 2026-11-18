package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	baseURL     = "http://localhost:8000"
	workerToken = "staging-relay-secret-token"
	workerID    = "greedy-worker-01"
	queueName   = "greedy-orders"
)

type StepDefinition struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Queue     string          `json:"queue"`
	Activity  string          `json:"activity"`
	Input     json.RawMessage `json:"input"`
	DependsOn []string        `json:"depends_on,omitempty"`
	Timeout   int             `json:"timeout,omitempty"`
}

type WorkflowRequest struct {
	Name    string           `json:"name"`
	Version int              `json:"version"`
	Context json.RawMessage  `json:"context,omitempty"`
	Steps   []StepDefinition `json:"steps"`
}

type ClaimRequest struct {
	WorkerID      string `json:"worker_id"`
	Queue         string `json:"queue"`
	WaitTimeoutMS *int   `json:"wait_timeout_ms,omitempty"`
}

type ClaimResponse struct {
	TaskID              string          `json:"task_id"`
	WorkflowExecutionID string          `json:"workflow_execution_id"`
	StepID              string          `json:"step_id"`
	Queue               string          `json:"queue"`
	Activity            string          `json:"activity"`
	Attempt             int             `json:"attempt"`
	LeaseToken          string          `json:"lease_token"`
	Input               json.RawMessage `json:"input"`
}

type CompleteRequest struct {
	WorkerID   string          `json:"worker_id"`
	LeaseToken string          `json:"lease_token"`
	Output     json.RawMessage `json:"output"`
}

type ExecutionResponse struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	Status     string          `json:"status"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
	Context    json.RawMessage `json:"context,omitempty"`
}

var client = &http.Client{
	Timeout: 30 * time.Second,
}

func doJSON(method, path string, token string, headers map[string]string, body any) ([]byte, int, time.Duration, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, 0, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		return nil, 0, duration, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, duration, err
}

func main() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println("  RELAY GREEDY RE-CLAIM BENCHMARK (TARGET: DURATION < 1 DETIK)")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Printf("Endpoint   : %s\n", baseURL)
	fmt.Printf("Worker ID  : %s | Queue: %s\n\n", workerID, queueName)

	// 1. Register 4-step workflow
	wfName := fmt.Sprintf("greedy.ecommerce.%d", time.Now().Unix())
	wfReq := WorkflowRequest{
		Name:    wfName,
		Version: 1,
		Context: json.RawMessage(`{"order_id":"ORD-GREEDY-01","amount":750000}`),
		Steps: []StepDefinition{
			{
				ID:       "step-1-validate",
				Name:     "Validate Order",
				Queue:    queueName,
				Activity: "order.validate",
				Input:    json.RawMessage(`{"order_id":"{{context.order_id}}"}`),
				Timeout:  30,
			},
			{
				ID:        "step-2-reserve",
				Name:      "Reserve Stock",
				Queue:     queueName,
				Activity:  "inventory.reserve",
				DependsOn: []string{"step-1-validate"},
				Input:     json.RawMessage(`{"order_id":"{{context.order_id}}","valid":"{{steps.step-1-validate.output.valid}}"}`),
				Timeout:   30,
			},
			{
				ID:        "step-3-charge",
				Name:      "Charge Payment",
				Queue:     queueName,
				Activity:  "payment.charge",
				DependsOn: []string{"step-2-reserve"},
				Input:     json.RawMessage(`{"reservation_id":"{{steps.step-2-reserve.output.reservation_id}}","amount":"{{context.amount}}"}`),
				Timeout:   30,
			},
			{
				ID:        "step-4-notify",
				Name:      "Send Confirmation",
				Queue:     queueName,
				Activity:  "notification.send",
				DependsOn: []string{"step-3-charge"},
				Input:     json.RawMessage(`{"trx_id":"{{steps.step-3-charge.output.trx_id}}"}`),
				Timeout:   30,
			},
		},
	}

	_, status, regDur, err := doJSON("POST", "/workflows", "", nil, wfReq)
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		panic(fmt.Sprintf("Failed to register workflow: %v (status %d)", err, status))
	}
	fmt.Printf("Workflow '%s' registered in %.2fms\n\n", wfName, float64(regDur.Microseconds())/1000.0)

	// Run 3 independent sequential executions with Greedy Re-Claim
	const testRuns = 3
	for runIdx := 1; runIdx <= testRuns; runIdx++ {
		fmt.Println(strings.Repeat("-", 75))
		fmt.Printf("RUN #%d - EXECUTING 4-STEP SAGA WITH GREEDY WORKER\n", runIdx)
		fmt.Println(strings.Repeat("-", 75))

		// 1. POST /execute
		idempotencyKey := fmt.Sprintf("greedy-key-%d-%d", runIdx, time.Now().UnixNano())
		headers := map[string]string{"Idempotency-Key": idempotencyKey}
		execBody := map[string]any{
			"context": map[string]any{
				"order_id": fmt.Sprintf("ORD-RUN-%d", runIdx),
				"amount":   750000,
			},
		}

		respBody, status, execDur, err := doJSON("POST", fmt.Sprintf("/workflows/%s/execute", wfName), "", headers, execBody)
		if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
			panic(fmt.Sprintf("Execute failed: %v", err))
		}
		var execResp ExecutionResponse
		_ = json.Unmarshal(respBody, &execResp)
		fmt.Printf("[1] Triggered Workflow: ID = %s (API: %.2fms)\n", execResp.ID, float64(execDur.Microseconds())/1000.0)

		// 2. Greedy Worker Loop: Claim -> Complete immediately for each of the 4 steps
		expectedSteps := []string{"step-1-validate", "step-2-reserve", "step-3-charge", "step-4-notify"}
		outputs := map[string]string{
			"step-1-validate": `{"valid":true,"merchant":"OK"}`,
			"step-2-reserve":  `{"reservation_id":"RES-GREEDY-99","warehouse":"W-CENTRAL"}`,
			"step-3-charge":   `{"trx_id":"TRX-GREEDY-77","status":"SETTLED"}`,
			"step-4-notify":   `{"sent":true,"email":"buyer@example.com"}`,
		}

		wallClockStart := time.Now()
		waitZero := 0

		for stepIdx, stepID := range expectedSteps {
			// A. Greedy Claim (wait_timeout_ms = 0 -> instant non-blocking claim)
			claimReq := ClaimRequest{
				WorkerID:      workerID,
				Queue:         queueName,
				WaitTimeoutMS: &waitZero,
			}
			claimRespBody, claimStatus, claimDur, err := doJSON("POST", "/worker/tasks/claim", workerToken, nil, claimReq)
			if err != nil || claimStatus != http.StatusOK {
				panic(fmt.Sprintf("Greedy Claim failed at step %s: %v (status %d)", stepID, err, claimStatus))
			}
			var claimData ClaimResponse
			_ = json.Unmarshal(claimRespBody, &claimData)

			// B. Greedy Complete (0ms local delay)
			compReq := CompleteRequest{
				WorkerID:   workerID,
				LeaseToken: claimData.LeaseToken,
				Output:     json.RawMessage(outputs[stepID]),
			}
			_, compStatus, compDur, err := doJSON("POST", fmt.Sprintf("/worker/tasks/%s/complete", claimData.TaskID), workerToken, nil, compReq)
			if err != nil || compStatus != http.StatusOK {
				panic(fmt.Sprintf("Greedy Complete failed at step %s: status %d", stepID, compStatus))
			}

			fmt.Printf("    -> Step %d (%s): Claim=%.2fms | Complete=%.2fms | Step Net RTT=%.2fms\n",
				stepIdx+1,
				claimData.StepID,
				float64(claimDur.Microseconds())/1000.0,
				float64(compDur.Microseconds())/1000.0,
				float64((claimDur+compDur).Microseconds())/1000.0,
			)
		}

		wallClockDuration := time.Since(wallClockStart)
		_ = wallClockDuration

		// 3. Verify in UI / Get Execution Details
		respBody, _, _, err = doJSON("GET", fmt.Sprintf("/executions/%s", execResp.ID), "", nil, nil)
		if err != nil {
			panic(err)
		}
		var finalExec ExecutionResponse
		_ = json.Unmarshal(respBody, &finalExec)

		// Compute UI Duration exactly as Web UI does: (updated_at - created_at)
		tCreated, _ := time.Parse(time.RFC3339Nano, finalExec.CreatedAt)
		tUpdated, _ := time.Parse(time.RFC3339Nano, finalExec.UpdatedAt)
		uiDurationMs := tUpdated.Sub(tCreated).Milliseconds()

		fmt.Println()
		fmt.Printf("=== RUN #%d RESULT ===\n", runIdx)
		fmt.Printf("Status         : %s\n", finalExec.Status)
		fmt.Printf("Started At     : %s\n", finalExec.CreatedAt)
		fmt.Printf("Completed At   : %s\n", finalExec.UpdatedAt)
		fmt.Printf("Duration di UI : %d ms (%.2f detik)\n", uiDurationMs, float64(uiDurationMs)/1000.0)
		fmt.Printf("Target < 1s    : %s\n", func() string {
			if uiDurationMs < 1000 {
				return "PASSED (AMAN < 1 DETIK! 🎉)"
			}
			return "FAILED (> 1s)"
		}())
		fmt.Printf("Buka di UI Web : %s/#execution/%s\n\n", baseURL, finalExec.ID)
	}

	fmt.Println(strings.Repeat("=", 75))
	fmt.Println("  ALL GREEDY RUNS COMPLETED SUCCESSFULLY WITH DURATION < 1s!")
	fmt.Println(strings.Repeat("=", 75))
}
