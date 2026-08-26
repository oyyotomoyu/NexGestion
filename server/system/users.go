package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrCurrentPasswordWrong = errors.New("current password is incorrect")
)

type User struct {
	ID                 string           `json:"id"`
	DisplayName        string           `json:"display_name"`
	Email              string           `json:"email"`
	Status             string           `json:"status"`
	Locale             *string          `json:"locale"`
	Timezone           *string          `json:"timezone"`
	MustChangePassword bool             `json:"must_change_password"`
	FailedLoginCount   int              `json:"failed_login_count"`
	LockedUntil        *string          `json:"locked_until"`
	LastLoginAt        *string          `json:"last_login_at"`
	PasswordChangedAt  *string          `json:"password_changed_at"`
	IsProtected        bool             `json:"is_protected"`
	CreatedAt          string           `json:"created_at"`
	UpdatedAt          string           `json:"updated_at"`
	DeletedAt          *string          `json:"deleted_at"`
	EmployeeProfile    *EmployeeProfile `json:"employee_profile"`
	Roles              []Role           `json:"roles"`
	Groups             []UserGroup      `json:"groups"`
}

type EmployeeProfile struct {
	ID                string  `json:"id"`
	UserID            string  `json:"user_id"`
	EmployeeNumber    string  `json:"employee_number"`
	LegalName         *string `json:"legal_name"`
	PreferredName     *string `json:"preferred_name"`
	WorkEmail         *string `json:"work_email"`
	WorkPhone         *string `json:"work_phone"`
	JobTitle          *string `json:"job_title"`
	EmploymentStatus  string  `json:"employment_status"`
	HireDate          *string `json:"hire_date"`
	TerminationDate   *string `json:"termination_date"`
	ManagerEmployeeID *string `json:"manager_employee_id"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type Role struct {
	ID                   string       `json:"id"`
	Title                string       `json:"title"`
	Description          *string      `json:"description"`
	IsSystem             bool         `json:"is_system"`
	GrantsAllPermissions bool         `json:"grants_all_permissions"`
	Permissions          []Permission `json:"permissions"`
}

type UserGroup struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Status   string  `json:"status"`
	Title    *string `json:"title"`
	JoinedAt *string `json:"joined_at"`
	LeftAt   *string `json:"left_at"`
}

type CreateUserInput struct {
	DisplayName        string  `json:"display_name"`
	Email              string  `json:"email"`
	Password           string  `json:"password"`
	Status             string  `json:"status"`
	Locale             *string `json:"locale"`
	Timezone           *string `json:"timezone"`
	MustChangePassword *bool   `json:"must_change_password"`
}

type UpdateUserInput struct {
	DisplayName        *string `json:"display_name"`
	Email              *string `json:"email"`
	Password           *string `json:"password"`
	CurrentPassword    *string `json:"current_password"`
	Status             *string `json:"status"`
	Locale             *string `json:"locale"`
	Timezone           *string `json:"timezone"`
	MustChangePassword *bool   `json:"must_change_password"`
}

type UserService struct{ databasePath string }

func NewUserService(databaseDirectory string) *UserService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &UserService{databasePath: filepath.Join(databaseDirectory, "user.db")}
}

func (s *UserService) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *UserService) List(ctx context.Context, query ListQuery) (ListResult[User], error) {
	query, sortExpression, err := NormalizeListQuery(query, "created_at", "desc", map[string]string{
		"display_name":  "u.display_name COLLATE NOCASE",
		"email":         "u.email COLLATE NOCASE",
		"status":        "u.status",
		"created_at":    "u.created_at",
		"updated_at":    "u.updated_at",
		"last_login_at": "u.last_login_at",
	})
	if err != nil {
		return ListResult[User]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[User]{}, err
	}
	defer db.Close()
	where := []string{"u.deleted_at IS NULL"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(u.display_name LIKE ? COLLATE NOCASE OR u.email LIKE ? COLLATE NOCASE OR ep.employee_number LIKE ? COLLATE NOCASE OR ep.legal_name LIKE ? COLLATE NOCASE OR ep.preferred_name LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	if status := strings.TrimSpace(query.Filters["status"]); status != "" {
		where = append(where, "u.status = ?")
		args = append(args, status)
	}
	if roleID := strings.TrimSpace(query.Filters["role_id"]); roleID != "" {
		where = append(where, "EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id AND ur.role_id = ?)")
		args = append(args, roleID)
	}
	if groupID := strings.TrimSpace(query.Filters["group_id"]); groupID != "" {
		where = append(where, "EXISTS (SELECT 1 FROM user_groups ug WHERE ug.user_id = u.id AND ug.group_id = ? AND ug.left_at IS NULL)")
		args = append(args, groupID)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u LEFT JOIN employee_profiles ep ON ep.user_id = u.id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[User]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT u.id FROM users u LEFT JOIN employee_profiles ep ON ep.user_id = u.id
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
	if err := rows.Close(); err != nil {
		return ListResult[User]{}, err
	}
	users := make([]User, 0, len(ids))
	for _, id := range ids {
		user, err := getUser(ctx, db, id)
		if err != nil {
			return ListResult[User]{}, err
		}
		users = append(users, *user)
	}
	return NewListResult(users, query, total), nil
}

func (s *UserService) ListAll(ctx context.Context) ([]User, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id FROM users WHERE deleted_at IS NULL ORDER BY created_at, id`)
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
	users := make([]User, 0, len(ids))
	for _, id := range ids {
		user, err := getUser(ctx, db, id)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, nil
}

