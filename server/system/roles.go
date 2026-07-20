package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRoleNotFound  = errors.New("role not found")
	ErrRoleProtected = errors.New("admin role is protected")
	ErrRoleAssigned  = errors.New("role is assigned to one or more users")
	ErrAdminRequired = errors.New("initial administrator access is required")
)

type Permission struct {
	ID            string  `json:"id"`
	PermissionKey string  `json:"permission_key"`
	Module        string  `json:"module"`
	Description   *string `json:"description"`
}

type CreateRoleInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
}

type UpdateRoleInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

func IsInitialAdministrator(userID string) bool { return userID == adminUserID }

func (s *UserService) ListRoles(ctx context.Context) ([]Role, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT id FROM roles ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	roles := make([]Role, 0, len(ids))
	for _, id := range ids {
		role, err := getRole(ctx, db, id)
		if err != nil {
			return nil, err
		}
		roles = append(roles, *role)
	}
	return roles, nil
}

func (s *UserService) GetRole(ctx context.Context, id string) (*Role, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return getRole(ctx, db, id)
}

func getRole(ctx context.Context, db *sql.DB, id string) (*Role, error) {
	var role Role
	var systemRole, grantsAll int
	err := db.QueryRowContext(ctx, `SELECT id, name, description, is_system, grants_all_permissions
		FROM roles WHERE id = ?`, id).Scan(
		&role.ID, &role.Title, &role.Description, &systemRole, &grantsAll,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	role.IsSystem = systemRole == 1
	role.GrantsAllPermissions = grantsAll == 1
	role.Permissions = []Permission{}

	rows, err := db.QueryContext(ctx, `SELECT p.id, p.permission_key, p.module, p.description
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = ?
		ORDER BY p.permission_key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var permission Permission
		if err := rows.Scan(&permission.ID, &permission.PermissionKey, &permission.Module, &permission.Description); err != nil {
			return nil, err
		}
		role.Permissions = append(role.Permissions, permission)
	}
	return &role, rows.Err()
}

func (s *UserService) CreateRole(ctx context.Context, actorUserID string, input CreateRoleInput) (*Role, error) {
	if !IsInitialAdministrator(actorUserID) {
		return nil, ErrAdminRequired
	}
	title, err := normalizeRoleTitle(input.Title)
	if err != nil {
		return nil, err
	}
	description := normalizeOptionalText(input.Description)
	now, id := time.Now().UTC().Format(time.RFC3339), uuid.NewString()

	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO roles
		(id, name, description, is_system, grants_all_permissions, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, ?, ?)`, id, title, description, now, now); err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}
	return getRole(ctx, db, id)
}

func (s *UserService) UpdateRole(ctx context.Context, actorUserID, id string, input UpdateRoleInput) (*Role, error) {
	if !IsInitialAdministrator(actorUserID) {
		return nil, ErrAdminRequired
	}
	if id == adminRoleID {
		return nil, ErrRoleProtected
	}

	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := getRole(ctx, db, id); err != nil {
		return nil, err
	}

	sets := []string{}
	args := []any{}
	if input.Title != nil {
		title, err := normalizeRoleTitle(*input.Title)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "name = ?")
		args = append(args, title)
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, normalizeOptionalText(input.Description))
	}
	if len(sets) == 0 {
		return getRole(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339), id)
	if _, err := db.ExecContext(ctx, `UPDATE roles SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}
	return getRole(ctx, db, id)
}

func (s *UserService) DeleteRole(ctx context.Context, actorUserID, id string) error {
	if !IsInitialAdministrator(actorUserID) {
		return ErrAdminRequired
	}
	if id == adminRoleID {
		return ErrRoleProtected
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists, assignments int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrRoleNotFound
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles WHERE role_id = ?`, id).Scan(&assignments); err != nil {
		return err
	}
	if assignments != 0 {
		return ErrRoleAssigned
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return tx.Commit()
}

func normalizeRoleTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("role title is required")
	}
	return value, nil
}

func normalizeOptionalText(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
