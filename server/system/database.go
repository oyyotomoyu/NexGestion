package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

const defaultDatabaseDirectory = "database"

// DatabaseSpec describes a database required by the application.
// Seed statements are executed only when the database file is first created.
type DatabaseSpec struct {
	Name     string
	Schema   []string
	Seed     []string
	SeedFunc func(context.Context, *sql.Tx) error
	SyncFunc func(context.Context, *sql.Tx) error
}

// RequiredDatabases is the list of SQLite databases needed by NexGestion.
// Add another entry here when a module needs its own database.
var RequiredDatabases = []DatabaseSpec{
	{
		Name: "system.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS system_settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)`,
		},
		Seed: []string{
			`INSERT INTO system_settings (key, value) VALUES ('language', 'CHT')`,
		},
		SyncFunc: syncSystemSettingsDefaults,
	},
	userDatabaseSpec(),
	attendanceDatabaseSpec(),
	notificationDatabaseSpec(),
	templateDatabaseSpec(),
}

// syncSystemSettingsDefaults inserts default settings that were added after a
// database was first created, without overwriting values an administrator
// already changed.
func syncSystemSettingsDefaults(ctx context.Context, tx *sql.Tx) error {
	defaults := map[string]string{
		templateFileMaxBytesKey:    strconv.FormatInt(defaultTemplateFileMaxBytes, 10),
		templateStorageMaxBytesKey: strconv.FormatInt(defaultTemplateStorageMaxBytes, 10),
	}
	for key, value := range defaults {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO system_settings (key, value) VALUES (?, ?)`, key, value); err != nil {
			return fmt.Errorf("seed default setting %q: %w", key, err)
		}
	}
	return nil
}

// EnsureRequiredDatabases creates and initializes every required database.
func EnsureRequiredDatabases(ctx context.Context, directory string) error {
	if directory == "" {
		directory = defaultDatabaseDirectory
	}

	return EnsureDatabases(ctx, directory, RequiredDatabases)
}

// EnsureDatabases makes sure all specs exist and have their schema installed.
func EnsureDatabases(ctx context.Context, directory string, specs []DatabaseSpec) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	for _, spec := range specs {
		if err := ensureDatabase(ctx, directory, spec); err != nil {
			return fmt.Errorf("initialize database %q: %w", spec.Name, err)
		}
	}

	return nil
}

func ensureDatabase(ctx context.Context, directory string, spec DatabaseSpec) error {
	if spec.Name == "" || filepath.Base(spec.Name) != spec.Name {
		return errors.New("database name must be a non-empty file name")
	}

	path := filepath.Join(directory, spec.Name)
	_, statErr := os.Stat(path)
	isNew := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !isNew {
		return fmt.Errorf("check database file: %w", statErr)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin initialization: %w", err)
	}
	defer tx.Rollback()

	for _, statement := range spec.Schema {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}
	if spec.SyncFunc != nil {
		if err := spec.SyncFunc(ctx, tx); err != nil {
			return fmt.Errorf("synchronize data: %w", err)
		}
	}

	if isNew {
		for _, statement := range spec.Seed {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("insert default data: %w", err)
			}
		}
		if spec.SeedFunc != nil {
			if err := spec.SeedFunc(ctx, tx); err != nil {
				return fmt.Errorf("insert generated default data: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit initialization: %w", err)
	}

	return nil
}
