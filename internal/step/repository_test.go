package step_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/vhysxl/relay/internal/step"
)

func getStepTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "relay:relay@tcp(localhost:3306)/relay_taskqueue?parseTime=true&clientFoundRows=true"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("skipping integration test, cannot open db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skipf("skipping integration test, cannot ping db: %v", err)
	}

	return db
}

func TestStepRepository_CreateAndGetSteps(t *testing.T) {
	db := getStepTestDB(t)
	repo := step.NewStepRepository(db)
	ctx := context.Background()

	wfDefID := uuid.NewString()

	// 1. Insert workflow definition
	_, err := db.ExecContext(ctx, "INSERT INTO workflow_definitions (id, name, version, context) VALUES (?, ?, 1, '{}')", wfDefID, "test-wf-"+wfDefID[:8])
	if err != nil {
		t.Fatalf("failed to insert workflow definition: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM step_definitions WHERE workflow_id = ?", wfDefID)
		_, _ = db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE id = ?", wfDefID)
	}()

	// 2. Create steps via CreateStepsTx
	stepsToCreate := []step.Step{
		{
			ID:         "step-1",
			WorkflowID: wfDefID,
			Name:       "First Step",
			Type:       step.StepTypeHTTP,
			Config:     json.RawMessage(`{"url":"https://example.com"}`),
			Timeout:    30,
			Retry: &step.RetryPolicy{
				MaxAttempts: 3,
				Backoff:     2,
			},
		},
		{
			ID:         "step-2",
			WorkflowID: wfDefID,
			Name:       "Second Step",
			Type:       step.StepTypeHTTP,
			Config:     json.RawMessage(`{"url":"https://example.com/2"}`),
			DependsOn:  []string{"step-1"},
			Timeout:    15,
		},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	if err := repo.CreateStepsTx(ctx, tx, wfDefID, stepsToCreate); err != nil {
		tx.Rollback()
		t.Fatalf("CreateStepsTx error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit tx: %v", err)
	}

	// 3. GetStepsByWorkflowID
	fetchedSteps, err := repo.GetStepsByWorkflowID(ctx, wfDefID)
	if err != nil {
		t.Fatalf("GetStepsByWorkflowID error: %v", err)
	}
	if len(fetchedSteps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(fetchedSteps))
	}

	// 4. GetStepByID
	step1, err := repo.GetStepByID(ctx, wfDefID, "step-1")
	if err != nil {
		t.Fatalf("GetStepByID error: %v", err)
	}
	if step1.Name != "First Step" || step1.Timeout != 30 {
		t.Errorf("unexpected step1 details: %+v", step1)
	}
	if step1.Retry == nil || step1.Retry.MaxAttempts != 3 {
		t.Errorf("expected retry max_attempts 3, got %+v", step1.Retry)
	}

	// 5. GetStepsByWorkflowIDs
	stepsMap, err := repo.GetStepsByWorkflowIDs(ctx, []string{wfDefID})
	if err != nil {
		t.Fatalf("GetStepsByWorkflowIDs error: %v", err)
	}
	if len(stepsMap[wfDefID]) != 2 {
		t.Errorf("expected 2 steps in map, got %d", len(stepsMap[wfDefID]))
	}
}
