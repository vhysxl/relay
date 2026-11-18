package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/execution"
)

type Repository interface {
	ClaimTask(ctx context.Context, filter execution.ClaimFilter) (*execution.ClaimResult, error)
	HeartbeatTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, duration time.Duration) (time.Time, time.Duration, error)
	CompleteTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, output json.RawMessage) error
	FailTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, errMsg string, retryable bool) error
	GetStepTaskQueue(ctx context.Context, stepExecID string) (string, error)
}

type ClaimRequest struct {
	WorkerID      string   `json:"worker_id"`
	Queue         string   `json:"queue"`
	ExecutorTypes []string `json:"executor_types"`
	Activities    []string `json:"activities"`
	WaitTimeoutMS *int     `json:"wait_timeout_ms"`
}

type ClaimTaskResponse struct {
	TaskID              string          `json:"task_id"`
	WorkflowExecutionID string          `json:"workflow_execution_id"`
	StepID              string          `json:"step_id"`
	Type                string          `json:"type"`
	TaskQueue           string          `json:"task_queue"`
	ActivityName        string          `json:"activity_name,omitempty"`
	Attempt             int             `json:"attempt"`
	IsCompensation      bool            `json:"is_compensation"`
	TimeoutSeconds      int             `json:"timeout_seconds"`
	Deadline            string          `json:"deadline"`
	HeartbeatIntervalMS int64           `json:"heartbeat_interval_ms"`
	LeaseToken          string          `json:"lease_token"`
	LeaseExpiresAt      string          `json:"lease_expires_at"`
	LeaseDurationMS     int64           `json:"lease_duration_ms"`
	Payload             json.RawMessage `json:"payload"`
}

type HeartbeatRequest struct {
	WorkerID        string `json:"worker_id"`
	LeaseToken      string `json:"lease_token"`
	LeaseDurationMS int64  `json:"lease_duration_ms,omitempty"`
}

type HeartbeatResponse struct {
	LeaseExpiresAt  string `json:"lease_expires_at"`
	LeaseDurationMS int64  `json:"lease_duration_ms"`
}

type CompleteRequest struct {
	WorkerID   string          `json:"worker_id"`
	LeaseToken string          `json:"lease_token"`
	Output     json.RawMessage `json:"output"`
}

type CompleteResponse struct {
	Accepted bool `json:"accepted"`
}

type TaskError struct {
	Message   string `json:"message"`
	Retryable *bool  `json:"retryable,omitempty"`
}

type FailRequest struct {
	WorkerID   string    `json:"worker_id"`
	LeaseToken string    `json:"lease_token"`
	Error      TaskError `json:"error"`
}

type FailResponse struct {
	Accepted bool `json:"accepted"`
}

type WorkerLogic interface {
	ClaimTask(ctx context.Context, serviceID string, req ClaimRequest) (*ClaimTaskResponse, error)
	HeartbeatTask(ctx context.Context, serviceID, taskID string, req HeartbeatRequest) (*HeartbeatResponse, error)
	CompleteTask(ctx context.Context, serviceID, taskID string, req CompleteRequest) (*CompleteResponse, error)
	FailTask(ctx context.Context, serviceID, taskID string, req FailRequest) (*FailResponse, error)
	GetStepTaskQueue(ctx context.Context, stepExecID string) (string, error)
	NotifyQueue(queue string)
}
