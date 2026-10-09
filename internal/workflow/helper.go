package workflow

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

func (r *WorkflowRequest) ValidateAndNormalize() error {
	if r == nil {
		return ErrInvalidWorkflowName
	}

	// Name
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return ErrInvalidWorkflowName
	}
	if utf8.RuneCountInString(r.Name) > 255 {
		return ErrWorkflowNameTooLong
	}

	// Version
	if r.Version < 0 {
		return ErrInvalidWorkflowVersion
	}

	if r.Version == 0 {
		r.Version = 1
	}

	// Steps
	if len(r.Steps) == 0 {
		return ErrWorkflowEmptySteps
	}

	// Context
	if len(r.Context) > 0 {
		ctx, err := validateWorkflowContext(r.Context)
		if err != nil {
			return err
		}
		r.Context = ctx
	}

	return nil
}

func validateWorkflowContext(data json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	// make sure context is an object, not array or primitive
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil || obj == nil {
		return nil, ErrInvalidWorkflowContext
	}
	return trimmed, nil
}
