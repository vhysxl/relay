package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterRoutes_Dashboard(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil)

	// Test GET /
	reqRoot := httptest.NewRequest(http.MethodGet, "/", nil)
	recRoot := httptest.NewRecorder()
	mux.ServeHTTP(recRoot, reqRoot)

	if recRoot.Code != http.StatusOK {
		t.Fatalf("expected status 200 for GET /, got %d", recRoot.Code)
	}

	contentType := recRoot.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected Content-Type text/html, got %s", contentType)
	}

	body := recRoot.Body.String()
	if !strings.Contains(body, "Relay • Workflows Orchestration Engine") {
		t.Errorf("expected body to contain title, got: %s", body[:200])
	}
	if !strings.Contains(body, "view-execution-detail") || !strings.Contains(body, "renderExecutionDetail") {
		t.Errorf("expected body to contain execution detail view and script")
	}

	// Test GET /dashboard
	reqDash := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	recDash := httptest.NewRecorder()
	mux.ServeHTTP(recDash, reqDash)

	if recDash.Code != http.StatusOK {
		t.Fatalf("expected status 200 for GET /dashboard, got %d", recDash.Code)
	}

	if !strings.Contains(recDash.Body.String(), ">Workflows<") && !strings.Contains(recDash.Body.String(), "Workflows") {
		t.Errorf("expected body to contain dashboard header")
	}

	// Test GET /history
	reqHist := httptest.NewRequest(http.MethodGet, "/history", nil)
	recHist := httptest.NewRecorder()
	mux.ServeHTTP(recHist, reqHist)

	if recHist.Code != http.StatusOK {
		t.Fatalf("expected status 200 for GET /history, got %d", recHist.Code)
	}

	// Test GET /health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	mux.ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected status 200 for GET /health, got %d", recHealth.Code)
	}
}
