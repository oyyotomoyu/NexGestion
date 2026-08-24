package system

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultTemplateFileMaxBytes    int64 = 20 * 1024 * 1024
	defaultTemplateStorageMaxBytes int64 = 500 * 1024 * 1024

	templateFileMaxBytesKey    = "template_file_max_bytes"
	templateStorageMaxBytesKey = "template_storage_max_bytes"

	templateManagePermission = "templates.manage"
)

var (
	ErrTemplateNotFound     = errors.New("template file not found")
	ErrTemplateInvalid      = errors.New("invalid template file")
	ErrTemplatePermission   = errors.New("template file permission denied")
	ErrTemplateFileTooLarge = errors.New("template file exceeds the maximum file size")
	ErrTemplateStorageFull  = errors.New("template storage limit exceeded")
)

type TemplateAudience struct {
	ID            string  `json:"id"`
	Scope         string  `json:"scope"`
	TargetGroupID *string `json:"target_group_id"`
	TargetRoleID  *string `json:"target_role_id"`
	TargetUserID  *string `json:"target_user_id"`
}

type TemplateAudienceInput struct {
	Scope         string `json:"scope"`
	TargetGroupID string `json:"target_group_id"`
	TargetRoleID  string `json:"target_role_id"`
	TargetUserID  string `json:"target_user_id"`
}

type TemplateFile struct {
	ID               string             `json:"id"`
	OriginalFilename string             `json:"original_filename"`
	StoredPath       string             `json:"-"`
	ContentType      string             `json:"content_type"`
	SizeBytes        int64              `json:"size_bytes"`
	ChecksumSHA256   string             `json:"checksum_sha256"`
	Description      *string            `json:"description"`
	UploadedByUserID string             `json:"uploaded_by_user_id"`
	Audiences        []TemplateAudience `json:"audiences"`
	CreatedAt        string             `json:"created_at"`
	UpdatedAt        string             `json:"updated_at"`
}

type UploadTemplateInput struct {
	Description string                  `json:"description"`
	Audiences   []TemplateAudienceInput `json:"audiences"`
}

type TemplateStorageUsage struct {
	UsedBytes    int64 `json:"used_bytes"`
	MaxBytes     int64 `json:"max_bytes"`
	MaxFileBytes int64 `json:"max_file_bytes"`
	FileCount    int   `json:"file_count"`
}

type TemplateService struct {
	databasePath       string
	systemDatabasePath string
	storageRoot        string
	users              *UserService
	now                func() time.Time
}

func NewTemplateService(databaseDirectory, storageRoot string, users *UserService) *TemplateService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	if strings.TrimSpace(storageRoot) == "" {
		storageRoot = "template"
	}
	return &TemplateService{
		databasePath:       filepath.Join(databaseDirectory, "template.db"),
		systemDatabasePath: filepath.Join(databaseDirectory, "system.db"),
		storageRoot:        storageRoot,
		users:              users,
		now:                time.Now,
	}
}

func (s *TemplateService) open() (*sql.DB, error) {
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

func (s *TemplateService) EnsureRoot() error {
	return os.MkdirAll(s.storageRoot, 0o755)
}

// MaxFileBytes returns the configured per-file size limit from system_settings.
func (s *TemplateService) MaxFileBytes(ctx context.Context) (int64, error) {
	return s.settingInt(ctx, templateFileMaxBytesKey, defaultTemplateFileMaxBytes)
}

// MaxStorageBytes returns the configured total template storage limit from system_settings.
func (s *TemplateService) MaxStorageBytes(ctx context.Context) (int64, error) {
	return s.settingInt(ctx, templateStorageMaxBytesKey, defaultTemplateStorageMaxBytes)
}

func (s *TemplateService) settingInt(ctx context.Context, key string, fallback int64) (int64, error) {
	db, err := sql.Open("sqlite", s.systemDatabasePath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return 0, err
	}
	var value string
	err = db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key=?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback, nil
	}
	return parsed, nil
}