func (s *UserService) Get(ctx context.Context, id string) (*User, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return getUser(ctx, db, id)
}

func getUser(ctx context.Context, db *sql.DB, id string) (*User, error) {
	var u User
	var mustChange, protected int
	err := db.QueryRowContext(ctx, `SELECT id, display_name, email, status, locale, timezone,
		must_change_password, failed_login_count, locked_until, last_login_at, password_changed_at,
		is_protected, created_at, updated_at, deleted_at FROM users WHERE id = ? AND deleted_at IS NULL`, id).Scan(
		&u.ID, &u.DisplayName, &u.Email, &u.Status, &u.Locale, &u.Timezone, &mustChange,
		&u.FailedLoginCount, &u.LockedUntil, &u.LastLoginAt, &u.PasswordChangedAt, &protected,
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	u.MustChangePassword, u.IsProtected = mustChange == 1, protected == 1

	var e EmployeeProfile
	err = db.QueryRowContext(ctx, `SELECT id, user_id, employee_number, legal_name, preferred_name,
		work_email, work_phone, job_title, employment_status, hire_date, termination_date,
		manager_employee_id, created_at, updated_at FROM employee_profiles WHERE user_id = ?`, id).Scan(
		&e.ID, &e.UserID, &e.EmployeeNumber, &e.LegalName, &e.PreferredName, &e.WorkEmail,
		&e.WorkPhone, &e.JobTitle, &e.EmploymentStatus, &e.HireDate, &e.TerminationDate,
		&e.ManagerEmployeeID, &e.CreatedAt, &e.UpdatedAt)
	if err == nil {
		u.EmployeeProfile = &e
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	u.Roles = []Role{}
	rows, err := db.QueryContext(ctx, `SELECT r.id
		FROM roles r JOIN user_roles ur ON ur.role_id = r.id WHERE ur.user_id = ? ORDER BY r.name`, id)
	if err != nil {
		return nil, err
	}
	roleIDs := []string{}
	for rows.Next() {
		var roleID string
		if err := rows.Scan(&roleID); err != nil {
			rows.Close()
			return nil, err
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, roleID := range roleIDs {
		role, err := getRole(ctx, db, roleID)
		if err != nil {
			return nil, err
		}
		u.Roles = append(u.Roles, *role)
	}

	u.Groups = []UserGroup{}
	rows, err = db.QueryContext(ctx, `SELECT g.id, g.name, g.type, g.status, ug.title, ug.joined_at, ug.left_at
		FROM groups g JOIN user_groups ug ON ug.group_id = g.id WHERE ug.user_id = ? ORDER BY g.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var g UserGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Type, &g.Status, &g.Title, &g.JoinedAt, &g.LeftAt); err != nil {
			return nil, err
		}
		u.Groups = append(u.Groups, g)
	}
	return &u, rows.Err()
}

func (s *UserService) Create(ctx context.Context, actorUserID string, input CreateUserInput) (*User, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	email, err := normalizeUserEmail(input.Email)
	if err != nil {
		return nil, err
	}
	if input.DisplayName == "" {
		return nil, errors.New("display_name is required")
	}
	if len(input.Password) < 12 {
		return nil, errors.New("password must contain at least 12 characters")
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if !validUserStatus(input.Status) {
		return nil, errors.New("invalid user status")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	mustChange := true
	if input.MustChangePassword != nil {
		mustChange = *input.MustChangePassword
	}
	now, id := time.Now().UTC().Format(time.RFC3339), uuid.NewString()
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, display_name, email, password_hash, status, locale,
		timezone, must_change_password, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.DisplayName, email, string(hash), input.Status, input.Locale, input.Timezone, boolInt(mustChange), now, now)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return getUser(ctx, db, id)
}

func (s *UserService) Update(ctx context.Context, actorUserID, id string, input UpdateUserInput) (*User, error) {
	if actorUserID == id && (input.Status != nil || input.MustChangePassword != nil) {
		return nil, ErrPermissionDenied
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	current, err := getUser(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if actorUserID == id {
		if input.CurrentPassword == nil || strings.TrimSpace(*input.CurrentPassword) == "" {
			return nil, ErrCurrentPasswordWrong
		}
		if err := verifyUserPassword(ctx, db, id, *input.CurrentPassword); err != nil {
			return nil, err
		}
	}
	sets, args := []string{}, []any{}
	if input.DisplayName != nil {
		v := strings.TrimSpace(*input.DisplayName)
		if v == "" {
			return nil, errors.New("display_name cannot be empty")
		}
		sets = append(sets, "display_name = ?")
		args = append(args, v)
	}
	if input.Email != nil {
		v, err := normalizeUserEmail(*input.Email)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "email = ?")
		args = append(args, v)
	}
	if input.Status != nil {
		if current.IsProtected && *input.Status != "active" {
			return nil, errors.New("protected user must remain active")
		}
		if !validUserStatus(*input.Status) {
			return nil, errors.New("invalid user status")
		}
		sets = append(sets, "status = ?")
		args = append(args, *input.Status)
	}
	if input.Locale != nil {
		sets = append(sets, "locale = ?")
		args = append(args, strings.TrimSpace(*input.Locale))
	}
	if input.Timezone != nil {
		sets = append(sets, "timezone = ?")
		args = append(args, strings.TrimSpace(*input.Timezone))
	}
	if input.MustChangePassword != nil {
		sets = append(sets, "must_change_password = ?")
		args = append(args, boolInt(*input.MustChangePassword))
	}
	if input.Password != nil {
		if len(*input.Password) < 12 {
			return nil, errors.New("password must contain at least 12 characters")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*input.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "password_hash = ?", "password_changed_at = ?")
		args = append(args, string(hash), time.Now().UTC().Format(time.RFC3339))
	}
	if len(sets) == 0 {
		return getUser(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339), id)
	if _, err := db.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	if input.Password != nil {
		if _, err := db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), id); err != nil {
			return nil, err
		}
	}
	return getUser(ctx, db, id)
}

func verifyUserPassword(ctx context.Context, db *sql.DB, id, password string) error {
	if strings.TrimSpace(password) == "" {
		return ErrCurrentPasswordWrong
	}
	var passwordHash string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ? AND deleted_at IS NULL`, id).Scan(&passwordHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCurrentPasswordWrong
		}
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return ErrCurrentPasswordWrong
	}
	return nil
}

func (s *UserService) Delete(ctx context.Context, actorUserID, id string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.ExecContext(ctx, `UPDATE users SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrUserNotFound
	}
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, now, id); err != nil {
		return err
	}
	return nil
}

func normalizeUserEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return "", errors.New("invalid email")
	}
	return value, nil
}

func validUserStatus(value string) bool {
	return value == "pending" || value == "active" || value == "disabled" || value == "locked"
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
