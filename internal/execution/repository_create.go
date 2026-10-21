package execution

import (
	"context"
	"encoding/json"
	"fmt"
)

func (r *executionRepository) CreateExecution(ctx context.Context, exec *WorkflowExecution) (*WorkflowExecution, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin execution: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_executions
		(id, workflow_id, status, context, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		exec.ID, exec.WorkflowID, exec.Status, string(exec.Context), exec.CreatedAt, exec.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert execution: %w", err)
	}
	for _, s := range exec.Steps {
		deps, err := json.Marshal(s.DependsOn)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on, status,
			attempt, max_attempts, timeout_seconds, backoff_seconds, multiplier, max_backoff_seconds,
			is_compensation, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, exec.ID, s.StepID, s.Name, s.Type, string(s.Config), string(deps), s.Status,
			s.Attempt, s.MaxAttempts, s.TimeoutSeconds, s.BackoffSeconds, s.Multiplier, s.MaxBackoffSeconds,
			s.IsCompensation, s.CreatedAt, s.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("insert step %s: %w", s.StepID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit execution: %w", err)
	}
	return exec, nil
}
