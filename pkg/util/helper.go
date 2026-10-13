package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

var (
	quotedPlaceholderRegex = regexp.MustCompile(`"\{\{\s*(?:context\.)?([a-zA-Z0-9_.-]+)\s*\}\}"`)
	placeholderRegex       = regexp.MustCompile(`\{\{\s*(?:context\.)?([a-zA-Z0-9_.-]+)\s*\}\}`)
)

func GenerateUUID() string {
	return uuid.New().String()
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// InterpolateContext substitutes {{context.key}} or {{key}} placeholders in templateBytes
// using key-value pairs from contextJSON.
func InterpolateContext(templateBytes []byte, contextJSON []byte) []byte {
	if len(templateBytes) == 0 || len(contextJSON) == 0 {
		return templateBytes
	}

	var ctxMap map[string]interface{}
	if err := json.Unmarshal(contextJSON, &ctxMap); err != nil {
		return templateBytes
	}
	if len(ctxMap) == 0 {
		return templateBytes
	}

	// 1. First pass: Replace quoted placeholders when the resolved value is a complex type
	// (map or slice), transforming `"{{path}}"` into valid inlined JSON `{...}` or `[...]`.
	res := quotedPlaceholderRegex.ReplaceAllFunc(templateBytes, func(match []byte) []byte {
		m := quotedPlaceholderRegex.FindSubmatch(match)
		if len(m) < 2 {
			return match
		}
		keyPath := string(m[1])
		val, ok := getNestedValue(ctxMap, keyPath)
		if !ok {
			return match
		}
		switch val.(type) {
		case map[string]interface{}, []interface{}:
			b, err := json.Marshal(val)
			if err == nil {
				return b
			}
			return match
		default:
			// Primitives (string, number, bool) leave quotes intact for second pass
			return match
		}
	})

	// 2. Second pass: Replace remaining placeholders (embedded within strings or unquoted)
	res = placeholderRegex.ReplaceAllFunc(res, func(match []byte) []byte {
		m := placeholderRegex.FindSubmatch(match)
		if len(m) < 2 {
			return match
		}
		keyPath := string(m[1])
		val, ok := getNestedValue(ctxMap, keyPath)
		if !ok {
			return match
		}
		switch v := val.(type) {
		case string:
			return escapeJSONString(v)
		case float64:
			if v == float64(int64(v)) {
				return []byte(fmt.Sprintf("%d", int64(v)))
			}
			return []byte(fmt.Sprintf("%g", v))
		case bool:
			return []byte(fmt.Sprintf("%t", v))
		default:
			b, err := json.Marshal(v)
			if err == nil {
				return b
			}
			return match
		}
	})

	return res
}

func escapeJSONString(s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err == nil {
		b := bytes.TrimRight(buf.Bytes(), "\n")
		if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
			return b[1 : len(b)-1]
		}
	}
	return []byte(s)
}

func getNestedValue(m map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var current interface{} = m
	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, exists := v[part]
			if !exists {
				return nil, false
			}
			current = val
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			current = v[idx]
		default:
			return nil, false
		}
	}
	return current, true
}

func AreJSONEqual(a, b json.RawMessage) bool {
	trimmedA := bytes.TrimSpace(a)
	trimmedB := bytes.TrimSpace(b)
	aEmpty := len(trimmedA) == 0 || bytes.Equal(trimmedA, []byte("null"))
	bEmpty := len(trimmedB) == 0 || bytes.Equal(trimmedB, []byte("null"))
	if aEmpty && bEmpty {
		return true
	}
	if aEmpty || bEmpty {
		return false
	}
	var valA, valB any
	if err := json.Unmarshal(trimmedA, &valA); err != nil {
		return false
	}
	if err := json.Unmarshal(trimmedB, &valB); err != nil {
		return false
	}
	return reflect.DeepEqual(valA, valB)
}

// AreStringSlicesEqualUnordered checks whether two string slices contain the same elements
// regardless of order, trimming surrounding whitespace from each element.
func AreStringSlicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[strings.TrimSpace(v)]++
	}
	for _, v := range b {
		trimmed := strings.TrimSpace(v)
		counts[trimmed]--
		if counts[trimmed] < 0 {
			return false
		}
	}
	return true
}
