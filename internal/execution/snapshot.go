package execution

import (
	"encoding/json"
	"time"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

func snapshotSteps(execID string, definitions []step.Step, context json.RawMessage, now time.Time) []StepExecution {
	reverse := make(map[string][]string, len(definitions))
	for _, s := range definitions {
		for _, dep := range s.DependsOn {
			reverse[dep] = append(reverse[dep], s.ID)
		}
	}
	result := make([]StepExecution, 0, len(definitions)*2)
	for _, s := range definitions {
		normal := snapshotStep(execID, s.ID, s.Name, s.Type, s.Config, s.DependsOn, s.Timeout, s.Retry, context, now)
		if len(s.When) > 0 {
			normal.When = append(json.RawMessage(nil), s.When...)
		}
		if len(s.DependsOn) == 0 {
			normal.Status = StatusPending
		}
		result = append(result, normal)
		if c := s.Compensation; c != nil {
			comp := snapshotStep(execID, s.ID, s.Name+" (Compensation)", c.Type, c.Config, reverse[s.ID], c.Timeout, c.Retry, context, now)
			comp.IsCompensation = true
			result = append(result, comp)
		}
	}
	return result
}

func snapshotStep(execID, stepID, name string, kind step.StepType, config json.RawMessage, deps []string, timeout int, retry *step.RetryPolicy, context json.RawMessage, now time.Time) StepExecution {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	s := StepExecution{ID: util.GenerateUUID(), WorkflowExecutionID: execID, StepID: stepID, Name: name,
		Type: string(kind), Config: append(json.RawMessage(nil), util.InterpolateContext(config, context)...), DependsOn: append([]string{}, deps...),
		Status: StatusWaiting, MaxAttempts: 1, Multiplier: 1, TimeoutSeconds: timeout, CreatedAt: now, UpdatedAt: now}
	if retry != nil {
		if retry.MaxAttempts > 0 {
			s.MaxAttempts = retry.MaxAttempts
		}
		if retry.Backoff > 0 {
			s.BackoffSeconds = retry.Backoff
		}
		if retry.Multiplier > 0 {
			s.Multiplier = retry.Multiplier
		}
		if retry.MaxBackoff > 0 {
			s.MaxBackoffSeconds = retry.MaxBackoff
		}
	}
	return s
}
