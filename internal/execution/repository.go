package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/vhysxl/relay/pkg/errdefs"
)

type executionRepository struct{ db *sql.DB }

func NewExecutionRepository(db *sql.DB) Repository { return &executionRepository{db: db} }

func (r *executionRepository) ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error) {
	if r.db == nil {
		return []WorkflowExecution{}, nil
	}

	query := `
		SELECT e.id, e.workflow_id, w.name, e.status, e.context, e.error_message, e.created_at, e.updated_at
		FROM workflow_executions e
		LEFT JOIN workflow_definitions w ON e.workflow_id = w.id
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []WorkflowExecution
	var execIDs []string
	for rows.Next() {
		var (
			exec      WorkflowExecution
			wfName    sql.NullString
			ctxStr    sql.NullString
			statusStr string
		)
		err := rows.Scan(
			&exec.ID,
			&exec.WorkflowID,
			&wfName,
			&statusStr,
			&ctxStr,
			&exec.ErrorMessage,
			&exec.CreatedAt,
			&exec.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		exec.Status = ExecutionStatus(statusStr)
		if wfName.Valid {
			exec.WorkflowName = wfName.String
		}
		if ctxStr.Valid {
			exec.Context = json.RawMessage(ctxStr.String)
		}
		execs = append(execs, exec)
		execIDs = append(execIDs, exec.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(execs) == 0 {
		return []WorkflowExecution{}, nil
	}

	stepsMap, err := r.getStepExecutionsByWorkflowExecIDs(ctx, execIDs)
	if err != nil {
		return nil, err
	}

	for i := range execs {
		if steps, ok := stepsMap[execs[i].ID]; ok {
			execs[i].Steps = steps
		} else {
			execs[i].Steps = []StepExecution{}
		}
	}

	return execs, nil
}

func (r *executionRepository) GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error) {
	if r.db == nil {
		return nil, errdefs.ErrExecutionNotFound
	}

	query := `
		SELECT e.id, e.workflow_id, w.name, e.status, e.context, e.error_message, e.created_at, e.updated_at
		FROM workflow_executions e
		LEFT JOIN workflow_definitions w ON e.workflow_id = w.id
		WHERE e.id = ?
	`
	var (
		exec      WorkflowExecution
		wfName    sql.NullString
		ctxStr    sql.NullString
		statusStr string
	)
	err := r.db.QueryRowContext(ctx, query, executionID).Scan(
		&exec.ID,
		&exec.WorkflowID,
		&wfName,
		&statusStr,
		&ctxStr,
		&exec.ErrorMessage,
		&exec.CreatedAt,
		&exec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrExecutionNotFound
		}
		return nil, err
	}
	exec.Status = ExecutionStatus(statusStr)
	if wfName.Valid {
		exec.WorkflowName = wfName.String
	}
	if ctxStr.Valid {
		exec.Context = json.RawMessage(ctxStr.String)
	}

	steps, err := r.getStepExecutions(ctx, exec.ID)
	if err != nil {
		return nil, err
	}
	exec.Steps = steps

	return &exec, nil
}

func (r *executionRepository) getStepExecutions(ctx context.Context, workflowExecID string) ([]StepExecution, error) {
	stepsMap, err := r.getStepExecutionsByWorkflowExecIDs(ctx, []string{workflowExecID})
	if err != nil {
		return nil, err
	}
	return stepsMap[workflowExecID], nil
}

func (r *executionRepository) getStepExecutionsByWorkflowExecIDs(ctx context.Context, workflowExecIDs []string) (map[string][]StepExecution, error) {
	result := make(map[string][]StepExecution)
	if r.db == nil || len(workflowExecIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(workflowExecIDs))
	args := make([]any, len(workflowExecIDs))
	for i, id := range workflowExecIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM step_executions
		WHERE workflow_execution_id IN (%s)
		ORDER BY is_compensation ASC, created_at ASC
	`, stepColumns, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		se, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		se.Attempts = []StepExecutionAttempt{}
		result[se.WorkflowExecutionID] = append(result[se.WorkflowExecutionID], se)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := r.loadAttemptsForStepExecutions(ctx, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *executionRepository) loadAttemptsForStepExecutions(ctx context.Context, stepsMap map[string][]StepExecution) error {
	var stepExecIDs []string
	for _, steps := range stepsMap {
		for _, s := range steps {
			stepExecIDs = append(stepExecIDs, s.ID)
		}
	}
	if len(stepExecIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(stepExecIDs))
	args := make([]any, len(stepExecIDs))
	for i, id := range stepExecIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT step_execution_id, attempt, service_id, worker_id, started_at, deadline, finished_at, outcome, error_message, report_type, created_at, updated_at
		FROM step_execution_attempts
		WHERE step_execution_id IN (%s)
		ORDER BY step_execution_id, attempt ASC
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	attemptsMap := make(map[string][]StepExecutionAttempt)
	for rows.Next() {
		var a StepExecutionAttempt
		var finishedAt sql.NullTime
		var errorMsg, reportType sql.NullString
		err := rows.Scan(
			&a.StepExecutionID,
			&a.Attempt,
			&a.ServiceID,
			&a.WorkerID,
			&a.StartedAt,
			&a.Deadline,
			&finishedAt,
			&a.Outcome,
			&errorMsg,
			&reportType,
			&a.CreatedAt,
			&a.UpdatedAt,
		)
		if err != nil {
			return err
		}
		if finishedAt.Valid {
			a.FinishedAt = &finishedAt.Time
		}
		if errorMsg.Valid {
			a.ErrorMessage = &errorMsg.String
		}
		if reportType.Valid {
			a.ReportType = &reportType.String
		}
		attemptsMap[a.StepExecutionID] = append(attemptsMap[a.StepExecutionID], a)
	}

	for wfID, steps := range stepsMap {
		for i := range steps {
			if atts, ok := attemptsMap[steps[i].ID]; ok {
				steps[i].Attempts = atts
			}
		}
		stepsMap[wfID] = steps
	}

	return rows.Err()
}
