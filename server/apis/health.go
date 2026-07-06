package apis

import (
	"encoding/json"
	"log"
	"net/http"
)

// Health reports whether the HTTP service is running.
func Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// NotFound handles API paths that have not been registered in InitRouter.
func NotFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{
		"error": "API endpoint not found",
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write API response failed: %v", err)
	}
}
