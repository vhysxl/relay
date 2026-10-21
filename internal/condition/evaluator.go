package condition

import (
	"encoding/json"
	"fmt"
	"time"
)

// Evaluate executes a condition against workflow context and completed step outputs.
// It returns the boolean decision, the full evaluation audit trail, or an evaluation error.
func Evaluate(cond *Condition, contextJSON json.RawMessage, stepOutputs map[string]json.RawMessage) (bool, *EvaluationAudit, error) {
	if cond == nil {
		return true, nil, nil
	}

	audit := &EvaluationAudit{
		EvaluatedAt: time.Now().UTC(),
		Checks:      make([]CheckDetail, 0),
	}

	var evalNode func(curr *Condition) (bool, error)
	evalNode = func(curr *Condition) (bool, error) {
		if curr == nil {
			return true, nil
		}

		if len(curr.All) > 0 {
			for i := range curr.All {
				pass, err := evalNode(&curr.All[i])
				if err != nil {
					return false, err
				}
				if !pass {
					return false, nil // short-circuit
				}
			}
			return true, nil
		}

		if len(curr.Any) > 0 {
			for i := range curr.Any {
				pass, err := evalNode(&curr.Any[i])
				if err != nil {
					return false, err
				}
				if pass {
					return true, nil // short-circuit
				}
			}
			return false, nil
		}

		// Leaf check
		check := CheckDetail{
			Ref:      curr.Ref,
			Op:       curr.Op,
			Expected: curr.Value,
		}

		val, present, err := ResolveAnchor(curr.Ref, contextJSON, stepOutputs)
		if err != nil {
			check.Error = err.Error()
			audit.Checks = append(audit.Checks, check)
			return false, err
		}

		if !present {
			check.Missing = true
			if curr.Op == OpExists {
				f := false
				check.Result = &f
				audit.Checks = append(audit.Checks, check)
				return false, nil
			}
			evalErr := fmt.Errorf("anchor path %q is missing", curr.Ref)
			check.Error = evalErr.Error()
			audit.Checks = append(audit.Checks, check)
			return false, evalErr
		}

		// Path is present
		check.Actual = val

		if curr.Op == OpExists {
			t := true
			check.Result = &t
			audit.Checks = append(audit.Checks, check)
			return true, nil
		}

		pass, err := CompareValues(curr.Op, val, curr.Value)
		if err != nil {
			check.Error = err.Error()
			audit.Checks = append(audit.Checks, check)
			return false, err
		}

		check.Result = &pass
		audit.Checks = append(audit.Checks, check)
		return pass, nil
	}

	pass, err := evalNode(cond)
	if err != nil {
		audit.Error = err.Error()
		return false, audit, err
	}

	audit.Result = &pass
	return pass, audit, nil
}
