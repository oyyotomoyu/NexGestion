package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HR permission keys. See docs/System/hr-system.md Section 6.
const (
	PermissionHRAccess                    = "hr.access"
	PermissionHREmploymentReadSelf        = "hr.employment.read.self"
	PermissionHREmploymentRead            = "hr.employment.read"
	PermissionHREmploymentManage          = "hr.employment.manage"
	PermissionHRTasksManage               = "hr.tasks.manage"
	PermissionHRPerformanceReadSelf       = "hr.performance.read.self"
	PermissionHRPerformanceRead           = "hr.performance.read"
	PermissionHRPerformanceCyclesManage   = "hr.performance.cycles.manage"
	PermissionHRPerformanceReview         = "hr.performance.review"
	PermissionHREmployeeRelationsReadSelf = "hr.employee_relations.read.self"
	PermissionHREmployeeRelationsManage   = "hr.employee_relations.manage"
	PermissionHREmployeeRelationsRead     = "hr.employee_relations.read"
)

const (
	hrSettingProbationRequired    = "probation_required"
	hrSettingProbationDefaultDays = "probation_default_days"
	hrSettingRatingScale          = "performance_rating_scale"
)

const hrDateLayout = "2006-01-02"

var (
	ErrHRNotFound          = errors.New("hr record not found")
	ErrHRInvalidInput      = errors.New("invalid hr input")
	ErrHRInvalidTransition = errors.New("invalid hr status transition")
	ErrHRForbidden         = errors.New("hr access denied")
)

// HRActor is the authenticated caller together with their effective
// permissions. HR visibility depends on more than the single route
// permission (reporting line, case assignment), so the service needs both.
type HRActor struct {
	UserID      string
	Permissions map[string]bool
}

func NewHRActor(userID string, permissionKeys []string) HRActor {
	permissions := make(map[string]bool, len(permissionKeys))
	for _, key := range permissionKeys {
		permissions[key] = true
	}
	return HRActor{UserID: userID, Permissions: permissions}
}

func (a HRActor) Has(permission string) bool { return a.Permissions[permission] }

type HRSettings struct {
	ProbationRequired      bool     `json:"probation_required"`
	ProbationDefaultDays   int      `json:"probation_default_days"`
	PerformanceRatingScale []string `json:"performance_rating_scale"`
}

type UpdateHRSettingsInput struct {
	ProbationRequired      *bool     `json:"probation_required"`
	ProbationDefaultDays   *int      `json:"probation_default_days"`
	PerformanceRatingScale *[]string `json:"performance_rating_scale"`
}

type HRService struct {
	databasePath  string
	users         *UserService
	notifications *NotificationService
	now           func() time.Time
}

func NewHRService(databaseDirectory string, users *UserService, notifications *NotificationService) *HRService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &HRService{
		databasePath:  filepath.Join(databaseDirectory, "hr.db"),
		users:         users,
		notifications: notifications,
		now:           time.Now,
	}
}

func (s *HRService) open() (*sql.DB, error) {
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

func (s *HRService) timestamp() string { return s.now().UTC().Format(time.RFC3339) }

func (s *HRService) Settings(ctx context.Context) (*HRSettings, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return readHRSettings(ctx, db)
}

type hrQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func readHRSettings(ctx context.Context, db hrQueryer) (*HRSettings, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM hr_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := &HRSettings{ProbationRequired: true, ProbationDefaultDays: 90, PerformanceRatingScale: []string{}}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		switch key {
		case hrSettingProbationRequired:
			settings.ProbationRequired = value == "true"
		case hrSettingProbationDefaultDays:
			if days, err := strconv.Atoi(value); err == nil {
				settings.ProbationDefaultDays = days
			}
		case hrSettingRatingScale:
			_ = json.Unmarshal([]byte(value), &settings.PerformanceRatingScale)
		}
	}
	return settings, rows.Err()
}

// UpdateSettings changes organization-level HR configuration. Probation
// settings require hr.employment.manage; the rating scale requires
// hr.performance.cycles.manage.
func (s *HRService) UpdateSettings(ctx context.Context, actor HRActor, input UpdateHRSettingsInput) (*HRSettings, error) {
	if (input.ProbationRequired != nil || input.ProbationDefaultDays != nil) && !actor.Has(PermissionHREmploymentManage) {
		return nil, ErrHRForbidden
	}
	if input.PerformanceRatingScale != nil && !actor.Has(PermissionHRPerformanceCyclesManage) {
		return nil, ErrHRForbidden
	}
	updates := map[string]string{}
	if input.ProbationRequired != nil {
		updates[hrSettingProbationRequired] = strconv.FormatBool(*input.ProbationRequired)
	}
	if input.ProbationDefaultDays != nil {
		if *input.ProbationDefaultDays < 1 || *input.ProbationDefaultDays > 3650 {
			return nil, fmt.Errorf("%w: probation_default_days must be between 1 and 3650", ErrHRInvalidInput)
		}
		updates[hrSettingProbationDefaultDays] = strconv.Itoa(*input.ProbationDefaultDays)
	}
	if input.PerformanceRatingScale != nil {
		scale := []string{}
		seen := map[string]bool{}
		for _, value := range *input.PerformanceRatingScale {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				return nil, fmt.Errorf("%w: performance_rating_scale values must be non-empty and unique", ErrHRInvalidInput)
			}
			seen[value] = true
			scale = append(scale, value)
		}
		if len(scale) < 2 {
			return nil, fmt.Errorf("%w: performance_rating_scale needs at least two values", ErrHRInvalidInput)
		}
		encoded, _ := json.Marshal(scale)
		updates[hrSettingRatingScale] = string(encoded)
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for key, value := range updates {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hr_settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return nil, err
		}
	}
	if len(updates) > 0 {
		if err := s.audit(ctx, tx, "settings", "hr_settings", "updated", actor.UserID, updates); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return readHRSettings(ctx, db)
}

// audit appends one Section 8 change-history row in the caller's transaction.
func (s *HRService) audit(ctx context.Context, tx *sql.Tx, entityType, entityID, action, actorUserID string, detail any) error {
	encoded, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO hr_audit_log (id, entity_type, entity_id, action, actor_user_id, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), entityType, entityID, action, actorUserID, string(encoded), s.timestamp())
	return err
}

// notifyUser sends a pointer-only notification (Section 7): it names the
// record but never embeds HR content. Delivery is best-effort because the
// HR change has already been committed.
func (s *HRService) notifyUser(ctx context.Context, userID, title, message string) {
	if s.notifications == nil || userID == "" {
		return
	}
	_, _ = s.notifications.Create(ctx, adminUserID, CreateNotificationInput{
		Title:        title,
		Message:      message,
		TypeCode:     "info",
		DurationCode: "month",
		Audiences:    []NotificationAudienceInput{{Scope: "user", TargetUserID: userID}},
	})
}

func parseHRDate(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if _, err := time.Parse(hrDateLayout, value); err != nil {
		return "", fmt.Errorf("%w: %s must be formatted as YYYY-MM-DD", ErrHRInvalidInput, field)
	}
	return value, nil
}

func parseOptionalHRDate(field string, value *string) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := parseHRDate(field, *value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func trimmedOrNil(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
