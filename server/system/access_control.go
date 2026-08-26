package system

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrPermissionNotFound = errors.New("permission not found")
	ErrPermissionDenied   = errors.New("permission denied")
)

// EffectivePermissionKeys returns permission data derived from every role
// assigned to a user. Request authorization decisions belong to the API layer.
func (s *UserService) EffectivePermissionKeys(ctx context.Context, userID string) ([]string, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT p.permission_key
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN permissions p ON r.grants_all_permissions = 1
			OR EXISTS (
				SELECT 1 FROM role_permissions rp
				WHERE rp.role_id = r.id AND rp.permission_id = p.id
			)
		WHERE ur.user_id = ?
		ORDER BY p.permission_key`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *UserService) ListPermissions(ctx context.Context, query ListQuery) (ListResult[Permission], error) {
	query, sortExpression, err := NormalizeListQuery(query, "permission_key", "asc", map[string]string{
		"permission_key": "permission_key",
		"module":         "module",
	})
	if err != nil {
		return ListResult[Permission]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[Permission]{}, err
	}
	defer db.Close()
	where := []string{"1=1"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(permission_key LIKE ? COLLATE NOCASE OR module LIKE ? COLLATE NOCASE OR description LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern, pattern)
	}
	if module := strings.TrimSpace(query.Filters["module"]); module != "" {
		where = append(where, "module = ?")
		args = append(args, module)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM permissions WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[Permission]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT id,permission_key,module,description,high_risk,high_risk_reason,requires_password FROM permissions WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[Permission]{}, err
	}
	defer rows.Close()
	result := []Permission{}
	for rows.Next() {
		var p Permission
		var highRisk, requiresPassword int
		if err := rows.Scan(&p.ID, &p.PermissionKey, &p.Module, &p.Description, &highRisk, &p.HighRiskReason, &requiresPassword); err != nil {
			return ListResult[Permission]{}, err
		}
		p.HighRisk = highRisk == 1
		p.RequiresPassword = requiresPassword == 1
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return ListResult[Permission]{}, err
	}
	return NewListResult(result, query, total), nil
}

func getPermission(ctx context.Context, db *sql.DB, id string) (*Permission, error) {
	var p Permission
	var highRisk, requiresPassword int
	err := db.QueryRowContext(ctx, `SELECT id,permission_key,module,description,high_risk,high_risk_reason,requires_password FROM permissions WHERE id=?`, id).Scan(&p.ID, &p.PermissionKey, &p.Module, &p.Description, &highRisk, &p.HighRiskReason, &requiresPassword)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPermissionNotFound
	}
	p.HighRisk = highRisk == 1
	p.RequiresPassword = requiresPassword == 1
	return &p, err
}

func (s *UserService) SetRolePermission(ctx context.Context, actor, roleID, permissionID string, grant bool, currentPassword string) error {
	// Role metadata management may be delegated through roles.manage, but the
	// authority to change what a role grants is reserved for the initial
	// administrator. Keeping this check independent of permissions.assign
	// prevents a delegated role editor from escalating privileges.
	if !IsInitialAdministrator(actor) {
		return ErrPermissionDenied
	}
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
	permission, err := getPermission(ctx, db, permissionID)
	if err != nil {
		return err
	}
	catalog, err := LoadPermissionCatalog()
	if err != nil {
		return err
	}
	if grant && catalog.RequiresPasswordForGrant(permission.PermissionKey) {
		if err := verifyUserPassword(ctx, db, actor, currentPassword); err != nil {
			return err
		}
	}
	if grant {
		_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_id,permission_id,created_at) VALUES(?,?,?)`, roleID, permissionID, time.Now().UTC().Format(time.RFC3339))
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id=? AND permission_id=?`, roleID, permissionID)
	}
	return err
}

func (s *UserService) ListRoleUsers(ctx context.Context, roleID string, query ListQuery) (ListResult[User], error) {
	query, sortExpression, err := NormalizeListQuery(query, "display_name", "asc", map[string]string{
		"display_name": "u.display_name COLLATE NOCASE",
		"email":        "u.email COLLATE NOCASE",
		"created_at":   "u.created_at",
		"assigned_at":  "ur.created_at",
	})
	if err != nil {
		return ListResult[User]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[User]{}, err
	}
	defer db.Close()
	if _, err = getRole(ctx, db, roleID); err != nil {
		return ListResult[User]{}, err
	}
	where := []string{"ur.role_id = ?", "u.deleted_at IS NULL"}
	args := []any{roleID}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(u.display_name LIKE ? COLLATE NOCASE OR u.email LIKE ? COLLATE NOCASE OR ep.employee_number LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern, pattern)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id = ur.user_id LEFT JOIN employee_profiles ep ON ep.user_id = u.id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[User]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT u.id FROM user_roles ur JOIN users u ON u.id = ur.user_id LEFT JOIN employee_profiles ep ON ep.user_id = u.id
		WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, u.id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[User]{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ListResult[User]{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	result := []User{}
	for _, id := range ids {
		u, err := getUser(ctx, db, id)
		if err != nil {
			return ListResult[User]{}, err
		}
		result = append(result, *u)
	}
	return NewListResult(result, query, total), nil
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
