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
	ErrPermissionNotFound = errors.New("permission not found")
	ErrPermissionDenied   = errors.New("permission denied")
)

type CreatePermissionInput struct {
	PermissionKey string  `json:"permission_key"`
	Module        string  `json:"module"`
	Description   *string `json:"description"`
}
type UpdatePermissionInput struct {
	Module      *string `json:"module"`
	Description *string `json:"description"`
}

func hasPermission(ctx context.Context, db *sql.DB, userID, key string) bool {
	if IsInitialAdministrator(userID) {
		return true
	}
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (
		SELECT p.id FROM permissions p JOIN role_permissions rp ON rp.permission_id=p.id JOIN user_roles ur ON ur.role_id=rp.role_id WHERE ur.user_id=? AND p.permission_key=?
		UNION SELECT p.id FROM permissions p JOIN group_permissions gp ON gp.permission_id=p.id JOIN user_groups ug ON ug.group_id=gp.group_id JOIN groups g ON g.id=ug.group_id
		WHERE ug.user_id=? AND ug.left_at IS NULL AND g.status='active' AND p.permission_key=?)`, userID, key, userID, key).Scan(&count)
	return err == nil && count > 0
}

func (s *UserService) HasPermission(ctx context.Context, userID, key string) bool {
	db, err := s.open()
	if err != nil {
		return false
	}
	defer db.Close()
	return hasPermission(ctx, db, userID, key)
}

func (s *UserService) ListPermissions(ctx context.Context) ([]Permission, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id,permission_key,module,description FROM permissions ORDER BY permission_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Permission{}
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.PermissionKey, &p.Module, &p.Description); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *UserService) CreatePermission(ctx context.Context, actor string, input CreatePermissionInput) (*Permission, error) {
	if !IsInitialAdministrator(actor) {
		return nil, ErrPermissionDenied
	}
	key, module := strings.TrimSpace(input.PermissionKey), strings.TrimSpace(input.Module)
	if key == "" || module == "" {
		return nil, errors.New("permission key and module are required")
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	if _, err = db.ExecContext(ctx, `INSERT INTO permissions(id,permission_key,module,description,created_at) VALUES(?,?,?,?,?)`, id, key, module, normalizeOptionalText(input.Description), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("create permission: %w", err)
	}
	return getPermission(ctx, db, id)
}

func (s *UserService) UpdatePermission(ctx context.Context, actor, id string, input UpdatePermissionInput) (*Permission, error) {
	if !IsInitialAdministrator(actor) {
		return nil, ErrPermissionDenied
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err = getPermission(ctx, db, id); err != nil {
		return nil, err
	}
	if input.Module != nil {
		module := strings.TrimSpace(*input.Module)
		if module == "" {
			return nil, errors.New("permission module is required")
		}
		if _, err = db.ExecContext(ctx, `UPDATE permissions SET module=? WHERE id=?`, module, id); err != nil {
			return nil, err
		}
	}
	if input.Description != nil {
		if _, err = db.ExecContext(ctx, `UPDATE permissions SET description=? WHERE id=?`, normalizeOptionalText(input.Description), id); err != nil {
			return nil, err
		}
	}
	return getPermission(ctx, db, id)
}

func getPermission(ctx context.Context, db *sql.DB, id string) (*Permission, error) {
	var p Permission
	err := db.QueryRowContext(ctx, `SELECT id,permission_key,module,description FROM permissions WHERE id=?`, id).Scan(&p.ID, &p.PermissionKey, &p.Module, &p.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPermissionNotFound
	}
	return &p, err
}

func (s *UserService) SetRolePermission(ctx context.Context, actor, roleID, permissionID string, grant bool) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	role, err := getRole(ctx, db, roleID)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return ErrRoleProtected
	}
	if !hasPermission(ctx, db, actor, "permissions.assign") {
		return ErrPermissionDenied
	}
	if _, err = getPermission(ctx, db, permissionID); err != nil {
		return err
	}
	if !IsInitialAdministrator(actor) && !hasPermission(ctx, db, actor, permissionID) {
		return ErrPermissionDenied
	}
	if grant {
		_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_id,permission_id,created_at) VALUES(?,?,?)`, roleID, permissionID, time.Now().UTC().Format(time.RFC3339))
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id=? AND permission_id=?`, roleID, permissionID)
	}
	return err
}

func (s *UserService) SetGroupPermission(ctx context.Context, actor, groupID, permissionID string, grant bool) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err = getGroup(ctx, db, groupID); err != nil {
		return err
	}
	if !hasPermission(ctx, db, actor, "permissions.assign") {
		return ErrPermissionDenied
	}
	if _, err = getPermission(ctx, db, permissionID); err != nil {
		return err
	}
	if !IsInitialAdministrator(actor) && !hasPermission(ctx, db, actor, permissionID) {
		return ErrPermissionDenied
	}
	if grant {
		_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO group_permissions(group_id,permission_id,created_at) VALUES(?,?,?)`, groupID, permissionID, time.Now().UTC().Format(time.RFC3339))
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM group_permissions WHERE group_id=? AND permission_id=?`, groupID, permissionID)
	}
	return err
}

func (s *UserService) ListRoleUsers(ctx context.Context, roleID string) ([]User, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err = getRole(ctx, db, roleID); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT user_id FROM user_roles WHERE role_id=? ORDER BY user_id`, roleID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	result := []User{}
	for _, id := range ids {
		u, err := getUser(ctx, db, id)
		if err != nil {
			return nil, err
		}
		result = append(result, *u)
	}
	return result, nil
}

func (s *UserService) SetRoleUser(ctx context.Context, actor, roleID, userID string, assign bool) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	role, err := getRole(ctx, db, roleID)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return ErrRoleProtected
	}
	if !hasPermission(ctx, db, actor, "roles.assign") {
		return ErrPermissionDenied
	}
	var exists int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id=? AND deleted_at IS NULL`, userID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrUserNotFound
	}
	if assign {
		_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO user_roles(user_id,role_id,created_at) VALUES(?,?,?)`, userID, roleID, time.Now().UTC().Format(time.RFC3339))
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id=? AND role_id=?`, userID, roleID)
	}
	return err
}