// Upload stores a new template file on disk and records it, provided the
// uploader supplied at least one audience and the file fits within the
// configured per-file and total-storage limits.
func (s *TemplateService) Upload(ctx context.Context, uploaderUserID string, input UploadTemplateInput, filename, contentType string, content io.Reader) (*TemplateFile, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || len(input.Audiences) == 0 {
		return nil, ErrTemplateInvalid
	}
	if err := validateTemplateAudiences(input.Audiences); err != nil {
		return nil, err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	maxFile, err := s.MaxFileBytes(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.EnsureRoot(); err != nil {
		return nil, err
	}

	id := uuid.NewString()
	storedName := id + strings.ToLower(filepath.Ext(filename))
	storedPath := filepath.Join(s.storageRoot, storedName)

	written, checksum, err := writeTemplateFile(storedPath, content, maxFile)
	if err != nil {
		return nil, err
	}

	db, err := s.open()
	if err != nil {
		os.Remove(storedPath)
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		os.Remove(storedPath)
		return nil, err
	}
	defer tx.Rollback()

	maxTotal, err := s.MaxStorageBytes(ctx)
	if err != nil {
		os.Remove(storedPath)
		return nil, err
	}
	var currentTotal int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes),0) FROM template_files`).Scan(&currentTotal); err != nil {
		os.Remove(storedPath)
		return nil, err
	}
	if currentTotal+written > maxTotal {
		os.Remove(storedPath)
		return nil, ErrTemplateStorageFull
	}

	now := s.now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO template_files
		(id,original_filename,stored_path,content_type,size_bytes,checksum_sha256,description,uploaded_by_user_id,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, filename, storedName, contentType, written, checksum, nullableTrim(input.Description), uploaderUserID, now, now); err != nil {
		os.Remove(storedPath)
		return nil, err
	}
	if err := insertTemplateAudiences(ctx, tx, id, input.Audiences, now); err != nil {
		os.Remove(storedPath)
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		os.Remove(storedPath)
		return nil, err
	}
	return s.Get(ctx, id)
}

// writeTemplateFile copies content to disk, rejecting the write once it
// exceeds maxBytes so an oversized upload cannot exhaust disk space.
func writeTemplateFile(storedPath string, content io.Reader, maxBytes int64) (int64, string, error) {
	out, err := os.OpenFile(storedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, "", err
	}
	hasher := sha256.New()
	limited := io.LimitReader(content, maxBytes+1)
	written, copyErr := io.Copy(io.MultiWriter(out, hasher), limited)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(storedPath)
		return 0, "", copyErr
	}
	if closeErr != nil {
		os.Remove(storedPath)
		return 0, "", closeErr
	}
	if written > maxBytes {
		os.Remove(storedPath)
		return 0, "", ErrTemplateFileTooLarge
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func validateTemplateAudiences(audiences []TemplateAudienceInput) error {
	for _, audience := range audiences {
		switch strings.TrimSpace(audience.Scope) {
		case "organization":
			if strings.TrimSpace(audience.TargetGroupID) != "" || strings.TrimSpace(audience.TargetRoleID) != "" || strings.TrimSpace(audience.TargetUserID) != "" {
				return ErrTemplateInvalid
			}
		case "group":
			if strings.TrimSpace(audience.TargetGroupID) == "" {
				return ErrTemplateInvalid
			}
		case "role":
			if strings.TrimSpace(audience.TargetRoleID) == "" {
				return ErrTemplateInvalid
			}
		case "user":
			if strings.TrimSpace(audience.TargetUserID) == "" {
				return ErrTemplateInvalid
			}
		default:
			return ErrTemplateInvalid
		}
	}
	return nil
}

func insertTemplateAudiences(ctx context.Context, tx *sql.Tx, templateFileID string, audiences []TemplateAudienceInput, stamp string) error {
	for _, audience := range audiences {
		scope := strings.TrimSpace(audience.Scope)
		groupID, roleID, userID := nullableTrim(audience.TargetGroupID), nullableTrim(audience.TargetRoleID), nullableTrim(audience.TargetUserID)
		if _, err := tx.ExecContext(ctx, `INSERT INTO template_file_audiences
			(id,template_file_id,scope,target_group_id,target_role_id,target_user_id,created_at)
			VALUES(?,?,?,?,?,?,?)`, uuid.NewString(), templateFileID, scope, groupID, roleID, userID, stamp); err != nil {
			return fmt.Errorf("%w: %v", ErrTemplateInvalid, err)
		}
	}
	return nil
}

func (s *TemplateService) Get(ctx context.Context, id string) (*TemplateFile, error) {
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
	item, err := templateFileByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return item, tx.Commit()
}

func templateFileByID(ctx context.Context, tx *sql.Tx, id string) (*TemplateFile, error) {
	var item TemplateFile
	err := tx.QueryRowContext(ctx, `SELECT id,original_filename,stored_path,content_type,size_bytes,checksum_sha256,description,uploaded_by_user_id,created_at,updated_at
		FROM template_files WHERE id=?`, id).Scan(
		&item.ID, &item.OriginalFilename, &item.StoredPath, &item.ContentType, &item.SizeBytes, &item.ChecksumSHA256,
		&item.Description, &item.UploadedByUserID, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTemplateNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,scope,target_group_id,target_role_id,target_user_id
		FROM template_file_audiences WHERE template_file_id=? ORDER BY created_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var audience TemplateAudience
		if err := rows.Scan(&audience.ID, &audience.Scope, &audience.TargetGroupID, &audience.TargetRoleID, &audience.TargetUserID); err != nil {
			return nil, err
		}
		item.Audiences = append(item.Audiences, audience)
	}
	return &item, rows.Err()
}

// ListVisible returns every template file for a caller with templates.manage,
// or only the files whose audience matches the caller otherwise.
func (s *TemplateService) ListVisible(ctx context.Context, userID string, query ListQuery) (ListResult[TemplateFile], error) {
	keys, err := s.users.EffectivePermissionKeys(ctx, userID)
	if err != nil {
		return ListResult[TemplateFile]{}, err
	}
	if containsString(keys, templateManagePermission) {
		return s.ListAll(ctx, query)
	}
	return s.ListForUser(ctx, userID, query)
}

func (s *TemplateService) ListForUser(ctx context.Context, userID string, query ListQuery) (ListResult[TemplateFile], error) {
	query, err := normalizeTemplateListQuery(query)
	if err != nil {
		return ListResult[TemplateFile]{}, err
	}
	items, err := s.listForUserAll(ctx, userID)
	if err != nil {
		return ListResult[TemplateFile]{}, err
	}
	items = filterSortTemplates(items, query)
	total := len(items)
	items = pageTemplates(items, query)
	return NewListResult(items, query, total), nil
}

func (s *TemplateService) listForUserAll(ctx context.Context, userID string) ([]TemplateFile, error) {
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
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT f.id FROM template_files f
		JOIN template_file_audiences a ON a.template_file_id=f.id
		WHERE a.scope='organization' OR (a.scope='user' AND a.target_user_id=?)`, userID)
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
	extra, err := templateAudienceMatches(ctx, db, groupIDs, roleIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range extra {
		if !containsString(ids, id) {
			ids = append(ids, id)
		}
	}
	items := make([]TemplateFile, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *TemplateService) ListAll(ctx context.Context, query ListQuery) (ListResult[TemplateFile], error) {
	query, err := normalizeTemplateListQuery(query)
	if err != nil {
		return ListResult[TemplateFile]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[TemplateFile]{}, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id FROM template_files ORDER BY created_at DESC, id`)
	if err != nil {
		return ListResult[TemplateFile]{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ListResult[TemplateFile]{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return ListResult[TemplateFile]{}, err
	}
	items := make([]TemplateFile, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(ctx, id)
		if err != nil {
			return ListResult[TemplateFile]{}, err
		}
		items = append(items, *item)
	}
	items = filterSortTemplates(items, query)
	total := len(items)
	items = pageTemplates(items, query)
	return NewListResult(items, query, total), nil
}

func templateAudienceMatches(ctx context.Context, db *sql.DB, groupIDs, roleIDs []string) ([]string, error) {
	clauses := []string{}
	args := []any{}
	if len(groupIDs) > 0 {
		clauses = append(clauses, "(a.scope='group' AND a.target_group_id IN ("+questionMarks(len(groupIDs))+"))")
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
	query := `SELECT DISTINCT a.template_file_id FROM template_file_audiences a
		WHERE ` + strings.Join(clauses, " OR ")
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

func normalizeTemplateListQuery(query ListQuery) (ListQuery, error) {
	query, _, err := NormalizeListQuery(query, "created_at", "desc", map[string]string{
		"created_at":        "created_at",
		"updated_at":        "updated_at",
		"original_filename": "original_filename",
		"size_bytes":        "size_bytes",
	})
	return query, err
}

func filterSortTemplates(items []TemplateFile, query ListQuery) []TemplateFile {
	filtered := make([]TemplateFile, 0, len(items))
	for _, item := range items {
		if query.Keyword != "" {
			keyword := strings.ToLower(query.Keyword)
			if !strings.Contains(strings.ToLower(item.OriginalFilename), keyword) &&
				!strings.Contains(strings.ToLower(optionalString(item.Description)), keyword) {
				continue
			}
		}
		if value := strings.TrimSpace(query.Filters["uploaded_by_user_id"]); value != "" && item.UploadedByUserID != value {
			continue
		}
		if value := strings.TrimSpace(query.Filters["audience_scope"]); value != "" && !templateHasAudienceScope(item, value) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		less := templateLess(filtered[i], filtered[j], query.Sort)
		if query.Order == "desc" {
			return !less && filtered[i].ID != filtered[j].ID
		}
		return less
	})
	return filtered
}

func templateLess(a, b TemplateFile, field string) bool {
	switch field {
	case "original_filename":
		return strings.ToLower(a.OriginalFilename) < strings.ToLower(b.OriginalFilename) ||
			(strings.EqualFold(a.OriginalFilename, b.OriginalFilename) && a.ID < b.ID)
	case "size_bytes":
		return a.SizeBytes < b.SizeBytes || (a.SizeBytes == b.SizeBytes && a.ID < b.ID)
	case "updated_at":
		return a.UpdatedAt < b.UpdatedAt || (a.UpdatedAt == b.UpdatedAt && a.ID < b.ID)
	default:
		return a.CreatedAt < b.CreatedAt || (a.CreatedAt == b.CreatedAt && a.ID < b.ID)
	}
}

func templateHasAudienceScope(item TemplateFile, scope string) bool {
	for _, audience := range item.Audiences {
		if audience.Scope == scope {
			return true
		}
	}
	return false
}

func pageTemplates(items []TemplateFile, query ListQuery) []TemplateFile {
	start := ListOffset(query)
	if start >= len(items) {
		return []TemplateFile{}
	}
	end := start + query.PageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// DownloadPath resolves the file location for a caller, allowing it only when
// the caller holds templates.manage or the file's audience matches them.
func (s *TemplateService) DownloadPath(ctx context.Context, userID, id string) (path, filename string, err error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return "", "", err
	}
	keys, err := s.users.EffectivePermissionKeys(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if !containsString(keys, templateManagePermission) {
		visible, err := s.itemVisibleToUser(ctx, *item, userID)
		if err != nil {
			return "", "", err
		}
		if !visible {
			return "", "", ErrTemplatePermission
		}
	}
	return filepath.Join(s.storageRoot, item.StoredPath), item.OriginalFilename, nil
}

func (s *TemplateService) itemVisibleToUser(ctx context.Context, item TemplateFile, userID string) (bool, error) {
	for _, audience := range item.Audiences {
		if audience.Scope == "organization" {
			return true, nil
		}
		if audience.Scope == "user" && audience.TargetUserID != nil && *audience.TargetUserID == userID {
			return true, nil
		}
	}
	userDB, err := s.users.open()
	if err != nil {
		return false, err
	}
	defer userDB.Close()
	groupIDs, roleIDs, err := notificationUserAudienceIDs(ctx, userDB, userID)
	if err != nil {
		return false, err
	}
	for _, audience := range item.Audiences {
		if audience.Scope == "group" && audience.TargetGroupID != nil && containsString(groupIDs, *audience.TargetGroupID) {
			return true, nil
		}
		if audience.Scope == "role" && audience.TargetRoleID != nil && containsString(roleIDs, *audience.TargetRoleID) {
			return true, nil
		}
	}
	return false, nil
}

// Delete removes a template file. The original uploader may delete their own
// upload; anyone else needs templates.manage.
func (s *TemplateService) Delete(ctx context.Context, actorUserID, id string) error {
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
	item, err := templateFileByID(ctx, tx, id)
	if err != nil {
		return err
	}
	if item.UploadedByUserID != actorUserID {
		keys, err := s.users.EffectivePermissionKeys(ctx, actorUserID)
		if err != nil {
			return err
		}
		if !containsString(keys, templateManagePermission) {
			return ErrTemplatePermission
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM template_files WHERE id=?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.storageRoot, item.StoredPath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *TemplateService) StorageUsage(ctx context.Context) (*TemplateStorageUsage, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var used int64
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes),0),COUNT(*) FROM template_files`).Scan(&used, &count); err != nil {
		return nil, err
	}
	maxTotal, err := s.MaxStorageBytes(ctx)
	if err != nil {
		return nil, err
	}
	maxFile, err := s.MaxFileBytes(ctx)
	if err != nil {
		return nil, err
	}
	return &TemplateStorageUsage{UsedBytes: used, MaxBytes: maxTotal, MaxFileBytes: maxFile, FileCount: count}, nil
}
