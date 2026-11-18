package execution

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

func lockWorkflow(ctx context.Context, tx *sql.Tx, id string) (WorkflowStatus, json.RawMessage, error) {
	var status WorkflowStatus
	var raw sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT status, context FROM workflow_executions WHERE id = ? FOR UPDATE`, id).Scan(&status, &raw)
	return status, json.RawMessage(raw.String), err
}

func (r *executionRepository) GetStepTaskQueue(ctx context.Context, stepExecID string) (string, error) {
	var queue string
	err := r.db.QueryRowContext(ctx, `SELECT task_queue FROM step_executions WHERE id = ?`, stepExecID).Scan(&queue)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errdefs.ErrStepNotFound
	}
	if err != nil {
		return "", err
	}
	return queue, nil
}

func (r *executionRepository) CompleteTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, output json.RawMessage) error {
	output = bytes.TrimSpace(output)
	if len(output) == 0 || bytes.Equal(output, []byte("null")) {
		output = json.RawMessage(`{}`)
	}
	if !json.Valid(output) {
		return errdefs.ErrInvalidStepOutput
	}

	payloadHash, err := util.CanonicalJSONHash(output)
	if err != nil {
		return fmt.Errorf("canonical output hash: %w", err)
	}
	tokenHash := util.HashLeaseToken(leaseToken)

	// Idempotency pre-check on attempt history
	var (
		attAttempt           int
		attServiceID         string
		attWorkerID          string
		attOutcome           string
		attReportPayloadHash sql.NullString
	)
	err = r.db.QueryRowContext(ctx, `SELECT attempt, service_id, worker_id, outcome, report_payload_hash
		FROM step_execution_attempts
		WHERE step_execution_id = ? AND lease_token_hash = ?`, stepExecID, tokenHash).Scan(
		&attAttempt, &attServiceID, &attWorkerID, &attOutcome, &attReportPayloadHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		_ = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM step_executions WHERE id = ?)`, stepExecID).Scan(&exists)
		if !exists {
			return errdefs.ErrStepNotFound
		}
		return errdefs.ErrInvalidLeaseToken
	}
	if err != nil {
		return err
	}

	if attServiceID != serviceID {
		return errdefs.ErrForbiddenTask
	}
	if attWorkerID != workerID {
		return errdefs.ErrLockLost
	}
	if attOutcome == AttemptOutcomeCompleted {
		if attReportPayloadHash.Valid && attReportPayloadHash.String == payloadHash {
			return nil // Idempotent replay
		}
		return errdefs.ErrReportConflict
	}
	if attOutcome != AttemptOutcomeRunning {
		return errdefs.ErrLockLost
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var execID string
	err = tx.QueryRowContext(ctx, `SELECT workflow_execution_id FROM step_executions WHERE id = ?`, stepExecID).Scan(&execID)
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrLockLost
	}
	if err != nil {
		return err
	}

	status, raw, err := lockWorkflow(ctx, tx, execID)
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrLockLost
	}
	if err != nil {
		return err
	}

	// Verify attempt inside transaction under lock BEFORE demanding active lease
	var txOutcome string
	var txPayloadHash sql.NullString
	var txDeadline time.Time
	var txNow time.Time
	err = tx.QueryRowContext(ctx, `SELECT outcome, report_payload_hash, deadline, NOW(3) FROM step_execution_attempts
		WHERE step_execution_id = ? AND attempt = ? FOR UPDATE`, stepExecID, attAttempt).Scan(&txOutcome, &txPayloadHash, &txDeadline, &txNow)
	if err != nil {
		return err
	}
	if txOutcome == AttemptOutcomeCompleted {
		if txPayloadHash.Valid && txPayloadHash.String == payloadHash {
			return nil
		}
		return errdefs.ErrReportConflict
	}
	if txOutcome != AttemptOutcomeRunning {
		return errdefs.ErrLockLost
	}
	if !txNow.Before(txDeadline) {
		return errdefs.ErrLockLost
	}

	s, err := scanStep(tx.QueryRowContext(ctx, `SELECT `+stepColumns+` FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING' AND locked_until > NOW(3) FOR UPDATE`, stepExecID, workerID))
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrLockLost
	}
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
		locked_by = NULL, locked_until = NULL, next_retry_at = NULL, updated_at = NOW(3) WHERE id = ?`, next, string(output), stepExecID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `UPDATE step_execution_attempts SET
		outcome = 'COMPLETED',
		finished_at = NOW(3),
		report_type = 'COMPLETE',
		report_payload_hash = ?,
		updated_at = NOW(3)
		WHERE step_execution_id = ? AND attempt = ?`, payloadHash, stepExecID, attAttempt)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET context = ?, updated_at = NOW(3) WHERE id = ?`, string(merged), s.WorkflowExecutionID)
	if err != nil {
		return err
	}
	if err := reconcileTx(ctx, tx, s.WorkflowExecutionID, status); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *executionRepository) FailTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, errMsg string, retryable bool) error {
	failPayload := map[string]any{
		"error": map[string]any{
			"message":   errMsg,
			"retryable": retryable,
		},
	}
	payloadBytes, _ := json.Marshal(failPayload)
	payloadHash, err := util.CanonicalJSONHash(payloadBytes)
	if err != nil {
		return fmt.Errorf("canonical fail hash: %w", err)
	}
	tokenHash := util.HashLeaseToken(leaseToken)

	var (
		attAttempt           int
		attServiceID         string
		attWorkerID          string
		attOutcome           string
		attReportPayloadHash sql.NullString
	)
	err = r.db.QueryRowContext(ctx, `SELECT attempt, service_id, worker_id, outcome, report_payload_hash
		FROM step_execution_attempts
		WHERE step_execution_id = ? AND lease_token_hash = ?`, stepExecID, tokenHash).Scan(
		&attAttempt, &attServiceID, &attWorkerID, &attOutcome, &attReportPayloadHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		_ = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM step_executions WHERE id = ?)`, stepExecID).Scan(&exists)
		if !exists {
			return errdefs.ErrStepNotFound
		}
		return errdefs.ErrInvalidLeaseToken
	}
	if err != nil {
		return err
	}

	if attServiceID != serviceID {
		return errdefs.ErrForbiddenTask
	}
	if attWorkerID != workerID {
		return errdefs.ErrLockLost
	}
	if attOutcome == AttemptOutcomeFailed {
		if attReportPayloadHash.Valid && attReportPayloadHash.String == payloadHash {
			return nil // Idempotent replay
		}
		return errdefs.ErrReportConflict
	}
	if attOutcome != AttemptOutcomeRunning {
		return errdefs.ErrLockLost
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var execID string
	err = tx.QueryRowContext(ctx, `SELECT workflow_execution_id FROM step_executions WHERE id = ?`, stepExecID).Scan(&execID)
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrLockLost
	}
	if err != nil {
		return err
	}

	status, _, err := lockWorkflow(ctx, tx, execID)
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrLockLost
	}
	if err != nil {
		return err
	}

	// Verify attempt inside transaction under lock BEFORE demanding active lease
	var txOutcome string
	var txPayloadHash sql.NullString
	var txDeadline time.Time
	var txNow time.Time
	err = tx.QueryRowContext(ctx, `SELECT outcome, report_payload_hash, deadline, NOW(3) FROM step_execution_attempts
		WHERE step_execution_id = ? AND attempt = ? FOR UPDATE`, stepExecID, attAttempt).Scan(&txOutcome, &txPayloadHash, &txDeadline, &txNow)
	if err != nil {
		return err
	}
	if txOutcome == AttemptOutcomeFailed {
		if txPayloadHash.Valid && txPayloadHash.String == payloadHash {
			return nil
		}
		return errdefs.ErrReportConflict
	}
	if txOutcome != AttemptOutcomeRunning {
		return errdefs.ErrLockLost
	}
	if !txNow.Before(txDeadline) {
		return errdefs.ErrLockLost
	}

	s, err := scanStep(tx.QueryRowContext(ctx, `SELECT `+stepColumns+` FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING' AND locked_until > NOW(3) FOR UPDATE`, stepExecID, workerID))
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrLockLost
	}
	if err != nil {
		return err
	}

	if err := recordFailureWithRetryableTx(ctx, tx, s, status, errMsg, retryable); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `UPDATE step_execution_attempts SET
		outcome = 'FAILED',
		finished_at = NOW(3),
		report_type = 'FAIL',
		report_payload_hash = ?,
		error_message = ?,
		updated_at = NOW(3)
		WHERE step_execution_id = ? AND attempt = ?`, payloadHash, errMsg, stepExecID, attAttempt)
	if err != nil {
		return err
	}

	if err := reconcileTx(ctx, tx, s.WorkflowExecutionID, status); err != nil {
		return err
	}
	return tx.Commit()
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
