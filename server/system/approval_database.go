package system

func approvalDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "approval.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS approval_flow_templates (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				request_type TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS approval_flow_templates_request_type
				ON approval_flow_templates(request_type, status)`,
			`CREATE TABLE IF NOT EXISTS approval_step_templates (
				id TEXT PRIMARY KEY,
				flow_template_id TEXT NOT NULL REFERENCES approval_flow_templates(id) ON DELETE CASCADE,
				step_order INTEGER NOT NULL,
				approver_type TEXT NOT NULL CHECK (approver_type IN ('specific_user','role','requester_manager','group_manager')),
				approver_user_id TEXT,
				approver_role_id TEXT,
				approver_group_id TEXT,
				min_amount TEXT,
				UNIQUE(flow_template_id, step_order)
			)`,
			`CREATE TABLE IF NOT EXISTS approval_notification_targets (
				id TEXT PRIMARY KEY,
				flow_template_id TEXT NOT NULL REFERENCES approval_flow_templates(id) ON DELETE CASCADE,
				target_type TEXT NOT NULL CHECK (target_type IN ('specific_user','role','group_manager')),
				target_user_id TEXT,
				target_role_id TEXT,
				target_group_id TEXT,
				notify_on TEXT NOT NULL CHECK (notify_on IN ('approved','rejected','both'))
			)`,
			`CREATE TABLE IF NOT EXISTS approval_requests (
				id TEXT PRIMARY KEY,
				flow_template_id TEXT NOT NULL REFERENCES approval_flow_templates(id),
				source_module TEXT NOT NULL,
				source_reference_id TEXT NOT NULL,
				requested_by_user_id TEXT NOT NULL,
				amount TEXT,
				status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled','requires_assignment')),
				current_step_order INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				completed_at TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS approval_requests_source
				ON approval_requests(source_module, source_reference_id)`,
			`CREATE INDEX IF NOT EXISTS approval_requests_requester
				ON approval_requests(requested_by_user_id)`,
			`CREATE TABLE IF NOT EXISTS approval_steps (
				id TEXT PRIMARY KEY,
				approval_request_id TEXT NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
				step_order INTEGER NOT NULL,
				decision TEXT NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending','approved','rejected','skipped')),
				decided_by_user_id TEXT,
				decided_at TEXT,
				comment TEXT NOT NULL DEFAULT '',
				UNIQUE(approval_request_id, step_order)
			)`,
			`CREATE TABLE IF NOT EXISTS approval_step_assignees (
				approval_step_id TEXT NOT NULL REFERENCES approval_steps(id) ON DELETE CASCADE,
				user_id TEXT NOT NULL,
				PRIMARY KEY (approval_step_id, user_id)
			)`,
		},
	}
}
