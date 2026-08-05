package system

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotificationNotFound   = errors.New("notification not found")
	ErrNotificationInvalid    = errors.New("invalid notification")
	ErrNotificationPermission = errors.New("notification permission denied")
)

type NotificationType struct {
	ID                    string  `json:"id"`
	Code                  string  `json:"code"`
	Name                  string  `json:"name"`
	Description           *string `json:"description"`
	Severity              int     `json:"severity"`
	RequiredPermissionKey *string `json:"required_permission_key"`
	IsActive              bool    `json:"is_active"`
}

type NotificationAudience struct {
	ID            string  `json:"id"`
	Scope         string  `json:"scope"`
	TargetGroupID *string `json:"target_group_id"`
	TargetRoleID  *string `json:"target_role_id"`
	TargetUserID  *string `json:"target_user_id"`
}

type Notification struct {
	ID           string                 `json:"id"`
	SenderUserID string                 `json:"sender_user_id"`
	Title        string                 `json:"title"`
	Message      string                 `json:"message"`
	Type         NotificationType       `json:"type"`
	Status       string                 `json:"status"`
	ShowFrom     string                 `json:"show_from"`
	ShowUntil    *string                `json:"show_until"`
	RetainUntil  *string                `json:"retain_until"`
	DurationCode string                 `json:"duration_code"`
	Audiences    []NotificationAudience `json:"audiences"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
	EditedAt     *string                `json:"edited_at"`
	HiddenAt     *string                `json:"hidden_at"`
	ExpiredAt    *string                `json:"expired_at"`
}

type CreateNotificationInput struct {
	Title        string                      `json:"title"`
	Message      string                      `json:"message"`
	TypeCode     string                      `json:"type"`
	DurationCode string                      `json:"show_time"`
	Audiences    []NotificationAudienceInput `json:"audiences"`
}

type UpdateNotificationInput struct {
	Title        *string                      `json:"title"`
	Message      *string                      `json:"message"`
	TypeCode     *string                      `json:"type"`
	DurationCode *string                      `json:"show_time"`
	Audiences    *[]NotificationAudienceInput `json:"audiences"`
}

type NotificationAudienceInput struct {
	Scope         string `json:"scope"`
	TargetGroupID string `json:"target_group_id"`
	TargetRoleID  string `json:"target_role_id"`
	TargetUserID  string `json:"target_user_id"`
}

type NotificationService struct {
	databasePath string
	users        *UserService
	now          func() time.Time
}

func NewNotificationService(databaseDirectory string, users *UserService) *NotificationService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &NotificationService{
		databasePath: filepath.Join(databaseDirectory, "notification.db"),
		users:        users,
		now:          time.Now,
	}
}

func (s *NotificationService) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *NotificationService) ListTypes(ctx context.Context, query ListQuery) (ListResult[NotificationType], error) {
	query, _, err := NormalizeListQuery(query, "severity", "asc", map[string]string{
		"severity":  "severity",
		"code":      "code",
		"name":      "name",
		"is_active": "is_active",
	})
	if err != nil {
		return ListResult[NotificationType]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[NotificationType]{}, err
	}
	defer db.Close()
	where := []string{"1=1"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(code LIKE ? COLLATE NOCASE OR name LIKE ? COLLATE NOCASE OR description LIKE ? COLLATE NOCASE OR required_permission_key LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if value := strings.TrimSpace(query.Filters["is_active"]); value != "" {
		where = append(where, "is_active = ?")
		args = append(args, boolFilter(value))
	}
	if value := strings.TrimSpace(query.Filters["required_permission_key"]); value != "" {
		where = append(where, "required_permission_key = ?")
		args = append(args, value)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_types WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[NotificationType]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT id,code,name,description,severity,required_permission_key,is_active
		FROM notification_types WHERE `+whereSQL+` ORDER BY `+map[string]string{"severity": "severity", "code": "code", "name": "name", "is_active": "is_active"}[query.Sort]+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[NotificationType]{}, err
	}
	defer rows.Close()
	var types []NotificationType
	for rows.Next() {
		var item NotificationType
		var active int
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Description, &item.Severity, &item.RequiredPermissionKey, &active); err != nil {
			return ListResult[NotificationType]{}, err
		}
		item.IsActive = active == 1
		types = append(types, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult[NotificationType]{}, err
	}
	return NewListResult(types, query, total), nil
}

