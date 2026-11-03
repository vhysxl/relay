package condition_test

import (
	"encoding/json"
	"testing"

	"github.com/vhysxl/relay/internal/condition"
	"github.com/vhysxl/relay/pkg/errdefs"
)

func TestConditionParsingAndValidation(t *testing.T) {
	t.Run("valid leaf eq condition", func(t *testing.T) {
		raw := []byte(`{"ref":"user.tier","op":"eq","value":"diamond"}`)
		cond, err := condition.ParseCondition(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cond.Ref != "user.tier" || cond.Op != "eq" || string(cond.Value) != `"diamond"` {
			t.Fatalf("unexpected parsed condition: %+v", cond)
		}
	})

	t.Run("valid exists operator without value", func(t *testing.T) {
		raw := []byte(`{"ref":"user.tier","op":"exists"}`)
		cond, err := condition.ParseCondition(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cond.Op != "exists" || cond.Value != nil {
			t.Fatalf("expected nil value for exists: %+v", cond)
		}
	})

	t.Run("exists operator rejects explicit non-null value", func(t *testing.T) {
		raw := []byte(`{"ref":"user.tier","op":"exists","value":"diamond"}`)
		_, err := condition.ParseCondition(raw)
		if err == nil {
			t.Fatal("expected error when exists has value, got nil")
		}
	})

	t.Run("valid group all and any", func(t *testing.T) {
		raw := []byte(`{
			"all": [
				{"ref": "user.tier", "op": "exists"},
				{"any": [
					{"ref": "user.tier", "op": "eq", "value": "diamond"},
					{"ref": "user.tier", "op": "eq", "value": "platinum"}
				]}
			]
		}`)
		cond, err := condition.ParseCondition(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cond.All) != 2 || len(cond.All[1].Any) != 2 {
			t.Fatalf("unexpected tree structure: %+v", cond)
		}
	})

	t.Run("rejects empty group", func(t *testing.T) {
		raw := []byte(`{"all": []}`)
		_, err := condition.ParseCondition(raw)
		if err == nil {
			t.Fatal("expected error for empty group, got nil")
		}
	})

	t.Run("rejects unknown field", func(t *testing.T) {
		raw := []byte(`{"ref":"user.tier","op":"eq","value":"diamond","foo":"bar"}`)
		_, err := condition.ParseCondition(raw)
		if err == nil {
			t.Fatal("expected error for unknown field, got nil")
		}
	})

	t.Run("rejects multiple forms in one node", func(t *testing.T) {
		raw := []byte(`{"ref":"user.tier","op":"eq","value":"diamond","all":[{"ref":"a","op":"exists"}]}`)
		_, err := condition.ParseCondition(raw)
		if err == nil {
			t.Fatal("expected error for multiple forms, got nil")
		}
	})

	t.Run("in operator requires array of uniform scalars", func(t *testing.T) {
		valid := []byte(`{"ref":"user.tier","op":"in","value":["diamond","platinum"]}`)
		if _, err := condition.ParseCondition(valid); err != nil {
			t.Fatalf("unexpected error for valid in: %v", err)
		}

		nonUniform := []byte(`{"ref":"user.tier","op":"in","value":["diamond", 123]}`)
		if _, err := condition.ParseCondition(nonUniform); err == nil {
			t.Fatal("expected error for non-uniform in array, got nil")
		}

		empty := []byte(`{"ref":"user.tier","op":"in","value":[]}`)
		if _, err := condition.ParseCondition(empty); err == nil {
			t.Fatal("expected error for empty in array, got nil")
		}
	})

	t.Run("tree limits depth and nodes", func(t *testing.T) {
		// Deep tree depth 4
		raw := []byte(`{"all":[{"all":[{"all":[{"ref":"a","op":"exists"}]}]}]}`)
		cond, err := condition.ParseCondition(raw)
		if err != nil {
			t.Fatal(err)
		}

		if err := condition.ValidateTree(cond, 3, 10); err == nil {
			t.Fatal("expected max depth error for depth 4 with limit 3, got nil")
		}
		if err := condition.ValidateTree(cond, 4, 10); err != nil {
			t.Fatalf("unexpected error for depth 4: %v", err)
		}

		// Node limit
		if err := condition.ValidateTree(cond, 10, 3); err == nil {
			t.Fatal("expected max nodes error, got nil")
		}

		// Test limits during ParseConditionWithLimits
		if _, err := condition.ParseConditionWithLimits(raw, 3, 10); err != errdefs.ErrConditionMaxDepth {
			t.Fatalf("expected ErrConditionMaxDepth from ParseConditionWithLimits, got %v", err)
		}
		if _, err := condition.ParseConditionWithLimits(raw, 10, 3); err != errdefs.ErrConditionMaxNodes {
			t.Fatalf("expected ErrConditionMaxNodes from ParseConditionWithLimits, got %v", err)
		}
	})
}

func TestEvaluator(t *testing.T) {
	ctx := json.RawMessage(`{
		"user": {
			"tier": "diamond",
			"score": 9007199254740993,
			"active": true,
			"null_field": null
		},
		"items": [
			{"id": "item-1", "price": 150000},
			{"id": "item-2", "price": 250000}
		]
	}`)

	stepOutputs := map[string]json.RawMessage{
		"step-a": json.RawMessage(`{"status":"SUCCESS","data":{"amount":500000}}`),
		"step-b": json.RawMessage(`{"user":{"tier":"gold"}}`),
	}

	t.Run("context string eq and ne", func(t *testing.T) {
		cond, _ := condition.ParseCondition([]byte(`{"ref":"user.tier","op":"eq","value":"diamond"}`))
		pass, audit, err := condition.Evaluate(cond, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected true, got %v (err %v)", pass, err)
		}
		if len(audit.Checks) != 1 || *audit.Checks[0].Result != true {
			t.Fatalf("bad audit: %+v", audit)
		}

		condNe, _ := condition.ParseCondition([]byte(`{"ref":"user.tier","op":"ne","value":"bronze"}`))
		pass, _, err = condition.Evaluate(condNe, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected true for ne, got %v (err %v)", pass, err)
		}
	})

	t.Run("step output anchor canonical resolution", func(t *testing.T) {
		cond, _ := condition.ParseCondition([]byte(`{"ref":"steps.step-a.output.data.amount","op":"eq","value":500000}`))
		pass, audit, err := condition.Evaluate(cond, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected true, got %v (err %v)", pass, err)
		}
		if len(audit.Checks) != 1 || audit.Checks[0].Missing {
			t.Fatalf("bad audit check: %+v", audit.Checks[0])
		}
	})

	t.Run("array index traversal in context", func(t *testing.T) {
		cond, _ := condition.ParseCondition([]byte(`{"ref":"items.1.id","op":"eq","value":"item-2"}`))
		pass, _, err := condition.Evaluate(cond, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected true for array index, got %v (err %v)", pass, err)
		}
	})

	t.Run("high precision numbers greater than 2^53", func(t *testing.T) {
		// Score is 9007199254740993 (2^53 + 1)
		condEq, _ := condition.ParseCondition([]byte(`{"ref":"user.score","op":"eq","value":9007199254740993}`))
		pass, _, err := condition.Evaluate(condEq, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected eq to be true for exact large number, got %v (%v)", pass, err)
		}

		// 9007199254740992 should NOT equal 9007199254740993
		condDiff, _ := condition.ParseCondition([]byte(`{"ref":"user.score","op":"eq","value":9007199254740992}`))
		pass, _, err = condition.Evaluate(condDiff, ctx, stepOutputs)
		if err != nil || pass {
			t.Fatalf("expected eq to be false when differing by 1 above 2^53, got %v", pass)
		}

		// Order: 9007199254740993 > 9007199254740992
		condGt, _ := condition.ParseCondition([]byte(`{"ref":"user.score","op":"gt","value":9007199254740992}`))
		pass, _, err = condition.Evaluate(condGt, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected gt to be true for exact large number, got %v (%v)", pass, err)
		}
	})

	t.Run("membership in operator", func(t *testing.T) {
		condIn, _ := condition.ParseCondition([]byte(`{"ref":"user.tier","op":"in","value":["gold","diamond","silver"]}`))
		pass, _, err := condition.Evaluate(condIn, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected true for in, got %v (%v)", pass, err)
		}

		condNotIn, _ := condition.ParseCondition([]byte(`{"ref":"user.tier","op":"in","value":["bronze","silver"]}`))
		pass, _, err = condition.Evaluate(condNotIn, ctx, stepOutputs)
		if err != nil || pass {
			t.Fatalf("expected false for in, got %v (%v)", pass, err)
		}
	})

	t.Run("differentiating missing path vs null value", func(t *testing.T) {
		// null_field exists and is null
		condExistsNull, _ := condition.ParseCondition([]byte(`{"ref":"user.null_field","op":"exists"}`))
		pass, audit, err := condition.Evaluate(condExistsNull, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected exists to be true for explicit null, got %v (%v)", pass, err)
		}
		if audit.Checks[0].Missing {
			t.Fatal("expected missing=false for explicit null")
		}
		if string(audit.Checks[0].Actual) != "null" {
			t.Fatalf("expected actual 'null', got %s", string(audit.Checks[0].Actual))
		}

		// comparing explicit null with eq
		condEqNull, _ := condition.ParseCondition([]byte(`{"ref":"user.null_field","op":"eq","value":null}`))
		pass, _, err = condition.Evaluate(condEqNull, ctx, stepOutputs)
		if err != nil || !pass {
			t.Fatalf("expected true for eq null, got %v (%v)", pass, err)
		}

		// missing field
		condExistsMissing, _ := condition.ParseCondition([]byte(`{"ref":"user.missing_field","op":"exists"}`))
		pass, audit, err = condition.Evaluate(condExistsMissing, ctx, stepOutputs)
		if err != nil || pass {
			t.Fatalf("expected exists to be false for missing path, got %v (%v)", pass, err)
		}
		if !audit.Checks[0].Missing {
			t.Fatal("expected missing=true for missing path")
		}

		// operator other than exists on missing field must return evaluation error!
		condEqMissing, _ := condition.ParseCondition([]byte(`{"ref":"user.missing_field","op":"eq","value":"foo"}`))
		pass, audit, err = condition.Evaluate(condEqMissing, ctx, stepOutputs)
		if err == nil {
			t.Fatal("expected error when path is missing for eq, got nil")
		}
		if !audit.Checks[0].Missing {
			t.Fatal("expected check audit to mark missing=true")
		}
	})

	t.Run("short circuiting with exists guard in all group", func(t *testing.T) {
		// If optional_step output does not exist, exists returns false, and subsequent eq check is never evaluated!
		cond, _ := condition.ParseCondition([]byte(`{
			"all": [
				{"ref": "steps.optional_step.output.user.tier", "op": "exists"},
				{"ref": "steps.optional_step.output.user.tier", "op": "eq", "value": "diamond"}
			]
		}`))
		pass, audit, err := condition.Evaluate(cond, ctx, stepOutputs)
		if err != nil {
			t.Fatalf("expected short-circuit without error, got %v", err)
		}
		if pass {
			t.Fatal("expected false because exists was false")
		}
		// Only 1 check evaluated!
		if len(audit.Checks) != 1 {
			t.Fatalf("expected exactly 1 check evaluated due to short-circuit, got %d", len(audit.Checks))
		}
		if audit.Checks[0].Op != "exists" {
			t.Fatalf("expected first check to be exists, got %s", audit.Checks[0].Op)
		}
	})

	t.Run("short circuiting in any group", func(t *testing.T) {
		cond, _ := condition.ParseCondition([]byte(`{
			"any": [
				{"ref": "user.tier", "op": "eq", "value": "diamond"},
				{"ref": "steps.missing_step.output.fail", "op": "eq", "value": "fatal"}
			]
		}`))
		pass, audit, err := condition.Evaluate(cond, ctx, stepOutputs)
		if err != nil {
			t.Fatalf("expected short-circuit on any without error, got %v", err)
		}
		if !pass {
			t.Fatal("expected true because first branch matched")
		}
		if len(audit.Checks) != 1 {
			t.Fatalf("expected 1 check due to any short-circuit, got %d", len(audit.Checks))
		}
	})

	t.Run("incompatible types return error", func(t *testing.T) {
		// String compared with number using ordering
		condGt, _ := condition.ParseCondition([]byte(`{"ref":"user.tier","op":"gt","value":100}`))
		_, _, err := condition.Evaluate(condGt, ctx, stepOutputs)
		if err == nil {
			t.Fatal("expected error when comparing string with number for gt, got nil")
		}

		// Number compared with boolean
		condEqBool, _ := condition.ParseCondition([]byte(`{"ref":"user.score","op":"eq","value":true}`))
		_, _, err = condition.Evaluate(condEqBool, ctx, stepOutputs)
		if err == nil {
			t.Fatal("expected error when comparing number with bool for eq, got nil")
		}
	})
}
