package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

// escapeJSONPointer escapes characters for RFC 6901 JSON Pointer tokens:
// '~' is escaped as '~0' and '/' is escaped as '~1'.
func escapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

// CompareWorkflows computes the differences between two workflow versions.
// It ignores workflow ID, timestamps, JSON object key order, step order, and dependencies order.
func CompareWorkflows(from, to *Workflow) []WorkflowChange {
	var changes []WorkflowChange

	// 1. Context diff
	contextChanges := diffContext(from.Context, to.Context)
	changes = append(changes, contextChanges...)

	// 2. Steps diff
	stepChanges := diffSteps(from.Steps, to.Steps)
	changes = append(changes, stepChanges...)

	if changes == nil {
		return []WorkflowChange{}
	}
	return changes
}

func diffContext(fromRaw, toRaw json.RawMessage) []WorkflowChange {
	if util.AreJSONEqual(fromRaw, toRaw) {
		return nil
	}

	fromMap := parseRawToMap(fromRaw)
	toMap := parseRawToMap(toRaw)

	return diffJSONMaps(fromMap, toMap, "/context")
}

func diffSteps(fromSteps, toSteps []step.Step) []WorkflowChange {
	fromMap := make(map[string]step.Step, len(fromSteps))
	for _, s := range fromSteps {
		fromMap[s.ID] = s
	}

	toMap := make(map[string]step.Step, len(toSteps))
	for _, s := range toSteps {
		toMap[s.ID] = s
	}

	// Collect unique step IDs sorted deterministically
	idSet := make(map[string]struct{}, len(fromSteps)+len(toSteps))
	for id := range fromMap {
		idSet[id] = struct{}{}
	}
	for id := range toMap {
		idSet[id] = struct{}{}
	}

	sortedIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	var changes []WorkflowChange
	for _, id := range sortedIDs {
		escapedID := escapeJSONPointer(id)
		stepPath := "/steps/" + escapedID

		fromStep, inFrom := fromMap[id]
		toStep, inTo := toMap[id]

		if inTo && !inFrom {
			changes = append(changes, WorkflowChange{
				Path:  stepPath,
				Kind:  ChangeKindAdded,
				After: cleanStepForDiff(toStep),
			})
			continue
		}

		if inFrom && !inTo {
			changes = append(changes, WorkflowChange{
				Path:   stepPath,
				Kind:   ChangeKindRemoved,
				Before: cleanStepForDiff(fromStep),
			})
			continue
		}

		// Both present -> compare fields
		stepDiffs := compareStepFields(fromStep, toStep, stepPath)
		changes = append(changes, stepDiffs...)
	}

	return changes
}

