package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const stepColumns = `id, workflow_execution_id, step_id, name, type, config, depends_on,
	status, output, error_message, attempt, max_attempts, timeout_seconds, backoff_seconds,
	multiplier, max_backoff_seconds, locked_by, locked_until, is_compensation,
	executed_at, next_retry_at, created_at, updated_at`

type rowScanner interface{ Scan(...any) error }

func scanStep(row rowScanner) (StepExecution, error) {
	var s StepExecution
	var name, deps, output, message, owner sql.NullString
	var config string
	var until, executed, retry sql.NullTime
	err := row.Scan(&s.ID, &s.WorkflowExecutionID, &s.StepID, &name, &s.Type, &config, &deps,
		&s.Status, &output, &message, &s.Attempt, &s.MaxAttempts, &s.TimeoutSeconds, &s.BackoffSeconds,
		&s.Multiplier, &s.MaxBackoffSeconds, &owner, &until, &s.IsCompensation, &executed, &retry, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return s, err
	}
	s.Name, s.Config = name.String, json.RawMessage(config)
	if deps.Valid && len(deps.String) > 0 {
		if err := json.Unmarshal([]byte(deps.String), &s.DependsOn); err != nil {
			return s, fmt.Errorf("decode dependencies for %s: %w", s.ID, err)
		}
	}
	if output.Valid {
		s.Output = json.RawMessage(output.String)
	}
	if message.Valid {
		s.ErrorMessage = &message.String
	}
	if owner.Valid {
		s.LockedBy = &owner.String
	}
	if until.Valid {
		s.LockedUntil = &until.Time
	}
	if executed.Valid {
		s.ExecutedAt = &executed.Time
	}
	if retry.Valid {
		s.NextRetryAt = &retry.Time
	}
	return s, nil
}

func loadStepsTx(ctx context.Context, tx *sql.Tx, execID string) ([]StepExecution, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+stepColumns+` FROM step_executions
		WHERE workflow_execution_id = ? ORDER BY id FOR UPDATE`, execID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []StepExecution
	for rows.Next() {
		s, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

func reconcileTx(ctx context.Context, tx *sql.Tx, execID string, status WorkflowStatus) error {
	steps, err := loadStepsTx(ctx, tx, execID)
	if err != nil {
		return err
	}
	nextStatus, nextSteps := reconcile(status, steps)
	for i, s := range nextSteps {
		if s.Status == steps[i].Status {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE step_executions SET status = ?, updated_at = NOW() WHERE id = ?`, s.Status, s.ID); err != nil {
			return err
		}
	}
	if nextStatus != status {
		_, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET status = ?, updated_at = NOW() WHERE id = ?`, nextStatus, execID)
	}
	return err
}
