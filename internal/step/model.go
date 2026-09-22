package step

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type StepType string

const (
	StepTypeHTTP    StepType = "http"
	StepTypeMessage StepType = "message"
)

type Step struct {
	ID           string          `json:"id" db:"id"`
	WorkflowID   string          `json:"workflow_id,omitempty" db:"workflow_id"`
	Name         string          `json:"name,omitempty" db:"name"`
	Type         StepType        `json:"type" db:"type"`
	Config       json.RawMessage `json:"config" db:"config"`
	DependsOn    []string        `json:"depends_on,omitempty" db:"depends_on"`
	Timeout      int             `json:"timeout,omitempty" db:"timeout"`
	Retry        *RetryPolicy    `json:"retry,omitempty" db:"retry"`
	Compensation *Compensation   `json:"compensation,omitempty" db:"compensation"`
	Irreversible bool            `json:"irreversible,omitempty" db:"irreversible"`
	CreatedAt    time.Time       `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at,omitempty" db:"updated_at"`
}

type Compensation struct {
	Type    StepType        `json:"type"`
	Config  json.RawMessage `json:"config"`
	Timeout int             `json:"timeout,omitempty"`
	Retry   *RetryPolicy    `json:"retry,omitempty"`
}

type RetryPolicy struct {
	MaxAttempts int     `json:"max_attempts"`
	Backoff     int     `json:"backoff"`
	Multiplier  float64 `json:"multiplier,omitempty"`
	MaxBackoff  int     `json:"max_backoff,omitempty"`
}

type HTTPConfig struct {
	Method  string            `json:"method"`            // "GET", "POST", "PUT", dll.
	URL     string            `json:"url"`               // Endpoint target
	Headers map[string]string `json:"headers,omitempty"` // Header kustom seperti Authorization
	Body    json.RawMessage   `json:"body,omitempty"`    // Payload JSON yang akan dikirim
}

type RabbitMQConfig struct {
	Exchange    string          `json:"exchange"`               // Nama exchange
	RoutingKey  string          `json:"routing_key"`            // Routing key / Queue tujuan
	ContentType string          `json:"content_type,omitempty"` // Default: "application/json"
	Headers     map[string]any  `json:"headers,omitempty"`      // AMQP Table headers
	Payload     json.RawMessage `json:"payload"`                // Data pesan yang akan dipublish
}

type ExecutionStatus string

const (
	StatusPending            ExecutionStatus = "PENDING"
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

const DefaultLockDuration = 30 * time.Second

type StepValidator interface {
	ValidateSteps(steps []Step) error
}

type StepRepository interface {
	CreateStepsTx(ctx context.Context, tx *sql.Tx, workflowID string, steps []Step) error
	GetStepsByWorkflowID(ctx context.Context, workflowID string) ([]Step, error)
	GetStepByID(ctx context.Context, workflowID string, stepID string) (*Step, error)
	ClaimNext(ctx context.Context, workerID string, lockDuration time.Duration) (*Step, *StepExecution, error)
}
