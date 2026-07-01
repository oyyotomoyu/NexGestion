package system

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	defaultAdminEmail = "admin@nexgestion.local"
	adminUserID       = "00000000-0000-0000-0000-000000000001"
	adminEmployeeID   = "00000000-0000-0000-0000-000000000001"
	adminRoleID       = "00000000-0000-0000-0000-000000000001"
)

func userDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "user.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				display_name TEXT NOT NULL,
				email TEXT NOT NULL COLLATE NOCASE UNIQUE,
				password_hash TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('pending', 'active', 'disabled', 'locked')),
				locale TEXT,
				timezone TEXT,
				must_change_password INTEGER NOT NULL DEFAULT 1 CHECK (must_change_password IN (0, 1)),
				failed_login_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
				locked_until TEXT,
				last_login_at TEXT,
				password_changed_at TEXT,
				is_protected INTEGER NOT NULL DEFAULT 0 CHECK (is_protected IN (0, 1)),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				deleted_at TEXT
			)`,
			`CREATE TABLE IF NOT EXISTS employee_profiles (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL UNIQUE REFERENCES users(id),
				employee_number TEXT NOT NULL UNIQUE,
				legal_name TEXT,
				preferred_name TEXT,
				work_email TEXT COLLATE NOCASE,
				work_phone TEXT,
				job_title TEXT,
				employment_status TEXT NOT NULL DEFAULT 'active',
				hire_date TEXT,
				termination_date TEXT,
				manager_employee_id TEXT REFERENCES employee_profiles(id),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS roles (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL COLLATE NOCASE UNIQUE,
				description TEXT,
				is_system INTEGER NOT NULL DEFAULT 0 CHECK (is_system IN (0, 1)),
				grants_all_permissions INTEGER NOT NULL DEFAULT 0 CHECK (grants_all_permissions IN (0, 1)),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS permissions (
				id TEXT PRIMARY KEY,
				permission_key TEXT NOT NULL UNIQUE,
				module TEXT NOT NULL,
				description TEXT,
				created_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS user_roles (
				user_id TEXT NOT NULL REFERENCES users(id),
				role_id TEXT NOT NULL REFERENCES roles(id),
				created_at TEXT NOT NULL,
				PRIMARY KEY (user_id, role_id)
			)`,
			`CREATE TABLE IF NOT EXISTS role_permissions (
				role_id TEXT NOT NULL REFERENCES roles(id),
				permission_id TEXT NOT NULL REFERENCES permissions(id),
				created_at TEXT NOT NULL,
				PRIMARY KEY (role_id, permission_id)
			)`,
			`CREATE TABLE IF NOT EXISTS groups (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL COLLATE NOCASE UNIQUE,
				type TEXT NOT NULL,
				parent_group_id TEXT REFERENCES groups(id),
				status TEXT NOT NULL DEFAULT 'active',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS user_groups (
				user_id TEXT NOT NULL REFERENCES users(id),
				group_id TEXT NOT NULL REFERENCES groups(id),
				title TEXT,
				joined_at TEXT,
				left_at TEXT,
				created_at TEXT NOT NULL,
				PRIMARY KEY (user_id, group_id)
			)`,
			`CREATE TRIGGER IF NOT EXISTS prevent_protected_user_delete
			BEFORE DELETE ON users
			WHEN OLD.is_protected = 1
			BEGIN
				SELECT RAISE(ABORT, 'protected user cannot be deleted');
			END`,
			`CREATE TRIGGER IF NOT EXISTS prevent_protected_user_unprotect
			BEFORE UPDATE OF is_protected ON users
			WHEN OLD.is_protected = 1 AND NEW.is_protected = 0
			BEGIN
				SELECT RAISE(ABORT, 'protected user cannot be unprotected');
			END`,
			`CREATE TRIGGER IF NOT EXISTS prevent_protected_user_soft_delete
			BEFORE UPDATE OF deleted_at ON users
			WHEN OLD.is_protected = 1 AND NEW.deleted_at IS NOT NULL
			BEGIN
				SELECT RAISE(ABORT, 'protected user cannot be deleted');
			END`,
		},
		SeedFunc: seedAdminUser,
	}
}

func seedAdminUser(ctx context.Context, tx *sql.Tx) error {
	email, err := adminEmail()
	if err != nil {
		return err
	}

	password := strings.TrimSpace(os.Getenv("NEXGESTION_ADMIN_PASSWORD"))
	generatedPassword := password == ""
	if generatedPassword {
		password, err = generatePassword()
		if err != nil {
			return fmt.Errorf("generate administrator password: %w", err)
		}
	} else if len(password) < 12 {
		return errors.New("NEXGESTION_ADMIN_PASSWORD must contain at least 12 characters")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash administrator password: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (
			id, display_name, email, password_hash, status, locale, timezone,
			must_change_password, is_protected, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'active', 'CHT', 'Asia/Taipei', 1, 1, ?, ?)`,
		adminUserID, "Administrator", email, string(passwordHash), now, now,
	); err != nil {
		return fmt.Errorf("create administrator account: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO employee_profiles (
			id, user_id, employee_number, preferred_name, work_email,
			employment_status, created_at, updated_at
		) VALUES (?, ?, '0', 'Administrator', ?, 'active', ?, ?)`,
		adminEmployeeID, adminUserID, email, now, now,
	); err != nil {
		return fmt.Errorf("create administrator employee profile: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO roles (
			id, name, description, is_system, grants_all_permissions, created_at, updated_at
		) VALUES (?, 'Administrator', 'Initial system administrator', 1, 1, ?, ?)`,
		adminRoleID, now, now,
	); err != nil {
		return fmt.Errorf("create administrator role: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id, created_at) VALUES (?, ?, ?)`,
		adminUserID, adminRoleID, now,
	); err != nil {
		return fmt.Errorf("assign administrator role: %w", err)
	}

	if generatedPassword {
		log.Printf("initial administrator created: email=%s temporary_password=%s", email, password)
	} else {
		log.Printf("initial administrator created: email=%s", email)
	}

	return nil
}

func adminEmail() (string, error) {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("NEXGESTION_ADMIN_EMAIL")))
	if email == "" {
		email = defaultAdminEmail
	}

	address, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(address.Address, email) {
		return "", errors.New("NEXGESTION_ADMIN_EMAIL must be a valid email address")
	}

	return email, nil
}

func generatePassword() (string, error) {
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}
