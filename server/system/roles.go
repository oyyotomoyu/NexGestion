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
	ID               string  `json:"id"`
	PermissionKey    string  `json:"permission_key"`
	Module           string  `json:"module"`
	Description      *string `json:"description"`
	HighRisk         bool    `json:"high_risk"`
	HighRiskReason   *string `json:"high_risk_reason"`
	RequiresPassword bool    `json:"requires_password"`
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

func (s *UserService) ListRoles(ctx context.Context, query ListQuery) (ListResult[Role], error) {
	query, sortExpression, err := NormalizeListQuery(query, "title", "asc", map[string]string{
		"title":               "r.name COLLATE NOCASE",
		"created_at":          "r.created_at",
		"updated_at":          "r.updated_at",
		"is_system":           "r.is_system",
		"permission_count":    "permission_count",
		"assigned_user_count": "assigned_user_count",
	})
	if err != nil {
		return ListResult[Role]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[Role]{}, err
	}
	defer db.Close()
	where := []string{"1=1"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(r.name LIKE ? COLLATE NOCASE OR r.description LIKE ? COLLATE NOCASE OR EXISTS (
			SELECT 1 FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.role_id = r.id AND p.permission_key LIKE ? COLLATE NOCASE
		))`)
		args = append(args, pattern, pattern, pattern)
	}
	if value := strings.TrimSpace(query.Filters["is_system"]); value != "" {
		where = append(where, "r.is_system = ?")
		args = append(args, boolFilter(value))
	}
	if value := strings.TrimSpace(query.Filters["grants_all_permissions"]); value != "" {
		where = append(where, "r.grants_all_permissions = ?")
		args = append(args, boolFilter(value))
	}
	if permissionKey := strings.TrimSpace(query.Filters["permission_key"]); permissionKey != "" {
		where = append(where, `EXISTS (
			SELECT 1 FROM permissions p
			WHERE p.permission_key = ? AND (r.grants_all_permissions = 1 OR EXISTS (
				SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_id = p.id
			))
		)`)
		args = append(args, permissionKey)
	}
	whereSQL := strings.Join(where, " AND ")
	base := `FROM roles r
		LEFT JOIN (SELECT role_id, COUNT(*) permission_count FROM role_permissions GROUP BY role_id) pc ON pc.role_id = r.id
		LEFT JOIN (SELECT role_id, COUNT(*) assigned_user_count FROM user_roles GROUP BY role_id) uc ON uc.role_id = r.id`
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) `+base+` WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[Role]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT r.id, COALESCE(pc.permission_count,0) permission_count, COALESCE(uc.assigned_user_count,0) assigned_user_count
		`+base+` WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, r.id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[Role]{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		var permissionCount, assignedUserCount int
		if err := rows.Scan(&id, &permissionCount, &assignedUserCount); err != nil {
			rows.Close()
			return ListResult[Role]{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return ListResult[Role]{}, err
	}
	roles := make([]Role, 0, len(ids))
	for _, id := range ids {
		role, err := getRole(ctx, db, id)
		if err != nil {
			return ListResult[Role]{}, err
		}
		roles = append(roles, *role)
	}
	return NewListResult(roles, query, total), nil
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

	permissionQuery := `SELECT p.id, p.permission_key, p.module, p.description, p.high_risk, p.high_risk_reason, p.requires_password
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = ?
		ORDER BY p.permission_key`
	args := []any{id}
	if role.GrantsAllPermissions {
		permissionQuery = `SELECT id, permission_key, module, description, high_risk, high_risk_reason, requires_password
			FROM permissions ORDER BY permission_key`
		args = nil
	}
	rows, err := db.QueryContext(ctx, permissionQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var permission Permission
		var highRisk, requiresPassword int
		if err := rows.Scan(&permission.ID, &permission.PermissionKey, &permission.Module, &permission.Description, &highRisk, &permission.HighRiskReason, &requiresPassword); err != nil {
			return nil, err
		}
		permission.HighRisk = highRisk == 1
		permission.RequiresPassword = requiresPassword == 1
		role.Permissions = append(role.Permissions, permission)
	}
	return &role, rows.Err()
}

func (s *UserService) CreateRole(ctx context.Context, actorUserID string, input CreateRoleInput) (*Role, error) {
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
	if id == adminRoleID {
		return nil, ErrRoleProtected
	}

	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	role, err := getRole(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if role.IsSystem {
		return nil, ErrRoleProtected
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
	if id == adminRoleID {
		return ErrRoleProtected
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	role, err := getRole(ctx, db, id)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return ErrRoleProtected
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrRoleNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE role_id = ?`, id); err != nil {
		return err
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

func boolFilter(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return 1
	default:
		return 0
	}
}
