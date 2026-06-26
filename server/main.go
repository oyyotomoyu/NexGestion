package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	urlpath "path"
	"path/filepath"
	"strings"
	"time"
)

const defaultAddress = ":8080"

func main() {
	address := getAddress()
	distDir, err := findClientDist()
	if err != nil {
		log.Fatalf("client dist not found: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", healthHandler)
	mux.Handle("/", spaHandler(distDir))

	server := &http.Server{
		Addr:              address,
		Handler:           logRequest(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("NexGestion server listening on http://localhost%s", address)
	log.Printf("Serving UI from %s", distDir)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func getAddress() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		return defaultAddress
	}

	if strings.HasPrefix(port, ":") {
		return port
	}

	return ":" + port
}

func findClientDist() (string, error) {
	candidates := []string{
		filepath.Join("client", "dist"),
		filepath.Join("..", "client", "dist"),
	}

	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}

		info, err := os.Stat(filepath.Join(absPath, "index.html"))
		if err == nil && !info.IsDir() {
			return absPath, nil
		}
	}

	return "", errors.New("expected client/dist/index.html; run `pnpm build` in client first")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func spaHandler(distDir string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(distDir))

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cleanPath := strings.TrimPrefix(urlpath.Clean(r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			cleanPath = "index.html"
		}

		if strings.HasPrefix(cleanPath, "../") || cleanPath == ".." {
			http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
			return
		}

		filePath := filepath.Join(distDir, filepath.FromSlash(cleanPath))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json failed: %v", err)
	}
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
