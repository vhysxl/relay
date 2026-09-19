package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

func GenerateUUID() string {
	return uuid.New().String()
}

func GenerateWorkflowHash(name string, version int, context []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%d:%s", name, version, string(context))
	return hex.EncodeToString(h.Sum(nil))
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
