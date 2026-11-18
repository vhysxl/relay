package execution

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vhysxl/relay/internal/condition"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

func (r *executionRepository) leaseSeconds(duration time.Duration) int64 {
	if duration <= 0 {
		duration = r.defaultLeaseDuration
	}
	// TIMESTAMP stores whole seconds. Round up rather than granting a zero lease.
	return int64((duration-1)/time.Second) + 1
}

func (r *executionRepository) HeartbeatTask(ctx context.Context, serviceID, stepExecID, workerID, leaseToken string, duration time.Duration) (time.Time, time.Duration, error) {
	tokenHash := util.HashLeaseToken(leaseToken)
	leaseSec := r.leaseSeconds(duration)

	res, err := r.db.ExecContext(ctx, `UPDATE step_executions s
		JOIN step_execution_attempts a ON s.id = a.step_execution_id AND s.attempt = a.attempt
		SET s.locked_until = LEAST(DATE_ADD(NOW(3), INTERVAL ? SECOND), a.deadline),
		    s.updated_at = NOW(3),
		    a.updated_at = NOW(3)
		WHERE s.id = ? AND a.lease_token_hash = ? AND a.service_id = ? AND s.locked_by = ?
		  AND s.status = 'RUNNING' AND a.outcome = 'RUNNING'
		  AND s.locked_until > NOW(3) AND a.deadline > NOW(3)`, leaseSec, stepExecID, tokenHash, serviceID, workerID)
	if err != nil {
		return time.Time{}, 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return time.Time{}, 0, err
	}

	var (
		lockedUntil      sql.NullTime
		deadline         time.Time
		dbNow            time.Time
		currentStatus    string
		currentOutcome   string
		currentService   string
		currentWorker    sql.NullString
		currentTokenHash string
	)
	readErr := r.db.QueryRowContext(ctx, `SELECT s.status, s.locked_until, a.outcome, a.service_id, s.locked_by, a.lease_token_hash, a.deadline, NOW(3)
		FROM step_executions s
		JOIN step_execution_attempts a ON s.id = a.step_execution_id AND s.attempt = a.attempt
		WHERE s.id = ?`, stepExecID).Scan(&currentStatus, &lockedUntil, &currentOutcome, &currentService, &currentWorker, &currentTokenHash, &deadline, &dbNow)
	if readErr != nil {
		if errors.Is(readErr, sql.ErrNoRows) {
			return time.Time{}, 0, errdefs.ErrLockLost
		}
		return time.Time{}, 0, readErr
	}

	if currentTokenHash != tokenHash {
		return time.Time{}, 0, errdefs.ErrInvalidLeaseToken
	}
	if currentService != serviceID {
		return time.Time{}, 0, errdefs.ErrForbiddenTask
	}
	if !currentWorker.Valid || currentWorker.String != workerID || currentStatus != string(StatusRunning) || currentOutcome != AttemptOutcomeRunning {
		return time.Time{}, 0, errdefs.ErrLockLost
	}

	if !lockedUntil.Valid || !dbNow.Before(lockedUntil.Time) || !dbNow.Before(deadline) || n == 0 {
		return time.Time{}, 0, errdefs.ErrLockLost
	}

	return lockedUntil.Time, time.Duration(leaseSec) * time.Second, nil
}

func buildRoutingClause(prefix string, filter ClaimFilter) (string, []any) {
	var clauses []string
	var args []any

	clauses = append(clauses, prefix+"task_queue = ?")
	args = append(args, filter.Queue)

	execTypes := filter.ExecutorTypes
	if len(execTypes) == 0 {
		execTypes = []string{"http", "message", "activity"}
	}
	typePlaceholders := make([]string, len(execTypes))
	for i, t := range execTypes {
		typePlaceholders[i] = "?"
		args = append(args, t)
	}
	clauses = append(clauses, fmt.Sprintf("%stype IN (%s)", prefix, strings.Join(typePlaceholders, ",")))

	hasActivity := false
	for _, t := range execTypes {
		if t == "activity" {
			hasActivity = true
			break
		}
	}

	if hasActivity {
		if len(filter.Activities) > 0 {
			actPlaceholders := make([]string, len(filter.Activities))
			for i, a := range filter.Activities {
				actPlaceholders[i] = "?"
				args = append(args, a)
			}
			clauses = append(clauses, fmt.Sprintf("(%stype != 'activity' OR %sactivity_name IN (%s))", prefix, prefix, strings.Join(actPlaceholders, ",")))
		} else {
			clauses = append(clauses, fmt.Sprintf("%stype != 'activity'", prefix))
		}
	}

	return strings.Join(clauses, " AND "), args
}

