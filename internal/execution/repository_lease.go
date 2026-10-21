package execution

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vhysxl/relay/internal/condition"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
)

func (r *executionRepository) leaseSeconds(duration time.Duration) int64 {
	if duration <= 0 {
		duration = r.defaultLeaseDuration
	}
	// TIMESTAMP stores whole seconds. Round up rather than granting a zero lease.
	return int64((duration-1)/time.Second) + 1
}

func (r *executionRepository) Heartbeat(ctx context.Context, id, worker string, duration time.Duration) error {
	res, err := r.db.ExecContext(ctx, `UPDATE step_executions
		SET locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND), updated_at = NOW()
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING' AND locked_until > NOW()`, r.leaseSeconds(duration), id, worker)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	// Support DSNs without clientFoundRows when a same-second heartbeat is a no-op.
	var exists bool
	err = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING' AND locked_until > NOW())`, id, worker).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errdefs.ErrLockLost
	}
	return nil
}

// ClaimPending locks the parent first and skips busy workflows. Candidate
// discovery stays in SQL rather than loading the entire queue into memory.
func (r *executionRepository) ClaimPending(ctx context.Context, worker string, duration time.Duration) (*step.Step, *StepExecution, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	var execID, workflowID string
	var status WorkflowStatus
	var raw sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT we.id, we.workflow_id, we.status, we.context
 FROM workflow_executions we WHERE we.status IN ('RUNNING','COMPENSATING')
 AND EXISTS (SELECT 1 FROM step_executions se WHERE se.workflow_execution_id=we.id
   AND se.status='PENDING' AND (se.next_retry_at IS NULL OR se.next_retry_at<=NOW())
   AND (se.locked_until IS NULL OR se.locked_until<=NOW())
   AND ((we.status='RUNNING' AND se.is_compensation=FALSE)
     OR (we.status='COMPENSATING' AND se.is_compensation=TRUE)))
 ORDER BY CASE WHEN we.status='COMPENSATING' THEN 0 ELSE 1 END, we.created_at, we.id
 LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&execID, &workflowID, &status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	// Recheck legacy PENDING compensation tickets against the current graph.
	// Older executions may have released them while a downstream original ran.
	if status == StatusCompensating {
		if err := reconcileTx(ctx, tx, execID, status); err != nil {
			return nil, nil, err
		}
	}
	s, err := scanStep(tx.QueryRowContext(ctx, `SELECT `+stepColumns+` FROM step_executions
 WHERE workflow_execution_id=? AND is_compensation=? AND status='PENDING'
 AND (next_retry_at IS NULL OR next_retry_at<=NOW()) AND (locked_until IS NULL OR locked_until<=NOW())
 ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, execID, status == StatusCompensating))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, tx.Commit()
	}
	if err != nil {
		return nil, nil, err
	}
	id := s.ID
	var original sql.NullString
	if s.IsCompensation {
		var originalStatus StepStatus
		err = tx.QueryRowContext(ctx, `SELECT status, output FROM step_executions
			WHERE workflow_execution_id = ? AND step_id = ? AND is_compensation = FALSE`, execID, s.StepID).Scan(&originalStatus, &original)
		if err != nil {
			return nil, nil, err
		}
		if originalStatus != StatusCompleted {
			if err := reconcileTx(ctx, tx, execID, status); err != nil {
				return nil, nil, err
			}
			return nil, nil, tx.Commit()
		}
	}

	var condEvalRaw []byte
	if !s.IsCompensation && len(bytes.TrimSpace(s.When)) > 0 && !bytes.Equal(bytes.TrimSpace(s.When), []byte("null")) {
		if len(bytes.TrimSpace(s.ConditionEvaluation)) > 0 {
			// Reuse previously saved true decision (e.g. retry or lease recovery)
			condEvalRaw = s.ConditionEvaluation
		} else {
			cond, err := condition.ParseCondition(s.When)
			if err != nil {
				return nil, nil, err
			}

			rows, err := tx.QueryContext(ctx, `SELECT step_id, output FROM step_executions
				WHERE workflow_execution_id = ? AND is_compensation = FALSE AND status = 'COMPLETED'`, execID)
			if err != nil {
				return nil, nil, err
			}
			stepOutputs := make(map[string]json.RawMessage)
			for rows.Next() {
				var sid string
				var out sql.NullString
				if err := rows.Scan(&sid, &out); err == nil && out.Valid {
					stepOutputs[sid] = json.RawMessage(out.String)
				}
			}
			rows.Close()

			pass, audit, evalErr := condition.Evaluate(cond, json.RawMessage(raw.String), stepOutputs)
			auditBytes, _ := json.Marshal(audit)
			condEvalRaw = auditBytes

			if evalErr != nil {
				errMsg := fmt.Sprintf("condition evaluation error: %v", evalErr)
				_, err = tx.ExecContext(ctx, `UPDATE step_executions SET status = 'FAILED', condition_evaluation = ?,
					error_message = ?, updated_at = NOW() WHERE id = ?`, string(auditBytes), errMsg, id)
				if err != nil {
					return nil, nil, err
				}
				_, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET error_message = ?, updated_at = NOW()
					WHERE id = ? AND status NOT IN ('COMPENSATION_FAILED', 'COMPLETED', 'COMPENSATED')`,
					fmt.Sprintf("step %s failed: %s", id, errMsg), execID)
				if err != nil {
					return nil, nil, err
				}
				if err := reconcileTx(ctx, tx, execID, status); err != nil {
					return nil, nil, err
				}
				return nil, nil, tx.Commit()
			}

			if !pass {
				_, err = tx.ExecContext(ctx, `UPDATE step_executions SET status = 'SKIPPED', condition_evaluation = ?,
					updated_at = NOW() WHERE id = ?`, string(auditBytes), id)
				if err != nil {
					return nil, nil, err
				}
				if err := reconcileTx(ctx, tx, execID, status); err != nil {
					return nil, nil, err
				}
				return nil, nil, tx.Commit()
			}
		}
	}

	resolved, err := resolveConfig(s.Config, json.RawMessage(raw.String), json.RawMessage(original.String), s.IsCompensation)
	if err != nil {
		return nil, nil, err
	}
	var condEvalVal any
	if len(condEvalRaw) > 0 {
		condEvalVal = string(condEvalRaw)
	}
	_, err = tx.ExecContext(ctx, `UPDATE step_executions SET status = 'RUNNING', attempt = attempt + 1,
		locked_by = ?, locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND), config = ?,
		condition_evaluation = ?, executed_at = NOW(), next_retry_at = NULL, updated_at = NOW() WHERE id = ?`,
		worker, r.leaseSeconds(duration), string(resolved), condEvalVal, id)
	if err != nil {
		return nil, nil, err
	}
	s, err = scanStep(tx.QueryRowContext(ctx, `SELECT `+stepColumns+` FROM step_executions WHERE id = ?`, id))
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	definition := &step.Step{ID: s.StepID, WorkflowID: workflowID, Name: s.Name, Type: step.StepType(s.Type), Config: s.Config,
		DependsOn: s.DependsOn, Timeout: s.TimeoutSeconds, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		Retry: &step.RetryPolicy{MaxAttempts: s.MaxAttempts, Backoff: s.BackoffSeconds, Multiplier: s.Multiplier, MaxBackoff: s.MaxBackoffSeconds}}
	return definition, &s, nil
}
