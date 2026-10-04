package system

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func securityDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "security.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS security_events (
				id TEXT PRIMARY KEY,
				occurred_at TEXT NOT NULL,
				event_type TEXT NOT NULL,
				severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
				actor_user_id TEXT,
				source_ip TEXT NOT NULL,
				ip_class TEXT NOT NULL CHECK (ip_class IN ('private', 'public')),
				module TEXT NOT NULL,
				record_ref TEXT,
				summary TEXT NOT NULL,
				prev_hash TEXT NOT NULL,
				hash TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS security_events_type_time
				ON security_events(event_type, occurred_at)`,
			`CREATE INDEX IF NOT EXISTS security_events_actor_time
				ON security_events(actor_user_id, occurred_at)`,
			`CREATE INDEX IF NOT EXISTS security_events_ip_time
				ON security_events(source_ip, occurred_at)`,
			`CREATE TABLE IF NOT EXISTS security_rules (
				rule_key TEXT PRIMARY KEY,
				enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
				config_json TEXT NOT NULL DEFAULT '{}',
				updated_at TEXT NOT NULL,
				updated_by_user_id TEXT
			)`,
			`CREATE TABLE IF NOT EXISTS security_settings (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				deployment_posture TEXT NOT NULL DEFAULT 'lan_only' CHECK (deployment_posture IN ('lan_only', 'internet_exposed')),
				retention_days INTEGER NOT NULL DEFAULT 365 CHECK (retention_days > 0),
				updated_at TEXT NOT NULL,
				updated_by_user_id TEXT
			)`,
			`CREATE TABLE IF NOT EXISTS security_alert_recipients (
				id TEXT PRIMARY KEY,
				kind TEXT NOT NULL CHECK (kind IN ('user', 'email')),
				user_id TEXT,
				email TEXT,
				scope_type TEXT NOT NULL CHECK (scope_type IN ('severity', 'rule')),
				scope_value TEXT NOT NULL,
				channel TEXT NOT NULL CHECK (channel IN ('in_app', 'email', 'both')),
				created_at TEXT NOT NULL,
				created_by_user_id TEXT NOT NULL,
				CHECK (
					(kind = 'user' AND user_id IS NOT NULL AND email IS NULL)
					OR (kind = 'email' AND email IS NOT NULL AND user_id IS NULL)
				)
			)`,
			`CREATE INDEX IF NOT EXISTS security_alert_recipients_scope
				ON security_alert_recipients(scope_type, scope_value)`,
			`CREATE TABLE IF NOT EXISTS security_alert_deliveries (
				id TEXT PRIMARY KEY,
				event_id TEXT NOT NULL REFERENCES security_events(id) ON DELETE CASCADE,
				recipient_user_id TEXT,
				recipient_email TEXT,
				channel TEXT NOT NULL CHECK (channel IN ('in_app', 'email')),
				status TEXT NOT NULL CHECK (status IN ('sent', 'failed', 'skipped')),
				detail TEXT,
				attempted_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS security_alert_deliveries_event
				ON security_alert_deliveries(event_id)`,
		},
		SyncFunc: syncSecurityDefaults,
	}
}

// syncSecurityDefaults seeds the shipped rule catalog and singleton settings
// row without overwriting values an administrator already changed - the same
// INSERT OR IGNORE convention used for system_settings defaults
// (syncSystemSettingsDefaults, database.go).
func syncSecurityDefaults(ctx context.Context, tx *sql.Tx) error {
	now := time.Now().UTC().Format(time.RFC3339)
	defaults := []struct {
		key     string
		enabled bool
		config  string
	}{
		// network_change and off_hours_privileged_action ship disabled: their
		// event sources (System Config network settings, a formal working-hours
		// concept) don't exist in this codebase yet, so nothing ever emits the
		// events these rules would evaluate. Enabling them here would be inert.
		{"brute_force_login", true, `{"threshold":5,"window_minutes":10}`},
		{"privilege_escalation", true, `{}`},
		{"mass_export", true, `{"threshold":20,"window_minutes":10}`},
		{"new_device_privileged_login", true, `{}`},
		{"network_change", false, `{}`},
		{"off_hours_privileged_action", false, `{}`},
	}
	for _, rule := range defaults {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO security_rules
			(rule_key, enabled, config_json, updated_at) VALUES (?, ?, ?, ?)`,
			rule.key, boolInt(rule.enabled), rule.config, now); err != nil {
			return fmt.Errorf("seed security rule %q: %w", rule.key, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO security_settings
		(id, deployment_posture, retention_days, updated_at) VALUES (1, 'lan_only', 365, ?)`, now); err != nil {
		return fmt.Errorf("seed security settings: %w", err)
	}
	return nil
}
