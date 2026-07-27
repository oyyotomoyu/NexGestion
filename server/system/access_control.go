package system

import (
	"context"
	"database/sql"
	"errors"
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

func getPermission(ctx context.Context, db *sql.DB, id string) (*Permission, error) {
	var p Permission
	err := db.QueryRowContext(ctx, `SELECT id,permission_key,module,description FROM permissions WHERE id=?`, id).Scan(&p.ID, &p.PermissionKey, &p.Module, &p.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPermissionNotFound
	}
	return &p, err
}

func (s *UserService) SetRolePermission(ctx context.Context, actor, roleID, permissionID string, grant bool) error {
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
	if _, err = getPermission(ctx, db, permissionID); err != nil {
		return err
	}
	if grant {
		_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_id,permission_id,created_at) VALUES(?,?,?)`, roleID, permissionID, time.Now().UTC().Format(time.RFC3339))
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id=? AND permission_id=?`, roleID, permissionID)
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
