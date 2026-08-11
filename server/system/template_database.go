package system

func templateDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "template.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS template_files (
				id TEXT PRIMARY KEY,
				original_filename TEXT NOT NULL,
				stored_path TEXT NOT NULL UNIQUE,
				content_type TEXT NOT NULL,
				size_bytes INTEGER NOT NULL CHECK (size_bytes > 0),
				checksum_sha256 TEXT NOT NULL,
				description TEXT,
				uploaded_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS template_files_created
				ON template_files(created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS template_files_uploader
				ON template_files(uploaded_by_user_id)`,
			`CREATE TABLE IF NOT EXISTS template_file_audiences (
				id TEXT PRIMARY KEY,
				template_file_id TEXT NOT NULL REFERENCES template_files(id) ON DELETE CASCADE,
				scope TEXT NOT NULL CHECK (scope IN ('organization', 'group', 'role', 'user')),
				target_group_id TEXT,
				target_role_id TEXT,
				target_user_id TEXT,
				created_at TEXT NOT NULL,
				CHECK (
					(scope = 'organization' AND target_group_id IS NULL AND target_role_id IS NULL AND target_user_id IS NULL)
					OR (scope = 'group' AND target_group_id IS NOT NULL AND target_role_id IS NULL AND target_user_id IS NULL)
					OR (scope = 'role' AND target_group_id IS NULL AND target_role_id IS NOT NULL AND target_user_id IS NULL)
					OR (scope = 'user' AND target_group_id IS NULL AND target_role_id IS NULL AND target_user_id IS NOT NULL)
				)
			)`,
			`CREATE INDEX IF NOT EXISTS template_file_audiences_file
				ON template_file_audiences(template_file_id)`,
			`CREATE INDEX IF NOT EXISTS template_file_audiences_group
				ON template_file_audiences(scope, target_group_id)`,
			`CREATE INDEX IF NOT EXISTS template_file_audiences_role
				ON template_file_audiences(scope, target_role_id)`,
			`CREATE INDEX IF NOT EXISTS template_file_audiences_user
				ON template_file_audiences(scope, target_user_id)`,
		},
	}
}
