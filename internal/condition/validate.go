package condition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/vhysxl/relay/pkg/errdefs"
)

// ParseCondition unmarshals raw JSON into a Condition with strict structure checking.
func ParseCondition(raw []byte) (*Condition, error) {
	return ParseConditionWithLimits(raw, 0, 0)
}

// ParseConditionWithLimits unmarshals raw JSON into a Condition while enforcing maxDepth and maxNodes during parsing.
// Parsing aborts immediately if maxDepth or maxNodes is exceeded, protecting against resource exhaustion.
func ParseConditionWithLimits(raw []byte, maxDepth, maxNodes int) (*Condition, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, fmt.Errorf("%w: condition cannot be empty", errdefs.ErrInvalidCondition)
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &rawMap); err != nil || rawMap == nil {
		return nil, fmt.Errorf("%w: condition must be a JSON object: %v", errdefs.ErrInvalidCondition, err)
	}

	nodeCount := 0
	return parseNode(rawMap, 1, &nodeCount, maxDepth, maxNodes)
}

func parseNode(rawMap map[string]json.RawMessage, depth int, nodeCount *int, maxDepth, maxNodes int) (*Condition, error) {
	*nodeCount++
	if maxNodes > 0 && *nodeCount > maxNodes {
		return nil, errdefs.ErrConditionMaxNodes
	}
	if maxDepth > 0 && depth > maxDepth {
		return nil, errdefs.ErrConditionMaxDepth
	}

	hasAll := rawMap["all"] != nil
	hasAny := rawMap["any"] != nil
	hasRef := rawMap["ref"] != nil
	hasOp := rawMap["op"] != nil
	hasVal := rawMap["value"] != nil

	// Check for unknown keys
	for k := range rawMap {
		if k != "all" && k != "any" && k != "ref" && k != "op" && k != "value" {
			return nil, fmt.Errorf("%w: unknown field %q", errdefs.ErrInvalidCondition, k)
		}
	}

	// Validate shape exclusivity
	groupCount := 0
	if hasAll {
		groupCount++
	}
	if hasAny {
		groupCount++
	}

	if groupCount > 1 {
		return nil, fmt.Errorf("%w: node cannot define both 'all' and 'any'", errdefs.ErrInvalidCondition)
	}

	if groupCount == 1 {
		if hasRef || hasOp || hasVal {
			return nil, fmt.Errorf("%w: group node cannot contain leaf fields (ref, op, value)", errdefs.ErrInvalidCondition)
		}

		if hasAll {
			var rawList []json.RawMessage
			if err := json.Unmarshal(rawMap["all"], &rawList); err != nil {
				return nil, fmt.Errorf("%w: 'all' must be an array: %v", errdefs.ErrInvalidCondition, err)
			}
			if len(rawList) == 0 {
				return nil, fmt.Errorf("%w: 'all' group cannot be empty", errdefs.ErrInvalidCondition)
			}
			children := make([]Condition, len(rawList))
			for i, r := range rawList {
				var childMap map[string]json.RawMessage
				if err := json.Unmarshal(bytes.TrimSpace(r), &childMap); err != nil || childMap == nil {
					return nil, fmt.Errorf("%w: child condition must be an object", errdefs.ErrInvalidCondition)
				}
				child, err := parseNode(childMap, depth+1, nodeCount, maxDepth, maxNodes)
				if err != nil {
					return nil, err
				}
				children[i] = *child
			}
			return &Condition{All: children}, nil
		}

		// hasAny
		var rawList []json.RawMessage
		if err := json.Unmarshal(rawMap["any"], &rawList); err != nil {
			return nil, fmt.Errorf("%w: 'any' must be an array: %v", errdefs.ErrInvalidCondition, err)
		}
		if len(rawList) == 0 {
			return nil, fmt.Errorf("%w: 'any' group cannot be empty", errdefs.ErrInvalidCondition)
		}
		children := make([]Condition, len(rawList))
		for i, r := range rawList {
			var childMap map[string]json.RawMessage
			if err := json.Unmarshal(bytes.TrimSpace(r), &childMap); err != nil || childMap == nil {
				return nil, fmt.Errorf("%w: child condition must be an object", errdefs.ErrInvalidCondition)
			}
			child, err := parseNode(childMap, depth+1, nodeCount, maxDepth, maxNodes)
			if err != nil {
				return nil, err
			}
			children[i] = *child
		}
		return &Condition{Any: children}, nil
	}

	// Leaf condition
	if !hasRef || !hasOp {
		return nil, fmt.Errorf("%w: leaf condition requires both 'ref' and 'op'", errdefs.ErrInvalidCondition)
	}

	var refStr, opStr string
	if err := json.Unmarshal(rawMap["ref"], &refStr); err != nil {
		return nil, fmt.Errorf("%w: 'ref' must be a string", errdefs.ErrInvalidCondition)
	}
	refStr = strings.TrimSpace(refStr)
	if refStr == "" {
		return nil, fmt.Errorf("%w: 'ref' cannot be empty", errdefs.ErrInvalidCondition)
	}

	if err := json.Unmarshal(rawMap["op"], &opStr); err != nil {
		return nil, fmt.Errorf("%w: 'op' must be a string", errdefs.ErrInvalidCondition)
	}
	opStr = strings.TrimSpace(opStr)

	c := &Condition{
		Ref:   refStr,
		Op:    opStr,
		Value: rawMap["value"],
	}

	if err := validateLeafOperator(c); err != nil {
		return nil, err
	}

	return c, nil
}

