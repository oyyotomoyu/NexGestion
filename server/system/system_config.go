package system

// System Config (docs/System/system-config.md): editing configuration from
// the UI without a code change or redeploy.
//
//   - Business catalogs (Section 3): the database is the runtime source of
//     truth; config/*.json is the seed for a fresh installation and the
//     "restore defaults" source.
//   - ODM branding (Section 4): odm/theme.json and odm/img/* stay the
//     build-time defaults baked into the client; administrator edits are
//     runtime overrides stored here and served to the client at startup, so a
//     change applies on the next page load without a rebuild.
//   - config/permission.json is route-coupled and only ever shown read-only.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var systemConfigSchema = []string{
	`CREATE TABLE IF NOT EXISTS config_leave_types (
		key TEXT PRIMARY KEY,
		label TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		retired INTEGER NOT NULL DEFAULT 0 CHECK (retired IN (0, 1)),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS config_branding_assets (
		name TEXT PRIMARY KEY,
		content_type TEXT NOT NULL,
		data BLOB NOT NULL,
		sha256 TEXT NOT NULL,
		updated_by_user_id TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS config_audit_events (
		id TEXT PRIMARY KEY,
		actor_user_id TEXT NOT NULL,
		surface TEXT NOT NULL,
		action TEXT NOT NULL,
		detail TEXT,
		occurred_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS config_audit_events_time ON config_audit_events(occurred_at)`,
}

const brandingColorsSettingKey = "branding.theme_colors"

var (
	ErrConfigNotFound = errors.New("configuration entry not found")
	ErrConfigInvalid  = errors.New("invalid configuration")
	ErrConfigConflict = errors.New("configuration conflict")
)

func configInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConfigInvalid, fmt.Sprintf(format, args...))
}

func configConflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConfigConflict, fmt.Sprintf(format, args...))
}

// seedLeaveTypeCatalog copies config/leave-types.json into the database the
// first time the catalog table is empty. After that the database is
// authoritative and the file is only read by RestoreLeaveTypeDefaults.
func seedLeaveTypeCatalog(ctx context.Context, tx *sql.Tx) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM config_leave_types`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults, err := LoadLeaveTypes()
	if err != nil {
		// No seed file (e.g. a stripped-down deployment): start empty and
		// let an administrator define the catalog.
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for index, item := range defaults {
		if _, err := tx.ExecContext(ctx, `INSERT INTO config_leave_types (key, label, sort_order, retired, created_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?)`, item.Key, item.Label, index+1, now, now); err != nil {
			return fmt.Errorf("seed leave type %q: %w", item.Key, err)
		}
	}
	return nil
}

type SystemConfigService struct {
	databaseDirectory string
	now               func() time.Time
}

func NewSystemConfigService(databaseDirectory string) *SystemConfigService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &SystemConfigService{databaseDirectory: databaseDirectory, now: func() time.Time { return time.Now().UTC() }}
}

func (s *SystemConfigService) open() (*sql.DB, error) {
	return openSQLite(filepath.Join(s.databaseDirectory, "system.db"))
}

func (s *SystemConfigService) timestamp() string { return s.now().Format(time.RFC3339) }

func recordConfigAudit(ctx context.Context, db sqlExecer, actorID, surface, action, detail string, now string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO config_audit_events (id, actor_user_id, surface, action, detail, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)`, uuid.NewString(), actorID, surface, action, nullIfEmpty(truncate(detail, 2000)), now)
	return err
}

type ConfigAuditEvent struct {
	ID          string  `json:"id"`
	ActorUserID string  `json:"actor_user_id"`
	Surface     string  `json:"surface"`
	Action      string  `json:"action"`
	Detail      *string `json:"detail"`
	OccurredAt  string  `json:"occurred_at"`
}

