package util

import (
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
	if !contains(string(interpolated), expectedStr) {
		t.Errorf("expected %s in result, got: %s", expectedStr, string(interpolated))
	}

	expectedURL := `http://cart-service.internal/validate/CART-USER-9921`
	if !contains(string(interpolated), expectedURL) {
		t.Errorf("expected %s in result, got: %s", expectedURL, string(interpolated))
	}

	expectedUserID := `"user_id": "USR-881"`
	if !contains(string(interpolated), expectedUserID) {
		t.Errorf("expected %s in result, got: %s", expectedUserID, string(interpolated))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
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
	if !contains(string(interpolated), "https://api.com/items/item-101") {
		t.Errorf("expected item-101 in url, got: %s", string(interpolated))
	}
	if !contains(string(interpolated), `"item_name": "Beta"`) {
		t.Errorf("expected Beta in item_name, got: %s", string(interpolated))
	}

	// 2. Object substitution without broken quotes
	if !contains(string(interpolated), `"full_obj": {"active":true,"score":99}`) {
		t.Errorf("expected embedded full_obj json, got: %s", string(interpolated))
	}

	// 3. Array substitution without broken quotes
	if !contains(string(interpolated), `"array_val": ["go","distributed"]`) {
		t.Errorf("expected embedded array_val json, got: %s", string(interpolated))
	}

	// 4. Safe string escaping
	if !contains(string(interpolated), `quote \"here\" and newline \n`) {
		t.Errorf("expected escaped quote and newline, got: %s", string(interpolated))
	}
}
