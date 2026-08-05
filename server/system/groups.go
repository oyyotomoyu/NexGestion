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
	ErrGroupNotFound  = errors.New("group not found")
	ErrGroupInUse     = errors.New("group has child groups")
	ErrInvalidParent  = errors.New("invalid parent group")
	ErrGroupAccess    = errors.New("group manager access is required")
	ErrMemberNotFound = errors.New("group member not found")
)

type Group struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	OrganizationLevel *int    `json:"organization_level"`
	ParentGroupID     *string `json:"parent_group_id"`
	Status            string  `json:"status"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	MemberCount       int     `json:"member_count"`
	ManagerRoleID     string  `json:"manager_role_id"`
	MemberRoleID      string  `json:"member_role_id"`
}

type GroupMember struct {
	UserID                string  `json:"user_id"`
	DisplayName           string  `json:"display_name"`
	Email                 string  `json:"email"`
	Role                  string  `json:"role"`
	Title                 *string `json:"title"`
	JoinedAt              *string `json:"joined_at"`
	IsPrimaryOrganization bool    `json:"is_primary_organization"`
}

type SetGroupMemberInput struct {
	Role                  string  `json:"role"`
	Title                 *string `json:"title"`
	JoinedAt              *string `json:"joined_at"`
	IsPrimaryOrganization bool    `json:"is_primary_organization"`
}

type CreateGroupInput struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	OrganizationLevel *int    `json:"organization_level"`
	ParentGroupID     *string `json:"parent_group_id"`
	Status            string  `json:"status"`
}

type UpdateGroupInput struct {
	Name              *string `json:"name"`
	Type              *string `json:"type"`
	OrganizationLevel *int    `json:"organization_level"`
	ParentGroupID     *string `json:"parent_group_id"`
	Status            *string `json:"status"`
}

func (s *UserService) ListGroups(ctx context.Context, query ListQuery) (ListResult[Group], error) {
	query, sortExpression, err := NormalizeListQuery(query, "name", "asc", map[string]string{
		"name":               "g.name COLLATE NOCASE",
		"type":               "g.type",
		"organization_level": "g.organization_level",
		"status":             "g.status",
		"member_count":       "member_count",
		"created_at":         "g.created_at",
		"updated_at":         "g.updated_at",
	})
	if err != nil {
		return ListResult[Group]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[Group]{}, err
	}
	defer db.Close()
	where := []string{"1=1"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(g.name LIKE ? COLLATE NOCASE OR EXISTS (
			SELECT 1 FROM group_roles gr JOIN roles r ON r.id = gr.role_id
			WHERE gr.group_id = g.id AND r.name LIKE ? COLLATE NOCASE
		))`)
		args = append(args, pattern, pattern)
	}
	for _, filter := range []string{"type", "status", "parent_group_id", "organization_level"} {
		if value := strings.TrimSpace(query.Filters[filter]); value != "" {
			where = append(where, "g."+filter+" = ?")
			args = append(args, value)
		}
	}
	whereSQL := strings.Join(where, " AND ")
	base := `FROM groups g
		LEFT JOIN (SELECT group_id, COUNT(*) member_count FROM user_groups WHERE left_at IS NULL GROUP BY group_id) mc ON mc.group_id = g.id`
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) `+base+` WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[Group]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT g.id, COALESCE(mc.member_count,0) member_count `+base+`
		WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, g.id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[Group]{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		var memberCount int
		if err := rows.Scan(&id, &memberCount); err != nil {
			rows.Close()
			return ListResult[Group]{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return ListResult[Group]{}, err
	}
	result := make([]Group, 0, len(ids))
	for _, id := range ids {
		group, err := getGroup(ctx, db, id)
		if err != nil {
			return ListResult[Group]{}, err
		}
		result = append(result, *group)
	}
	return NewListResult(result, query, total), nil
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
	err := db.QueryRowContext(ctx, `SELECT g.id, g.name, g.type, g.organization_level, g.parent_group_id, g.status,
		g.created_at, g.updated_at, COUNT(ug.user_id)
		FROM groups g LEFT JOIN user_groups ug ON ug.group_id = g.id AND ug.left_at IS NULL
		WHERE g.id = ? GROUP BY g.id`, id).Scan(&group.ID, &group.Name, &group.Type,
		&group.OrganizationLevel, &group.ParentGroupID, &group.Status, &group.CreatedAt, &group.UpdatedAt, &group.MemberCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT role_id, kind FROM group_roles WHERE group_id = ?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var roleID, kind string
		if err := rows.Scan(&roleID, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == "manager" {
			group.ManagerRoleID = roleID
		} else {
			group.MemberRoleID = roleID
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return &group, nil
}

func (s *UserService) CreateGroup(ctx context.Context, actorUserID string, input CreateGroupInput) (*Group, error) {
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
	if err := validateGroupStructure(ctx, db, "", groupType, input.OrganizationLevel, parent); err != nil {
		return nil, err
	}
	now, id := time.Now().UTC().Format(time.RFC3339), uuid.NewString()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO groups (id, name, type, organization_level, parent_group_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, name, groupType, input.OrganizationLevel, parent, status, now, now); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	managerRoleID, memberRoleID := uuid.NewString(), uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO roles (id, name, description, is_system, grants_all_permissions, created_at, updated_at) VALUES
		(?, ?, ?, 1, 0, ?, ?), (?, ?, ?, 1, 0, ?, ?)`,
		managerRoleID, name+" Manager", "Manages members of the "+name+" group", now, now,
		memberRoleID, name+" Member", "Member of the "+name+" group", now, now); err != nil {
		return nil, fmt.Errorf("create group roles: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO group_roles (group_id, role_id, kind) VALUES (?, ?, 'manager'), (?, ?, 'member')`,
		id, managerRoleID, id, memberRoleID); err != nil {
		return nil, fmt.Errorf("link group roles: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getGroup(ctx, db, id)
}

func (s *UserService) UpdateGroup(ctx context.Context, actorUserID, id string, input UpdateGroupInput) (*Group, error) {
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
	level := current.OrganizationLevel
	if input.Name != nil {
		name = *input.Name
	}
	if input.Type != nil {
		groupType = *input.Type
	}
	if input.OrganizationLevel != nil {
		level = input.OrganizationLevel
	}
	if input.Status != nil {
		status = *input.Status
	}
	if input.ParentGroupID != nil {
		parent = normalizeGroupParent(input.ParentGroupID)
	}
	if strings.TrimSpace(groupType) == "project" {
		level, parent = nil, nil
	}
	name, groupType, status, err = normalizeGroup(name, groupType, status)
	if err != nil {
		return nil, err
	}
	if groupType != current.Type && current.MemberCount > 0 {
		return nil, errors.New("group type cannot change while the group has members")
	}
	if err := validateGroupStructure(ctx, db, id, groupType, level, parent); err != nil {
		return nil, err
	}
	var invalidChildren int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE parent_group_id=? AND
		(type<>'organization' OR organization_level<>?)`, id, valueOrZero(level)+1).Scan(&invalidChildren); err != nil {
		return nil, err
	}
	if invalidChildren > 0 {
		return nil, ErrInvalidParent
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE groups SET name=?, type=?, organization_level=?, parent_group_id=?, status=?, updated_at=? WHERE id=?`,
		name, groupType, level, parent, status, now, id); err != nil {
		return nil, fmt.Errorf("update group: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE roles SET name=? || CASE gr.kind WHEN 'manager' THEN ' Manager' ELSE ' Member' END, description=CASE gr.kind WHEN 'manager' THEN 'Manages members of the ' || ? || ' group' ELSE 'Member of the ' || ? || ' group' END, updated_at=? FROM group_roles gr WHERE roles.id=gr.role_id AND gr.group_id=?`, name, name, name, now, id); err != nil {
		return nil, fmt.Errorf("update group roles: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getGroup(ctx, db, id)
}

func (s *UserService) DeleteGroup(ctx context.Context, actorUserID, id string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	var exists, children int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE id=?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrGroupNotFound
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE parent_group_id=?`, id).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return ErrGroupInUse
	}
	rows, err := db.QueryContext(ctx, `SELECT role_id FROM group_roles WHERE group_id=?`, id)
	if err != nil {
		return err
	}
	var roleIDs []string
	for rows.Next() {
		var roleID string
		if err := rows.Scan(&roleID); err != nil {
			rows.Close()
			return err
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE role_id IN (SELECT role_id FROM group_roles WHERE group_id=?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_groups WHERE group_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_permissions WHERE group_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_roles WHERE group_id=?`, id); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE id=?`, roleID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM groups WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserService) ListGroupMembers(ctx context.Context, actorUserID, groupID string, query ListQuery) (ListResult[GroupMember], error) {
	query, sortExpression, err := NormalizeListQuery(query, "display_name", "asc", map[string]string{
		"display_name": "u.display_name COLLATE NOCASE",
		"email":        "u.email COLLATE NOCASE",
		"role":         "gr.kind",
		"title":        "ug.title COLLATE NOCASE",
		"joined_at":    "ug.joined_at",
	})
	if err != nil {
		return ListResult[GroupMember]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[GroupMember]{}, err
	}
	defer db.Close()
	if _, err := getGroup(ctx, db, groupID); err != nil {
		return ListResult[GroupMember]{}, err
	}
	where := []string{"ug.group_id=?", "ug.left_at IS NULL"}
	args := []any{groupID}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(u.display_name LIKE ? COLLATE NOCASE OR u.email LIKE ? COLLATE NOCASE OR ug.title LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern, pattern)
	}
	if role := strings.TrimSpace(query.Filters["role"]); role != "" {
		where = append(where, "gr.kind = ?")
		args = append(args, role)
	}
	if primary := strings.TrimSpace(query.Filters["is_primary_organization"]); primary != "" {
		where = append(where, "ug.is_primary_organization = ?")
		args = append(args, boolFilter(primary))
	}
	whereSQL := strings.Join(where, " AND ")
	base := `FROM user_groups ug JOIN users u ON u.id=ug.user_id
		JOIN group_roles gr ON gr.group_id=ug.group_id
		JOIN user_roles ur ON ur.user_id=u.id AND ur.role_id=gr.role_id`
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) `+base+` WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[GroupMember]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT u.id, u.display_name, u.email, gr.kind, ug.title, ug.joined_at,ug.is_primary_organization
		FROM user_groups ug JOIN users u ON u.id=ug.user_id
		JOIN group_roles gr ON gr.group_id=ug.group_id
		JOIN user_roles ur ON ur.user_id=u.id AND ur.role_id=gr.role_id
		WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, u.id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[GroupMember]{}, err
	}
	defer rows.Close()
	result := []GroupMember{}
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.Email, &m.Role, &m.Title, &m.JoinedAt, &m.IsPrimaryOrganization); err != nil {
			return ListResult[GroupMember]{}, err
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return ListResult[GroupMember]{}, err
	}
	return NewListResult(result, query, total), nil
}

func (s *UserService) SetGroupMember(ctx context.Context, actorUserID, groupID, userID string, input SetGroupMemberInput) (*GroupMember, error) {
	roleKind := strings.ToLower(strings.TrimSpace(input.Role))
	if roleKind == "" {
		roleKind = "member"
	}
	if roleKind != "manager" && roleKind != "member" {
		return nil, errors.New("invalid group member role")
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	group, err := getGroup(ctx, db, groupID)
	if err != nil {
		return nil, err
	}
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id=? AND deleted_at IS NULL`, userID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrUserNotFound
	}
	var roleID string
	if err := db.QueryRowContext(ctx, `SELECT role_id FROM group_roles WHERE group_id=? AND kind=?`, groupID, roleKind).Scan(&roleID); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	joinedAt := input.JoinedAt
	if joinedAt == nil {
		joinedAt = &now
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	isPrimary := input.IsPrimaryOrganization && group.Type == "organization"
	if group.Type == "organization" && !isPrimary {
		var primaryCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_groups WHERE user_id=? AND is_primary_organization=1 AND left_at IS NULL`, userID).Scan(&primaryCount); err != nil {
			return nil, err
		}
		isPrimary = primaryCount == 0
	}
	if isPrimary {
		if _, err := tx.ExecContext(ctx, `UPDATE user_groups SET is_primary_organization=0 WHERE user_id=?`, userID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_groups (user_id, group_id, title, joined_at, left_at, is_primary_organization, created_at) VALUES (?, ?, ?, ?, NULL, ?, ?)
		ON CONFLICT(user_id, group_id) DO UPDATE SET title=excluded.title, joined_at=excluded.joined_at, left_at=NULL,is_primary_organization=excluded.is_primary_organization`, userID, groupID, normalizeOptionalText(input.Title), joinedAt, isPrimary, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id=? AND role_id IN (SELECT role_id FROM group_roles WHERE group_id=?)`, userID, groupID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles (user_id, role_id, created_at) VALUES (?, ?, ?)`, userID, roleID, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var member GroupMember
	err = db.QueryRowContext(ctx, `SELECT u.id,u.display_name,u.email,?,ug.title,ug.joined_at,ug.is_primary_organization FROM users u JOIN user_groups ug ON ug.user_id=u.id WHERE u.id=? AND ug.group_id=?`, roleKind, userID, groupID).Scan(&member.UserID, &member.DisplayName, &member.Email, &member.Role, &member.Title, &member.JoinedAt, &member.IsPrimaryOrganization)
	return &member, err
}

func (s *UserService) RemoveGroupMember(ctx context.Context, actorUserID, groupID, userID string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := getGroup(ctx, db, groupID); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var wasPrimary bool
	if err := tx.QueryRowContext(ctx, `SELECT is_primary_organization FROM user_groups WHERE group_id=? AND user_id=?`, groupID, userID).Scan(&wasPrimary); errors.Is(err, sql.ErrNoRows) {
		return ErrMemberNotFound
	} else if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM user_groups WHERE group_id=? AND user_id=?`, groupID, userID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrMemberNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id=? AND role_id IN (SELECT role_id FROM group_roles WHERE group_id=?)`, userID, groupID); err != nil {
		return err
	}
	if wasPrimary {
		if _, err := tx.ExecContext(ctx, `UPDATE user_groups SET is_primary_organization=1
			WHERE user_id=? AND group_id=(
				SELECT ug.group_id FROM user_groups ug JOIN groups g ON g.id=ug.group_id
				WHERE ug.user_id=? AND ug.left_at IS NULL AND g.type='organization' AND g.status='active'
				ORDER BY g.organization_level DESC,g.name COLLATE NOCASE LIMIT 1
			)`, userID, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeGroup(name, groupType, status string) (string, string, string, error) {
	name, groupType, status = strings.TrimSpace(name), strings.TrimSpace(groupType), strings.TrimSpace(status)
	if name == "" {
		return "", "", "", errors.New("group name is required")
	}
	if groupType == "" {
		return "", "", "", errors.New("group type is required")
	}
	if groupType != "organization" && groupType != "project" {
		return "", "", "", errors.New("invalid group type: must be organization or project")
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

func validateGroupStructure(ctx context.Context, db *sql.DB, id, groupType string, level *int, parent *string) error {
	if groupType == "project" {
		if level != nil || parent != nil {
			return ErrInvalidParent
		}
		return nil
	}
	if level == nil || *level < 1 || *level > 5 {
		return errors.New("invalid organization level: must be between 1 and 5")
	}
	if *level == 1 {
		if parent != nil {
			return ErrInvalidParent
		}
		return nil
	}
	if parent == nil {
		return ErrInvalidParent
	}
	if *parent == id {
		return ErrInvalidParent
	}
	var parentType string
	var parentLevel *int
	var parentStatus string
	if err := db.QueryRowContext(ctx, `SELECT type,organization_level,status FROM groups WHERE id=?`, *parent).Scan(&parentType, &parentLevel, &parentStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidParent
		}
		return err
	}
	if parentType != "organization" || parentStatus != "active" || parentLevel == nil || *parentLevel != *level-1 {
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

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