func (s *SystemConfigService) ListAudit(ctx context.Context, surface string, limit int) ([]ConfigAuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query, args := `SELECT id, actor_user_id, surface, action, detail, occurred_at FROM config_audit_events`, []any{}
	if surface != "" {
		query, args = query+` WHERE surface = ?`, append(args, surface)
	}
	rows, err := db.QueryContext(ctx, query+` ORDER BY occurred_at DESC, rowid DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []ConfigAuditEvent{}
	for rows.Next() {
		var event ConfigAuditEvent
		if err := rows.Scan(&event.ID, &event.ActorUserID, &event.Surface, &event.Action, &event.Detail, &event.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// ---------------------------------------------------------------------------
// Business catalog: leave types (Section 3)
// ---------------------------------------------------------------------------

type ConfigLeaveType struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order"`
	Retired   bool   `json:"retired"`
	InUse     int    `json:"in_use"`
	IsDefault bool   `json:"is_default"`
	UpdatedAt string `json:"updated_at"`
}

type CreateLeaveTypeInput struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type UpdateLeaveTypeInput struct {
	Label   *string `json:"label"`
	Retired *bool   `json:"retired"`
}

var leaveTypeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// ActiveLeaveTypes is what employees may choose from when applying for leave.
func ActiveLeaveTypes(ctx context.Context, databaseDirectory string) ([]LeaveType, error) {
	db, err := openSQLite(filepath.Join(databaseDirectory, "system.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT key, label FROM config_leave_types WHERE retired = 0 ORDER BY sort_order, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	types := []LeaveType{}
	for rows.Next() {
		var item LeaveType
		if err := rows.Scan(&item.Key, &item.Label); err != nil {
			return nil, err
		}
		types = append(types, item)
	}
	return types, rows.Err()
}

// leaveTypeUsage counts leave requests per type, so deleting an entry that
// historical records still reference can be refused (Section 3).
func (s *SystemConfigService) leaveTypeUsage(ctx context.Context) (map[string]int, error) {
	usage := map[string]int{}
	path := filepath.Join(s.databaseDirectory, "attendance.db")
	if _, err := os.Stat(path); err != nil {
		return usage, nil
	}
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT leave_type, COUNT(*) FROM leave_requests GROUP BY leave_type`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return usage, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		usage[key] = count
	}
	return usage, rows.Err()
}

func (s *SystemConfigService) ListLeaveTypes(ctx context.Context) ([]ConfigLeaveType, error) {
	usage, err := s.leaveTypeUsage(ctx)
	if err != nil {
		return nil, err
	}
	defaults := map[string]bool{}
	if seed, err := LoadLeaveTypes(); err == nil {
		for _, item := range seed {
			defaults[item.Key] = true
		}
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT key, label, sort_order, retired, updated_at FROM config_leave_types ORDER BY sort_order, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ConfigLeaveType{}
	for rows.Next() {
		var item ConfigLeaveType
		var retired int
		if err := rows.Scan(&item.Key, &item.Label, &item.SortOrder, &retired, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Retired, item.InUse, item.IsDefault = retired == 1, usage[item.Key], defaults[item.Key]
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SystemConfigService) CreateLeaveType(ctx context.Context, actorID string, input CreateLeaveTypeInput) ([]ConfigLeaveType, error) {
	key := strings.TrimSpace(input.Key)
	if !leaveTypeKeyPattern.MatchString(key) {
		return nil, configInvalid("key must start with a lowercase letter and contain only a-z, 0-9 and _ (max 50)")
	}
	label, err := validLeaveTypeLabel(input.Label)
	if err != nil {
		return nil, err
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
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM config_leave_types`).Scan(&next); err != nil {
		return nil, err
	}
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO config_leave_types (key, label, sort_order, retired, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?)`, key, label, next, now, now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, configConflict("leave type %q already exists (retired entries can be reactivated)", key)
		}
		return nil, err
	}
	if err := recordConfigAudit(ctx, tx, actorID, "catalog.leave_types", "create", fmt.Sprintf("key=%s label=%q", key, label), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListLeaveTypes(ctx)
}

// UpdateLeaveType edits the label or retires/reactivates an entry. The key is
// immutable because leave requests store it.
func (s *SystemConfigService) UpdateLeaveType(ctx context.Context, actorID, key string, input UpdateLeaveTypeInput) ([]ConfigLeaveType, error) {
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
	var label string
	var retired int
	err = tx.QueryRowContext(ctx, `SELECT label, retired FROM config_leave_types WHERE key = ?`, key).Scan(&label, &retired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConfigNotFound
	}
	if err != nil {
		return nil, err
	}
	changes := []string{}
	if input.Label != nil {
		next, err := validLeaveTypeLabel(*input.Label)
		if err != nil {
			return nil, err
		}
		if next != label {
			changes = append(changes, fmt.Sprintf("label %q -> %q", label, next))
			label = next
		}
	}
	if input.Retired != nil && boolInt(*input.Retired) != retired {
		if *input.Retired {
			var active int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM config_leave_types WHERE retired = 0 AND key <> ?`, key).Scan(&active); err != nil {
				return nil, err
			}
			if active == 0 {
				return nil, configConflict("at least one leave type must stay active")
			}
			changes = append(changes, "retired")
		} else {
			changes = append(changes, "reactivated")
		}
		retired = boolInt(*input.Retired)
	}
	if len(changes) == 0 {
		return s.ListLeaveTypes(ctx)
	}
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `UPDATE config_leave_types SET label = ?, retired = ?, updated_at = ? WHERE key = ?`, label, retired, now, key); err != nil {
		return nil, err
	}
	if err := recordConfigAudit(ctx, tx, actorID, "catalog.leave_types", "update", "key="+key+" "+strings.Join(changes, ", "), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListLeaveTypes(ctx)
}

