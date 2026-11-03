package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type workflowRepository struct {
	db       *sql.DB
	stepRepo step.StepRepository
}

func NewWorkflowRepository(db *sql.DB, stepRepo step.StepRepository) WorkflowRepository {
	return &workflowRepository{
		db:       db,
		stepRepo: stepRepo,
	}
}

func (r *workflowRepository) CreateWorkflow(ctx context.Context, wf *Workflow) (*Workflow, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO workflow_definitions (id, name, version, context, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	var contextString *string
	if len(wf.Context) > 0 {
		s := string(wf.Context)
		contextString = &s
	}

	_, err = tx.ExecContext(ctx, query, wf.ID, wf.Name, wf.Version, contextString, wf.CreatedAt, wf.UpdatedAt)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, errdefs.ErrWorkflowAlreadyExists
		}
		return nil, err
	}

	if len(wf.Steps) > 0 {
		if err := r.stepRepo.CreateStepsTx(ctx, tx, wf.ID, wf.Steps); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return wf, nil
}

func (r *workflowRepository) GetWorkflow(ctx context.Context, name string) (*Workflow, error) {
	query := `
		SELECT id, name, version, context, created_at, updated_at
		FROM workflow_definitions
		WHERE name = ?
		ORDER BY version DESC LIMIT 1
	`

	var (
		wf          Workflow
		contextData sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&wf.ID,
		&wf.Name,
		&wf.Version,
		&contextData,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrWorkflowNotFound
		}
		return nil, err
	}
	if contextData.Valid && len(contextData.String) > 0 {
		wf.Context = json.RawMessage(contextData.String)
	}

	if r.stepRepo != nil {
		steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, wf.ID)
		if err != nil {
			return nil, err
		}
		wf.Steps = steps
	}

	return &wf, nil
}

func (r *workflowRepository) ListWorkflows(ctx context.Context, limit, offset int) ([]WorkflowSummary, error) {
	if r.db == nil {
		return []WorkflowSummary{}, nil
	}
	// Paginate metadata first so step counts are evaluated only for this page.
	query := `
		SELECT page.id, page.name, page.version, page.created_at, page.updated_at,
		       (SELECT COUNT(*) FROM step_definitions sd WHERE sd.workflow_id = page.id) AS step_count
		FROM (
			SELECT wd.id, wd.name, wd.version, wd.created_at, wd.updated_at
			FROM workflow_definitions wd
			INNER JOIN (
				SELECT name, MAX(version) AS max_version
				FROM workflow_definitions
				GROUP BY name
			) latest ON wd.name = latest.name AND wd.version = latest.max_version
			ORDER BY wd.created_at DESC, wd.id DESC
			LIMIT ? OFFSET ?
		) page
		ORDER BY page.created_at DESC, page.id DESC
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wfs []WorkflowSummary
	for rows.Next() {
		var wf WorkflowSummary
		if err := rows.Scan(
			&wf.ID,
			&wf.Name,
			&wf.Version,
			&wf.CreatedAt,
			&wf.UpdatedAt,
			&wf.StepCount,
		); err != nil {
			return nil, err
		}
		wfs = append(wfs, wf)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return wfs, nil
}

func (r *workflowRepository) ListWorkflowVersions(ctx context.Context, name string, limit, offset int) ([]WorkflowSummary, error) {
	if r.db == nil {
		return []WorkflowSummary{}, nil
	}

	// Verify that workflow name exists
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workflow_definitions WHERE name = ?", name).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, errdefs.ErrWorkflowNotFound
	}

	query := `
		SELECT wd.id, wd.name, wd.version, wd.created_at, wd.updated_at,
		       (SELECT COUNT(*) FROM step_definitions sd WHERE sd.workflow_id = wd.id) AS step_count
		FROM workflow_definitions wd
		WHERE wd.name = ?
		ORDER BY wd.version DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, name, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wfs []WorkflowSummary
	for rows.Next() {
		var wf WorkflowSummary
		if err := rows.Scan(
			&wf.ID,
			&wf.Name,
			&wf.Version,
			&wf.CreatedAt,
			&wf.UpdatedAt,
			&wf.StepCount,
		); err != nil {
			return nil, err
		}
		wfs = append(wfs, wf)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if wfs == nil {
		wfs = []WorkflowSummary{}
	}
	return wfs, nil
}

func (r *workflowRepository) GetWorkflowVersion(ctx context.Context, name string, version int) (*Workflow, error) {
	query := `
		SELECT id, name, version, context, created_at, updated_at
		FROM workflow_definitions
		WHERE name = ? AND version = ?
	`

	var (
		wf          Workflow
		contextData sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, name, version).Scan(
		&wf.ID,
		&wf.Name,
		&wf.Version,
		&contextData,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrWorkflowNotFound
		}
		return nil, err
	}
	if contextData.Valid && len(contextData.String) > 0 {
		wf.Context = json.RawMessage(contextData.String)
	}

	if r.stepRepo != nil {
		steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, wf.ID)
		if err != nil {
			return nil, err
		}
		wf.Steps = steps
	}

	return &wf, nil
}

func (r *workflowRepository) GetPreviousWorkflowVersion(ctx context.Context, name string, version int) (*Workflow, error) {
	query := `
		SELECT id, name, version, context, created_at, updated_at
		FROM workflow_definitions
		WHERE name = ? AND version < ?
		ORDER BY version DESC LIMIT 1
	`

	var (
		wf          Workflow
		contextData sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, name, version).Scan(
		&wf.ID,
		&wf.Name,
		&wf.Version,
		&contextData,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrWorkflowNotFound
		}
		return nil, err
	}
	if contextData.Valid && len(contextData.String) > 0 {
		wf.Context = json.RawMessage(contextData.String)
	}

	if r.stepRepo != nil {
		steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, wf.ID)
		if err != nil {
			return nil, err
		}
		wf.Steps = steps
	}

	return &wf, nil
}
