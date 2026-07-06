package logs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndFilterLogs(t *testing.T) {
	service, err := NewService(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	logger := service.With("192.0.2.10", "user-1")
	if err := logger.Log("INFO", "created user"); err != nil {
		t.Fatal(err)
	}
	if err := logger.Log("warning", "login failed"); err != nil {
		t.Fatal(err)
	}
	if err := logger.Log("debug", "invalid"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected invalid status, got %v", err)
	}

	result, err := service.Read(Query{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour), Statuses: map[string]bool{"warning": true}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Logs) != 1 || result.Logs[0].Status != "warning" || result.Logs[0].IP != "192.0.2.10" || result.Logs[0].UserID != "user-1" {
		t.Fatalf("unexpected logs: %+v", result.Logs)
	}
}

func TestLoggerFromContext(t *testing.T) {
	service, err := NewService(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	ctx := IntoContext(context.Background(), service.With("192.0.2.20", "user-2"))
	if logger := FromContext(ctx); logger == nil {
		t.Fatal("logger missing from context")
	} else if err := logger.Log("error", "operation failed"); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupRemovesRecordsOlderThanSevenDays(t *testing.T) {
	directory := t.TempDir()
	service, err := NewService(directory, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	now := time.Now().UTC()
	if err := service.write(now.Add(-8*24*time.Hour), "info", "", "", "expired"); err != nil {
		t.Fatal(err)
	}
	if err := service.write(now.Add(-6*24*time.Hour), "info", "", "", "retained"); err != nil {
		t.Fatal(err)
	}
	if err := service.Cleanup(now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, now.Add(-8*24*time.Hour).Format("2006-01-02")+".log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired log still exists: %v", err)
	}
	result, err := service.Read(Query{Start: now.Add(-7 * 24 * time.Hour), End: now, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Logs) != 1 || result.Logs[0].Content != "retained" {
		t.Fatalf("unexpected retained logs: %+v", result.Logs)
	}
}
