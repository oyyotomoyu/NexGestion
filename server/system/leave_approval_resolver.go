package system

import (
	"context"
	"database/sql"
	"errors"
)

type LeaveApprovalRoute struct {
	GroupID            *string
	ApproverUserIDs    []string
	AdministratorRoute bool
	RequiresAssignment bool
}

func (s *UserService) ResolveLeaveApproval(ctx context.Context, requesterID string) (*LeaveApprovalRoute, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var groupID string
	err = db.QueryRowContext(ctx, `SELECT g.id FROM user_groups ug
		JOIN groups g ON g.id=ug.group_id
		WHERE ug.user_id=? AND ug.left_at IS NULL AND ug.is_primary_organization=1
			AND g.type='organization' AND g.status='active'`, requesterID).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return resolveAdministratorApproval(ctx, db, requesterID)
	}
	if err != nil {
		return nil, err
	}

	for groupID != "" {
		var requesterIsManager int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM group_roles gr
			JOIN user_roles ur ON ur.role_id=gr.role_id
			WHERE gr.group_id=? AND gr.kind='manager' AND ur.user_id=?`, groupID, requesterID).Scan(&requesterIsManager); err != nil {
			return nil, err
		}
		if requesterIsManager == 0 {
			approvers, err := groupManagerIDs(ctx, db, groupID, requesterID)
			if err != nil {
				return nil, err
			}
			if len(approvers) > 0 {
				resolvedGroup := groupID
				return &LeaveApprovalRoute{GroupID: &resolvedGroup, ApproverUserIDs: approvers}, nil
			}
		}
		var parent *string
		if err := db.QueryRowContext(ctx, `SELECT parent_group_id FROM groups WHERE id=?`, groupID).Scan(&parent); err != nil {
			return nil, err
		}
		if parent == nil {
			break
		}
		groupID = *parent
	}
	return resolveAdministratorApproval(ctx, db, requesterID)
}

func groupManagerIDs(ctx context.Context, db *sql.DB, groupID, requesterID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT u.id FROM group_roles gr
		JOIN user_roles ur ON ur.role_id=gr.role_id
		JOIN users u ON u.id=ur.user_id
		WHERE gr.group_id=? AND gr.kind='manager' AND u.id<>?
			AND u.status='active' AND u.deleted_at IS NULL ORDER BY u.id`, groupID, requesterID)
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

func resolveAdministratorApproval(ctx context.Context, db *sql.DB, requesterID string) (*LeaveApprovalRoute, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT u.id FROM users u
		LEFT JOIN user_roles ur ON ur.user_id=u.id
		LEFT JOIN roles r ON r.id=ur.role_id
		WHERE u.id<>? AND u.status='active' AND u.deleted_at IS NULL
			AND (u.is_protected=1 OR r.grants_all_permissions=1) ORDER BY u.id`, requesterID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &LeaveApprovalRoute{
		ApproverUserIDs: ids, AdministratorRoute: true, RequiresAssignment: len(ids) == 0,
	}, nil
}
