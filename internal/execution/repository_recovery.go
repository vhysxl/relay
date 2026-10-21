package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (r *executionRepository) ReapExpiredSteps(ctx context.Context) (int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT workflow_execution_id FROM step_executions
		WHERE status = 'RUNNING' AND locked_until <= NOW() ORDER BY workflow_execution_id`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	var count int64
	var failures []error
	for _, id := range ids {
		n, err := r.reapWorkflow(ctx, id)
		if err != nil {
			failures = append(failures, fmt.Errorf("recover workflow %s: %w", id, err))
		} else {
			count += n
		}
		if ctx.Err() != nil {
			break
		}
	}
	return count, errors.Join(failures...)
}

func (r *executionRepository) reapWorkflow(ctx context.Context, id string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	status, _, err := lockWorkflow(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	steps, err := loadStepsTx(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT NOW()`).Scan(&now); err != nil {
		return 0, err
	}
	var count int64
	for _, s := range steps {
		if s.Status != StatusRunning || s.LockedUntil == nil || s.LockedUntil.After(now) {
			continue
		}
		message := "lease expired: max attempts reached"
		if mayRetry(status, s) {
			message = "lease expired: retry scheduled"
		}
		if err := recordFailureTx(ctx, tx, s, status, message); err != nil {
			return 0, err
		}
		count++
	}
	if err := reconcileTx(ctx, tx, id, status); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}
