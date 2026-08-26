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
				high_risk INTEGER NOT NULL DEFAULT 0 CHECK (high_risk IN (0, 1)),
				high_risk_reason TEXT,
				requires_password INTEGER NOT NULL DEFAULT 0 CHECK (requires_password IN (0, 1)),
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
				type TEXT NOT NULL CHECK (type IN ('organization', 'project')),
				organization_level INTEGER CHECK (organization_level BETWEEN 1 AND 5),
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
				is_primary_organization INTEGER NOT NULL DEFAULT 0 CHECK (is_primary_organization IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (user_id, group_id)
			)`,
			`CREATE TABLE IF NOT EXISTS group_permissions (
				group_id TEXT NOT NULL REFERENCES groups(id),
				permission_id TEXT NOT NULL REFERENCES permissions(id),
				created_at TEXT NOT NULL,
				PRIMARY KEY (group_id, permission_id)
			)`,
			`CREATE TABLE IF NOT EXISTS group_roles (
				group_id TEXT NOT NULL REFERENCES groups(id),
				role_id TEXT NOT NULL UNIQUE REFERENCES roles(id),
				kind TEXT NOT NULL CHECK (kind IN ('manager', 'member')),
				PRIMARY KEY (group_id, kind)
			)`,
			`CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL REFERENCES users(id),
				refresh_token_hash TEXT NOT NULL UNIQUE,
				expires_at TEXT NOT NULL,
				created_at TEXT NOT NULL,
				last_used_at TEXT,
				revoked_at TEXT,
				ip_address TEXT,
				user_agent TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id)`,
			`CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at)`,
			`CREATE TABLE IF NOT EXISTS auth_secrets (
				name TEXT PRIMARY KEY,
				secret TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
			`UPDATE roles SET name = 'Admin' WHERE id = '00000000-0000-0000-0000-000000000001' AND name = 'Administrator'`,
			`CREATE TRIGGER IF NOT EXISTS prevent_admin_role_update
			BEFORE UPDATE ON roles
			WHEN OLD.id = '00000000-0000-0000-0000-000000000001'
			BEGIN
				SELECT RAISE(ABORT, 'admin role cannot be edited');
			END`,
			`CREATE TRIGGER IF NOT EXISTS prevent_admin_role_delete
			BEFORE DELETE ON roles
			WHEN OLD.id = '00000000-0000-0000-0000-000000000001'
			BEGIN
				SELECT RAISE(ABORT, 'admin role cannot be deleted');
			END`,
			`CREATE TRIGGER IF NOT EXISTS prevent_admin_role_assignment
			BEFORE INSERT ON user_roles
			WHEN NEW.role_id = '00000000-0000-0000-0000-000000000001'
				AND NEW.user_id <> '00000000-0000-0000-0000-000000000001'
			BEGIN
				SELECT RAISE(ABORT, 'admin role cannot be assigned to another user');
			END`,
			`CREATE TRIGGER IF NOT EXISTS prevent_admin_role_removal
			BEFORE DELETE ON user_roles
			WHEN OLD.role_id = '00000000-0000-0000-0000-000000000001'
				AND OLD.user_id = '00000000-0000-0000-0000-000000000001'
			BEGIN
				SELECT RAISE(ABORT, 'admin role cannot be removed from the administrator');
			END`,
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
		SyncFunc: syncUserDatabase,
		SeedFunc: seedAdminUser,
	}
}

func syncUserDatabase(ctx context.Context, tx *sql.Tx) error {
	if err := ensureUserColumn(ctx, tx, "groups", "organization_level", "INTEGER"); err != nil {
		return err
	}
	if err := ensureUserColumn(ctx, tx, "user_groups", "is_primary_organization", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureUserColumn(ctx, tx, "permissions", "high_risk", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureUserColumn(ctx, tx, "permissions", "high_risk_reason", "TEXT"); err != nil {
		return err
	}
	if err := ensureUserColumn(ctx, tx, "permissions", "requires_password", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// Legacy hierarchical groups become organizations. Standalone legacy groups
	// become projects so existing installations remain usable after migration.
	if _, err := tx.ExecContext(ctx, `UPDATE groups SET type='organization'
		WHERE type NOT IN ('organization','project') AND
		(parent_group_id IS NOT NULL OR id IN (SELECT parent_group_id FROM groups WHERE parent_group_id IS NOT NULL))`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE groups SET type='project'
		WHERE type NOT IN ('organization','project')`); err != nil {
		return err
	}
	for level := 1; level <= 5; level++ {
		if level == 1 {
			if _, err := tx.ExecContext(ctx, `UPDATE groups SET organization_level=1
				WHERE type='organization' AND parent_group_id IS NULL`); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE groups SET organization_level=?
			WHERE type='organization' AND parent_group_id IN
			(SELECT id FROM groups WHERE type='organization' AND organization_level=?)`, level, level-1); err != nil {
			return err
		}
	}
	var invalidLegacyHierarchy int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups
		WHERE type='organization' AND organization_level IS NULL`).Scan(&invalidLegacyHierarchy); err != nil {
		return err
	}
	if invalidLegacyHierarchy > 0 {
		return errors.New("legacy organization hierarchy exceeds five levels or has an invalid parent")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE groups SET parent_group_id=NULL,organization_level=NULL WHERE type='project'`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS user_one_primary_organization
		ON user_groups(user_id) WHERE is_primary_organization=1 AND left_at IS NULL`); err != nil {
		return err
	}
	return syncPermissionCatalog(ctx, tx)
}

func ensureUserColumn(ctx context.Context, tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
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
		) VALUES (?, 'Admin', 'Initial system administrator', 1, 1, ?, ?)`,
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
