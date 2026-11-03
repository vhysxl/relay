package execution

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type ExecutionStatus string

type WorkflowStatus = ExecutionStatus
type StepStatus = ExecutionStatus

// Shared statuses (Workflow & Step)
const (
	StatusRunning            ExecutionStatus = "RUNNING"
	StatusCompleted          ExecutionStatus = "COMPLETED"
	StatusFailed             ExecutionStatus = "FAILED"
	StatusCompensated        ExecutionStatus = "COMPENSATED"
	StatusCompensationFailed ExecutionStatus = "COMPENSATION_FAILED"
)

// Workflow-only status
const (
	StatusCompensating ExecutionStatus = "COMPENSATING"
)

// Step-only statuses
const (
	StatusWaiting   ExecutionStatus = "WAITING"
	StatusPending   ExecutionStatus = "PENDING"
	StatusCancelled ExecutionStatus = "CANCELLED"
	StatusSkipped   ExecutionStatus = "SKIPPED"
)

type WorkflowExecution struct {
	ID           string          `json:"id" db:"id"`
	WorkflowID   string          `json:"workflow_id" db:"workflow_id"`
	WorkflowName string          `json:"workflow_name,omitempty" db:"-"`
	Status       WorkflowStatus  `json:"status" db:"status"`
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
	Status              StepStatus      `json:"status" db:"status"`

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

	IsCompensation      bool            `json:"is_compensation" db:"is_compensation"`
	When                json.RawMessage `json:"when,omitempty" db:"condition"`
	ConditionEvaluation json.RawMessage `json:"condition_evaluation,omitempty" db:"condition_evaluation"`
	ExecutedAt          *time.Time      `json:"executed_at,omitempty" db:"executed_at"`
	NextRetryAt         *time.Time      `json:"next_retry_at,omitempty" db:"next_retry_at"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type ExecuteWorkflowRequest struct {
	Context        json.RawMessage `json:"context,omitempty"`
	IdempotencyKey string          `json:"-"`
}

type ExecutionRequestRecord struct {
	WorkflowName        string          `db:"workflow_name"`
	IdempotencyKey      string          `db:"idempotency_key"`
	WorkflowExecutionID string          `db:"workflow_execution_id"`
	InputContext        json.RawMessage `db:"input_context"`
	ResponseReceipt     json.RawMessage `db:"response_receipt"`
	CreatedAt           time.Time       `db:"created_at"`
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
	ExecuteWorkflow(ctx context.Context, name string, req *ExecuteWorkflowRequest) (*ExecuteWorkflowResponse, bool, error)
	ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error)
}

type ExecutionRepository interface {
	CreateExecutionWithIdempotency(ctx context.Context, exec *WorkflowExecution, record *ExecutionRequestRecord, targetIsExplicitID bool, targetID string) (*ExecuteWorkflowResponse, bool, error)
	GetExecutionRequest(ctx context.Context, workflowName string, idempotencyKey string) (*ExecutionRequestRecord, error)
	CreateExecution(ctx context.Context, execution *WorkflowExecution) (*WorkflowExecution, error)
	ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error)
}

func ValidateIdempotencyKey(key string) error {
	if len(key) < 1 || len(key) > 128 {
		return errdefs.ErrInvalidIdempotencyKey
	}
	for i := 0; i < len(key); i++ {
		b := key[i]
		if b < 33 || b > 126 {
			return errdefs.ErrInvalidIdempotencyKey
		}
	}
	return nil
}

type WorkflowReader interface {
	GetWorkflow(ctx context.Context, name string) (*workflow.Workflow, error)
}

// Repository is the SQL store's combined contract. Consumers depend on the
// smaller interfaces they need, rather than this wiring interface.
type Repository interface {
	ExecutionRepository
	ClaimPending(ctx context.Context, workerID string, leaseDuration time.Duration) (*step.Step, *StepExecution, error)
	ReapExpiredSteps(ctx context.Context) (int64, error)
	Heartbeat(ctx context.Context, stepExecutionID string, workerID string, leaseDuration time.Duration) error
	CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error
	FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error
}