// ClaimTask claims the next runnable step matching the worker's routing filter.
func (r *executionRepository) ClaimTask(ctx context.Context, filter ClaimFilter) (*ClaimResult, error) {
	worker := strings.TrimSpace(filter.WorkerID)
	if worker == "" {
		return nil, errdefs.ErrEmptyWorkerID
	}
	serviceID := strings.TrimSpace(filter.ServiceID)
	if serviceID == "" {
		return nil, errdefs.ErrUnauthorized
	}

	filter.Queue = strings.TrimSpace(filter.Queue)
	if err := util.ValidateQueueName(filter.Queue); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	routingClause, routingArgs := buildRoutingClause("se.", filter)

	var execID, workflowID string
	var status WorkflowStatus
	var raw sql.NullString
	wfQuery := fmt.Sprintf(`SELECT we.id, we.workflow_id, we.status, we.context
 FROM workflow_executions we WHERE we.status IN ('RUNNING','COMPENSATING')
 AND EXISTS (SELECT 1 FROM step_executions se WHERE se.workflow_execution_id=we.id
   AND %s
   AND se.status='PENDING' AND (se.next_retry_at IS NULL OR se.next_retry_at<=NOW(3))
   AND (se.locked_until IS NULL OR se.locked_until<=NOW(3))
   AND ((we.status='RUNNING' AND se.is_compensation=FALSE)
     OR (we.status='COMPENSATING' AND se.is_compensation=TRUE)))
 ORDER BY CASE WHEN we.status='COMPENSATING' THEN 0 ELSE 1 END, we.created_at, we.id
 LIMIT 1 FOR UPDATE SKIP LOCKED`, routingClause)

	err = tx.QueryRowContext(ctx, wfQuery, routingArgs...).Scan(&execID, &workflowID, &status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if status == StatusCompensating {
		if err := reconcileTx(ctx, tx, execID, status); err != nil {
			return nil, err
		}
	}

	stepRoutingClause, stepRoutingArgs := buildRoutingClause("", filter)
	var stepArgs []any
	stepArgs = append(stepArgs, execID, status == StatusCompensating)
	stepArgs = append(stepArgs, stepRoutingArgs...)

	stepQuery := fmt.Sprintf(`SELECT `+stepColumns+` FROM step_executions
 WHERE workflow_execution_id=? AND is_compensation=?
   AND %s
   AND status='PENDING'
   AND (next_retry_at IS NULL OR next_retry_at<=NOW(3)) AND (locked_until IS NULL OR locked_until<=NOW(3))
 ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, stepRoutingClause)

	s, err := scanStep(tx.QueryRowContext(ctx, stepQuery, stepArgs...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}

	id := s.ID
	var original sql.NullString
	if s.IsCompensation {
		var originalStatus StepStatus
		err = tx.QueryRowContext(ctx, `SELECT status, output FROM step_executions
			WHERE workflow_execution_id = ? AND step_id = ? AND is_compensation = FALSE`, execID, s.StepID).Scan(&originalStatus, &original)
		if err != nil {
			return nil, err
		}
		if originalStatus != StatusCompleted {
			if err := reconcileTx(ctx, tx, execID, status); err != nil {
				return nil, err
			}
			return nil, tx.Commit()
		}
	}

	var condEvalRaw []byte
	if !s.IsCompensation && len(bytes.TrimSpace(s.When)) > 0 && !bytes.Equal(bytes.TrimSpace(s.When), []byte("null")) {
		if len(bytes.TrimSpace(s.ConditionEvaluation)) > 0 {
			condEvalRaw = s.ConditionEvaluation
		} else {
			cond, err := condition.ParseCondition(s.When)
			if err != nil {
				return nil, err
			}

			rows, err := tx.QueryContext(ctx, `SELECT step_id, output FROM step_executions
				WHERE workflow_execution_id = ? AND is_compensation = FALSE AND status = 'COMPLETED'`, execID)
			if err != nil {
				return nil, err
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
					error_message = ?, updated_at = NOW(3) WHERE id = ?`, string(auditBytes), errMsg, id)
				if err != nil {
					return nil, err
				}
				_, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET error_message = ?, updated_at = NOW(3)
					WHERE id = ? AND status NOT IN ('COMPENSATION_FAILED', 'COMPLETED', 'COMPENSATED')`,
					fmt.Sprintf("step %s failed: %s", id, errMsg), execID)
				if err != nil {
					return nil, err
				}
				if err := reconcileTx(ctx, tx, execID, status); err != nil {
					return nil, err
				}
				return nil, tx.Commit()
			}

			if !pass {
				_, err = tx.ExecContext(ctx, `UPDATE step_executions SET status = 'SKIPPED', condition_evaluation = ?,
					updated_at = NOW(3) WHERE id = ?`, string(auditBytes), id)
				if err != nil {
					return nil, err
				}
				if err := reconcileTx(ctx, tx, execID, status); err != nil {
					return nil, err
				}
				return nil, tx.Commit()
			}
		}
	}

	resolved, err := resolveConfig(s.Config, json.RawMessage(raw.String), json.RawMessage(original.String), s.IsCompensation)
	if err != nil {
		return nil, err
	}
	var condEvalVal any
	if len(condEvalRaw) > 0 {
		condEvalVal = string(condEvalRaw)
	}

	leaseToken, err := util.GenerateLeaseToken()
	if err != nil {
		return nil, fmt.Errorf("generate lease token: %w", err)
	}
	leaseTokenHash := util.HashLeaseToken(leaseToken)

	timeoutSec := s.TimeoutSeconds
	if timeoutSec <= 0 && filter.DefaultTimeout > 0 {
		timeoutSec = int((filter.DefaultTimeout-1)/time.Second) + 1
	}
	if timeoutSec <= 0 {
		timeoutSec = 15
	}

	leaseSec := int(r.leaseSeconds(filter.LeaseDuration))
	if leaseSec <= 0 {
		leaseSec = 30
	}
	if leaseSec > timeoutSec {
		leaseSec = timeoutSec
	}

	nextAttempt := s.Attempt + 1

	// Insert attempt record
	_, err = tx.ExecContext(ctx, `INSERT INTO step_execution_attempts (
		step_execution_id, attempt, service_id, worker_id, lease_token_hash,
		started_at, deadline, outcome, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, NOW(3), DATE_ADD(NOW(3), INTERVAL ? SECOND), 'RUNNING', NOW(3), NOW(3))`,
		id, nextAttempt, serviceID, worker, leaseTokenHash, timeoutSec)
	if err != nil {
		return nil, fmt.Errorf("insert attempt: %w", err)
	}

	// Update step execution, strictly bounding locked_until by the saved attempt deadline
	_, err = tx.ExecContext(ctx, `UPDATE step_executions s
		JOIN step_execution_attempts a ON s.id = a.step_execution_id AND a.attempt = ?
		SET s.status = 'RUNNING', s.attempt = a.attempt,
		    s.locked_by = ?, s.locked_until = LEAST(DATE_ADD(NOW(3), INTERVAL ? SECOND), a.deadline),
		    s.config = ?, s.condition_evaluation = ?, s.executed_at = NOW(3), s.next_retry_at = NULL, s.updated_at = NOW(3)
		WHERE s.id = ?`,
		nextAttempt, worker, leaseSec, string(resolved), condEvalVal, id)
	if err != nil {
		return nil, err
	}

	var lockedUntil, deadline time.Time
	err = tx.QueryRowContext(ctx, `SELECT s.locked_until, a.deadline
		FROM step_executions s
		JOIN step_execution_attempts a ON s.id = a.step_execution_id AND s.attempt = a.attempt
		WHERE s.id = ?`, id).Scan(&lockedUntil, &deadline)
	if err != nil {
		return nil, err
	}

	s, err = scanStep(tx.QueryRowContext(ctx, `SELECT `+stepColumns+` FROM step_executions WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ClaimResult{
		StepExecution:  &s,
		LeaseToken:     leaseToken,
		LeaseDuration:  time.Duration(leaseSec) * time.Second,
		LeaseExpiresAt: lockedUntil,
		Deadline:       deadline,
	}, nil
}