func validateLeafOperator(c *Condition) error {
	switch c.Op {
	case OpExists:
		if len(bytes.TrimSpace(c.Value)) > 0 && !bytes.Equal(bytes.TrimSpace(c.Value), []byte("null")) {
			return fmt.Errorf("%w: operator 'exists' does not accept a value", errdefs.ErrInvalidCondition)
		}
		c.Value = nil
	case OpEq, OpNe:
		val := bytes.TrimSpace(c.Value)
		if len(val) == 0 {
			return fmt.Errorf("%w: operator %q requires a value", errdefs.ErrInvalidCondition, c.Op)
		}
		if !isScalarOrNull(val) {
			return fmt.Errorf("%w: operator %q value must be a scalar or null", errdefs.ErrInvalidCondition, c.Op)
		}
	case OpGt, OpGte, OpLt, OpLte:
		val := bytes.TrimSpace(c.Value)
		if len(val) == 0 {
			return fmt.Errorf("%w: operator %q requires a value", errdefs.ErrInvalidCondition, c.Op)
		}
		if !isJSONNumber(val) {
			return fmt.Errorf("%w: operator %q value must be a number", errdefs.ErrInvalidCondition, c.Op)
		}
	case OpIn:
		val := bytes.TrimSpace(c.Value)
		if len(val) == 0 {
			return fmt.Errorf("%w: operator 'in' requires a non-empty array of scalars", errdefs.ErrInvalidCondition)
		}
		if err := validateInArray(val); err != nil {
			return fmt.Errorf("%w: %v", errdefs.ErrInvalidCondition, err)
		}
	default:
		return fmt.Errorf("%w: unsupported operator %q", errdefs.ErrInvalidCondition, c.Op)
	}
	return nil
}

func isScalarOrNull(raw []byte) bool {
	if bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false")) {
		return true
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return true
	}
	return isJSONNumber(raw)
}

func isJSONNumber(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	// Attempt parsing using big.Rat or big.Float to accept any precision
	r := new(big.Rat)
	_, ok := r.SetString(string(raw))
	return ok
}

func validateInArray(raw []byte) error {
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return fmt.Errorf("'in' value must be a JSON array: %w", err)
	}
	if len(elements) == 0 {
		return fmt.Errorf("'in' array cannot be empty")
	}

	var expectedType string // "string", "number", "bool"
	for i, elem := range elements {
		e := bytes.TrimSpace(elem)
		var elemType string
		switch {
		case bytes.Equal(e, []byte("true")), bytes.Equal(e, []byte("false")):
			elemType = "bool"
		case len(e) >= 2 && e[0] == '"' && e[len(e)-1] == '"':
			elemType = "string"
		case isJSONNumber(e):
			elemType = "number"
		default:
			return fmt.Errorf("'in' array element at index %d is not a valid scalar", i)
		}

		if expectedType == "" {
			expectedType = elemType
		} else if expectedType != elemType {
			return fmt.Errorf("'in' array elements must have uniform scalar type (got %s and %s)", expectedType, elemType)
		}
	}
	return nil
}

// CollectRefs recursively extracts all anchor references in a Condition.
func CollectRefs(c *Condition) []string {
	var refs []string
	var walk func(curr *Condition)
	walk = func(curr *Condition) {
		if curr == nil {
			return
		}
		if curr.Ref != "" {
			refs = append(refs, curr.Ref)
		}
		for i := range curr.All {
			walk(&curr.All[i])
		}
		for i := range curr.Any {
			walk(&curr.Any[i])
		}
	}
	walk(c)
	return refs
}
