package step

import (
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrEmptyStepType       = errors.New("step type is required")
	ErrInvalidStepType     = errors.New("invalid step type, must be http or message")
	ErrInvalidStepConfig   = errors.New("step config must be valid non-empty JSON")
	ErrDuplicateStepID     = errors.New("duplicate step id in workflow")
	ErrDependencyNotFound  = errors.New("dependency step not found in workflow")
	ErrSelfDependency      = errors.New("step cannot depend on itself")
	ErrCyclicDependency    = errors.New("cyclic dependency detected in workflow steps")
	ErrInvalidRetryPolicy  = errors.New("invalid retry policy")
	ErrInvalidCompensation = errors.New("invalid compensation configuration")
)

type stepValidator struct{}

func NewStepValidator() StepValidator {
	return &stepValidator{}
}

func (v *stepValidator) ValidateSteps(steps []Step) error {
	if len(steps) == 0 {
		return nil
	}

	idMap := make(map[string]bool, len(steps))

	for _, s := range steps {
		stepID := strings.TrimSpace(s.ID)
		if stepID == "" {
			return errors.New("step id cannot be empty")
		}

		if idMap[stepID] {
			return ErrDuplicateStepID
		}
		idMap[stepID] = true

		if s.Type == "" {
			return ErrEmptyStepType
		}
		if s.Type != StepTypeHTTP && s.Type != StepTypeMessage {
			return ErrInvalidStepType
		}

		if len(s.Config) == 0 {
			return ErrInvalidStepConfig
		}
		trimmedConfig := strings.TrimSpace(string(s.Config))
		if trimmedConfig == "" || trimmedConfig == "null" || !json.Valid(s.Config) {
			return ErrInvalidStepConfig
		}

		if s.Retry != nil {
			if s.Retry.MaxAttempts < 1 || s.Retry.Backoff < 0 || s.Retry.Multiplier < 0 || s.Retry.MaxBackoff < 0 {
				return ErrInvalidRetryPolicy
			}
			if s.Retry.MaxBackoff > 0 && s.Retry.MaxBackoff < s.Retry.Backoff {
				return ErrInvalidRetryPolicy
			}
		}

		if s.Compensation != nil {
			if s.Compensation.Type != StepTypeHTTP && s.Compensation.Type != StepTypeMessage {
				return ErrInvalidCompensation
			}
			if len(s.Compensation.Config) == 0 {
				return ErrInvalidCompensation
			}
			trimmedComp := strings.TrimSpace(string(s.Compensation.Config))
			if trimmedComp == "" || trimmedComp == "null" || !json.Valid(s.Compensation.Config) {
				return ErrInvalidCompensation
			}
			if s.Compensation.Retry != nil {
				if s.Compensation.Retry.MaxAttempts < 1 || s.Compensation.Retry.Backoff < 0 || s.Compensation.Retry.Multiplier < 0 || s.Compensation.Retry.MaxBackoff < 0 {
					return ErrInvalidRetryPolicy
				}
				if s.Compensation.Retry.MaxBackoff > 0 && s.Compensation.Retry.MaxBackoff < s.Compensation.Retry.Backoff {
					return ErrInvalidRetryPolicy
				}
			}
		}
	}

	// kahn's algorithm
	inDegree := make(map[string]int, len(steps))
	dependents := make(map[string][]string, len(steps))

	for _, s := range steps {
		inDegree[s.ID] = len(s.DependsOn)

		for _, dep := range s.DependsOn {
			depID := strings.TrimSpace(dep)
			if depID == s.ID {
				return ErrSelfDependency
			}
			if !idMap[depID] {
				return ErrDependencyNotFound
			}
			dependents[depID] = append(dependents[depID], s.ID)
		}
	}

	var queue []string
	for _, s := range steps {
		if inDegree[s.ID] == 0 {
			queue = append(queue, s.ID)
		}
	}

	visited := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited++

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if visited != len(steps) {
		return ErrCyclicDependency
	}

	return nil
}
