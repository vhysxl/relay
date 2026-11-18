package execution

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

func contextObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return map[string]json.RawMessage{}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, errdefs.ErrInvalidWorkflowContext
	}
	return obj, nil
}

func mergeContext(base, override json.RawMessage) (json.RawMessage, error) {
	obj, err := contextObject(base)
	if err != nil {
		return nil, fmt.Errorf("stored workflow context: %w", err)
	}
	extra, err := contextObject(override)
	if err != nil {
		return nil, err
	}
	for k, v := range extra {
		obj[k] = v
	}
	return json.Marshal(obj)
}

func normalizeInputContext(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage("{}"), nil
	}
	obj, err := contextObject(trimmed)
	if err != nil {
		return nil, err
	}
	if len(obj) == 0 {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(trimmed), nil
}

func evaluateReplay(record *ExecutionRequestRecord, normalizedInput json.RawMessage, targetIsExplicitID bool, targetID string) (*ExecuteWorkflowResponse, error) {
	if !util.AreJSONEqual(record.InputContext, normalizedInput) {
		return nil, errdefs.ErrIdempotencyConflict
	}
	var receipt ExecuteWorkflowResponse
	if err := json.Unmarshal(record.ResponseReceipt, &receipt); err != nil {
		return nil, fmt.Errorf("unmarshal stored receipt: %w", err)
	}
	if targetIsExplicitID && targetID != receipt.WorkflowID {
		return nil, errdefs.ErrIdempotencyConflict
	}
	return &receipt, nil
}

// RawMessage keeps identifiers and monetary values exact while updating context.
func accumulateOutput(raw json.RawMessage, s StepExecution, output json.RawMessage) (json.RawMessage, error) {
	obj, err := contextObject(raw)
	if err != nil {
		return nil, err
	}
	steps, err := contextObject(obj["steps"])
	if err != nil {
		return nil, fmt.Errorf("invalid steps context: %w", err)
	}
	entry, err := contextObject(steps[s.StepID])
	if err != nil {
		return nil, fmt.Errorf("invalid step output context: %w", err)
	}
	if s.IsCompensation {
		comp, err := contextObject(entry["compensation"])
		if err != nil {
			return nil, err
		}
		comp["output"] = output
		entry["compensation"], err = json.Marshal(comp)
		if err != nil {
			return nil, err
		}
	} else {
		entry["output"] = output
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	steps[s.StepID] = encoded
	obj["steps"], err = json.Marshal(steps)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

func resolveConfig(raw, context, original json.RawMessage, compensation bool) (json.RawMessage, error) {
	obj, err := contextObject(context)
	if err != nil {
		return nil, err
	}
	if compensation && len(original) > 0 {
		if !json.Valid(original) {
			return nil, errdefs.ErrInvalidStepOutput
		}
		obj["this"], err = json.Marshal(map[string]json.RawMessage{"output": original})
		if err != nil {
			return nil, err
		}
	}
	ctxBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	resolved := util.InterpolateContext(raw, ctxBytes)
	if !json.Valid(resolved) {
		return nil, errdefs.ErrInvalidStepConfig
	}
	return resolved, nil
}
