package condition

import (
	"encoding/json"
	"time"
)

// Supported comparison and membership operators
const (
	OpEq     = "eq"
	OpNe     = "ne"
	OpExists = "exists"
	OpGt     = "gt"
	OpGte    = "gte"
	OpLt     = "lt"
	OpLte    = "lte"
	OpIn     = "in"
)

// Condition represents either a leaf comparison or a logical group (all/any).
type Condition struct {
	Ref   string          `json:"ref,omitempty"`
	Op    string          `json:"op,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`

	All []Condition `json:"all,omitempty"`
	Any []Condition `json:"any,omitempty"`
}

// CheckDetail records audit information for one executed comparison check.
type CheckDetail struct {
	Ref      string          `json:"ref"`
	Op       string          `json:"op"`
	Expected json.RawMessage `json:"expected,omitempty"`
	Actual   json.RawMessage `json:"actual,omitempty"`
	Missing  bool            `json:"missing,omitempty"`
	Result   *bool           `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// EvaluationAudit captures the timestamp, overall outcome, and list of executed checks.
type EvaluationAudit struct {
	EvaluatedAt time.Time     `json:"evaluated_at"`
	Result      *bool         `json:"result,omitempty"`
	Error       string        `json:"error,omitempty"`
	Checks      []CheckDetail `json:"checks"`
}
