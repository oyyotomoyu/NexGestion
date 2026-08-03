package system

import (
	"context"
	"database/sql"
)

func notificationDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "notification.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS notification_types (
				id TEXT PRIMARY KEY,
				code TEXT NOT NULL COLLATE NOCASE UNIQUE,
				name TEXT NOT NULL,
				description TEXT,
				severity INTEGER NOT NULL DEFAULT 0 CHECK (severity BETWEEN 0 AND 100),
				required_permission_key TEXT,
				is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS notifications (
				id TEXT PRIMARY KEY,
				sender_user_id TEXT NOT NULL,
				title TEXT NOT NULL,
				message TEXT NOT NULL,
				type_id TEXT NOT NULL REFERENCES notification_types(id),
				status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'edited', 'hidden', 'expired')),
				show_from TEXT NOT NULL,
				show_until TEXT,
				retain_until TEXT,
				duration_code TEXT NOT NULL CHECK (duration_code IN ('hour', 'day', 'week', 'month', 'year', 'forever')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				edited_at TEXT,
				hidden_at TEXT,
				expired_at TEXT,
				CHECK (
					(duration_code = 'forever' AND show_until IS NULL AND retain_until IS NULL)
					OR (duration_code <> 'forever' AND show_until IS NOT NULL AND retain_until IS NOT NULL)
				)
			)`,
			`CREATE INDEX IF NOT EXISTS notifications_sender
				ON notifications(sender_user_id, created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS notifications_status_window
				ON notifications(status, show_from, show_until)`,
			`CREATE INDEX IF NOT EXISTS notifications_retain_until
				ON notifications(retain_until)`,
			`CREATE TABLE IF NOT EXISTS notification_audiences (
				id TEXT PRIMARY KEY,
				notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
				scope TEXT NOT NULL CHECK (scope IN ('organization', 'group', 'own_group', 'role', 'user')),
				target_group_id TEXT,
				target_role_id TEXT,
				target_user_id TEXT,
				created_at TEXT NOT NULL,
				CHECK (
					(scope = 'organization' AND target_group_id IS NULL AND target_role_id IS NULL AND target_user_id IS NULL)
					OR (scope IN ('group', 'own_group') AND target_group_id IS NOT NULL AND target_role_id IS NULL AND target_user_id IS NULL)
					OR (scope = 'role' AND target_group_id IS NULL AND target_role_id IS NOT NULL AND target_user_id IS NULL)
					OR (scope = 'user' AND target_group_id IS NULL AND target_role_id IS NULL AND target_user_id IS NOT NULL)
				)
			)`,
			`CREATE INDEX IF NOT EXISTS notification_audiences_notification
				ON notification_audiences(notification_id)`,
			`CREATE INDEX IF NOT EXISTS notification_audiences_group
				ON notification_audiences(scope, target_group_id)`,
			`CREATE INDEX IF NOT EXISTS notification_audiences_role
				ON notification_audiences(scope, target_role_id)`,
			`CREATE INDEX IF NOT EXISTS notification_audiences_user
				ON notification_audiences(scope, target_user_id)`,
			`CREATE TABLE IF NOT EXISTS notification_deliveries (
				notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
				user_id TEXT NOT NULL,
				delivered_at TEXT NOT NULL,
				read_at TEXT,
				dismissed_at TEXT,
				last_seen_version_at TEXT,
				PRIMARY KEY (notification_id, user_id)
			)`,
			`CREATE INDEX IF NOT EXISTS notification_deliveries_user
				ON notification_deliveries(user_id, delivered_at DESC)`,
			`CREATE TABLE IF NOT EXISTS notification_events (
				id TEXT PRIMARY KEY,
				notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
				actor_user_id TEXT NOT NULL,
				event_type TEXT NOT NULL CHECK (event_type IN ('created', 'published', 'edited', 'hidden', 'expired', 'deleted')),
				occurred_at TEXT NOT NULL,
				note TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS notification_events_notification
				ON notification_events(notification_id, occurred_at)`,
			`CREATE TABLE IF NOT EXISTS notification_exports (
				id TEXT PRIMARY KEY,
				export_month TEXT NOT NULL,
				format TEXT NOT NULL CHECK (format = 'csv'),
				relative_path TEXT,
				api_requested_by_user_id TEXT,
				sha256 TEXT,
				row_count INTEGER NOT NULL DEFAULT 0 CHECK (row_count >= 0),
				generated_at TEXT NOT NULL,
				UNIQUE(export_month, format, generated_at)
			)`,
		},
		SyncFunc: syncNotificationDefaults,
	}
}

func syncNotificationDefaults(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_types
		(id, code, name, description, severity, required_permission_key, created_by_user_id, created_at, updated_at)
		VALUES
		('00000000-0000-0000-0000-000000000101', 'info', 'Info', 'General information or routine notice', 10, 'notifications.type.info', '00000000-0000-0000-0000-000000000001', datetime('now'), datetime('now')),
		('00000000-0000-0000-0000-000000000102', 'success', 'Success', 'Positive completion or confirmation notice', 20, 'notifications.type.success', '00000000-0000-0000-0000-000000000001', datetime('now'), datetime('now')),
		('00000000-0000-0000-0000-000000000103', 'warning', 'Warning', 'Warning notice that needs attention', 60, 'notifications.type.warning', '00000000-0000-0000-0000-000000000001', datetime('now'), datetime('now')),
		('00000000-0000-0000-0000-000000000104', 'important', 'Important', 'Important business notice', 80, 'notifications.type.important', '00000000-0000-0000-0000-000000000001', datetime('now'), datetime('now')),
		('00000000-0000-0000-0000-000000000105', 'urgent', 'Urgent', 'Urgent notice requiring fast action', 100, 'notifications.type.urgent', '00000000-0000-0000-0000-000000000001', datetime('now'), datetime('now'))
		ON CONFLICT(code) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			severity = excluded.severity,
			required_permission_key = excluded.required_permission_key,
			updated_at = datetime('now')`)
	return err
}
