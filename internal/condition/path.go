package condition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ResolveAnchor navigates ref against context and completed step outputs.
// Returns val as raw JSON bytes, whether the path exists, and any resolution error.
func ResolveAnchor(ref string, contextJSON json.RawMessage, stepOutputs map[string]json.RawMessage) (json.RawMessage, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, false, fmt.Errorf("empty anchor reference")
	}

	if strings.HasPrefix(ref, "steps.") {
		parts := strings.Split(ref, ".")
		if len(parts) < 3 || parts[2] != "output" {
			return nil, false, fmt.Errorf("invalid step output reference %q: must follow 'steps.<id>.output...'", ref)
		}
		stepID := parts[1]
		if stepID == "" {
			return nil, false, fmt.Errorf("empty step id in reference %q", ref)
		}

		outputRaw, ok := stepOutputs[stepID]
		if !ok || len(bytes.TrimSpace(outputRaw)) == 0 {
			// Step was not executed, skipped, failed, or produced no output
			return nil, false, nil
		}

		if len(parts) == 3 {
			// e.g. "steps.b.output"
			return bytes.TrimSpace(outputRaw), true, nil
		}

		remainder := strings.Join(parts[3:], ".")
		return navigateJSON(outputRaw, remainder)
	}

	// Context anchor
	if len(bytes.TrimSpace(contextJSON)) == 0 {
		return nil, false, nil
	}
	return navigateJSON(contextJSON, ref)
}

func navigateJSON(raw json.RawMessage, path string) (json.RawMessage, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, nil
	}
	if path == "" {
		return trimmed, true, nil
	}

	segments := strings.Split(path, ".")
	current := trimmed

	for _, seg := range segments {
		current = bytes.TrimSpace(current)
		if len(current) == 0 {
			return nil, false, nil
		}

		switch current[0] {
		case '{':
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(current, &obj); err != nil {
				return nil, false, nil
			}
			val, ok := obj[seg]
			if !ok {
				return nil, false, nil
			}
			current = val
		case '[':
			var arr []json.RawMessage
			if err := json.Unmarshal(current, &arr); err != nil {
				return nil, false, nil
			}
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(arr) {
				return nil, false, nil
			}
			current = arr[idx]
		default:
			// Scalar primitive or null cannot have nested child properties
			return nil, false, nil
		}
	}

	return bytes.TrimSpace(current), true, nil
}
