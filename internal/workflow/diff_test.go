package workflow_test

import (
	"encoding/json"
	"testing"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

func TestCompareWorkflows_FirstVersionOrIdentical(t *testing.T) {
	wf1 := &workflow.Workflow{
		ID:      "id-1",
		Name:    "order-flow",
		Version: 1,
		Context: json.RawMessage(`{"currency":"IDR","val":9007199254740992}`),
		Steps: []step.Step{
			{
				ID:        "step-1",
				Name:      "Step One",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"queue":"orders","url":"https://api.test/1","amount":100}`),
				DependsOn: []string{"step-a", "step-b"},
			},
		},
	}

	// Identical workflow except key order, step order, and dependency order
	wf2 := &workflow.Workflow{
		ID:      "id-2",
		Name:    "order-flow",
		Version: 2,
		Context: json.RawMessage(`{"val":9007199254740992,"currency":"IDR"}`),
		Steps: []step.Step{
			{
				ID:        "step-1",
				Name:      "Step One",
				Type:      step.StepTypeHTTP,
				Config:    json.RawMessage(`{"queue":"orders","amount":100,"url":"https://api.test/1"}`),
				DependsOn: []string{"step-b", "step-a"},
			},
		},
	}

	changes := workflow.CompareWorkflows(wf1, wf2)
	if len(changes) != 0 {
		t.Fatalf("expected 0 changes for logically identical workflows, got %+v", changes)
	}
}

func TestCompareWorkflows_ContextChanges(t *testing.T) {
	wf1 := &workflow.Workflow{
		Context: json.RawMessage(`{"domain":"order","ratio":10.5,"removed_prop":"old"}`),
	}
	wf2 := &workflow.Workflow{
		Context: json.RawMessage(`{"domain":"checkout","ratio":10.5,"added_prop":"new"}`),
	}

	changes := workflow.CompareWorkflows(wf1, wf2)
	if len(changes) != 3 {
		t.Fatalf("expected 3 context changes, got %d: %+v", len(changes), changes)
	}

	changeMap := make(map[string]workflow.WorkflowChange)
	for _, c := range changes {
		changeMap[c.Path] = c
	}

	// /context/domain modified
	domainChange, ok := changeMap["/context/domain"]
	if !ok || domainChange.Kind != workflow.ChangeKindModified {
		t.Errorf("expected /context/domain to be modified, got %+v", domainChange)
	}

	// /context/removed_prop removed
	remChange, ok := changeMap["/context/removed_prop"]
	if !ok || remChange.Kind != workflow.ChangeKindRemoved {
		t.Errorf("expected /context/removed_prop to be removed, got %+v", remChange)
	}

	// /context/added_prop added
	addChange, ok := changeMap["/context/added_prop"]
	if !ok || addChange.Kind != workflow.ChangeKindAdded {
		t.Errorf("expected /context/added_prop to be added, got %+v", addChange)
	}
}

func TestCompareWorkflows_StepAddedAndRemoved(t *testing.T) {
	wf1 := &workflow.Workflow{
		Steps: []step.Step{
			{ID: "step-deleted", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders"}`)},
			{ID: "step-common", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders"}`)},
		},
	}
	wf2 := &workflow.Workflow{
		Steps: []step.Step{
			{ID: "step-common", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders"}`)},
			{ID: "step/added~one", Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders"}`)},
		},
	}

	changes := workflow.CompareWorkflows(wf1, wf2)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes (1 added, 1 removed), got %d: %+v", len(changes), changes)
	}

	changeMap := make(map[string]workflow.WorkflowChange)
	for _, c := range changes {
		changeMap[c.Path] = c
	}

	// Escaped JSON pointer for step/added~one: /steps/step~1added~0one
	addChange, ok := changeMap["/steps/step~1added~0one"]
	if !ok || addChange.Kind != workflow.ChangeKindAdded {
		t.Errorf("expected /steps/step~1added~0one added, got %+v", addChange)
	}

	remChange, ok := changeMap["/steps/step-deleted"]
	if !ok || remChange.Kind != workflow.ChangeKindRemoved {
		t.Errorf("expected /steps/step-deleted removed, got %+v", remChange)
	}
}

func TestCompareWorkflows_StepFieldModifications(t *testing.T) {
	wf1 := &workflow.Workflow{
		Steps: []step.Step{
			{
				ID:           "step-1",
				Name:         "Old Name",
				Type:         step.StepTypeHTTP,
				Config:       json.RawMessage(`{"queue":"orders","url":"https://old.com","num":9007199254740992}`),
				DependsOn:    []string{"dep-a"},
				Timeout:      10,
				Retry:        &step.RetryPolicy{MaxAttempts: 2, Backoff: 1},
				Compensation: &step.Compensation{Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders","url":"https://refund.old"}`)},
				Irreversible: false,
				When:         json.RawMessage(`{"ref":"user.tier","op":"eq","value":"gold"}`),
			},
		},
	}

	wf2 := &workflow.Workflow{
		Steps: []step.Step{
			{
				ID:           "step-1",
				Name:         "New Name",
				Type:         step.StepTypeMessage,
				Config:       json.RawMessage(`{"queue":"orders","url":"https://old.com","num":9007199254740993}`),
				DependsOn:    []string{"dep-a", "dep-b"},
				Timeout:      30,
				Retry:        &step.RetryPolicy{MaxAttempts: 5, Backoff: 1},
				Compensation: &step.Compensation{Type: step.StepTypeHTTP, Config: json.RawMessage(`{"queue":"orders","url":"https://refund.new"}`)},
				Irreversible: true,
				When:         json.RawMessage(`{"ref":"user.tier","op":"eq","value":"platinum"}`),
			},
		},
	}

	changes := workflow.CompareWorkflows(wf1, wf2)
	changeMap := make(map[string]workflow.WorkflowChange)
	for _, c := range changes {
		changeMap[c.Path] = c
	}

	expectedPaths := []string{
		"/steps/step-1/name",
		"/steps/step-1/type",
		"/steps/step-1/config/num",
		"/steps/step-1/depends_on",
		"/steps/step-1/timeout",
		"/steps/step-1/retry/max_attempts",
		"/steps/step-1/compensation/config/url",
		"/steps/step-1/irreversible",
		"/steps/step-1/when",
	}

	for _, p := range expectedPaths {
		if c, ok := changeMap[p]; !ok {
			t.Errorf("missing expected change for path %s", p)
		} else if c.Kind != workflow.ChangeKindModified {
			t.Errorf("expected path %s to have kind modified, got %s", p, c.Kind)
		}
	}
}

func TestCompareWorkflows_ArrayOrderSignificanceInConfig(t *testing.T) {
	wf1 := &workflow.Workflow{
		Steps: []step.Step{
			{
				ID:     "s1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","items":[1, 2, 3]}`),
			},
		},
	}
	wf2 := &workflow.Workflow{
		Steps: []step.Step{
			{
				ID:     "s1",
				Type:   step.StepTypeHTTP,
				Config: json.RawMessage(`{"queue":"orders","items":[3, 2, 1]}`),
			},
		},
	}

	changes := workflow.CompareWorkflows(wf1, wf2)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change for config array order difference, got %d: %+v", len(changes), changes)
	}
	if changes[0].Path != "/steps/s1/config/items" || changes[0].Kind != workflow.ChangeKindModified {
		t.Errorf("unexpected change: %+v", changes[0])
	}
}

func TestWorkflowChange_JSONSerialization_PreservesNullFalseZero(t *testing.T) {
	// 1. Added: before must NOT be present; after must be present even if false or 0 or nil
	addedChange := workflow.WorkflowChange{
		Path:  "/steps/s1/irreversible",
		Kind:  workflow.ChangeKindAdded,
		After: false,
	}
	bytesAdded, err := json.Marshal(addedChange)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var mapAdded map[string]any
	if err := json.Unmarshal(bytesAdded, &mapAdded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := mapAdded["before"]; ok {
		t.Fatalf("expected 'before' to be absent for added change, got %v", mapAdded)
	}
	if val, ok := mapAdded["after"]; !ok || val != false {
		t.Fatalf("expected 'after' to be false, got %v", val)
	}

	// 2. Removed: after must NOT be present; before must be present even if 0
	removedChange := workflow.WorkflowChange{
		Path:   "/steps/s1/timeout",
		Kind:   workflow.ChangeKindRemoved,
		Before: 0,
	}
	bytesRemoved, err := json.Marshal(removedChange)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var mapRemoved map[string]any
	if err := json.Unmarshal(bytesRemoved, &mapRemoved); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := mapRemoved["after"]; ok {
		t.Fatalf("expected 'after' to be absent for removed change, got %v", mapRemoved)
	}
	if val, ok := mapRemoved["before"]; !ok || val != float64(0) {
		t.Fatalf("expected 'before' to be 0, got %v", val)
	}

	// 3. Modified: both before and after must be present, even if nil (JSON null)
	modifiedChange := workflow.WorkflowChange{
		Path:   "/steps/s1/condition",
		Kind:   workflow.ChangeKindModified,
		Before: nil,
		After:  false,
	}
	bytesModified, err := json.Marshal(modifiedChange)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var mapModified map[string]any
	if err := json.Unmarshal(bytesModified, &mapModified); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if val, ok := mapModified["before"]; !ok || val != nil {
		t.Fatalf("expected 'before' to be explicitly null, got %v", val)
	}
	if val, ok := mapModified["after"]; !ok || val != false {
		t.Fatalf("expected 'after' to be false, got %v", val)
	}
}
