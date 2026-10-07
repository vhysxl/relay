package step

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	ErrStepNotFound = errors.New("step not found")
)

type stepRepository struct {
	db *sql.DB
}

func NewStepRepository(db *sql.DB) StepRepository {
	return &stepRepository{db: db}
}

func (r *stepRepository) CreateStepsTx(ctx context.Context, tx *sql.Tx, workflowID string, steps []Step) error {
	query := `
		INSERT INTO step_definitions (
			id, workflow_id, name, type, config, depends_on,
			timeout, retry, compensation, irreversible, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, s := range steps {
		stepCreatedAt := s.CreatedAt
		if stepCreatedAt.IsZero() {
			stepCreatedAt = now
		}
		stepUpdatedAt := s.UpdatedAt
		if stepUpdatedAt.IsZero() {
			stepUpdatedAt = now
		}

		configVal := string(s.Config)

		var dependsOnVal any
		if len(s.DependsOn) > 0 {
			b, err := json.Marshal(s.DependsOn)
			if err != nil {
				return err
			}
			dependsOnVal = string(b)
		}

		var retryVal any
		if s.Retry != nil {
			b, err := json.Marshal(s.Retry)
			if err != nil {
				return err
			}
			retryVal = string(b)
		}

		var compVal any
		if s.Compensation != nil {
			b, err := json.Marshal(s.Compensation)
			if err != nil {
				return err
			}
			compVal = string(b)
		}

		_, err = stmt.ExecContext(
			ctx,
			s.ID,
			workflowID,
			s.Name,
			string(s.Type),
			configVal,
			dependsOnVal,
			s.Timeout,
			retryVal,
			compVal,
			s.Irreversible,
			stepCreatedAt,
			stepUpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *stepRepository) GetStepsByWorkflowID(ctx context.Context, workflowID string) ([]Step, error) {
	if r.db == nil {
		return []Step{}, nil
	}
	query := `
		SELECT id, workflow_id, name, type, config, depends_on,
		       timeout, retry, compensation, irreversible, created_at, updated_at
		FROM step_definitions
		WHERE workflow_id = ?
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []Step
	for rows.Next() {
		var (
			s            Step
			typeStr      string
			configStr    string
			dependsOnStr sql.NullString
			retryStr     sql.NullString
			compStr      sql.NullString
		)

		err := rows.Scan(
			&s.ID,
			&s.WorkflowID,
			&s.Name,
			&typeStr,
			&configStr,
			&dependsOnStr,
			&s.Timeout,
			&retryStr,
			&compStr,
			&s.Irreversible,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		s.Type = StepType(typeStr)
		if len(configStr) > 0 {
			s.Config = json.RawMessage(configStr)
		}

		if dependsOnStr.Valid && len(dependsOnStr.String) > 0 {
			var deps []string
			if err := json.Unmarshal([]byte(dependsOnStr.String), &deps); err == nil {
				s.DependsOn = deps
			}
		}

		if retryStr.Valid && len(retryStr.String) > 0 {
			var rp RetryPolicy
			if err := json.Unmarshal([]byte(retryStr.String), &rp); err == nil {
				s.Retry = &rp
			}
		}

		if compStr.Valid && len(compStr.String) > 0 {
			var comp Compensation
			if err := json.Unmarshal([]byte(compStr.String), &comp); err == nil {
				s.Compensation = &comp
			}
		}

		steps = append(steps, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return steps, nil
}

func (r *stepRepository) GetStepByID(ctx context.Context, workflowID string, stepID string) (*Step, error) {
	query := `
		SELECT id, workflow_id, name, type, config, depends_on,
		       timeout, retry, compensation, irreversible, created_at, updated_at
		FROM step_definitions
		WHERE workflow_id = ? AND id = ?
	`

	var (
		s            Step
		typeStr      string
		configStr    string
		dependsOnStr sql.NullString
		retryStr     sql.NullString
		compStr      sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, workflowID, stepID).Scan(
		&s.ID,
		&s.WorkflowID,
		&s.Name,
		&typeStr,
		&configStr,
		&dependsOnStr,
		&s.Timeout,
		&retryStr,
		&compStr,
		&s.Irreversible,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrStepNotFound
		}
		return nil, err
	}

	s.Type = StepType(typeStr)
	if len(configStr) > 0 {
		s.Config = json.RawMessage(configStr)
	}

	if dependsOnStr.Valid && len(dependsOnStr.String) > 0 {
		var deps []string
		if err := json.Unmarshal([]byte(dependsOnStr.String), &deps); err == nil {
			s.DependsOn = deps
		}
	}

	if retryStr.Valid && len(retryStr.String) > 0 {
		var rp RetryPolicy
		if err := json.Unmarshal([]byte(retryStr.String), &rp); err == nil {
			s.Retry = &rp
		}
	}

	if compStr.Valid && len(compStr.String) > 0 {
		var comp Compensation
		if err := json.Unmarshal([]byte(compStr.String), &comp); err == nil {
			s.Compensation = &comp
		}
	}

	return &s, nil
}

// ClaimNext atomically selects, locks, and returns the next eligible step and execution ticket
func (r *stepRepository) ClaimNext(ctx context.Context, workerID string, lockDuration time.Duration) (*Step, *StepExecution, error) {
	// default to 30s lock lease if unspecified
	if lockDuration <= 0 {
		lockDuration = DefaultLockDuration
	}

	// begin transaction with read committed isolation
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// select candidate step with row lock and skip locked
	selectQuery := `
		SELECT 
			se.id, se.workflow_execution_id, se.step_id, se.name, se.type, se.config, se.depends_on,
			se.status, se.output, se.error_message, se.attempt, se.max_attempts,
			se.timeout_seconds, se.backoff_seconds, se.multiplier, se.max_backoff_seconds,
			se.locked_by, se.locked_until, se.is_compensation, se.executed_at, se.next_retry_at,
			se.created_at, se.updated_at,
			sd.id, sd.workflow_id, sd.name, sd.type, sd.config, sd.depends_on,
			sd.timeout, sd.retry, sd.compensation, sd.irreversible, sd.created_at, sd.updated_at
		FROM step_executions se
		JOIN workflow_executions we ON se.workflow_execution_id = we.id
		LEFT JOIN step_definitions sd ON sd.workflow_id = we.workflow_id AND sd.id = se.step_id
		LEFT JOIN step_executions orig 
		    ON orig.workflow_execution_id = se.workflow_execution_id 
		   AND orig.step_id = se.step_id 
		   AND orig.is_compensation = FALSE
		WHERE (
		      (se.status = 'PENDING' AND (se.locked_until IS NULL OR se.locked_until < NOW()))
		      OR (se.status = 'RUNNING' AND se.locked_until < NOW() AND se.attempt < se.max_attempts)
		  )
		  AND (se.next_retry_at IS NULL OR se.next_retry_at <= NOW())
		  AND (
		      (
		          we.status = 'COMPENSATING' 
		          AND se.is_compensation = TRUE
		          AND NOT EXISTS (
		              SELECT 1
		              FROM step_executions dep
		              WHERE dep.workflow_execution_id = se.workflow_execution_id
		                AND dep.is_compensation = TRUE
		                AND JSON_CONTAINS(se.depends_on, JSON_QUOTE(dep.step_id))
		                AND dep.status NOT IN ('COMPENSATED', 'CANCELLED')
		          )
		      )
		      OR (
		          we.status = 'RUNNING'
		          AND se.is_compensation = FALSE
		          AND NOT EXISTS (
		              SELECT 1
		              FROM step_executions dep
		              WHERE dep.workflow_execution_id = se.workflow_execution_id
		                AND dep.is_compensation = FALSE
		                AND JSON_CONTAINS(se.depends_on, JSON_QUOTE(dep.step_id))
		                AND dep.status != 'COMPLETED'
		          )
		      )
		  )
		ORDER BY 
		    CASE WHEN we.status = 'COMPENSATING' THEN 1 ELSE 2 END ASC,
		    we.created_at ASC,
		    CASE WHEN we.status = 'COMPENSATING' THEN COALESCE(orig.updated_at, se.created_at) END DESC,
		    se.created_at ASC
		LIMIT 1
		FOR UPDATE OF se SKIP LOCKED
	`

	var (
		se             StepExecution
		seName         sql.NullString
		seConfigStr    string
		seDependsOnStr sql.NullString
		seOutputStr    sql.NullString
		seErrorMsg     sql.NullString
		seLockedBy     sql.NullString
		seLockedUntil  sql.NullTime
		seExecutedAt   sql.NullTime
		seNextRetryAt  sql.NullTime

		sdID           sql.NullString
		sdWorkflowID   sql.NullString
		sdName         sql.NullString
		sdType         sql.NullString
		sdConfigStr    sql.NullString
		sdDependsOnStr sql.NullString
		sdTimeout      sql.NullInt64
		sdRetryStr     sql.NullString
		sdCompStr      sql.NullString
		sdIrreversible sql.NullBool
		sdCreatedAt    sql.NullTime
		sdUpdatedAt    sql.NullTime
	)

	// execute candidate query and scan columns
	row := tx.QueryRowContext(ctx, selectQuery)
	err = row.Scan(
		&se.ID,
		&se.WorkflowExecutionID,
		&se.StepID,
		&seName,
		&se.Type,
		&seConfigStr,
		&seDependsOnStr,
		&se.Status,
		&seOutputStr,
		&seErrorMsg,
		&se.Attempt,
		&se.MaxAttempts,
		&se.TimeoutSeconds,
		&se.BackoffSeconds,
		&se.Multiplier,
		&se.MaxBackoffSeconds,
		&seLockedBy,
		&seLockedUntil,
		&se.IsCompensation,
		&seExecutedAt,
		&seNextRetryAt,
		&se.CreatedAt,
		&se.UpdatedAt,
		&sdID,
		&sdWorkflowID,
		&sdName,
		&sdType,
		&sdConfigStr,
		&sdDependsOnStr,
		&sdTimeout,
		&sdRetryStr,
		&sdCompStr,
		&sdIrreversible,
		&sdCreatedAt,
		&sdUpdatedAt,
	)
	if err != nil {
		// return nil if no pending step is ready
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to scan candidate step: %w", err)
	}

	// update claimed row with worker lease and increment attempt
	updateQuery := `
		UPDATE step_executions
		SET locked_by = ?,
		    locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND),
		    status = 'RUNNING',
		    attempt = attempt + 1,
		    executed_at = NOW(),
		    updated_at = NOW()
		WHERE id = ?
	`
	lockSeconds := int(lockDuration.Seconds())
	_, err = tx.ExecContext(ctx, updateQuery, workerID, lockSeconds, se.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to update locked step execution: %w", err)
	}

	// commit transaction to finalize lock lease
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit claim transaction: %w", err)
	}

	// populate updated fields in step execution
	se.LockedBy = &workerID
	lockedUntilTime := time.Now().UTC().Add(lockDuration)
	se.LockedUntil = &lockedUntilTime
	se.Status = StatusRunning
	se.Attempt++
	now := time.Now().UTC()
	se.ExecutedAt = &now
	se.UpdatedAt = now

	// hydrate nullable step execution fields
	if seName.Valid {
		se.Name = seName.String
	}
	if len(seConfigStr) > 0 {
		se.Config = json.RawMessage(seConfigStr)
	}
	if seDependsOnStr.Valid && len(seDependsOnStr.String) > 0 {
		var deps []string
		if err := json.Unmarshal([]byte(seDependsOnStr.String), &deps); err == nil {
			se.DependsOn = deps
		}
	}
	if seOutputStr.Valid && len(seOutputStr.String) > 0 {
		se.Output = json.RawMessage(seOutputStr.String)
	}
	if seErrorMsg.Valid {
		se.ErrorMessage = &seErrorMsg.String
	}
	if seNextRetryAt.Valid {
		se.NextRetryAt = &seNextRetryAt.Time
	}

	// construct blueprint step model
	s := Step{
		ID:         se.StepID,
		WorkflowID: se.WorkflowExecutionID,
		Name:       se.Name,
		Type:       StepType(se.Type),
		Config:     se.Config,
		DependsOn:  se.DependsOn,
		Timeout:    se.TimeoutSeconds,
	}

	// overlay definition blueprint fields if present
	if sdID.Valid {
		s.ID = sdID.String
		if sdWorkflowID.Valid {
			s.WorkflowID = sdWorkflowID.String
		}
		if sdName.Valid {
			s.Name = sdName.String
		}
		if sdType.Valid {
			s.Type = StepType(sdType.String)
		}
		if sdConfigStr.Valid && len(sdConfigStr.String) > 0 {
			s.Config = json.RawMessage(sdConfigStr.String)
		}
		if sdDependsOnStr.Valid && len(sdDependsOnStr.String) > 0 {
			var deps []string
			if err := json.Unmarshal([]byte(sdDependsOnStr.String), &deps); err == nil {
				s.DependsOn = deps
			}
		}
		if sdTimeout.Valid {
			s.Timeout = int(sdTimeout.Int64)
		}
		if sdRetryStr.Valid && len(sdRetryStr.String) > 0 {
			var rp RetryPolicy
			if err := json.Unmarshal([]byte(sdRetryStr.String), &rp); err == nil {
				s.Retry = &rp
			}
		}
		if sdCompStr.Valid && len(sdCompStr.String) > 0 {
			var comp Compensation
			if err := json.Unmarshal([]byte(sdCompStr.String), &comp); err == nil {
				s.Compensation = &comp
			}
		}
		if sdIrreversible.Valid {
			s.Irreversible = sdIrreversible.Bool
		}
		if sdCreatedAt.Valid {
			s.CreatedAt = sdCreatedAt.Time
		}
		if sdUpdatedAt.Valid {
			s.UpdatedAt = sdUpdatedAt.Time
		}
	}

	return &s, &se, nil
}

// Heartbeat extends the lock lease (locked_until) for a step execution currently running and owned by workerID
func (r *stepRepository) Heartbeat(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
	if lockDuration <= 0 {
		lockDuration = DefaultLockDuration
	}

	query := `
		UPDATE step_executions
		SET locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND),
		    updated_at = NOW()
		WHERE id = ?
		  AND locked_by = ?
		  AND status = 'RUNNING'
	`
	lockSeconds := int(lockDuration.Seconds())
	res, err := r.db.ExecContext(ctx, query, lockSeconds, stepExecutionID, workerID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows for heartbeat: %w", err)
	}

	if rows == 0 {
		// In MySQL, if an UPDATE is executed within the same second, columns may not change value,
		// causing RowsAffected() to return 0 unless clientFoundRows=true is set in the DSN.
		// Verify if the row exists and is currently owned by this worker in RUNNING state.
		var exists bool
		checkQuery := `
			SELECT EXISTS(
				SELECT 1 FROM step_executions
				WHERE id = ? AND locked_by = ? AND status = 'RUNNING'
			)
		`
		if err := r.db.QueryRowContext(ctx, checkQuery, stepExecutionID, workerID).Scan(&exists); err != nil {
			return fmt.Errorf("failed to verify lock ownership: %w", err)
		}
		if !exists {
			return ErrLockLost
		}
		return nil
	}

	return nil
}

// CompleteStep marks a running step execution as COMPLETED (or COMPENSATED), records the output,
// releases the worker lock lease, and checks if the entire workflow execution has finished.
func (r *stepRepository) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch and lock step execution to verify ownership and get workflow_execution_id & is_compensation
	var (
		workflowExecID string
		isCompensation bool
	)
	query := `
		SELECT workflow_execution_id, is_compensation
		FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING'
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, query, stepExecutionID, workerID).Scan(&workflowExecID, &isCompensation)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLockLost
		}
		return fmt.Errorf("failed to lock step execution for completion: %w", err)
	}

	// 2. Format output
	var outBytes []byte
	if len(output) > 0 && string(output) != "null" {
		outBytes = output
	} else {
		outBytes = []byte("{}")
	}

	// 3. Update step execution status
	targetStatus := StatusCompleted
	if isCompensation {
		targetStatus = StatusCompensated
	}

	updateQuery := `
		UPDATE step_executions
		SET status = ?,
		    output = ?,
		    locked_by = NULL,
		    locked_until = NULL,
		    updated_at = NOW()
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, updateQuery, targetStatus, outBytes, stepExecutionID); err != nil {
		return fmt.Errorf("failed to update step execution to %s: %w", targetStatus, err)
	}

	// 4. Check if workflow execution is complete
	if !isCompensation {
		// Check if there are any remaining non-compensation steps not COMPLETED
		var remainingNonComp int
		remQuery := `
			SELECT COUNT(*)
			FROM step_executions
			WHERE workflow_execution_id = ?
			  AND is_compensation = FALSE
			  AND status != 'COMPLETED'
		`
		if err := tx.QueryRowContext(ctx, remQuery, workflowExecID).Scan(&remainingNonComp); err != nil {
			return fmt.Errorf("failed to check remaining steps: %w", err)
		}

		if remainingNonComp == 0 {
			// All regular steps are completed! Update workflow to COMPLETED if it is currently RUNNING
			updateWfQuery := `
				UPDATE workflow_executions
				SET status = 'COMPLETED',
				    updated_at = NOW()
				WHERE id = ? AND status = 'RUNNING'
			`
			res, err := tx.ExecContext(ctx, updateWfQuery, workflowExecID)
			if err != nil {
				return fmt.Errorf("failed to mark workflow completed: %w", err)
			}
			rowsAffected, _ := res.RowsAffected()

			if rowsAffected > 0 {
				// Clean up / cancel unused compensation steps since workflow completed successfully
				cancelUnusedCompQuery := `
					UPDATE step_executions
					SET status = 'CANCELLED',
					    updated_at = NOW()
					WHERE workflow_execution_id = ?
					  AND is_compensation = TRUE
					  AND status = 'PENDING'
				`
				if _, err := tx.ExecContext(ctx, cancelUnusedCompQuery, workflowExecID); err != nil {
					return fmt.Errorf("failed to cancel unused compensation steps: %w", err)
				}
			}
		}
	} else {
		// Compensation step completed. Check if all active compensation steps and running normal steps are done
		var remainingWork int
		remCompQuery := `
			SELECT COUNT(*)
			FROM step_executions
			WHERE workflow_execution_id = ?
			  AND (
			      (is_compensation = TRUE AND status NOT IN ('COMPENSATED', 'CANCELLED'))
			      OR (is_compensation = FALSE AND status = 'RUNNING')
			  )
		`
		if err := tx.QueryRowContext(ctx, remCompQuery, workflowExecID).Scan(&remainingWork); err != nil {
			return fmt.Errorf("failed to check remaining compensation steps: %w", err)
		}

		if remainingWork == 0 {
			// All compensation steps are done! Mark workflow as COMPENSATED
			updateWfQuery := `
				UPDATE workflow_executions
				SET status = 'COMPENSATED',
				    updated_at = NOW()
				WHERE id = ? AND status = 'COMPENSATING'
			`
			if _, err := tx.ExecContext(ctx, updateWfQuery, workflowExecID); err != nil {
				return fmt.Errorf("failed to mark workflow compensated: %w", err)
			}
		}
	}

	return tx.Commit()
}

