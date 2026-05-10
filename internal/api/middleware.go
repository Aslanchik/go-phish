package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// safeError logs the full error server-side and writes a generic 500 to the client.
// Stack traces and DSN strings never reach the response body.
func safeError(w http.ResponseWriter, err error) {
	log.Printf("api error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

// clientError writes a 4xx response with a human-readable message.
func clientError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
