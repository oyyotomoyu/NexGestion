package system

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestUserDatabaseCreatesProtectedAdmin(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_EMAIL", "Admin@Example.com")
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")

	if err := EnsureDatabases(context.Background(), directory, []DatabaseSpec{userDatabaseSpec()}); err != nil {
		t.Fatalf("initialize user database: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(directory, "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var email, passwordHash, employeeNumber, roleName string
	var protected, grantsAllPermissions int
	err = db.QueryRow(`
		SELECT u.email, u.password_hash, u.is_protected, e.employee_number,
		       r.name, r.grants_all_permissions
		FROM users u
		JOIN employee_profiles e ON e.user_id = u.id
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles r ON r.id = ur.role_id
		WHERE u.id = ?`, adminUserID,
	).Scan(&email, &passwordHash, &protected, &employeeNumber, &roleName, &grantsAllPermissions)
	if err != nil {
		t.Fatalf("query administrator: %v", err)
	}

	if email != "admin@example.com" || employeeNumber != "0" || roleName != "Administrator" {
		t.Fatalf("unexpected administrator: email=%q employee=%q role=%q", email, employeeNumber, roleName)
	}
	if protected != 1 || grantsAllPermissions != 1 {
		t.Fatal("administrator must be protected and grant all permissions")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("a-secure-test-password")); err != nil {
		t.Fatal("administrator password was not hashed correctly")
	}

	if _, err := db.Exec(`DELETE FROM users WHERE id = ?`, adminUserID); err == nil || !strings.Contains(err.Error(), "protected user") {
		t.Fatalf("expected protected user deletion error, got %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET is_protected = 0 WHERE id = ?`, adminUserID); err == nil || !strings.Contains(err.Error(), "protected user") {
		t.Fatalf("expected protected user update error, got %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET deleted_at = 'now' WHERE id = ?`, adminUserID); err == nil || !strings.Contains(err.Error(), "protected user") {
		t.Fatalf("expected protected user soft deletion error, got %v", err)
	}
}

func TestUserDatabaseRejectsDuplicateEmailIgnoringCase(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")

	if err := EnsureDatabases(context.Background(), directory, []DatabaseSpec{userDatabaseSpec()}); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", filepath.Join(directory, "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		INSERT INTO users (id, display_name, email, password_hash, status, created_at, updated_at)
		VALUES ('another-user', 'Another', 'ADMIN@EXAMPLE.COM', 'hash', 'active', 'now', 'now')`)
	if err == nil {
		t.Fatal("expected case-insensitive duplicate email error")
	}
}
