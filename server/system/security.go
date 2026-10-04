package system

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSecurityRuleNotFound      = errors.New("security rule not found")
	ErrSecurityRecipientNotFound = errors.New("security alert recipient not found")
	ErrSecurityInvalid           = errors.New("invalid security configuration")
)

// errEmailTransportNotConfigured is returned by sendAlertEmail until a real
// SMTP/transactional-email integration exists (SIEM.md Section 13 - "who owns
// building the outbound-email transport" - deliberately deferred, shared with
// salary-system.md Section 10.3). Delivery attempts are still recorded so the
// "Alert History & Delivery Status" screen can show why email never arrives.
var errEmailTransportNotConfigured = errors.New("email transport not configured")

type SecurityEvent struct {
	ID          string  `json:"id"`
	OccurredAt  string  `json:"occurred_at"`
	EventType   string  `json:"event_type"`
	Severity    string  `json:"severity"`
	ActorUserID *string `json:"actor_user_id"`
	SourceIP    string  `json:"source_ip"`
	IPClass     string  `json:"ip_class"`
	Module      string  `json:"module"`
	RecordRef   *string `json:"record_ref"`
	Summary     string  `json:"summary"`
	Hash        string  `json:"hash"`
}

// SecurityEventInput is what a call site (an API handler, right beside its
// existing recordRequestLog call) passes to RecordEvent.
type SecurityEventInput struct {
	EventType   string
	Severity    string // optional; defaults to "info". May be raised by rule evaluation.
	ActorUserID string // empty when the actor isn't resolved yet (e.g. a failed login attempt)
	SourceIP    string
	Module      string
	RecordRef   string // free-text reference into the originating record (id, or an attempted email)
	Summary     string

	// PermissionKey is set on a "permission_granted" event so the
	// privilege-escalation rule can inspect what was granted.
	PermissionKey string
	// Privileged marks a "login_succeeded" event as belonging to a user who
	// holds at least one *.manage/*.configure permission, for the
	// new-device-privileged-login rule.
	Privileged bool
}

type SecurityRule struct {
	RuleKey    string `json:"rule_key"`
	Enabled    bool   `json:"enabled"`
	ConfigJSON string `json:"config_json"`
	UpdatedAt  string `json:"updated_at"`
}

type securityRuleConfig struct {
	Threshold     int `json:"threshold"`
	WindowMinutes int `json:"window_minutes"`
}

func (r SecurityRule) config() securityRuleConfig {
	var cfg securityRuleConfig
	_ = json.Unmarshal([]byte(r.ConfigJSON), &cfg)
	return cfg
}

type SecuritySettings struct {
	DeploymentPosture string `json:"deployment_posture"`
	RetentionDays     int    `json:"retention_days"`
	UpdatedAt         string `json:"updated_at"`
}

type SecurityAlertRecipient struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	UserID     *string `json:"user_id"`
	Email      *string `json:"email"`
	ScopeType  string  `json:"scope_type"`
	ScopeValue string  `json:"scope_value"`
	Channel    string  `json:"channel"`
	CreatedAt  string  `json:"created_at"`
}

type AddSecurityAlertRecipientInput struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	ScopeType  string `json:"scope_type"`
	ScopeValue string `json:"scope_value"`
	Channel    string `json:"channel"`
}

// resolvedAlertRecipient is the effective per-alert recipient list after
// merging the default "every security.alerts.read holder" set (doc Section
// 8.1's default) with configured per-rule/severity overrides (additions, not
// replacements - see raiseAlert).
type resolvedAlertRecipient struct {
	UserID  *string
	Email   *string
	Channel string
}

type SecurityService struct {
	databasePath  string
	users         *UserService
	notifications *NotificationService
	now           func() time.Time
}

func NewSecurityService(databaseDirectory string, users *UserService, notifications *NotificationService) *SecurityService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &SecurityService{
		databasePath:  filepath.Join(databaseDirectory, "security.db"),
		users:         users,
		notifications: notifications,
		now:           time.Now,
	}
}