func compareStepFields(from, to step.Step, stepPath string) []WorkflowChange {
	var changes []WorkflowChange

	// Name
	if from.Name != to.Name {
		path := stepPath + "/name"
		if from.Name == "" {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindAdded, After: to.Name})
		} else if to.Name == "" {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindRemoved, Before: from.Name})
		} else {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindModified, Before: from.Name, After: to.Name})
		}
	}

	// Type
	if from.Type != to.Type {
		changes = append(changes, WorkflowChange{
			Path:   stepPath + "/type",
			Kind:   ChangeKindModified,
			Before: string(from.Type),
			After:  string(to.Type),
		})
	}

	// Config
	if !util.AreJSONEqual(from.Config, to.Config) {
		fromCfgMap := parseRawToMap(from.Config)
		toCfgMap := parseRawToMap(to.Config)
		if fromCfgMap != nil && toCfgMap != nil {
			cfgChanges := diffJSONMaps(fromCfgMap, toCfgMap, stepPath+"/config")
			changes = append(changes, cfgChanges...)
		} else {
			changes = append(changes, WorkflowChange{
				Path:   stepPath + "/config",
				Kind:   ChangeKindModified,
				Before: parseRawToAny(from.Config),
				After:  parseRawToAny(to.Config),
			})
		}
	}

	// Dependencies (unordered)
	if !util.AreStringSlicesEqualUnordered(from.DependsOn, to.DependsOn) {
		path := stepPath + "/depends_on"
		if len(from.DependsOn) == 0 {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindAdded, After: to.DependsOn})
		} else if len(to.DependsOn) == 0 {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindRemoved, Before: from.DependsOn})
		} else {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindModified, Before: from.DependsOn, After: to.DependsOn})
		}
	}

	// Timeout
	if from.Timeout != to.Timeout {
		path := stepPath + "/timeout"
		if from.Timeout == 0 {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindAdded, After: to.Timeout})
		} else if to.Timeout == 0 {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindRemoved, Before: from.Timeout})
		} else {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindModified, Before: from.Timeout, After: to.Timeout})
		}
	}

	// Retry
	changes = append(changes, diffRetry(from.Retry, to.Retry, stepPath+"/retry")...)

	// Compensation
	changes = append(changes, diffCompensation(from.Compensation, to.Compensation, stepPath+"/compensation")...)

	// Irreversible
	if from.Irreversible != to.Irreversible {
		changes = append(changes, WorkflowChange{
			Path:   stepPath + "/irreversible",
			Kind:   ChangeKindModified,
			Before: from.Irreversible,
			After:  to.Irreversible,
		})
	}

	// Condition (When)
	if !util.AreJSONEqual(from.When, to.When) {
		path := stepPath + "/when"
		fromEmpty := len(bytes.TrimSpace(from.When)) == 0 || bytes.Equal(bytes.TrimSpace(from.When), []byte("null"))
		toEmpty := len(bytes.TrimSpace(to.When)) == 0 || bytes.Equal(bytes.TrimSpace(to.When), []byte("null"))
		if fromEmpty && !toEmpty {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindAdded, After: parseRawToAny(to.When)})
		} else if !fromEmpty && toEmpty {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindRemoved, Before: parseRawToAny(from.When)})
		} else {
			changes = append(changes, WorkflowChange{Path: path, Kind: ChangeKindModified, Before: parseRawToAny(from.When), After: parseRawToAny(to.When)})
		}
	}

	return changes
}

func diffRetry(from, to *step.RetryPolicy, path string) []WorkflowChange {
	if from == nil && to == nil {
		return nil
	}
	if from == nil && to != nil {
		return []WorkflowChange{{Path: path, Kind: ChangeKindAdded, After: to}}
	}
	if from != nil && to == nil {
		return []WorkflowChange{{Path: path, Kind: ChangeKindRemoved, Before: from}}
	}

	var changes []WorkflowChange
	if from.MaxAttempts != to.MaxAttempts {
		changes = append(changes, WorkflowChange{
			Path:   path + "/max_attempts",
			Kind:   ChangeKindModified,
			Before: from.MaxAttempts,
			After:  to.MaxAttempts,
		})
	}
	if from.Backoff != to.Backoff {
		changes = append(changes, WorkflowChange{
			Path:   path + "/backoff",
			Kind:   ChangeKindModified,
			Before: from.Backoff,
			After:  to.Backoff,
		})
	}
	if from.Multiplier != to.Multiplier {
		changes = append(changes, WorkflowChange{
			Path:   path + "/multiplier",
			Kind:   ChangeKindModified,
			Before: from.Multiplier,
			After:  to.Multiplier,
		})
	}
	if from.MaxBackoff != to.MaxBackoff {
		changes = append(changes, WorkflowChange{
			Path:   path + "/max_backoff",
			Kind:   ChangeKindModified,
			Before: from.MaxBackoff,
			After:  to.MaxBackoff,
		})
	}
	return changes
}

