package execution

import (
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

func (r *executionRepository) GetStepQueue(ctx context.Context, stepExecID string) (string, error) {
	var queue string
	err := r.db.QueryRowContext(ctx, `SELECT queue FROM step_executions WHERE id = ?`, stepExecID).Scan(&queue)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errdefs.ErrStepNotFound
	}
	if err != nil {
		return "", err
	}
	return queue, nil
}

func recordFailureTx(ctx context.Context, tx *sql.Tx, s StepExecution, status WorkflowStatus, message string) error {
	return recordFailureWithRetryableTx(ctx, tx, s, status, message, true)
}

func recordFailureWithRetryableTx(ctx context.Context, tx *sql.Tx, s StepExecution, status WorkflowStatus, message string, retryable bool) error {
	if retryable && mayRetry(status, s) {
		_, err := tx.ExecContext(ctx, `UPDATE step_executions SET status = 'PENDING', error_message = ?,
			next_retry_at = DATE_ADD(NOW(3), INTERVAL ? SECOND), locked_by = NULL, locked_until = NULL,
			updated_at = NOW(3) WHERE id = ?`, message, retryDelay(s), s.ID)
		return err
	}
	next := StatusFailed
	if s.IsCompensation {
		next = StatusCompensationFailed
	}
	_, err := tx.ExecContext(ctx, `UPDATE step_executions SET status = ?, error_message = ?,
		locked_by = NULL, locked_until = NULL, next_retry_at = NULL, updated_at = NOW(3) WHERE id = ?`, next, message, s.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET error_message = ?, updated_at = NOW(3)
		WHERE id = ? AND status NOT IN ('COMPENSATION_FAILED', 'COMPLETED', 'COMPENSATED')`,
		fmt.Sprintf("step %s failed: %s", s.ID, message), s.WorkflowExecutionID)
	return err
}
