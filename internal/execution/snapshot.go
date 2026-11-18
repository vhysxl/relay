package execution

import (
	"fmt"
	"time"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

func snapshotSteps(execID string, definitions []step.Step, defaultTimeout int, now time.Time) ([]StepExecution, error) {
	result := make([]StepExecution, 0, len(definitions)*2)
	for i, s := range definitions {
		createdAt := now.Add(time.Duration(i) * time.Second)
		normal, err := snapshotStep(execID, s.ID, s.Name, s.Queue, s.Activity, s.Input, s.Timeout, defaultTimeout, s.Retry, createdAt)
		if err != nil {
			return nil, fmt.Errorf("step %s: %w", s.ID, err)
		}
		normal.DependsOn = append([]string{}, s.DependsOn...)
		normal.When = append(normal.When, s.When...)
		if len(s.DependsOn) == 0 {
			normal.Status = StatusPending
		}
		result = append(result, normal)
		if c := s.Compensation; c != nil {
			comp, err := snapshotStep(execID, s.ID, s.Name+" (Compensation)", c.Queue, c.Activity, c.Input, c.Timeout, defaultTimeout, c.Retry, createdAt)
			if err != nil {
				return nil, fmt.Errorf("compensation %s: %w", s.ID, err)
			}
			comp.IsCompensation = true
			result = append(result, comp)
		}
	}
	return result, nil
}

func snapshotStep(execID, stepID, name, queue, activity string, input []byte, timeout, defaultTimeout int, retry *step.RetryPolicy, now time.Time) (StepExecution, error) {
	if err := util.ValidateQueueName(queue); err != nil {
		return StepExecution{}, err
	}
	if err := util.ValidateActivity(activity); err != nil {
		return StepExecution{}, err
	}
	if timeout == 0 {
		timeout = defaultTimeout
	}
	s := StepExecution{
		ID: util.GenerateUUID(), WorkflowExecutionID: execID, StepID: stepID, Name: name,
		Queue: queue, Activity: activity, Input: append([]byte(nil), util.NormalizeInput(input)...),
		Status: StatusWaiting, MaxAttempts: 1, Multiplier: 1, TimeoutSeconds: timeout,
		CreatedAt: now, UpdatedAt: now,
	}
	if retry != nil {
		s.MaxAttempts, s.BackoffSeconds, s.MaxBackoffSeconds = retry.MaxAttempts, retry.Backoff, retry.MaxBackoff
		if retry.Multiplier > 0 {
			s.Multiplier = retry.Multiplier
		}
	}
	return s, nil
}