func diffCompensation(from, to *step.Compensation, path string) []WorkflowChange {
	if from == nil && to == nil {
		return nil
	}
	if from == nil && to != nil {
		return []WorkflowChange{{Path: path, Kind: ChangeKindAdded, After: to}}
	}
	if from != nil && to == nil {
		return []WorkflowChange{{Path: path, Kind: ChangeKindRemoved, Before: from}}
	}

	var changes []WorkflowChange
	if from.Type != to.Type {
		changes = append(changes, WorkflowChange{
			Path:   path + "/type",
			Kind:   ChangeKindModified,
			Before: string(from.Type),
			After:  string(to.Type),
		})
	}
	if !util.AreJSONEqual(from.Config, to.Config) {
		fromCfg := parseRawToMap(from.Config)
		toCfg := parseRawToMap(to.Config)
		if fromCfg != nil && toCfg != nil {
			changes = append(changes, diffJSONMaps(fromCfg, toCfg, path+"/config")...)
		} else {
			changes = append(changes, WorkflowChange{
				Path:   path + "/config",
				Kind:   ChangeKindModified,
				Before: parseRawToAny(from.Config),
				After:  parseRawToAny(to.Config),
			})
		}
	}
	if from.Timeout != to.Timeout {
		p := path + "/timeout"
		if from.Timeout == 0 {
			changes = append(changes, WorkflowChange{Path: p, Kind: ChangeKindAdded, After: to.Timeout})
		} else if to.Timeout == 0 {
			changes = append(changes, WorkflowChange{Path: p, Kind: ChangeKindRemoved, Before: from.Timeout})
		} else {
			changes = append(changes, WorkflowChange{Path: p, Kind: ChangeKindModified, Before: from.Timeout, After: to.Timeout})
		}
	}
	changes = append(changes, diffRetry(from.Retry, to.Retry, path+"/retry")...)
	return changes
}

func diffJSONMaps(fromMap, toMap map[string]any, prefix string) []WorkflowChange {
	keySet := make(map[string]struct{}, len(fromMap)+len(toMap))
	for k := range fromMap {
		keySet[k] = struct{}{}
	}
	for k := range toMap {
		keySet[k] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(keySet))
	for k := range keySet {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	var changes []WorkflowChange
	for _, k := range sortedKeys {
		p := prefix + "/" + escapeJSONPointer(k)
		fromVal, inFrom := fromMap[k]
		toVal, inTo := toMap[k]

		if inTo && !inFrom {
			changes = append(changes, WorkflowChange{
				Path:  p,
				Kind:  ChangeKindAdded,
				After: toVal,
			})
			continue
		}

		if inFrom && !inTo {
			changes = append(changes, WorkflowChange{
				Path:   p,
				Kind:   ChangeKindRemoved,
				Before: fromVal,
			})
			continue
		}

		// Both present
		fromSubMap, fromIsMap := fromVal.(map[string]any)
		toSubMap, toIsMap := toVal.(map[string]any)
		if fromIsMap && toIsMap {
			changes = append(changes, diffJSONMaps(fromSubMap, toSubMap, p)...)
			continue
		}

		if !areJSONValuesEqual(fromVal, toVal) {
			changes = append(changes, WorkflowChange{
				Path:   p,
				Kind:   ChangeKindModified,
				Before: fromVal,
				After:  toVal,
			})
		}
	}

	return changes
}

func cleanStepForDiff(s step.Step) map[string]any {
	raw, err := json.Marshal(s)
	if err != nil {
		return map[string]any{"id": s.ID}
	}
	m := parseRawToMap(raw)
	delete(m, "workflow_id")
	delete(m, "created_at")
	delete(m, "updated_at")
	return m
}

func parseRawToMap(raw json.RawMessage) map[string]any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil
	}
	return m
}

func parseRawToAny(raw json.RawMessage) any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return string(trimmed)
	}
	return v
}

func areJSONValuesEqual(a, b any) bool {
	normA := normalizeValueForDiff(a)
	normB := normalizeValueForDiff(b)
	return reflect.DeepEqual(normA, normB)
}

type normalizedDiffNumber string

func normalizeValueForDiff(v any) any {
	switch val := v.(type) {
	case json.Number:
		if r, ok := new(big.Rat).SetString(string(val)); ok {
			return normalizedDiffNumber(r.String())
		}
		return normalizedDiffNumber(string(val))
	case float64:
		r := new(big.Rat).SetFloat64(val)
		if r != nil {
			return normalizedDiffNumber(r.String())
		}
		return normalizedDiffNumber(fmt.Sprintf("%v", val))
	case int:
		return normalizedDiffNumber(fmt.Sprintf("%d", val))
	case int64:
		return normalizedDiffNumber(fmt.Sprintf("%d", val))
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, child := range val {
			res[k] = normalizeValueForDiff(child)
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, child := range val {
			res[i] = normalizeValueForDiff(child)
		}
		return res
	default:
		return val
	}
}

