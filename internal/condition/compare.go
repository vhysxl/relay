package condition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
)

// CompareValues evaluates an actual JSON value against an expected JSON value using op.
// Both actual and expected must be trimmed valid JSON.
func CompareValues(op string, actual, expected json.RawMessage) (bool, error) {
	actual = bytes.TrimSpace(actual)
	expected = bytes.TrimSpace(expected)

	actIsNull := bytes.Equal(actual, []byte("null"))
	expIsNull := bytes.Equal(expected, []byte("null"))

	// Special handling when null is involved
	if actIsNull || expIsNull {
		switch op {
		case OpEq:
			return actIsNull && expIsNull, nil
		case OpNe:
			return actIsNull != expIsNull, nil
		default:
			return false, fmt.Errorf("operator %q does not support comparison with null", op)
		}
	}

	// Determine type of actual
	switch {
	case isJSONString(actual):
		return compareString(op, actual, expected)
	case isJSONBool(actual):
		return compareBool(op, actual, expected)
	case isJSONNumber(actual):
		return compareNumber(op, actual, expected)
	default:
		return false, fmt.Errorf("unsupported actual value type (only JSON scalars and null are supported)")
	}
}

func isJSONString(raw []byte) bool {
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func isJSONBool(raw []byte) bool {
	return bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false"))
}

func compareString(op string, actual, expected []byte) (bool, error) {
	var actStr string
	if err := json.Unmarshal(actual, &actStr); err != nil {
		return false, fmt.Errorf("failed to decode actual string: %w", err)
	}

	if op == OpIn {
		var list []string
		if err := json.Unmarshal(expected, &list); err != nil {
			return false, fmt.Errorf("incompatible types: expected string array for operator 'in': %w", err)
		}
		for _, item := range list {
			if item == actStr {
				return true, nil
			}
		}
		return false, nil
	}

	if !isJSONString(expected) {
		return false, fmt.Errorf("incompatible types: cannot compare string with non-string using %q", op)
	}

	var expStr string
	if err := json.Unmarshal(expected, &expStr); err != nil {
		return false, fmt.Errorf("failed to decode expected string: %w", err)
	}

	switch op {
	case OpEq:
		return actStr == expStr, nil
	case OpNe:
		return actStr != expStr, nil
	default:
		return false, fmt.Errorf("operator %q is not supported for string values", op)
	}
}

func compareBool(op string, actual, expected []byte) (bool, error) {
	actBool := bytes.Equal(actual, []byte("true"))

	if op == OpIn {
		var list []bool
		if err := json.Unmarshal(expected, &list); err != nil {
			return false, fmt.Errorf("incompatible types: expected boolean array for operator 'in': %w", err)
		}
		for _, item := range list {
			if item == actBool {
				return true, nil
			}
		}
		return false, nil
	}

	if !isJSONBool(expected) {
		return false, fmt.Errorf("incompatible types: cannot compare boolean with non-boolean using %q", op)
	}

	expBool := bytes.Equal(expected, []byte("true"))
	switch op {
	case OpEq:
		return actBool == expBool, nil
	case OpNe:
		return actBool != expBool, nil
	default:
		return false, fmt.Errorf("operator %q is not supported for boolean values", op)
	}
}

func compareNumber(op string, actual, expected []byte) (bool, error) {
	actRat := new(big.Rat)
	if _, ok := actRat.SetString(string(actual)); !ok {
		return false, fmt.Errorf("failed to parse actual number %q", string(actual))
	}

	if op == OpIn {
		var rawList []json.RawMessage
		if err := json.Unmarshal(expected, &rawList); err != nil {
			return false, fmt.Errorf("expected number array for operator 'in': %w", err)
		}
		for _, elem := range rawList {
			elem = bytes.TrimSpace(elem)
			itemRat := new(big.Rat)
			if _, ok := itemRat.SetString(string(elem)); !ok {
				return false, fmt.Errorf("element in 'in' array is not a valid number: %s", string(elem))
			}
			if actRat.Cmp(itemRat) == 0 {
				return true, nil
			}
		}
		return false, nil
	}

	if !isJSONNumber(expected) {
		return false, fmt.Errorf("incompatible types: cannot compare number with non-number using %q", op)
	}

	expRat := new(big.Rat)
	if _, ok := expRat.SetString(string(expected)); !ok {
		return false, fmt.Errorf("failed to parse expected number %q", string(expected))
	}

	cmp := actRat.Cmp(expRat)
	switch op {
	case OpEq:
		return cmp == 0, nil
	case OpNe:
		return cmp != 0, nil
	case OpGt:
		return cmp > 0, nil
	case OpGte:
		return cmp >= 0, nil
	case OpLt:
		return cmp < 0, nil
	case OpLte:
		return cmp <= 0, nil
	default:
		return false, fmt.Errorf("unsupported number comparison operator %q", op)
	}
}
