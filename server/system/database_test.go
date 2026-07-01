package system

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestEnsureDatabasesCreatesSchemaAndSeedsOnlyOnce(t *testing.T) {
	directory := t.TempDir()
	specs := []DatabaseSpec{
		{
			Name: "test.db",
			Schema: []string{
				`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
			},
			Seed: []string{
				`INSERT INTO items (name) VALUES ('default')`,
			},
		},
	}

	if err := EnsureDatabases(context.Background(), directory, specs); err != nil {
		t.Fatalf("first initialization failed: %v", err)
	}

	// IF NOT EXISTS is how application schemas remain safe on later starts.
	specs[0].Schema[0] = `CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`
	if err := EnsureDatabases(context.Background(), directory, specs); err != nil {
		t.Fatalf("second initialization failed: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(directory, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE name = 'default'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("default data count = %d, want 1", count)
	}
}

func TestEnsureDatabasesRejectsNestedDatabaseName(t *testing.T) {
	err := EnsureDatabases(context.Background(), t.TempDir(), []DatabaseSpec{{Name: "../outside.db"}})
	if err == nil {
		t.Fatal("expected invalid database name error")
	}
}
