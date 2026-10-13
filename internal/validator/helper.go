package validator

import (
	"bytes"
	"encoding/json"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

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

func areRetriesEqual(r1, r2 *step.RetryPolicy) bool {
	if r1 == nil && r2 == nil {
		return true
	}
	if r1 == nil || r2 == nil {
		return false
	}
	return r1.MaxAttempts == r2.MaxAttempts &&
		r1.Backoff == r2.Backoff &&
		r1.Multiplier == r2.Multiplier &&
		r1.MaxBackoff == r2.MaxBackoff
}

func areCompensationsEqual(c1, c2 *step.Compensation) bool {
	if c1 == nil && c2 == nil {
		return true
	}
	if c1 == nil || c2 == nil {
		return false
	}
	return c1.Type == c2.Type &&
		util.AreJSONEqual(c1.Config, c2.Config) &&
		c1.Timeout == c2.Timeout &&
		areRetriesEqual(c1.Retry, c2.Retry)
}

func isStepEqual(s1, s2 step.Step) bool {
	return s1.ID == s2.ID &&
		s1.Type == s2.Type &&
		s1.Name == s2.Name &&
		s1.Timeout == s2.Timeout &&
		s1.Irreversible == s2.Irreversible &&
		util.AreJSONEqual(s1.Config, s2.Config) &&
		util.AreStringSlicesEqual(s1.DependsOn, s2.DependsOn) &&
		areRetriesEqual(s1.Retry, s2.Retry) &&
		areCompensationsEqual(s1.Compensation, s2.Compensation)
}
