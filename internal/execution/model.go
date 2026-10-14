package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/step"
)

type ExecutionStatus string

const (
	StatusPending            ExecutionStatus = "PENDING"
	StatusWaiting            ExecutionStatus = "WAITING"
	StatusRunning            ExecutionStatus = "RUNNING"
	StatusCompleted          ExecutionStatus = "COMPLETED"
	StatusFailed             ExecutionStatus = "FAILED"
	StatusCompensating       ExecutionStatus = "COMPENSATING"
	StatusCompensated        ExecutionStatus = "COMPENSATED"
	StatusCompensationFailed ExecutionStatus = "COMPENSATION_FAILED"
	StatusCancelled          ExecutionStatus = "CANCELLED"
)

type WorkflowExecution struct {
	ID           string          `json:"id" db:"id"`
	WorkflowID   string          `json:"workflow_id" db:"workflow_id"`
	WorkflowName string          `json:"workflow_name,omitempty" db:"-"`
	Status       ExecutionStatus `json:"status" db:"status"`
	Context      json.RawMessage `json:"context,omitempty" db:"context"`
	ErrorMessage *string         `json:"error_message,omitempty" db:"error_message"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`

	Steps []StepExecution `json:"steps,omitempty" db:"-"`
}

type StepExecution struct {
	ID                  string          `json:"id" db:"id"`
	WorkflowExecutionID string          `json:"workflow_execution_id" db:"workflow_execution_id"`
	StepID              string          `json:"step_id" db:"step_id"`
	Name                string          `json:"name,omitempty" db:"name"`
	Type                string          `json:"type" db:"type"`
	Config              json.RawMessage `json:"config" db:"config"`
	DependsOn           []string        `json:"depends_on,omitempty" db:"depends_on"`
	Status              ExecutionStatus `json:"status" db:"status"`

	Output       json.RawMessage `json:"output,omitempty" db:"output"`
	ErrorMessage *string         `json:"error_message,omitempty" db:"error_message"`
	Attempt      int             `json:"attempt" db:"attempt"`
	MaxAttempts  int             `json:"max_attempts" db:"max_attempts"`

	TimeoutSeconds    int     `json:"timeout_seconds" db:"timeout_seconds"`
	BackoffSeconds    int     `json:"backoff_seconds" db:"backoff_seconds"`
	Multiplier        float64 `json:"multiplier" db:"multiplier"`
	MaxBackoffSeconds int     `json:"max_backoff_seconds" db:"max_backoff_seconds"`

	LockedBy    *string    `json:"locked_by,omitempty" db:"locked_by"`
	LockedUntil *time.Time `json:"locked_until,omitempty" db:"locked_until"`

	IsCompensation bool       `json:"is_compensation" db:"is_compensation"`
	ExecutedAt     *time.Time `json:"executed_at,omitempty" db:"executed_at"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty" db:"next_retry_at"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type ExecuteWorkflowRequest struct {
	Context json.RawMessage `json:"context,omitempty"`
}

type ExecuteWorkflowResponse struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	Status     string          `json:"status"`
	Context    json.RawMessage `json:"context,omitempty"`
	TotalSteps int             `json:"total_steps"`
	CreatedAt  time.Time       `json:"created_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type ExecutionLogic interface {
	ExecuteWorkflow(ctx context.Context, name string, req *ExecuteWorkflowRequest) (*WorkflowExecution, error)
	ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error)
}

type ExecutionRepository interface {
	ExecuteWorkflowTx(ctx context.Context, name string, initialContext json.RawMessage) (*WorkflowExecution, error)
	ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error)
	ClaimPending(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *StepExecution, error)
	ReapExpiredSteps(ctx context.Context) (int64, error)
	WakeUpDependentsTx(ctx context.Context, tx *sql.Tx, workflowExecID string, isCompensation bool) error
	Heartbeat(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error
	CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error
	FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error
}