// DeleteLeaveType removes an entry nothing references. An entry used by any
// leave request must be retired instead, so history keeps a valid key.
func (s *SystemConfigService) DeleteLeaveType(ctx context.Context, actorID, key string) ([]ConfigLeaveType, error) {
	usage, err := s.leaveTypeUsage(ctx)
	if err != nil {
		return nil, err
	}
	if usage[key] > 0 {
		return nil, configConflict("leave type %q is used by %d leave request(s); retire it instead", key, usage[key])
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
	var retired int
	err = tx.QueryRowContext(ctx, `SELECT retired FROM config_leave_types WHERE key = ?`, key).Scan(&retired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConfigNotFound
	}
	if err != nil {
		return nil, err
	}
	if retired == 0 {
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM config_leave_types WHERE retired = 0`).Scan(&active); err != nil {
			return nil, err
		}
		if active <= 1 {
			return nil, configConflict("at least one leave type must stay active")
		}
	}
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `DELETE FROM config_leave_types WHERE key = ?`, key); err != nil {
		return nil, err
	}
	if err := recordConfigAudit(ctx, tx, actorID, "catalog.leave_types", "delete", "key="+key, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListLeaveTypes(ctx)
}

// ReorderLeaveTypes sets the display order; keys must be exactly the
// catalog's keys.
func (s *SystemConfigService) ReorderLeaveTypes(ctx context.Context, actorID string, keys []string) ([]ConfigLeaveType, error) {
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
	rows, err := tx.QueryContext(ctx, `SELECT key FROM config_leave_types`)
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		existing[key] = true
	}
	rows.Close()
	seen := map[string]bool{}
	for _, key := range keys {
		if !existing[key] || seen[key] {
			return nil, configInvalid("keys must list every leave type exactly once")
		}
		seen[key] = true
	}
	if len(seen) != len(existing) {
		return nil, configInvalid("keys must list every leave type exactly once")
	}
	now := s.timestamp()
	for index, key := range keys {
		if _, err := tx.ExecContext(ctx, `UPDATE config_leave_types SET sort_order = ?, updated_at = ? WHERE key = ? AND sort_order <> ?`,
			index+1, now, key, index+1); err != nil {
			return nil, err
		}
	}
	if err := recordConfigAudit(ctx, tx, actorID, "catalog.leave_types", "reorder", strings.Join(keys, ","), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListLeaveTypes(ctx)
}

// RestoreLeaveTypeDefaults re-applies config/leave-types.json: every default
// entry gets its default label and position and is reactivated. Entries an
// administrator added are kept and placed after the defaults.
func (s *SystemConfigService) RestoreLeaveTypeDefaults(ctx context.Context, actorID string) ([]ConfigLeaveType, error) {
	defaults, err := LoadLeaveTypes()
	if err != nil {
		return nil, configConflict("the default catalog file is unavailable: %v", err)
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
	now := s.timestamp()
	defaultKeys := map[string]bool{}
	for index, item := range defaults {
		defaultKeys[item.Key] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO config_leave_types (key, label, sort_order, retired, created_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?) ON CONFLICT(key) DO UPDATE SET label = excluded.label, sort_order = excluded.sort_order,
			retired = 0, updated_at = excluded.updated_at`, item.Key, item.Label, index+1, now, now); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT key FROM config_leave_types ORDER BY sort_order, key`)
	if err != nil {
		return nil, err
	}
	var custom []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		if !defaultKeys[key] {
			custom = append(custom, key)
		}
	}
	rows.Close()
	for index, key := range custom {
		if _, err := tx.ExecContext(ctx, `UPDATE config_leave_types SET sort_order = ? WHERE key = ?`, len(defaults)+index+1, key); err != nil {
			return nil, err
		}
	}
	if err := recordConfigAudit(ctx, tx, actorID, "catalog.leave_types", "restore_defaults", "", now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListLeaveTypes(ctx)
}

func validLeaveTypeLabel(raw string) (string, error) {
	label := strings.TrimSpace(raw)
	if label == "" || len([]rune(label)) > 100 {
		return "", configInvalid("label is required and must be at most 100 characters")
	}
	return label, nil
}

// ---------------------------------------------------------------------------
// Permission catalog reference view (read-only, Section 2)
// ---------------------------------------------------------------------------

func (s *SystemConfigService) PermissionCatalog() ([]PermissionDefinition, error) {
	catalog, err := LoadPermissionCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.Permissions, nil
}

// ---------------------------------------------------------------------------
// ODM branding overrides (Section 4)
// ---------------------------------------------------------------------------

// BrandingColorTokens is NexColor's fixed token set
// (client/src/components/NexColor). An unknown key would be consumed by no
// component, so only these are accepted.
var BrandingColorTokens = []string{
	"primary", "primaryHover", "primarySoft", "secondary", "secondaryHover", "secondarySoft",
	"text", "heading", "content", "muted", "onPrimary", "background", "surface", "surfaceMuted",
	"border", "focusRing", "shadow", "overlay", "success", "warning", "danger", "info",
}

var brandingColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// brandingImageSpecs mirrors ui-architecture.md Section 13.
var brandingImageSpecs = map[string]struct {
	contentType string
	width       float64
	height      float64
}{
	"login-logo.svg":    {"image/svg+xml", 240, 56},
	"language-icon.svg": {"image/svg+xml", 24, 24},
	"favicon.ico":       {"image/x-icon", 32, 32},
}

const maxBrandingImageBytes = 256 << 10

type BrandingImage struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

// Branding is the public runtime override document. Colors holds only the
// overridden tokens; Images only the overridden files. Anything absent falls
// back to the defaults built into the client.
type Branding struct {
	Colors    map[string]string        `json:"colors"`
	Images    map[string]BrandingImage `json:"images"`
	UpdatedAt *string                  `json:"updated_at"`
}

func (s *SystemConfigService) Branding(ctx context.Context) (*Branding, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	branding := &Branding{Colors: map[string]string{}, Images: map[string]BrandingImage{}}
	var latest string
	var raw string
	err = db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = ?`, brandingColorsSettingKey).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if raw != "" {
		var stored struct {
			Colors    map[string]string `json:"colors"`
			UpdatedAt string            `json:"updated_at"`
		}
		if err := json.Unmarshal([]byte(raw), &stored); err == nil {
			for _, token := range BrandingColorTokens {
				if value, ok := stored.Colors[token]; ok {
					branding.Colors[token] = value
				}
			}
			latest = stored.UpdatedAt
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT name, sha256, updated_at FROM config_branding_assets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, sum, updatedAt string
		if err := rows.Scan(&name, &sum, &updatedAt); err != nil {
			return nil, err
		}
		// The hash in the URL busts browser caches when the file changes.
		branding.Images[name] = BrandingImage{Name: name, URL: "/api/branding/images/" + name + "?v=" + sum[:12], UpdatedAt: updatedAt}
		if updatedAt > latest {
			latest = updatedAt
		}
	}
	if latest != "" {
		branding.UpdatedAt = &latest
	}
	return branding, rows.Err()
}

// SetBrandingColors replaces the color override with the given tokens. An
// empty map removes the override (back to odm/theme.json).
func (s *SystemConfigService) SetBrandingColors(ctx context.Context, actorID string, colors map[string]string) (*Branding, error) {
	normalized := map[string]string{}
	allowed := map[string]bool{}
	for _, token := range BrandingColorTokens {
		allowed[token] = true
	}
	keys := make([]string, 0, len(colors))
	for token, value := range colors {
		if !allowed[token] {
			return nil, configInvalid("unknown color token %q", token)
		}
		value = strings.TrimSpace(value)
		if !brandingColorPattern.MatchString(value) {
			return nil, configInvalid("color %q must be #RRGGBB or #RRGGBBAA", token)
		}
		normalized[token] = strings.ToLower(value)
		keys = append(keys, token)
	}
	sort.Strings(keys)
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
	now := s.timestamp()
	action, detail := "reset_colors", ""
	if len(normalized) == 0 {
		_, err = tx.ExecContext(ctx, `DELETE FROM system_settings WHERE key = ?`, brandingColorsSettingKey)
	} else {
		encoded, _ := json.Marshal(map[string]any{"colors": normalized, "updated_at": now})
		_, err = tx.ExecContext(ctx, `INSERT INTO system_settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, brandingColorsSettingKey, string(encoded))
		parts := make([]string, 0, len(keys))
		for _, token := range keys {
			parts = append(parts, token+"="+normalized[token])
		}
		action, detail = "set_colors", strings.Join(parts, " ")
	}
	if err != nil {
		return nil, err
	}
	if err := recordConfigAudit(ctx, tx, actorID, "branding", action, detail, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Branding(ctx)
}

// SetBrandingImage validates and stores one ODM image override.
func (s *SystemConfigService) SetBrandingImage(ctx context.Context, actorID, name string, data []byte) (*Branding, error) {
	spec, ok := brandingImageSpecs[name]
	if !ok {
		return nil, ErrConfigNotFound
	}
	if len(data) == 0 {
		return nil, configInvalid("the uploaded file is empty")
	}
	if len(data) > maxBrandingImageBytes {
		return nil, configInvalid("the file must be at most %d KB", maxBrandingImageBytes>>10)
	}
	var err error
	if spec.contentType == "image/svg+xml" {
		err = validateBrandingSVG(data, spec.width, spec.height)
	} else {
		err = validateBrandingICO(data, int(spec.width))
	}
	if err != nil {
		return nil, configInvalid("%s: %v", name, err)
	}
	sum := sha256.Sum256(data)
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
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO config_branding_assets (name, content_type, data, sha256, updated_by_user_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(name) DO UPDATE SET content_type = excluded.content_type, data = excluded.data,
		sha256 = excluded.sha256, updated_by_user_id = excluded.updated_by_user_id, updated_at = excluded.updated_at`,
		name, spec.contentType, data, hex.EncodeToString(sum[:]), actorID, now); err != nil {
		return nil, err
	}
	if err := recordConfigAudit(ctx, tx, actorID, "branding", "set_image", fmt.Sprintf("name=%s bytes=%d sha256=%x", name, len(data), sum[:8]), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Branding(ctx)
}

func (s *SystemConfigService) ResetBrandingImage(ctx context.Context, actorID, name string) (*Branding, error) {
	if _, ok := brandingImageSpecs[name]; !ok {
		return nil, ErrConfigNotFound
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
	result, err := tx.ExecContext(ctx, `DELETE FROM config_branding_assets WHERE name = ?`, name)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count > 0 {
		if err := recordConfigAudit(ctx, tx, actorID, "branding", "reset_image", "name="+name, s.timestamp()); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Branding(ctx)
}

// BrandingImageData returns one stored override for serving.
func (s *SystemConfigService) BrandingImageData(ctx context.Context, name string) (contentType string, data []byte, etag string, err error) {
	if _, ok := brandingImageSpecs[name]; !ok {
		return "", nil, "", ErrConfigNotFound
	}
	db, err := s.open()
	if err != nil {
		return "", nil, "", err
	}
	defer db.Close()
	err = db.QueryRowContext(ctx, `SELECT content_type, data, sha256 FROM config_branding_assets WHERE name = ?`, name).Scan(&contentType, &data, &etag)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, "", ErrConfigNotFound
	}
	return contentType, data, etag, err
}

var (
	svgNumberSeparators = regexp.MustCompile(`[\s,]+`)
	svgURLReference     = regexp.MustCompile(`(?i)url\(\s*['"]?\s*([^)'"\s]*)`)
	svgForbiddenTags    = map[string]bool{"script": true, "foreignobject": true, "iframe": true, "embed": true, "object": true,
		"audio": true, "video": true, "handler": true, "listener": true}
)

// validateBrandingSVG checks the root element and its viewBox (or
// width/height) against the required size, and rejects active content —
// scripts, event handlers, external references, DTDs — so a stored logo can
// never execute anything even if opened directly.
func validateBrandingSVG(data []byte, width, height float64) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	rootSeen := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("not a well-formed SVG file (%v)", err)
		}
		switch element := token.(type) {
		case xml.Directive:
			return errors.New("DOCTYPE and entity declarations are not allowed")
		case xml.ProcInst:
			if !strings.EqualFold(element.Target, "xml") {
				return errors.New("processing instructions are not allowed")
			}
		case xml.StartElement:
			name := strings.ToLower(element.Name.Local)
			if !rootSeen {
				if name != "svg" {
					return errors.New("the root element must be <svg>")
				}
				rootSeen = true
				if err := checkSVGSize(element, width, height); err != nil {
					return err
				}
			}
			if svgForbiddenTags[name] {
				return fmt.Errorf("<%s> elements are not allowed", element.Name.Local)
			}
			for _, attr := range element.Attr {
				attrName := strings.ToLower(attr.Name.Local)
				value := strings.TrimSpace(attr.Value)
				if strings.HasPrefix(attrName, "on") {
					return fmt.Errorf("event handler attribute %q is not allowed", attr.Name.Local)
				}
				if attrName == "href" && !isSafeSVGReference(value) {
					return errors.New("links to external resources are not allowed")
				}
				if err := checkSVGStyleReferences(value); err != nil {
					return err
				}
			}
		case xml.CharData:
			if err := checkSVGStyleReferences(string(element)); err != nil {
				return err
			}
		}
	}
	if !rootSeen {
		return errors.New("no <svg> element found")
	}
	return nil
}

func isSafeSVGReference(value string) bool {
	return strings.HasPrefix(value, "#") || strings.HasPrefix(strings.ToLower(value), "data:image/png") ||
		strings.HasPrefix(strings.ToLower(value), "data:image/jpeg")
}

func checkSVGStyleReferences(text string) error {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "javascript:") || strings.Contains(lower, "@import") {
		return errors.New("scripts and imported styles are not allowed")
	}
	for _, match := range svgURLReference.FindAllStringSubmatch(text, -1) {
		if !isSafeSVGReference(match[1]) {
			return errors.New("url() references to external resources are not allowed")
		}
	}
	return nil
}

