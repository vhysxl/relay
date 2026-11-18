package execution

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

func (r *executionRepository) CompleteTask(ctx context.Context, serviceID, id, workerID, token string, output json.RawMessage) error {
	output = bytes.TrimSpace(output)
	if len(output) == 0 || bytes.Equal(output, []byte("null")) {
		output = json.RawMessage(`{}`)
	}
	if !json.Valid(output) {
		return errdefs.ErrInvalidStepOutput
	}
	return r.reportTask(ctx, serviceID, id, workerID, token, ReportTypeComplete, output, "", false)
}

func (r *executionRepository) FailTask(ctx context.Context, serviceID, id, workerID, token, message string, retryable bool) error {
	payload, err := json.Marshal(map[string]any{"error": map[string]any{"message": message, "retryable": retryable}})
	if err != nil {
		return err
	}
	return r.reportTask(ctx, serviceID, id, workerID, token, ReportTypeFail, payload, message, retryable)
}

// reportTask serializes reports with claim/recovery, including replays of an older attempt.
func (r *executionRepository) reportTask(ctx context.Context, serviceID, id, workerID, token, reportType string, payload json.RawMessage, message string, retryable bool) error {
	hash, err := util.CanonicalJSONHash(payload)
	if err != nil {
		return fmt.Errorf("canonical report hash: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var execID string
	err = tx.QueryRowContext(ctx, `SELECT workflow_execution_id FROM step_executions WHERE id=?`, id).Scan(&execID)
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrStepNotFound
	}
	if err != nil {
		return err
	}
	status, raw, err := lockWorkflow(ctx, tx, execID)
	if err != nil {
		return err
	}

	var attempt int
	var ownerService, ownerWorker, outcome string
	var previousHash, previousType sql.NullString
	var live bool
	err = tx.QueryRowContext(ctx, `SELECT attempt, service_id, worker_id, outcome, report_payload_hash, report_type, deadline>NOW(3)
	 FROM step_execution_attempts WHERE step_execution_id=? AND lease_token_hash=? FOR UPDATE`, id, util.HashLeaseToken(token)).Scan(
		&attempt, &ownerService, &ownerWorker, &outcome, &previousHash, &previousType, &live)
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrInvalidLeaseToken
	}
	if err != nil {
		return err
	}
	if ownerService != serviceID {
		return errdefs.ErrForbiddenTask
	}
	if ownerWorker != workerID {
		return errdefs.ErrLockLost
	}

	expectedOutcome := AttemptOutcomeCompleted
	if reportType == ReportTypeFail {
		expectedOutcome = AttemptOutcomeFailed
	}
	if outcome == expectedOutcome {
		if previousType.String == reportType && previousHash.Valid && previousHash.String == hash {
			return nil
		}
		return errdefs.ErrReportConflict
	}
	if outcome != AttemptOutcomeRunning || !live {
		return errdefs.ErrLockLost
	}
	s, err := scanStep(tx.QueryRowContext(ctx, `SELECT `+stepColumns+` FROM step_executions
	 WHERE id=? AND attempt=? AND locked_by=? AND status='RUNNING' AND locked_until>NOW(3) FOR UPDATE`, id, attempt, workerID))
	if errors.Is(err, sql.ErrNoRows) {
		return errdefs.ErrLockLost
	}
	if err != nil {
		return err
	}

	if reportType == ReportTypeComplete {
		merged, err := accumulateOutput(raw, s, payload)
		if err != nil {
			return fmt.Errorf("accumulate output: %w", err)
		}
		next := StatusCompleted
		if s.IsCompensation {
			next = StatusCompensated
		}
		if _, err := tx.ExecContext(ctx, `UPDATE step_executions SET status=?, output=?, locked_by=NULL,
		 locked_until=NULL, next_retry_at=NULL, updated_at=NOW(3) WHERE id=?`, next, string(payload), id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_executions SET context=?, updated_at=NOW(3) WHERE id=?`, string(merged), execID); err != nil {
			return err
		}
	} else {
		if err := recordFailureWithRetryableTx(ctx, tx, s, status, message, retryable); err != nil {
			return err
		}
	}
	var errorMessage any
	if reportType == ReportTypeFail {
		errorMessage = message
	}
	if _, err := tx.ExecContext(ctx, `UPDATE step_execution_attempts SET outcome=?, finished_at=NOW(3),
	 report_type=?, report_payload_hash=?, error_message=?, updated_at=NOW(3) WHERE step_execution_id=? AND attempt=?`,
		expectedOutcome, reportType, hash, errorMessage, id, attempt); err != nil {
		return err
	}
	if err := reconcileTx(ctx, tx, execID, status); err != nil {
		return err
	}
	return tx.Commit()
}