func (s *NotificationService) Create(ctx context.Context, senderUserID string, input CreateNotificationInput) (*Notification, error) {
	title, message := strings.TrimSpace(input.Title), strings.TrimSpace(input.Message)
	duration := strings.TrimSpace(input.DurationCode)
	if title == "" || message == "" || duration == "" || len(input.Audiences) == 0 {
		return nil, ErrNotificationInvalid
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
	typ, err := notificationTypeByCode(ctx, tx, input.TypeCode)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeNotification(ctx, senderUserID, typ, input.Audiences); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	stamp := now.Format(time.RFC3339)
	showUntil, retainUntil, err := notificationWindow(now, duration)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO notifications
		(id,sender_user_id,title,message,type_id,status,show_from,show_until,retain_until,duration_code,created_at,updated_at)
		VALUES(?,?,?,?,?,'active',?,?,?,?,?,?)`,
		id, senderUserID, title, message, typ.ID, stamp, showUntil, retainUntil, duration, stamp, stamp); err != nil {
		return nil, err
	}
	if err := replaceNotificationAudiences(ctx, tx, id, input.Audiences, stamp); err != nil {
		return nil, err
	}
	if err := insertNotificationEvent(ctx, tx, id, senderUserID, "created", stamp, nil); err != nil {
		return nil, err
	}
	if err := insertNotificationEvent(ctx, tx, id, senderUserID, "published", stamp, nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *NotificationService) Update(ctx context.Context, actorUserID, id string, input UpdateNotificationInput) (*Notification, error) {
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
	current, err := notificationByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if current.SenderUserID != actorUserID {
		return nil, ErrNotificationPermission
	}
	title, message, duration, typeCode := current.Title, current.Message, current.DurationCode, current.Type.Code
	if input.Title != nil {
		title = strings.TrimSpace(*input.Title)
	}
	if input.Message != nil {
		message = strings.TrimSpace(*input.Message)
	}
	if input.DurationCode != nil {
		duration = strings.TrimSpace(*input.DurationCode)
	}
	if input.TypeCode != nil {
		typeCode = strings.TrimSpace(*input.TypeCode)
	}
	if title == "" || message == "" {
		return nil, ErrNotificationInvalid
	}
	audiences := notificationInputsFromAudiences(current.Audiences)
	if input.Audiences != nil {
		audiences = *input.Audiences
		if len(audiences) == 0 {
			return nil, ErrNotificationInvalid
		}
	}
	typ, err := notificationTypeByCode(ctx, tx, typeCode)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeNotification(ctx, actorUserID, typ, audiences); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	stamp := now.Format(time.RFC3339)
	showUntil, retainUntil, err := notificationWindow(now, duration)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notifications SET
		title=?,message=?,type_id=?,status='edited',show_until=?,retain_until=?,duration_code=?,updated_at=?,edited_at=?
		WHERE id=?`, title, message, typ.ID, showUntil, retainUntil, duration, stamp, stamp, id); err != nil {
		return nil, err
	}
	if input.Audiences != nil {
		if err := replaceNotificationAudiences(ctx, tx, id, audiences, stamp); err != nil {
			return nil, err
		}
	}
	if err := insertNotificationEvent(ctx, tx, id, actorUserID, "edited", stamp, nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *NotificationService) Hide(ctx context.Context, actorUserID, id string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := notificationByID(ctx, tx, id)
	if err != nil {
		return err
	}
	if current.SenderUserID != actorUserID {
		return ErrNotificationPermission
	}
	stamp := s.now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE notifications SET status='hidden',hidden_at=?,updated_at=? WHERE id=?`, stamp, stamp, id); err != nil {
		return err
	}
	if err := insertNotificationEvent(ctx, tx, id, actorUserID, "hidden", stamp, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *NotificationService) Get(ctx context.Context, id string) (*Notification, error) {
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
	item, err := notificationByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return item, tx.Commit()
}

func (s *NotificationService) ListForUser(ctx context.Context, userID string, query ListQuery) (ListResult[Notification], error) {
	query, err := normalizeNotificationListQuery(query)
	if err != nil {
		return ListResult[Notification]{}, err
	}
	items, err := s.listForUserAll(ctx, userID)
	if err != nil {
		return ListResult[Notification]{}, err
	}
	items = filterSortNotifications(items, query)
	total := len(items)
	items = pageNotifications(items, query)
	return NewListResult(items, query, total), nil
}

func (s *NotificationService) listForUserAll(ctx context.Context, userID string) ([]Notification, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	userDB, err := s.users.open()
	if err != nil {
		return nil, err
	}
	defer userDB.Close()
	now := s.now().UTC().Format(time.RFC3339)
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT n.id FROM notifications n
		JOIN notification_audiences a ON a.notification_id=n.id
		WHERE n.status IN ('active','edited')
		AND n.show_from <= ?
		AND (n.show_until IS NULL OR n.show_until > ?)
		AND (
			a.scope='organization'
			OR (a.scope='user' AND a.target_user_id=?)
		)
		ORDER BY n.updated_at DESC`, now, now, userID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
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
	groupIDs, roleIDs, err := notificationUserAudienceIDs(ctx, userDB, userID)
	if err != nil {
		return nil, err
	}
	extra, err := notificationAudienceMatches(ctx, db, groupIDs, roleIDs, now)
	if err != nil {
		return nil, err
	}
	for _, id := range extra {
		if !containsString(ids, id) {
			ids = append(ids, id)
		}
	}
	items := make([]Notification, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *NotificationService) ListAll(ctx context.Context, query ListQuery) (ListResult[Notification], error) {
	query, err := normalizeNotificationListQuery(query)
	if err != nil {
		return ListResult[Notification]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[Notification]{}, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id FROM notifications ORDER BY created_at DESC, id`)
	if err != nil {
		return ListResult[Notification]{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ListResult[Notification]{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return ListResult[Notification]{}, err
	}
	items := make([]Notification, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(ctx, id)
		if err != nil {
			return ListResult[Notification]{}, err
		}
		items = append(items, *item)
	}
	items = filterSortNotifications(items, query)
	total := len(items)
	items = pageNotifications(items, query)
	return NewListResult(items, query, total), nil
}

func normalizeNotificationListQuery(query ListQuery) (ListQuery, error) {
	query, _, err := NormalizeListQuery(query, "updated_at", "desc", map[string]string{
		"created_at": "created_at",
		"updated_at": "updated_at",
		"show_from":  "show_from",
		"show_until": "show_until",
		"status":     "status",
		"type":       "type",
		"title":      "title",
	})
	return query, err
}

func filterSortNotifications(items []Notification, query ListQuery) []Notification {
	filtered := make([]Notification, 0, len(items))
	for _, item := range items {
		if query.Keyword != "" {
			keyword := strings.ToLower(query.Keyword)
			if !strings.Contains(strings.ToLower(item.Title), keyword) &&
				!strings.Contains(strings.ToLower(item.Message), keyword) &&
				!strings.Contains(strings.ToLower(item.Type.Code), keyword) {
				continue
			}
		}
		if value := strings.TrimSpace(query.Filters["status"]); value != "" && item.Status != value {
			continue
		}
		if value := strings.TrimSpace(query.Filters["type"]); value != "" && item.Type.Code != value {
			continue
		}
		if value := strings.TrimSpace(query.Filters["sender_user_id"]); value != "" && item.SenderUserID != value {
			continue
		}
		if value := strings.TrimSpace(query.Filters["audience_scope"]); value != "" && !notificationHasAudienceScope(item, value) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		less := notificationLess(filtered[i], filtered[j], query.Sort)
		if query.Order == "desc" {
			return !less && filtered[i].ID != filtered[j].ID
		}
		return less
	})
	return filtered
}

func notificationLess(a, b Notification, field string) bool {
	switch field {
	case "created_at":
		return a.CreatedAt < b.CreatedAt || (a.CreatedAt == b.CreatedAt && a.ID < b.ID)
	case "show_from":
		return a.ShowFrom < b.ShowFrom || (a.ShowFrom == b.ShowFrom && a.ID < b.ID)
	case "show_until":
		return optionalString(a.ShowUntil) < optionalString(b.ShowUntil) || (optionalString(a.ShowUntil) == optionalString(b.ShowUntil) && a.ID < b.ID)
	case "status":
		return a.Status < b.Status || (a.Status == b.Status && a.ID < b.ID)
	case "type":
		return a.Type.Code < b.Type.Code || (a.Type.Code == b.Type.Code && a.ID < b.ID)
	case "title":
		return strings.ToLower(a.Title) < strings.ToLower(b.Title) || (strings.EqualFold(a.Title, b.Title) && a.ID < b.ID)
	default:
		return a.UpdatedAt < b.UpdatedAt || (a.UpdatedAt == b.UpdatedAt && a.ID < b.ID)
	}
}

func pageNotifications(items []Notification, query ListQuery) []Notification {
	start := ListOffset(query)
	if start >= len(items) {
		return []Notification{}
	}
	end := start + query.PageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func notificationHasAudienceScope(item Notification, scope string) bool {
	for _, audience := range item.Audiences {
		if audience.Scope == scope {
			return true
		}
	}
	return false
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *NotificationService) ExpireAndCleanup(ctx context.Context) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	stamp := s.now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `UPDATE notifications SET status='expired',expired_at=?,updated_at=?
		WHERE status IN ('active','edited') AND show_until IS NOT NULL AND show_until <= ?`, stamp, stamp, stamp); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM notifications WHERE status='expired' AND retain_until IS NOT NULL AND retain_until <= ?`, stamp)
	return err
}

func (s *NotificationService) ExportCSV(ctx context.Context, month string, writer io.Writer) error {
	if _, err := time.Parse("2006-01", month); err != nil {
		return ErrNotificationInvalid
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT n.id,n.sender_user_id,n.title,n.message,t.code,n.status,n.show_from,
		COALESCE(n.show_until,''),n.duration_code,n.created_at,n.updated_at
		FROM notifications n JOIN notification_types t ON t.id=n.type_id
		WHERE substr(n.created_at,1,7)=? ORDER BY n.created_at`, month)
	if err != nil {
		return err
	}
	defer rows.Close()
	cw := csv.NewWriter(writer)
	if err := cw.Write([]string{"id", "sender_user_id", "title", "message", "type", "status", "show_from", "show_until", "duration", "created_at", "updated_at"}); err != nil {
		return err
	}
	for rows.Next() {
		record := make([]string, 11)
		if err := rows.Scan(&record[0], &record[1], &record[2], &record[3], &record[4], &record[5], &record[6], &record[7], &record[8], &record[9], &record[10]); err != nil {
			return err
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func (s *NotificationService) authorizeNotification(ctx context.Context, userID string, typ *NotificationType, audiences []NotificationAudienceInput) error {
	keys, err := s.users.EffectivePermissionKeys(ctx, userID)
	if err != nil {
		return err
	}
	has := func(key string) bool { return containsString(keys, key) }
	if typ.RequiredPermissionKey != nil && !has(*typ.RequiredPermissionKey) {
		return ErrNotificationPermission
	}
	for _, audience := range audiences {
		switch strings.TrimSpace(audience.Scope) {
		case "organization":
			if !has("notifications.send.organization") {
				return ErrNotificationPermission
			}
		case "group":
			if !has("notifications.send.group") {
				return ErrNotificationPermission
			}
		case "own_group":
			if !has("notifications.send.own_group") {
				return ErrNotificationPermission
			}
			member, err := s.userInGroup(ctx, userID, audience.TargetGroupID)
			if err != nil {
				return err
			}
			if !member {
				return ErrNotificationPermission
			}
		case "role", "user":
			if !has("notifications.send.group") && !has("notifications.send.organization") {
				return ErrNotificationPermission
			}
		default:
			return ErrNotificationInvalid
		}
	}
	return nil
}

func (s *NotificationService) userInGroup(ctx context.Context, userID, groupID string) (bool, error) {
	db, err := s.users.open()
	if err != nil {
		return false, err
	}
	defer db.Close()
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_groups
		WHERE user_id=? AND group_id=? AND left_at IS NULL`, userID, strings.TrimSpace(groupID)).Scan(&count)
	return count > 0, err
}

func notificationWindow(now time.Time, duration string) (any, any, error) {
	var until time.Time
	switch duration {
	case "hour":
		until = now.Add(time.Hour)
	case "day":
		until = now.AddDate(0, 0, 1)
	case "week":
		until = now.AddDate(0, 0, 7)
	case "month":
		until = now.AddDate(0, 1, 0)
	case "year":
		until = now.AddDate(1, 0, 0)
	case "forever":
		return nil, nil, nil
	default:
		return nil, nil, ErrNotificationInvalid
	}
	return until.Format(time.RFC3339), until.AddDate(0, 3, 0).Format(time.RFC3339), nil
}

func notificationTypeByCode(ctx context.Context, tx *sql.Tx, code string) (*NotificationType, error) {
	var item NotificationType
	var active int
	err := tx.QueryRowContext(ctx, `SELECT id,code,name,description,severity,required_permission_key,is_active
		FROM notification_types WHERE code=? AND is_active=1`, strings.TrimSpace(code)).Scan(
		&item.ID, &item.Code, &item.Name, &item.Description, &item.Severity, &item.RequiredPermissionKey, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotificationInvalid
	}
	if err != nil {
		return nil, err
	}
	item.IsActive = active == 1
	return &item, nil
}

func notificationByID(ctx context.Context, tx *sql.Tx, id string) (*Notification, error) {
	var item Notification
	var active int
	err := tx.QueryRowContext(ctx, `SELECT n.id,n.sender_user_id,n.title,n.message,n.status,n.show_from,n.show_until,
		n.retain_until,n.duration_code,n.created_at,n.updated_at,n.edited_at,n.hidden_at,n.expired_at,
		t.id,t.code,t.name,t.description,t.severity,t.required_permission_key,t.is_active
		FROM notifications n JOIN notification_types t ON t.id=n.type_id WHERE n.id=?`, id).Scan(
		&item.ID, &item.SenderUserID, &item.Title, &item.Message, &item.Status, &item.ShowFrom, &item.ShowUntil,
		&item.RetainUntil, &item.DurationCode, &item.CreatedAt, &item.UpdatedAt, &item.EditedAt, &item.HiddenAt, &item.ExpiredAt,
		&item.Type.ID, &item.Type.Code, &item.Type.Name, &item.Type.Description, &item.Type.Severity, &item.Type.RequiredPermissionKey, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotificationNotFound
	}
	if err != nil {
		return nil, err
	}
	item.Type.IsActive = active == 1
	rows, err := tx.QueryContext(ctx, `SELECT id,scope,target_group_id,target_role_id,target_user_id
		FROM notification_audiences WHERE notification_id=? ORDER BY created_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var audience NotificationAudience
		if err := rows.Scan(&audience.ID, &audience.Scope, &audience.TargetGroupID, &audience.TargetRoleID, &audience.TargetUserID); err != nil {
			return nil, err
		}
		item.Audiences = append(item.Audiences, audience)
	}
	return &item, rows.Err()
}

func replaceNotificationAudiences(ctx context.Context, tx *sql.Tx, notificationID string, audiences []NotificationAudienceInput, stamp string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM notification_audiences WHERE notification_id=?`, notificationID); err != nil {
		return err
	}
	for _, audience := range audiences {
		scope := strings.TrimSpace(audience.Scope)
		groupID, roleID, userID := nullableTrim(audience.TargetGroupID), nullableTrim(audience.TargetRoleID), nullableTrim(audience.TargetUserID)
		if _, err := tx.ExecContext(ctx, `INSERT INTO notification_audiences
			(id,notification_id,scope,target_group_id,target_role_id,target_user_id,created_at)
			VALUES(?,?,?,?,?,?,?)`, uuid.NewString(), notificationID, scope, groupID, roleID, userID, stamp); err != nil {
			return fmt.Errorf("%w: %v", ErrNotificationInvalid, err)
		}
	}
	return nil
}

func insertNotificationEvent(ctx context.Context, tx *sql.Tx, notificationID, actorID, eventType, stamp string, note *string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_events
		(id,notification_id,actor_user_id,event_type,occurred_at,note)
		VALUES(?,?,?,?,?,?)`, uuid.NewString(), notificationID, actorID, eventType, stamp, note)
	return err
}

func notificationInputsFromAudiences(items []NotificationAudience) []NotificationAudienceInput {
	inputs := make([]NotificationAudienceInput, 0, len(items))
	for _, item := range items {
		input := NotificationAudienceInput{Scope: item.Scope}
		if item.TargetGroupID != nil {
			input.TargetGroupID = *item.TargetGroupID
		}
		if item.TargetRoleID != nil {
			input.TargetRoleID = *item.TargetRoleID
		}
		if item.TargetUserID != nil {
			input.TargetUserID = *item.TargetUserID
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func notificationUserAudienceIDs(ctx context.Context, userDB *sql.DB, userID string) ([]string, []string, error) {
	groupRows, err := userDB.QueryContext(ctx, `SELECT group_id FROM user_groups WHERE user_id=? AND left_at IS NULL`, userID)
	if err != nil {
		return nil, nil, err
	}
	var groupIDs []string
	for groupRows.Next() {
		var id string
		if err := groupRows.Scan(&id); err != nil {
			groupRows.Close()
			return nil, nil, err
		}
		groupIDs = append(groupIDs, id)
	}
	if err := groupRows.Close(); err != nil {
		return nil, nil, err
	}
	roleRows, err := userDB.QueryContext(ctx, `SELECT role_id FROM user_roles WHERE user_id=?`, userID)
	if err != nil {
		return nil, nil, err
	}
	var roleIDs []string
	for roleRows.Next() {
		var id string
		if err := roleRows.Scan(&id); err != nil {
			roleRows.Close()
			return nil, nil, err
		}
		roleIDs = append(roleIDs, id)
	}
	if err := roleRows.Close(); err != nil {
		return nil, nil, err
	}
	return groupIDs, roleIDs, roleRows.Err()
}

func notificationAudienceMatches(ctx context.Context, db *sql.DB, groupIDs, roleIDs []string, now string) ([]string, error) {
	clauses := []string{}
	args := []any{}
	if len(groupIDs) > 0 {
		clauses = append(clauses, "(a.scope IN ('group','own_group') AND a.target_group_id IN ("+questionMarks(len(groupIDs))+"))")
		for _, id := range groupIDs {
			args = append(args, id)
		}
	}
	if len(roleIDs) > 0 {
		clauses = append(clauses, "(a.scope='role' AND a.target_role_id IN ("+questionMarks(len(roleIDs))+"))")
		for _, id := range roleIDs {
			args = append(args, id)
		}
	}
	if len(clauses) == 0 {
		return nil, nil
	}
	query := `SELECT DISTINCT a.notification_id FROM notification_audiences a
		JOIN notifications n ON n.id=a.notification_id
		WHERE n.status IN ('active','edited')
		AND n.show_from <= ?
		AND (n.show_until IS NULL OR n.show_until > ?)
		AND (` + strings.Join(clauses, " OR ") + `)`
	args = append([]any{now, now}, args...)
	rows, err := db.QueryContext(ctx, query, args...)
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

func questionMarks(count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = "?"
	}
	return strings.Join(values, ",")
}

func nullableTrim(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
