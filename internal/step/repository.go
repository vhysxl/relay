package step

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrStepNotFound = errors.New("step not found")
)

type StepRepository interface {
	CreateStepsTx(ctx context.Context, tx *sql.Tx, workflowID string, steps []Step) error
	GetStepsByWorkflowID(ctx context.Context, workflowID string) ([]Step, error)
	GetStepByID(ctx context.Context, workflowID string, stepID string) (*Step, error)
}

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
