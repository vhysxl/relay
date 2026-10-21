package workflow_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

func TestWorkflowRepository_ListWorkflows_MySQL(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "relay:relay@tcp(localhost:3306)/relay_taskqueue?parseTime=true&clientFoundRows=true"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		t.Skipf("skipping integration test, cannot ping db: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo := workflow.NewWorkflowRepository(db, step.NewStepRepository(db))
	name := "test-list-" + uuid.NewString()
	base := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	largeContext := json.RawMessage(`{"payload":"` + strings.Repeat("x", 1024*1024) + `"}`)
	largeConfig := json.RawMessage(`{"url":"https://example.test","body":"` + strings.Repeat("x", 512*1024) + `"}`)
	definitions := []workflow.Workflow{
		{ID: uuid.NewString(), Name: name, Version: 1, CreatedAt: base, UpdatedAt: base},
		{ID: uuid.NewString(), Name: name, Version: 2, Context: largeContext, CreatedAt: base.Add(time.Second), UpdatedAt: base},
		{ID: uuid.NewString(), Name: name + "-empty", Version: 1, CreatedAt: base.Add(2 * time.Second), UpdatedAt: base},
	}
	definitions[0].Steps = []step.Step{{ID: "old-step", Type: step.StepTypeHTTP, Config: largeConfig}}
	for i := range 3 {
		definitions[1].Steps = append(definitions[1].Steps, step.Step{ID: fmt.Sprintf("step-%d", i), Type: step.StepTypeHTTP, Config: largeConfig})
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, wf := range definitions {
			if _, err := db.ExecContext(cleanupCtx, "DELETE FROM workflow_definitions WHERE id = ?", wf.ID); err != nil {
				t.Errorf("clean up workflow fixture: %v", err)
			}
		}
	})
	for i := range definitions {
		if _, err := repo.CreateWorkflow(ctx, &definitions[i]); err != nil {
			t.Fatal(err)
		}
	}
	items, err := repo.ListWorkflows(ctx, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundLatest, foundEmpty := false, false
	for index, item := range items {
		if item.ID == definitions[0].ID {
			t.Fatal("list returned an outdated workflow version")
		}
		if item.ID == definitions[1].ID {
			foundLatest = true
			if item.Version != 2 || item.StepCount != 3 {
				t.Fatalf("latest version must count only its own steps: %+v", item)
			}
			page, err := repo.ListWorkflows(ctx, 1, index)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) != 1 || page[0].ID != item.ID || page[0].StepCount != 3 {
				t.Fatalf("unexpected paginated summary: %+v", page)
			}
		}
		if item.ID == definitions[2].ID {
			foundEmpty = true
			if item.StepCount != 0 {
				t.Fatalf("workflow without steps must have zero step_count: %+v", item)
			}
		}
	}
	if !foundLatest || !foundEmpty {
		t.Fatal("fixture workflows missing from latest workflow summaries")
	}
	detail, err := repo.GetWorkflow(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != definitions[1].ID || len(detail.Steps) != 3 || len(detail.Context) < 1024*1024 || len(detail.Steps[0].Config) < 512*1024 {
		t.Fatal("detail must retain the full latest workflow context and step configs")
	}
	response, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(response), "payload") || strings.Contains(string(response), "config") {
		t.Fatal("list response must omit definition payloads")
	}
}
