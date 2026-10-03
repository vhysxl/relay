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
