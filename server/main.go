package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	urlpath "path"
	"path/filepath"
	"strings"
	"time"

	"nexgestion/server/apis"
	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

const defaultAddress = ":8080"

func main() {
	databaseDirectory := getDatabaseDirectory()
	if err := system.EnsureRequiredDatabases(context.Background(), databaseDirectory); err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		log.Fatalf("load log timezone failed: %v", err)
	}
	logService, err := applogs.NewService(getLogDirectory(), location)
	if err != nil {
		log.Fatalf("log initialization failed: %v", err)
	}
	defer logService.Close()
	if err := logService.Log("info", "server started"); err != nil {
		log.Printf("write startup log failed: %v", err)
	}

	address := getAddress()
	distDir, err := findClientDist()
	if err != nil {
		log.Fatalf("client dist not found: %v", err)
	}

	mux := http.NewServeMux()
	users := system.NewUserService(databaseDirectory)
	attendance := system.NewAttendanceService(databaseDirectory, getAttendanceReportDirectory(), users)
	notifications := system.NewNotificationService(databaseDirectory, users)
	reports := system.NewReportFileService(getReportDirectory())
	if err := reports.EnsureRoot(); err != nil {
		log.Fatalf("report directory initialization failed: %v", err)
	}
	templates := system.NewTemplateService(databaseDirectory, getTemplateDirectory(), users)
	if err := templates.EnsureRoot(); err != nil {
		log.Fatalf("template directory initialization failed: %v", err)
	}
	apis.InitRouter(mux, users, attendance, notifications, reports, templates, system.NewAuthService(users), logService)
	go runAttendanceMaintenance(attendance, logService)
	go runNotificationMaintenance(notifications, logService)
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

func runNotificationMaintenance(notifications *system.NotificationService, logService *applogs.Service) {
	run := func() {
		if err := notifications.ExpireAndCleanup(context.Background()); err != nil {
			_ = logService.Log("error", "notification maintenance failed: "+err.Error())
		}
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
}

func getAttendanceReportDirectory() string {
	directory := strings.TrimSpace(os.Getenv("ATTENDANCE_REPORT_DIR"))
	if directory == "" {
		return filepath.Join("report", "attendance")
	}
	return directory
}

func getReportDirectory() string {
	directory := strings.TrimSpace(os.Getenv("REPORT_DIR"))
	if directory == "" {
		return "report"
	}
	return directory
}

func getTemplateDirectory() string {
	directory := strings.TrimSpace(os.Getenv("TEMPLATE_DIR"))
	if directory == "" {
		return "template"
	}
	return directory
}

func runAttendanceMaintenance(attendance *system.AttendanceService, logService *applogs.Service) {
	run := func() {
		if err := attendance.RunMaintenance(context.Background()); err != nil {
			_ = logService.Log("error", "attendance maintenance failed: "+err.Error())
		}
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
}

func getLogDirectory() string {
	directory := strings.TrimSpace(os.Getenv("LOG_DIR"))
	if directory == "" {
		workingDirectory, err := os.Getwd()
		if err == nil && filepath.Base(workingDirectory) == "server" {
			return filepath.Join("..", "log")
		}
		return "log"
	}
	return directory
}

func getDatabaseDirectory() string {
	directory := strings.TrimSpace(os.Getenv("DATABASE_DIR"))
	if directory == "" {
		return "database"
	}

	return directory
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

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
