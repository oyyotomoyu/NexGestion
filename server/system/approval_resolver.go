package system

import "context"

// GroupManagerUserIDs resolves the manager(s) of one specific group, without
// walking the organization hierarchy. Used by the Approval engine's
// approver_type = group_manager (docs/System/approval-system.md Section 2.2).
func (s *UserService) GroupManagerUserIDs(ctx context.Context, groupID, excludeUserID string) ([]string, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return groupManagerIDs(ctx, db, groupID, excludeUserID)
}

// RoleMemberUserIDs resolves every active holder of a role. Used by the
// Approval engine's approver_type = role (docs/System/approval-system.md
// Section 2.2), where any holder of the role is eligible to decide.
func (s *UserService) RoleMemberUserIDs(ctx context.Context, roleID, excludeUserID string) ([]string, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT u.id FROM user_roles ur
		JOIN users u ON u.id = ur.user_id
		WHERE ur.role_id=? AND u.id<>? AND u.status='active' AND u.deleted_at IS NULL ORDER BY u.id`, roleID, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