func checkSVGSize(root xml.StartElement, width, height float64) error {
	attrs := map[string]string{}
	for _, attr := range root.Attr {
		attrs[strings.ToLower(attr.Name.Local)] = strings.TrimSpace(attr.Value)
	}
	if viewBox, ok := attrs["viewbox"]; ok {
		parts := svgNumberSeparators.Split(strings.TrimSpace(viewBox), -1)
		if len(parts) == 4 {
			w, errW := strconv.ParseFloat(parts[2], 64)
			h, errH := strconv.ParseFloat(parts[3], 64)
			if errW == nil && errH == nil && w == width && h == height {
				return nil
			}
		}
		return fmt.Errorf("viewBox must be \"0 0 %g %g\" (found %q)", width, height, viewBox)
	}
	w, errW := strconv.ParseFloat(strings.TrimSuffix(attrs["width"], "px"), 64)
	h, errH := strconv.ParseFloat(strings.TrimSuffix(attrs["height"], "px"), 64)
	if errW == nil && errH == nil && w == width && h == height {
		return nil
	}
	return fmt.Errorf("the SVG must declare viewBox=\"0 0 %g %g\"", width, height)
}

// validateBrandingICO parses the ICONDIR header and requires a size×size
// image among its entries.
func validateBrandingICO(data []byte, size int) error {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return errors.New("not a Windows .ico file")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 || len(data) < 6+16*count {
		return errors.New("the .ico file is truncated")
	}
	found := false
	for index := 0; index < count; index++ {
		entry := data[6+16*index : 6+16*(index+1)]
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		length := binary.LittleEndian.Uint32(entry[8:12])
		offset := binary.LittleEndian.Uint32(entry[12:16])
		if uint64(offset)+uint64(length) > uint64(len(data)) || length == 0 {
			return errors.New("the .ico file is corrupt")
		}
		if width == size && height == size {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("the .ico file must contain a %d×%d image", size, size)
	}
	return nil
}
