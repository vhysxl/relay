package validator

import (
	"encoding/json"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

type Validator interface {
	ValidateAndNormalizeWorkflow(workflowReq *workflow.WorkflowRequest) error
	ValidateSteps(steps []step.Step) error
	IsWorkflowEqual(existingSteps []step.Step, existingContext json.RawMessage, req *workflow.WorkflowRequest) bool
}

