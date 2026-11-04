package step

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type StepType string

const (
	StepTypeHTTP     StepType = "http"
	StepTypeMessage  StepType = "message"
	StepTypeActivity StepType = "activity"
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
	When         json.RawMessage `json:"when,omitempty" db:"condition"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
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
	Queue   string            `json:"queue,omitempty"`   // Routing queue target (default: "legacy")
	Method  string            `json:"method"`            // "GET", "POST", "PUT", dll.
	URL     string            `json:"url"`               // Endpoint target
	Headers map[string]string `json:"headers,omitempty"` // Header kustom seperti Authorization
	Body    json.RawMessage   `json:"body,omitempty"`    // Payload JSON yang akan dikirim
}

type RabbitMQConfig struct {
	Queue       string          `json:"queue,omitempty"`       // Routing queue target (default: "legacy")
	Exchange    string          `json:"exchange"`               // Nama exchange
	RoutingKey  string          `json:"routing_key"`            // Routing key / Queue tujuan
	ContentType string          `json:"content_type,omitempty"` // Default: "application/json"
	Headers     map[string]any  `json:"headers,omitempty"`      // AMQP Table headers
	Payload     json.RawMessage `json:"payload"`                // Data pesan yang akan dipublish
}

type ActivityConfig struct {
	Queue    string          `json:"queue"`           // Target task queue (wajib)
	Activity string          `json:"activity"`        // Nama activity handler (wajib)
	Input    json.RawMessage `json:"input,omitempty"` // Input JSON payload untuk activity
}

type StepRepository interface {
	CreateStepsTx(ctx context.Context, tx *sql.Tx, workflowID string, steps []Step) error
	GetStepsByWorkflowID(ctx context.Context, workflowID string) ([]Step, error)
	GetStepsByWorkflowIDs(ctx context.Context, workflowIDs []string) (map[string][]Step, error)
	GetStepByID(ctx context.Context, workflowID string, stepID string) (*Step, error)
}
