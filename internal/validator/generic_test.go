package validator

import (
	"encoding/json"
	"testing"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

func TestGenericTaskNormalizesOnlyOmittedInput(t *testing.T) {
	for _, input := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`false`), json.RawMessage(`0`), json.RawMessage(`""`), json.RawMessage(`[1,2]`)} {
		req := &workflow.WorkflowRequest{Name: "generic", Steps: []step.Step{{ID: "work", Queue: "orders", Activity: "worker.customFunction", Input: input,
			Compensation: &step.Compensation{Queue: "orders", Activity: "worker.undo", Input: input}}}}
		if err := NewValidator().ValidateAndNormalizeWorkflow(req); err != nil {
			t.Fatalf("input %s: %v", input, err)
		}
		want := string(input)
		if input == nil {
			want = "{}"
		}
		if string(req.Steps[0].Input) != want || string(req.Steps[0].Compensation.Input) != want {
			t.Fatalf("input changed: %+v", req.Steps[0])
		}
	}
}

func TestGenericRoutingParticipatesInDefinitionEquality(t *testing.T) {
	v := NewValidator()
	original := step.Step{ID: "work", Queue: "orders", Activity: "inventory.reserve", Input: json.RawMessage(`{}`), Compensation: &step.Compensation{Queue: "orders", Activity: "inventory.release"}}
	req := &workflow.WorkflowRequest{Name: "generic", Steps: []step.Step{original}}
	if !v.IsWorkflowEqual([]step.Step{original}, nil, req) {
		t.Fatal("identical definition differs")
	}
	for _, change := range []func(*step.Step){
		func(s *step.Step) { s.Queue = "other" }, func(s *step.Step) { s.Activity = "different" }, func(s *step.Step) { s.Input = json.RawMessage(`null`) },
		func(s *step.Step) { c := *s.Compensation; c.Queue = "other"; s.Compensation = &c },
		func(s *step.Step) { c := *s.Compensation; c.Activity = "different"; s.Compensation = &c },
	} {
		modified := original
		change(&modified)
		req.Steps = []step.Step{modified}
		if v.IsWorkflowEqual([]step.Step{original}, nil, req) {
			t.Fatal("routing/input change ignored")
		}
	}
}
