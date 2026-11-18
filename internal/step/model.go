package step

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Step struct {
	ID           string          `json:"id" db:"id"`
	WorkflowID   string          `json:"workflow_id,omitempty" db:"workflow_id"`
	Name         string          `json:"name,omitempty" db:"name"`
	Queue        string          `json:"queue" db:"queue"`
	Activity     string          `json:"activity" db:"activity"`
	Input        json.RawMessage `json:"input" db:"input"`
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
	Queue    string          `json:"queue"`
	Activity string          `json:"activity"`
	Input    json.RawMessage `json:"input"`
	Timeout  int             `json:"timeout,omitempty"`
	Retry    *RetryPolicy    `json:"retry,omitempty"`
}

type RetryPolicy struct {
	MaxAttempts int     `json:"max_attempts"`
	Backoff     int     `json:"backoff"`
	Multiplier  float64 `json:"multiplier,omitempty"`
	MaxBackoff  int     `json:"max_backoff,omitempty"`
}

type StepRepository interface {
	CreateStepsTx(ctx context.Context, tx *sql.Tx, workflowID string, steps []Step) error
	GetStepsByWorkflowID(ctx context.Context, workflowID string) ([]Step, error)
	GetStepsByWorkflowIDs(ctx context.Context, workflowIDs []string) (map[string][]Step, error)
}
