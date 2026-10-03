package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var placeholderRegex = regexp.MustCompile(`\{\{\s*(?:context\.)?([a-zA-Z0-9_.-]+)\s*\}\}`)

func GenerateUUID() string {
	return uuid.New().String()
}

func GenerateWorkflowHash(name string, version int, context []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%d:%s", name, version, string(context))
	return hex.EncodeToString(h.Sum(nil))
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

	res := placeholderRegex.ReplaceAllFunc(templateBytes, func(match []byte) []byte {
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
			return []byte(v)
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

func getNestedValue(m map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var current interface{} = m
	for _, part := range parts {
		currMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, exists := currMap[part]
		if !exists {
			return nil, false
		}
		current = val
	}
	return current, true
}
