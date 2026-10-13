package util

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInterpolateContext(t *testing.T) {
	template := []byte(`{
		"method": "POST",
		"url": "http://cart-service.internal/validate/{{context.cart_id}}",
		"body": {
			"cart_id": "{{context.cart_id}}",
			"user_id": "{{user.id}}",
			"amount": "{{context.amount}}"
		}
	}`)

	ctxData := []byte(`{
		"cart_id": "CART-USER-9921",
		"user": {
			"id": "USR-881"
		},
		"amount": 150000
	}`)

	interpolated := InterpolateContext(template, ctxData)
	expectedStr := `"CART-USER-9921"`
	if !strings.Contains(string(interpolated), expectedStr) {
		t.Errorf("expected %s in result, got: %s", expectedStr, string(interpolated))
	}

	expectedURL := `http://cart-service.internal/validate/CART-USER-9921`
	if !strings.Contains(string(interpolated), expectedURL) {
		t.Errorf("expected %s in result, got: %s", expectedURL, string(interpolated))
	}

	expectedUserID := `"user_id": "USR-881"`
	if !strings.Contains(string(interpolated), expectedUserID) {
		t.Errorf("expected %s in result, got: %s", expectedUserID, string(interpolated))
	}
}

func TestInterpolateContext_Advanced(t *testing.T) {
	template := []byte(`{
		"url": "https://api.com/items/{{items.0.id}}",
		"item_name": "{{items.1.name}}",
		"full_obj": "{{nested.details}}",
		"array_val": "{{tags}}",
		"escaped_str": "{{special_str}}"
	}`)

	ctxData := []byte(`{
		"items": [
			{"id": "item-101", "name": "Alpha"},
			{"id": "item-102", "name": "Beta"}
		],
		"nested": {
			"details": {
				"active": true,
				"score": 99
			}
		},
		"tags": ["go", "distributed"],
		"special_str": "quote \"here\" and newline \n"
	}`)

	interpolated := InterpolateContext(template, ctxData)

	// 1. Array index
	if !strings.Contains(string(interpolated), "https://api.com/items/item-101") {
		t.Errorf("expected item-101 in url, got: %s", string(interpolated))
	}
	if !strings.Contains(string(interpolated), `"item_name": "Beta"`) {
		t.Errorf("expected Beta in item_name, got: %s", string(interpolated))
	}

	// 2. Object substitution without broken quotes
	if !strings.Contains(string(interpolated), `"full_obj": {"active":true,"score":99}`) {
		t.Errorf("expected embedded full_obj json, got: %s", string(interpolated))
	}

	// 3. Array substitution without broken quotes
	if !strings.Contains(string(interpolated), `"array_val": ["go","distributed"]`) {
		t.Errorf("expected embedded array_val json, got: %s", string(interpolated))
	}

	// 4. Safe string escaping
	if !strings.Contains(string(interpolated), `quote \"here\" and newline \n`) {
		t.Errorf("expected escaped quote and newline, got: %s", string(interpolated))
	}
}

func TestAreStringSlicesEqualUnordered(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{"both empty", []string{}, []string{}, true},
		{"identical order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different order", []string{"a", "b"}, []string{"b", "a"}, true},
		{"with whitespace", []string{" a ", "b"}, []string{"a", " b "}, true},
		{"different lengths", []string{"a"}, []string{"a", "b"}, false},
		{"duplicate mismatch", []string{"a", "a", "b"}, []string{"a", "b", "b"}, false},
		{"duplicate match", []string{"a", "b", "a"}, []string{"b", "a", "a"}, true},
		{"different content", []string{"a", "b"}, []string{"a", "c"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AreStringSlicesEqualUnordered(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("AreStringSlicesEqualUnordered(%v, %v) = %v; want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestAreJSONEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        json.RawMessage
		b        json.RawMessage
		expected bool
	}{
		{"both nil", nil, nil, true},
		{"both empty or null", json.RawMessage(""), json.RawMessage("null"), true},
		{"one empty one not", json.RawMessage(`{"a": 1}`), nil, false},
		{"different key order", json.RawMessage(`{"a": 1, "b": 2}`), json.RawMessage(`{"b": 2, "a": 1}`), true},
		{"different values", json.RawMessage(`{"a": 1}`), json.RawMessage(`{"a": 2}`), false},
		{"invalid json", json.RawMessage(`invalid`), json.RawMessage(`{"a": 1}`), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AreJSONEqual(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("AreJSONEqual(%s, %s) = %v; want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		defaultLimit int
		maxLimit     int
		wantLimit    int
		wantOffset   int
	}{
		{"empty query params use defaults", "/workflows", 50, 100, 50, 0},
		{"valid limit and offset", "/workflows?limit=25&offset=10", 50, 100, 25, 10},
		{"limit exceeds maxLimit is capped", "/workflows?limit=500&offset=5", 50, 100, 100, 5},
		{"negative limit falls back to default", "/workflows?limit=-10&offset=5", 50, 100, 50, 5},
		{"invalid limit string falls back to default", "/workflows?limit=abc", 50, 100, 50, 0},
		{"negative offset clamped to zero", "/workflows?limit=10&offset=-5", 50, 100, 10, 0},
		{"invalid offset string clamped to zero", "/workflows?offset=xyz", 50, 100, 50, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			pg := ParsePagination(req, tt.defaultLimit, tt.maxLimit)
			if pg.Limit != tt.wantLimit || pg.Offset != tt.wantOffset {
				t.Errorf("ParsePagination(%s) = {Limit: %d, Offset: %d}; want {Limit: %d, Offset: %d}",
					tt.url, pg.Limit, pg.Offset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

func TestNormalizePagination(t *testing.T) {
	limit, offset := NormalizePagination(0, -1, 50, 100)
	if limit != 50 || offset != 0 {
		t.Errorf("expected 50, 0; got %d, %d", limit, offset)
	}

	limit, offset = NormalizePagination(500, 20, 50, 100)
	if limit != 100 || offset != 20 {
		t.Errorf("expected 100, 20; got %d, %d", limit, offset)
	}
}