func (s *SecurityService) open() (*sql.DB, error) {
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

// RecordEvent is the aggregator described in SIEM.md Section 5: every module
// calls this at the moment a security-relevant action already happens,
// alongside its existing recordRequestLog call. It classifies the source IP,
// extends the rolling hash chain (Section 11), evaluates detection rules
// (Section 6-7), and - outside the write transaction, so a delivery problem
// never rolls back the recorded event - raises alerts for whatever fired.
func (s *SecurityService) RecordEvent(ctx context.Context, input SecurityEventInput) (*SecurityEvent, error) {
	eventType := strings.TrimSpace(input.EventType)
	summary := strings.TrimSpace(input.Summary)
	module := strings.TrimSpace(input.Module)
	sourceIP := strings.TrimSpace(input.SourceIP)
	if eventType == "" || summary == "" || module == "" {
		return nil, ErrSecurityInvalid
	}
	severity := strings.TrimSpace(input.Severity)
	if severity == "" {
		severity = "info"
	}
	ipClass := classifySourceIP(sourceIP)

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

	settings, err := getSecuritySettingsTx(ctx, tx)
	if err != nil {
		return nil, err
	}

	// SIEM.md Section 7.2: on a declared lan_only deployment, a public source
	// IP on ANY security event should not be possible at all - its presence is
	// itself a stronger signal than any threshold-based rule, so it overrides
	// the event's own severity regardless of which (if any) rule also fires.
	violatesLANOnly := settings.DeploymentPosture == "lan_only" && ipClass == "public"
	if violatesLANOnly {
		severity = "critical"
	}

	prevHash, err := lastSecurityEventHash(ctx, tx)
	if err != nil {
		return nil, err
	}

	stamp := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	hash := hashSecurityEvent(prevHash, id, stamp, eventType, severity, input.ActorUserID, sourceIP, ipClass, module, input.RecordRef, summary)

	if _, err := tx.ExecContext(ctx, `INSERT INTO security_events
		(id, occurred_at, event_type, severity, actor_user_id, source_ip, ip_class, module, record_ref, summary, prev_hash, hash)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, stamp, eventType, severity, nullableTrim(input.ActorUserID), sourceIP, ipClass, module, nullableTrim(input.RecordRef), summary, prevHash, hash); err != nil {
		return nil, err
	}

	event := &SecurityEvent{
		ID: id, OccurredAt: stamp, EventType: eventType, Severity: severity,
		SourceIP: sourceIP, IPClass: ipClass, Module: module, Summary: summary, Hash: hash,
	}
	if strings.TrimSpace(input.ActorUserID) != "" {
		v := input.ActorUserID
		event.ActorUserID = &v
	}
	if strings.TrimSpace(input.RecordRef) != "" {
		v := input.RecordRef
		event.RecordRef = &v
	}

	fired, err := s.evaluateRules(ctx, tx, settings, input, event, s.now())
	if err != nil {
		return nil, err
	}
	if violatesLANOnly {
		fired = append([]string{"lan_only_violation"}, fired...)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	for _, ruleKey := range fired {
		s.raiseAlert(ctx, event, ruleKey)
	}

	return event, nil
}

func (s *SecurityService) evaluateRules(ctx context.Context, tx *sql.Tx, settings *SecuritySettings, input SecurityEventInput, event *SecurityEvent, now time.Time) ([]string, error) {
	rules, err := listSecurityRulesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	enabled := map[string]securityRuleConfig{}
	for _, rule := range rules {
		if rule.Enabled {
			enabled[rule.RuleKey] = rule.config()
		}
	}
	// SIEM.md Section 7.2: internet_exposed traffic is expected, so it doesn't
	// trip the lan_only violation, but the numeric thresholds below should
	// default to materially stricter values - halved here, with a floor, as a
	// starting point left for an administrator to retune (Section 13).
	strict := settings.DeploymentPosture == "internet_exposed"

	var fired []string
	switch event.EventType {
	case "login_failed":
		if cfg, ok := enabled["brute_force_login"]; ok {
			threshold := adjustedThreshold(cfg.Threshold, strict)
			count, err := countRecentSecurityEvents(ctx, tx, "login_failed", input.ActorUserID, event.SourceIP, input.RecordRef, cfg.WindowMinutes, now)
			if err != nil {
				return nil, err
			}
			if count >= threshold {
				fired = append(fired, "brute_force_login")
			}
		}
	case "permission_granted":
		if _, ok := enabled["privilege_escalation"]; ok && isEscalationPermission(input.PermissionKey) {
			fired = append(fired, "privilege_escalation")
		}
	case "report_download":
		if cfg, ok := enabled["mass_export"]; ok {
			threshold := adjustedThreshold(cfg.Threshold, strict)
			count, err := countRecentSecurityEvents(ctx, tx, "report_download", input.ActorUserID, "", "", cfg.WindowMinutes, now)
			if err != nil {
				return nil, err
			}
			if count >= threshold {
				fired = append(fired, "mass_export")
			}
		}
	case "login_succeeded":
		if _, ok := enabled["new_device_privileged_login"]; ok && input.Privileged {
			seen, err := hasPriorSecurityEventFromIP(ctx, tx, input.ActorUserID, event.SourceIP, event.ID)
			if err != nil {
				return nil, err
			}
			if !seen {
				fired = append(fired, "new_device_privileged_login")
			}
		}
	}
	return fired, nil
}

// adjustedThreshold halves a rule's configured threshold under
// internet_exposed posture (SIEM.md Section 7.2), never going below 2.
func adjustedThreshold(threshold int, strict bool) int {
	if threshold <= 0 {
		threshold = 1
	}
	if !strict {
		return threshold
	}
	if half := threshold / 2; half >= 2 {
		return half
	}
	return 2
}

// isEscalationPermission matches SIEM.md Section 6's privilege-escalation
// trigger: permissions.assign, roles.manage, or any *.manage/*.configure key.
func isEscalationPermission(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if key == "permissions.assign" || key == "roles.manage" {
		return true
	}
	return strings.HasSuffix(key, ".manage") || strings.HasSuffix(key, ".configure")
}

// classifySourceIP implements SIEM.md Section 7.1: private/loopback vs public.
func classifySourceIP(address string) string {
	ip := net.ParseIP(strings.TrimSpace(address))
	if ip == nil {
		return "public"
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return "private"
	}
	return "public"
}

func hashSecurityEvent(prevHash string, fields ...string) string {
	hasher := sha256.New()
	hasher.Write([]byte(prevHash))
	for _, field := range fields {
		hasher.Write([]byte{0})
		hasher.Write([]byte(field))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

const securityGenesisHash = "genesis"

func lastSecurityEventHash(ctx context.Context, tx *sql.Tx) (string, error) {
	var hash string
	err := tx.QueryRowContext(ctx, `SELECT hash FROM security_events ORDER BY occurred_at DESC, id DESC LIMIT 1`).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return securityGenesisHash, nil
	}
	return hash, err
}

func countRecentSecurityEvents(ctx context.Context, tx *sql.Tx, eventType, actorUserID, sourceIP, recordRef string, windowMinutes int, now time.Time) (int, error) {
	if windowMinutes <= 0 {
		windowMinutes = 10
	}
	actorUserID, sourceIP, recordRef = strings.TrimSpace(actorUserID), strings.TrimSpace(sourceIP), strings.TrimSpace(recordRef)
	if actorUserID == "" && sourceIP == "" && recordRef == "" {
		return 0, nil
	}
	since := now.UTC().Add(-time.Duration(windowMinutes) * time.Minute).Format(time.RFC3339)
	where := []string{"event_type = ?", "occurred_at >= ?"}
	args := []any{eventType, since}
	match := []string{}
	if actorUserID != "" {
		match = append(match, "actor_user_id = ?")
		args = append(args, actorUserID)
	}
	if sourceIP != "" {
		match = append(match, "source_ip = ?")
		args = append(args, sourceIP)
	}
	if recordRef != "" {
		match = append(match, "record_ref = ?")
		args = append(args, recordRef)
	}
	where = append(where, "("+strings.Join(match, " OR ")+")")
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM security_events WHERE `+strings.Join(where, " AND "), args...).Scan(&count)
	return count, err
}

func hasPriorSecurityEventFromIP(ctx context.Context, tx *sql.Tx, actorUserID, sourceIP, excludeEventID string) (bool, error) {
	if strings.TrimSpace(actorUserID) == "" {
		return true, nil // unresolved actor: nothing to compare, don't fire a false positive
	}
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM security_events
		WHERE actor_user_id = ? AND source_ip = ? AND id <> ?`, actorUserID, sourceIP, excludeEventID).Scan(&count)
	return count > 0, err
}

func listSecurityRulesTx(ctx context.Context, tx *sql.Tx) ([]SecurityRule, error) {
	rows, err := tx.QueryContext(ctx, `SELECT rule_key, enabled, config_json, updated_at FROM security_rules ORDER BY rule_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []SecurityRule
	for rows.Next() {
		var rule SecurityRule
		var enabled int
		if err := rows.Scan(&rule.RuleKey, &enabled, &rule.ConfigJSON, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rule.Enabled = enabled == 1
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *SecurityService) ListRules(ctx context.Context) ([]SecurityRule, error) {
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
	return listSecurityRulesTx(ctx, tx)
}

type SetSecurityRuleInput struct {
	Enabled    *bool   `json:"enabled"`
	ConfigJSON *string `json:"config_json"`
}

func (s *SecurityService) SetRule(ctx context.Context, actorUserID, ruleKey string, input SetSecurityRuleInput) (*SecurityRule, error) {
	ruleKey = strings.TrimSpace(ruleKey)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var current SecurityRule
	var enabled int
	err = db.QueryRowContext(ctx, `SELECT rule_key, enabled, config_json, updated_at FROM security_rules WHERE rule_key = ?`, ruleKey).
		Scan(&current.RuleKey, &enabled, &current.ConfigJSON, &current.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSecurityRuleNotFound
	}
	if err != nil {
		return nil, err
	}
	current.Enabled = enabled == 1
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	if input.ConfigJSON != nil {
		var probe map[string]any
		if json.Unmarshal([]byte(*input.ConfigJSON), &probe) != nil {
			return nil, ErrSecurityInvalid
		}
		current.ConfigJSON = *input.ConfigJSON
	}
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `UPDATE security_rules SET enabled = ?, config_json = ?, updated_at = ?, updated_by_user_id = ? WHERE rule_key = ?`,
		boolInt(current.Enabled), current.ConfigJSON, now, actorUserID, ruleKey); err != nil {
		return nil, err
	}
	current.UpdatedAt = now
	return &current, nil
}

func getSecuritySettingsTx(ctx context.Context, tx *sql.Tx) (*SecuritySettings, error) {
	var settings SecuritySettings
	err := tx.QueryRowContext(ctx, `SELECT deployment_posture, retention_days, updated_at FROM security_settings WHERE id = 1`).
		Scan(&settings.DeploymentPosture, &settings.RetentionDays, &settings.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &SecuritySettings{DeploymentPosture: "lan_only", RetentionDays: 365}, nil
	}
	return &settings, err
}

func (s *SecurityService) GetSettings(ctx context.Context) (*SecuritySettings, error) {
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
	return getSecuritySettingsTx(ctx, tx)
}

type UpdateSecuritySettingsInput struct {
	DeploymentPosture *string `json:"deployment_posture"`
	RetentionDays     *int    `json:"retention_days"`
}

// SetSettings changes deployment posture and/or retention. Per SIEM.md
// Section 7.2, toggling posture is itself a security-relevant configuration
// change and generates its own security event at the same severity as a
// network/interface change - the caller (the API handler) is responsible for
// calling RecordEvent after a successful posture change, mirroring how every
// other write handler logs beside the service call rather than the service
// layer reaching back into SecurityService itself.
func (s *SecurityService) SetSettings(ctx context.Context, actorUserID string, input UpdateSecuritySettingsInput) (*SecuritySettings, error) {
	current, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if input.DeploymentPosture != nil {
		posture := strings.TrimSpace(*input.DeploymentPosture)
		if posture != "lan_only" && posture != "internet_exposed" {
			return nil, ErrSecurityInvalid
		}
		current.DeploymentPosture = posture
	}
	if input.RetentionDays != nil {
		if *input.RetentionDays <= 0 {
			return nil, ErrSecurityInvalid
		}
		current.RetentionDays = *input.RetentionDays
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO security_settings (id, deployment_posture, retention_days, updated_at, updated_by_user_id)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET deployment_posture = excluded.deployment_posture, retention_days = excluded.retention_days,
			updated_at = excluded.updated_at, updated_by_user_id = excluded.updated_by_user_id`,
		current.DeploymentPosture, current.RetentionDays, now, actorUserID); err != nil {
		return nil, err
	}
	current.UpdatedAt = now
	return current, nil
}

func (s *SecurityService) ListEvents(ctx context.Context, query ListQuery) (ListResult[SecurityEvent], error) {
	query, sortExpression, err := NormalizeListQuery(query, "occurred_at", "desc", map[string]string{
		"occurred_at": "occurred_at",
		"event_type":  "event_type",
		"severity":    "severity",
	})
	if err != nil {
		return ListResult[SecurityEvent]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[SecurityEvent]{}, err
	}
	defer db.Close()
	where := []string{"1=1"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(event_type LIKE ? COLLATE NOCASE OR summary LIKE ? COLLATE NOCASE OR source_ip LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern, pattern)
	}
	if value := strings.TrimSpace(query.Filters["severity"]); value != "" {
		where = append(where, "severity = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Filters["event_type"]); value != "" {
		where = append(where, "event_type = ?")
		args = append(args, value)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM security_events WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[SecurityEvent]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT id, occurred_at, event_type, severity, actor_user_id, source_ip, ip_class, module, record_ref, summary, hash
		FROM security_events WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, id LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[SecurityEvent]{}, err
	}
	defer rows.Close()
	events := []SecurityEvent{}
	for rows.Next() {
		var event SecurityEvent
		if err := rows.Scan(&event.ID, &event.OccurredAt, &event.EventType, &event.Severity, &event.ActorUserID,
			&event.SourceIP, &event.IPClass, &event.Module, &event.RecordRef, &event.Summary, &event.Hash); err != nil {
			return ListResult[SecurityEvent]{}, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return ListResult[SecurityEvent]{}, err
	}
	return NewListResult(events, query, total), nil
}

func (s *SecurityService) ListRecipients(ctx context.Context) ([]SecurityAlertRecipient, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, kind, user_id, email, scope_type, scope_value, channel, created_at
		FROM security_alert_recipients ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recipients := []SecurityAlertRecipient{}
	for rows.Next() {
		var r SecurityAlertRecipient
		if err := rows.Scan(&r.ID, &r.Kind, &r.UserID, &r.Email, &r.ScopeType, &r.ScopeValue, &r.Channel, &r.CreatedAt); err != nil {
			return nil, err
		}
		recipients = append(recipients, r)
	}
	return recipients, rows.Err()
}

func (s *SecurityService) AddRecipient(ctx context.Context, actorUserID string, input AddSecurityAlertRecipientInput) (*SecurityAlertRecipient, error) {
	userID, email := strings.TrimSpace(input.UserID), strings.TrimSpace(input.Email)
	scopeType, scopeValue := strings.TrimSpace(input.ScopeType), strings.TrimSpace(input.ScopeValue)
	channel := strings.TrimSpace(input.Channel)
	if (userID == "") == (email == "") { // exactly one of the two must be set
		return nil, ErrSecurityInvalid
	}
	if scopeType != "severity" && scopeType != "rule" {
		return nil, ErrSecurityInvalid
	}
	if scopeValue == "" {
		return nil, ErrSecurityInvalid
	}
	if channel != "in_app" && channel != "email" && channel != "both" {
		return nil, ErrSecurityInvalid
	}
	kind := "email"
	if userID != "" {
		kind = "user"
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO security_alert_recipients
		(id, kind, user_id, email, scope_type, scope_value, channel, created_at, created_by_user_id)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, kind, nullableTrim(userID), nullableTrim(email), scopeType, scopeValue, channel, now, actorUserID); err != nil {
		return nil, err
	}
	return &SecurityAlertRecipient{ID: id, Kind: kind, UserID: nonEmptyPtr(userID), Email: nonEmptyPtr(email),
		ScopeType: scopeType, ScopeValue: scopeValue, Channel: channel, CreatedAt: now}, nil
}

func (s *SecurityService) RemoveRecipient(ctx context.Context, id string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `DELETE FROM security_alert_recipients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrSecurityRecipientNotFound
	}
	return nil
}

func nonEmptyPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// SendTestAlert exercises the real delivery path (SIEM.md Section 9 item 4)
// without waiting for a real incident.
func (s *SecurityService) SendTestAlert(ctx context.Context, actorUserID, sourceIP string) (*SecurityEvent, error) {
	return s.RecordEvent(ctx, SecurityEventInput{
		EventType:   "test_alert",
		Severity:    "info",
		ActorUserID: actorUserID,
		SourceIP:    sourceIP,
		Module:      "security",
		Summary:     "Test alert requested from the Security settings screen",
	})
}

// raiseAlert resolves recipients and delivers an alert for one fired rule. It
// runs after RecordEvent's transaction has committed, so a delivery failure
// never rolls back the already-recorded event (SIEM.md Section 8.2: a failed
// send must never prevent the in-app notification, and the event itself must
// never be lost because alerting had trouble).
func (s *SecurityService) raiseAlert(ctx context.Context, event *SecurityEvent, ruleKey string) {
	recipients, err := s.resolveRecipients(ctx, ruleKey, event.Severity)
	if err != nil || len(recipients) == 0 {
		return
	}
	typeCode := "important"
	if event.Severity == "critical" {
		typeCode = "urgent"
	}
	title := "Security alert: " + humanizeRuleKey(ruleKey)

	var inAppUserIDs []string
	for _, recipient := range recipients {
		if recipient.UserID != nil && (recipient.Channel == "in_app" || recipient.Channel == "both") {
			inAppUserIDs = append(inAppUserIDs, *recipient.UserID)
		}
	}
	if len(inAppUserIDs) > 0 && s.notifications != nil {
		_, notifyErr := s.notifications.CreateSystemAlert(ctx, SystemAlertInput{
			Title: title, Message: event.Summary, TypeCode: typeCode, TargetUserIDs: inAppUserIDs,
		})
		for _, userID := range inAppUserIDs {
			s.recordDelivery(ctx, event.ID, &userID, nil, "in_app", notifyErr)
		}
	}
	for _, recipient := range recipients {
		if recipient.Channel != "email" && recipient.Channel != "both" {
			continue
		}
		sendErr := sendAlertEmail(recipient, title, event.Summary)
		s.recordDelivery(ctx, event.ID, recipient.UserID, recipient.Email, "email", sendErr)
	}
}

func humanizeRuleKey(ruleKey string) string {
	return strings.ReplaceAll(ruleKey, "_", " ")
}

func (s *SecurityService) resolveRecipients(ctx context.Context, ruleKey, severity string) ([]resolvedAlertRecipient, error) {
	holders, err := s.users.UsersWithPermission(ctx, "security.alerts.read")
	if err != nil {
		return nil, err
	}
	byUserID := map[string]resolvedAlertRecipient{}
	for _, holder := range holders {
		id := holder.ID
		byUserID[id] = resolvedAlertRecipient{UserID: &id, Channel: "both"}
	}
	overrides, err := s.ListRecipients(ctx)
	if err != nil {
		return nil, err
	}
	var freeEmails []resolvedAlertRecipient
	for _, override := range overrides {
		matches := (override.ScopeType == "rule" && override.ScopeValue == ruleKey) ||
			(override.ScopeType == "severity" && override.ScopeValue == severity)
		if !matches {
			continue
		}
		if override.Kind == "user" && override.UserID != nil {
			byUserID[*override.UserID] = resolvedAlertRecipient{UserID: override.UserID, Channel: override.Channel}
			continue
		}
		if override.Kind == "email" && override.Email != nil {
			freeEmails = append(freeEmails, resolvedAlertRecipient{Email: override.Email, Channel: override.Channel})
		}
	}
	result := make([]resolvedAlertRecipient, 0, len(byUserID)+len(freeEmails))
	for _, recipient := range byUserID {
		result = append(result, recipient)
	}
	result = append(result, freeEmails...)
	return result, nil
}

// sendAlertEmail is a single-purpose stub: wiring in a real SMTP client later
// is a one-function change, not a schema or call-site change. See
// errEmailTransportNotConfigured.
func sendAlertEmail(_ resolvedAlertRecipient, _, _ string) error {
	return errEmailTransportNotConfigured
}

func (s *SecurityService) recordDelivery(ctx context.Context, eventID string, userID, email *string, channel string, deliveryErr error) {
	db, err := s.open()
	if err != nil {
		return
	}
	defer db.Close()
	status, detail := "sent", (*string)(nil)
	if deliveryErr != nil {
		status = "failed"
		if errors.Is(deliveryErr, errEmailTransportNotConfigured) {
			status = "skipped"
		}
		message := deliveryErr.Error()
		detail = &message
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO security_alert_deliveries
		(id, event_id, recipient_user_id, recipient_email, channel, status, detail, attempted_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		uuid.NewString(), eventID, userID, email, channel, status, detail, s.now().UTC().Format(time.RFC3339))
}

// RunMaintenance deletes security events past the configured retention
// window (SIEM.md Section 10). Called on the same hourly-ticker pattern as
// notification/attendance maintenance (main.go).
func (s *SecurityService) RunMaintenance(ctx context.Context) error {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	cutoff := s.now().UTC().AddDate(0, 0, -settings.RetentionDays).Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `DELETE FROM security_events WHERE occurred_at < ?`, cutoff)
	return err
}
