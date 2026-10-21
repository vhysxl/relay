package execution

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vhysxl/relay/pkg/errdefs"
)

func lockWorkflow(ctx context.Context, tx *sql.Tx, id string) (WorkflowStatus, json.RawMessage, error) {
	var status WorkflowStatus
	var raw sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT status, context FROM workflow_executions WHERE id = ? FOR UPDATE`, id).Scan(&status, &raw)
	return status, json.RawMessage(raw.String), err
}

// Read the immutable parent ID first, then always acquire workflow before step.
// Claim, completion, failure, and recovery use the same lock order.
func (r *executionRepository) ownedStepTx(ctx context.Context, tx *sql.Tx, id, worker string) (StepExecution, WorkflowStatus, json.RawMessage, error) {
	var execID string
	err := tx.QueryRowContext(ctx, `SELECT workflow_execution_id FROM step_executions WHERE id = ?`, id).Scan(&execID)
	if errors.Is(err, sql.ErrNoRows) {
		return StepExecution{}, "", nil, errdefs.ErrLockLost
	}
	if err != nil {
		return StepExecution{}, "", nil, err
	}
	status, raw, err := lockWorkflow(ctx, tx, execID)
	if errors.Is(err, sql.ErrNoRows) {
		return StepExecution{}, "", nil, errdefs.ErrLockLost
	}
	if err != nil {
		return StepExecution{}, "", nil, err
	}
	s, err := scanStep(tx.QueryRowContext(ctx, `SELECT `+stepColumns+` FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING' AND locked_until > NOW() FOR UPDATE`, id, worker))
	if errors.Is(err, sql.ErrNoRows) {
		err = errdefs.ErrLockLost
	}
	return s, status, raw, err
}

func (r *executionRepository) CompleteStep(ctx context.Context, id, worker string, output json.RawMessage) error {
	output = bytes.TrimSpace(output)
	if len(output) == 0 || bytes.Equal(output, []byte("null")) {
		output = json.RawMessage(`{}`)
	}
	if !json.Valid(output) {
		return errdefs.ErrInvalidStepOutput
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	s, status, raw, err := r.ownedStepTx(ctx, tx, id, worker)
	if err != nil {
		return err
	}
	next := StatusCompleted
	if s.IsCompensation {
		next = StatusCompensated
	}
	merged, err := accumulateOutput(raw, s, output)
	if err != nil {
		return fmt.Errorf("accumulate output: %w", err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE step_executions SET status = ?, output = ?,
		locked_by = NULL, locked_until = NULL, next_retry_at = NULL, updated_at = NOW() WHERE id = ?`, next, string(output), id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET context = ?, updated_at = NOW() WHERE id = ?`, string(merged), s.WorkflowExecutionID)
	if err != nil {
		return err
	}
	if err := reconcileTx(ctx, tx, s.WorkflowExecutionID, status); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *executionRepository) FailStep(ctx context.Context, id, worker, message string) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	s, status, _, err := r.ownedStepTx(ctx, tx, id, worker)
	if err != nil {
		return err
	}
	if err := recordFailureTx(ctx, tx, s, status, message); err != nil {
		return err
	}
	if err := reconcileTx(ctx, tx, s.WorkflowExecutionID, status); err != nil {
		return err
	}
	return tx.Commit()
}

func recordFailureTx(ctx context.Context, tx *sql.Tx, s StepExecution, status WorkflowStatus, message string) error {
	if mayRetry(status, s) {
		_, err := tx.ExecContext(ctx, `UPDATE step_executions SET status = 'PENDING', error_message = ?,
			next_retry_at = DATE_ADD(NOW(), INTERVAL ? SECOND), locked_by = NULL, locked_until = NULL,
			updated_at = NOW() WHERE id = ?`, message, retryDelay(s), s.ID)
		return err
	}
	next := StatusFailed
	if s.IsCompensation {
		next = StatusCompensationFailed
	}
	_, err := tx.ExecContext(ctx, `UPDATE step_executions SET status = ?, error_message = ?,
		locked_by = NULL, locked_until = NULL, next_retry_at = NULL, updated_at = NOW() WHERE id = ?`, next, message, s.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET error_message = ?, updated_at = NOW()
		WHERE id = ? AND status NOT IN ('COMPENSATION_FAILED', 'COMPLETED', 'COMPENSATED')`,
		fmt.Sprintf("step %s failed: %s", s.ID, message), s.WorkflowExecutionID)
	return err
}
