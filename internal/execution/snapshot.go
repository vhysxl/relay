package execution

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

func snapshotSteps(execID string, definitions []step.Step, context json.RawMessage, now time.Time) ([]StepExecution, error) {
	reverse := make(map[string][]string, len(definitions))
	for _, s := range definitions {
		for _, dep := range s.DependsOn {
			reverse[dep] = append(reverse[dep], s.ID)
		}
	}
	result := make([]StepExecution, 0, len(definitions)*2)
	for i, s := range definitions {
		stepCreatedAt := now.Add(time.Duration(i) * time.Second)
		normal, err := snapshotStep(execID, s.ID, s.Name, s.Type, s.Config, s.DependsOn, s.Timeout, s.Retry, context, stepCreatedAt)
		if err != nil {
			return nil, fmt.Errorf("step %s: %w", s.ID, err)
		}
		if len(s.When) > 0 {
			normal.When = append(json.RawMessage(nil), s.When...)
		}
		if len(s.DependsOn) == 0 {
			normal.Status = StatusPending
		}
		result = append(result, normal)
		if c := s.Compensation; c != nil {
			comp, err := snapshotStep(execID, s.ID, s.Name+" (Compensation)", c.Type, c.Config, reverse[s.ID], c.Timeout, c.Retry, context, stepCreatedAt)
			if err != nil {
				return nil, fmt.Errorf("compensation %s: %w", s.ID, err)
			}
			comp.IsCompensation = true
			result = append(result, comp)
		}
	}
	return result, nil
}

func snapshotStep(execID, stepID, name string, kind step.StepType, config json.RawMessage, deps []string, timeout int, retry *step.RetryPolicy, context json.RawMessage, now time.Time) (StepExecution, error) {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	interpolatedConfig := append(json.RawMessage(nil), util.InterpolateContext(config, context)...)

	taskQueue, err := util.QueueFromConfig(interpolatedConfig)
	if err != nil {
		return StepExecution{}, err
	}
	var activityName *string
	var cfgMap map[string]json.RawMessage
	if err := json.Unmarshal(interpolatedConfig, &cfgMap); err == nil && cfgMap != nil {
		if kind == step.StepTypeActivity {
			if actRaw, ok := cfgMap["activity"]; ok {
				var act string
				if json.Unmarshal(actRaw, &act) == nil && act != "" {
					activityName = &act
				}
			}
		}
	}

	s := StepExecution{ID: util.GenerateUUID(), WorkflowExecutionID: execID, StepID: stepID, Name: name,
		Type: string(kind), TaskQueue: taskQueue, ActivityName: activityName, Config: interpolatedConfig, DependsOn: append([]string{}, deps...),
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
	return s, nil
}
