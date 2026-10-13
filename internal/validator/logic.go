package validator

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/util"
)

type validator struct{}

func NewValidator() Validator {
	return &validator{}
}

func (v *validator) ValidateAndNormalizeWorkflow(workflowReq *workflow.WorkflowRequest) error {
	if workflowReq == nil {
		return ErrInvalidWorkflowName
	}

	// Name
	workflowReq.Name = strings.TrimSpace(workflowReq.Name)
	if workflowReq.Name == "" {
		return ErrInvalidWorkflowName
	}
	if utf8.RuneCountInString(workflowReq.Name) > 255 {
		return ErrWorkflowNameTooLong
	}

	// Version
	if workflowReq.Version < 0 {
		return ErrInvalidWorkflowVersion
	}

	if workflowReq.Version == 0 {
		workflowReq.Version = 1
	}

	// Steps
	if len(workflowReq.Steps) == 0 {
		return ErrWorkflowEmptySteps
	}

	for i := range workflowReq.Steps {
		s := &workflowReq.Steps[i]
		s.ID = strings.TrimSpace(s.ID)
		if s.ID == "" {
			return ErrEmptyID
		}
		for j := range s.DependsOn {
			s.DependsOn[j] = strings.TrimSpace(s.DependsOn[j])
		}
	}

	// Context
	if len(workflowReq.Context) > 0 {
		ctx, err := validateWorkflowContext(workflowReq.Context)
		if err != nil {
			return err
		}
		workflowReq.Context = ctx
	}

	return nil
}

func (v *validator) ValidateSteps(steps []step.Step) error {
	if len(steps) == 0 {
		return nil
	}

	idMap := make(map[string]bool, len(steps))

	for _, s := range steps {
		stepID := strings.TrimSpace(s.ID)
		if stepID == "" {
			return ErrEmptyID
		}

		if idMap[stepID] {
			return ErrDuplicateStepID
		}
		idMap[stepID] = true

		if s.Type == "" {
			return ErrEmptyStepType
		}
		if s.Type != step.StepTypeHTTP && s.Type != step.StepTypeMessage {
			return ErrInvalidStepType
		}

		if len(s.Config) == 0 {
			return ErrInvalidStepConfig
		}
		trimmedConfig := strings.TrimSpace(string(s.Config))
		if trimmedConfig == "" || trimmedConfig == "null" || !json.Valid(s.Config) {
			return ErrInvalidStepConfig
		}

		if s.Timeout < 0 {
			return ErrStepTimeoutNegative
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
			if s.Irreversible {
				return ErrIrreversibleCompensation
			}
			if s.Compensation.Timeout < 0 {
				return ErrCompensationTimeoutNegative
			}
			if s.Compensation.Type != step.StepTypeHTTP && s.Compensation.Type != step.StepTypeMessage {
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

func (v *validator) IsWorkflowEqual(existingSteps []step.Step, existingContext json.RawMessage, req *workflow.WorkflowRequest) bool {
	if req == nil {
		return false
	}
	if !util.AreJSONEqual(existingContext, req.Context) {
		return false
	}
	if len(existingSteps) != len(req.Steps) {
		return false
	}

	existingByID := make(map[string]step.Step, len(existingSteps))
	for _, s := range existingSteps {
		existingByID[s.ID] = s
	}

	for _, reqStep := range req.Steps {
		existStep, ok := existingByID[reqStep.ID]
		if !ok {
			return false
		}
		if !isStepEqual(existStep, reqStep) {
			return false
		}
	}

	return true
}