// FailStep handles a step execution failure. If attempts remain under max_attempts,
// it schedules a retry with exponential backoff and returns status to PENDING.
// If retries are exhausted, it marks the step as FAILED (or COMPENSATION_FAILED),
// cancels remaining pending steps, and transitions the workflow to COMPENSATING (if compensations exist) or FAILED.
func (r *stepRepository) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch and lock step execution to verify ownership and get retry & workflow configs
	var (
		workflowExecID    string
		attempt           int
		maxAttempts       int
		backoffSeconds    int
		multiplier        float64
		maxBackoffSeconds int
		isCompensation    bool
	)
	query := `
		SELECT workflow_execution_id, attempt, max_attempts, backoff_seconds, multiplier, max_backoff_seconds, is_compensation
		FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING'
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, query, stepExecutionID, workerID).Scan(
		&workflowExecID,
		&attempt,
		&maxAttempts,
		&backoffSeconds,
		&multiplier,
		&maxBackoffSeconds,
		&isCompensation,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLockLost
		}
		return fmt.Errorf("failed to lock step execution for failure: %w", err)
	}

	// 2. Determine if we should retry
	if attempt < maxAttempts {
		// Calculate backoff
		// attempt was already incremented to 1 on first claim.
		power := float64(attempt - 1)
		if power < 0 {
			power = 0
		}
		if multiplier <= 0 {
			multiplier = 1.0
		}
		delay := float64(backoffSeconds) * math.Pow(multiplier, power)
		if maxBackoffSeconds > 0 && delay > float64(maxBackoffSeconds) {
			delay = float64(maxBackoffSeconds)
		}
		if delay < 0 {
			delay = 0
		}
		delaySeconds := int(math.Round(delay))

		retryQuery := `
			UPDATE step_executions
			SET status = 'PENDING',
			    error_message = ?,
			    next_retry_at = DATE_ADD(NOW(), INTERVAL ? SECOND),
			    locked_by = NULL,
			    locked_until = NULL,
			    updated_at = NOW()
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, retryQuery, errMsg, delaySeconds, stepExecutionID); err != nil {
			return fmt.Errorf("failed to schedule step retry: %w", err)
		}

		return tx.Commit()
	}

	// 3. Retries exhausted -> Permanently failed
	if isCompensation {
		failCompQuery := `
			UPDATE step_executions
			SET status = 'COMPENSATION_FAILED',
			    error_message = ?,
			    locked_by = NULL,
			    locked_until = NULL,
			    updated_at = NOW()
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, failCompQuery, errMsg, stepExecutionID); err != nil {
			return fmt.Errorf("failed to mark compensation step as COMPENSATION_FAILED: %w", err)
		}

		failWfQuery := `
			UPDATE workflow_executions
			SET status = 'COMPENSATION_FAILED',
			    error_message = ?,
			    updated_at = NOW()
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, failWfQuery, fmt.Sprintf("compensation step %s failed: %s", stepExecutionID, errMsg), workflowExecID); err != nil {
			return fmt.Errorf("failed to mark workflow as COMPENSATION_FAILED: %w", err)
		}

		return tx.Commit()
	}

	// Regular step permanently failed
	failStepQuery := `
		UPDATE step_executions
		SET status = 'FAILED',
		    error_message = ?,
		    locked_by = NULL,
		    locked_until = NULL,
		    updated_at = NOW()
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, failStepQuery, errMsg, stepExecutionID); err != nil {
		return fmt.Errorf("failed to mark step execution as FAILED: %w", err)
	}

	// Cancel any remaining PENDING normal steps for this workflow execution
	cancelPendingQuery := `
		UPDATE step_executions
		SET status = 'CANCELLED',
		    updated_at = NOW()
		WHERE workflow_execution_id = ?
		  AND is_compensation = FALSE
		  AND status = 'PENDING'
	`
	if _, err := tx.ExecContext(ctx, cancelPendingQuery, workflowExecID); err != nil {
		return fmt.Errorf("failed to cancel pending steps: %w", err)
	}

	// Cancel any PENDING compensation steps whose original step has failed or was cancelled
	cancelUnneededCompQuery := `
		UPDATE step_executions comp
		JOIN step_executions normal 
		    ON normal.workflow_execution_id = comp.workflow_execution_id
		   AND normal.step_id = comp.step_id
		   AND normal.is_compensation = FALSE
		SET comp.status = 'CANCELLED',
		    comp.updated_at = NOW()
		WHERE comp.workflow_execution_id = ?
		  AND comp.is_compensation = TRUE
		  AND comp.status = 'PENDING'
		  AND normal.status IN ('FAILED', 'CANCELLED')
	`
	if _, err := tx.ExecContext(ctx, cancelUnneededCompQuery, workflowExecID); err != nil {
		return fmt.Errorf("failed to cancel unneeded compensation steps: %w", err)
	}

	// Check if this workflow has any active compensation steps remaining to execute
	var hasCompensation bool
	checkCompQuery := `
		SELECT EXISTS(
			SELECT 1 FROM step_executions
			WHERE workflow_execution_id = ? 
			  AND is_compensation = TRUE
			  AND status = 'PENDING'
		)
	`
	if err := tx.QueryRowContext(ctx, checkCompQuery, workflowExecID).Scan(&hasCompensation); err != nil {
		return fmt.Errorf("failed to check for compensation steps: %w", err)
	}

	if hasCompensation {
		updateWfQuery := `
			UPDATE workflow_executions
			SET status = 'COMPENSATING',
			    error_message = ?,
			    updated_at = NOW()
			WHERE id = ? AND status = 'RUNNING'
		`
		if _, err := tx.ExecContext(ctx, updateWfQuery, fmt.Sprintf("step %s failed: %s", stepExecutionID, errMsg), workflowExecID); err != nil {
			return fmt.Errorf("failed to transition workflow to COMPENSATING: %w", err)
		}
	} else {
		updateWfQuery := `
			UPDATE workflow_executions
			SET status = 'FAILED',
			    error_message = ?,
			    updated_at = NOW()
			WHERE id = ? AND status = 'RUNNING'
		`
		if _, err := tx.ExecContext(ctx, updateWfQuery, fmt.Sprintf("step %s failed: %s", stepExecutionID, errMsg), workflowExecID); err != nil {
			return fmt.Errorf("failed to mark workflow as FAILED: %w", err)
		}
	}

	return tx.Commit()
}


