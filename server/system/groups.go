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
	ErrGroupNotFound = errors.New("group not found")
	ErrGroupInUse    = errors.New("group has members or child groups")
	ErrInvalidParent = errors.New("invalid parent group")
)

type Group struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Type          string       `json:"type"`
	ParentGroupID *string      `json:"parent_group_id"`
	Status        string       `json:"status"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	MemberCount   int          `json:"member_count"`
	Permissions   []Permission `json:"permissions"`
}

type CreateGroupInput struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	ParentGroupID *string `json:"parent_group_id"`
	Status        string  `json:"status"`
}

type UpdateGroupInput struct {
	Name          *string `json:"name"`
	Type          *string `json:"type"`
	ParentGroupID *string `json:"parent_group_id"`
	Status        *string `json:"status"`
}

func (s *UserService) ListGroups(ctx context.Context) ([]Group, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id FROM groups ORDER BY name COLLATE NOCASE, id`)
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
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]Group, 0, len(ids))
	for _, id := range ids {
		group, err := getGroup(ctx, db, id)
		if err != nil {
			return nil, err
		}
		result = append(result, *group)
	}
	return result, nil
}

func (s *UserService) GetGroup(ctx context.Context, id string) (*Group, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return getGroup(ctx, db, id)
}

func getGroup(ctx context.Context, db *sql.DB, id string) (*Group, error) {
	var group Group
	err := db.QueryRowContext(ctx, `SELECT g.id, g.name, g.type, g.parent_group_id, g.status,
		g.created_at, g.updated_at, COUNT(ug.user_id)
		FROM groups g LEFT JOIN user_groups ug ON ug.group_id = g.id AND ug.left_at IS NULL
		WHERE g.id = ? GROUP BY g.id`, id).Scan(&group.ID, &group.Name, &group.Type,
		&group.ParentGroupID, &group.Status, &group.CreatedAt, &group.UpdatedAt, &group.MemberCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	group.Permissions = []Permission{}
	rows, err := db.QueryContext(ctx, `SELECT p.id, p.permission_key, p.module, p.description
		FROM permissions p JOIN group_permissions gp ON gp.permission_id = p.id
		WHERE gp.group_id = ? ORDER BY p.permission_key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.PermissionKey, &p.Module, &p.Description); err != nil {
			return nil, err
		}
		group.Permissions = append(group.Permissions, p)
	}
	return &group, rows.Err()
}

func (s *UserService) CreateGroup(ctx context.Context, actorUserID string, input CreateGroupInput) (*Group, error) {
	if !IsInitialAdministrator(actorUserID) {
		return nil, ErrAdminRequired
	}
	name, groupType, status, err := normalizeGroup(input.Name, input.Type, input.Status)
	if err != nil {
		return nil, err
	}
	parent := normalizeGroupParent(input.ParentGroupID)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := validateGroupParent(ctx, db, "", parent); err != nil {
		return nil, err
	}
	now, id := time.Now().UTC().Format(time.RFC3339), uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO groups (id, name, type, parent_group_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, name, groupType, parent, status, now, now); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return getGroup(ctx, db, id)
}

func (s *UserService) UpdateGroup(ctx context.Context, actorUserID, id string, input UpdateGroupInput) (*Group, error) {
	if !IsInitialAdministrator(actorUserID) {
		return nil, ErrAdminRequired
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	current, err := getGroup(ctx, db, id)
	if err != nil {
		return nil, err
	}
	name, groupType, status := current.Name, current.Type, current.Status
	parent := current.ParentGroupID
	if input.Name != nil {
		name = *input.Name
	}
	if input.Type != nil {
		groupType = *input.Type
	}
	if input.Status != nil {
		status = *input.Status
	}
	if input.ParentGroupID != nil {
		parent = normalizeGroupParent(input.ParentGroupID)
	}
	name, groupType, status, err = normalizeGroup(name, groupType, status)
	if err != nil {
		return nil, err
	}
	if err := validateGroupParent(ctx, db, id, parent); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `UPDATE groups SET name=?, type=?, parent_group_id=?, status=?, updated_at=? WHERE id=?`,
		name, groupType, parent, status, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return nil, fmt.Errorf("update group: %w", err)
	}
	return getGroup(ctx, db, id)
}

func (s *UserService) DeleteGroup(ctx context.Context, actorUserID, id string) error {
	if !IsInitialAdministrator(actorUserID) {
		return ErrAdminRequired
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	var exists, users, children int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE id=?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrGroupNotFound
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_groups WHERE group_id=?`, id).Scan(&users); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE parent_group_id=?`, id).Scan(&children); err != nil {
		return err
	}
	if users > 0 || children > 0 {
		return ErrGroupInUse
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM group_permissions WHERE group_id=?`, id); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM groups WHERE id=?`, id)
	return err
}

func normalizeGroup(name, groupType, status string) (string, string, string, error) {
	name, groupType, status = strings.TrimSpace(name), strings.TrimSpace(groupType), strings.TrimSpace(status)
	if name == "" {
		return "", "", "", errors.New("group name is required")
	}
	if groupType == "" {
		return "", "", "", errors.New("group type is required")
	}
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "inactive" {
		return "", "", "", errors.New("invalid group status")
	}
	return name, groupType, status, nil
}

func normalizeGroupParent(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateGroupParent(ctx context.Context, db *sql.DB, id string, parent *string) error {
	if parent == nil {
		return nil
	}
	if *parent == id {
		return ErrInvalidParent
	}
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE id=?`, *parent).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrInvalidParent
	}
	for cursor := parent; cursor != nil; {
		if *cursor == id {
			return ErrInvalidParent
		}
		var next *string
		if err := db.QueryRowContext(ctx, `SELECT parent_group_id FROM groups WHERE id=?`, *cursor).Scan(&next); err != nil {
			return ErrInvalidParent
		}
		cursor = next
	}
	return nil
}
