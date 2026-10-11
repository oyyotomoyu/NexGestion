package system

import (
	"context"
	"database/sql"
)

// hrDatabaseSpec owns the HR module's own records (docs/System/hr-system.md).
// Every table references users by their immutable user_id and never
// duplicates User/Group/Role data, which stays in user.db.
func hrDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "hr.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS hr_settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)`,
			// Section 3.3. probation_end_date is an extension to the design
			// note so probation_* events can carry the review deadline.
			`CREATE TABLE IF NOT EXISTS employment_events (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				event_type TEXT NOT NULL CHECK (event_type IN (
					'onboarded','probation_passed','probation_extended','transferred','promoted',
					'suspended','reinstated','terminated','rehired'
				)),
				effective_date TEXT NOT NULL,
				previous_employment_status TEXT,
				new_employment_status TEXT,
				related_group_id TEXT,
				related_case_id TEXT REFERENCES employee_relations_cases(id),
				probation_end_date TEXT,
				reason TEXT,
				actor_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS employment_events_user_date
				ON employment_events(user_id, effective_date DESC, created_at DESC)`,
			`CREATE TRIGGER IF NOT EXISTS employment_events_append_only_update
			BEFORE UPDATE ON employment_events
			BEGIN
				SELECT RAISE(ABORT, 'employment events are append-only');
			END`,
			`CREATE TRIGGER IF NOT EXISTS employment_events_append_only_delete
			BEFORE DELETE ON employment_events
			BEGIN
				SELECT RAISE(ABORT, 'employment events are append-only');
			END`,
			// Section 3.4: one checklist model shared by onboarding and offboarding.
			`CREATE TABLE IF NOT EXISTS hr_tasks (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				kind TEXT NOT NULL CHECK (kind IN ('onboarding','offboarding')),
				title TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('pending','in_progress','done','skipped')),
				assigned_to_user_id TEXT,
				due_date TEXT,
				completed_at TEXT,
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS hr_tasks_user ON hr_tasks(user_id, created_at)`,
			`CREATE INDEX IF NOT EXISTS hr_tasks_assignee ON hr_tasks(assigned_to_user_id, status)`,
			// Section 4.1.
			`CREATE TABLE IF NOT EXISTS performance_cycles (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				start_date TEXT NOT NULL,
				end_date TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('draft','open','closed')),
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (end_date >= start_date)
			)`,
			// Section 4.2.
			`CREATE TABLE IF NOT EXISTS performance_goals (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				cycle_id TEXT NOT NULL REFERENCES performance_cycles(id),
				title TEXT NOT NULL,
				description TEXT,
				weight TEXT,
				status TEXT NOT NULL CHECK (status IN ('draft','active','completed','cancelled')),
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS performance_goals_user_cycle ON performance_goals(user_id, cycle_id)`,
			// Section 4.3: one review per employee per cycle.
			`CREATE TABLE IF NOT EXISTS performance_reviews (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				cycle_id TEXT NOT NULL REFERENCES performance_cycles(id),
				reviewer_user_id TEXT,
				status TEXT NOT NULL CHECK (status IN ('draft','self_review','manager_review','finalized')),
				rating TEXT,
				self_comments TEXT,
				manager_comments TEXT,
				finalized_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE (user_id, cycle_id)
			)`,
			// Section 4.3: a finalized review is corrected by a new record,
			// never by overwriting the finalized result.
			`CREATE TABLE IF NOT EXISTS performance_review_corrections (
				id TEXT PRIMARY KEY,
				review_id TEXT NOT NULL REFERENCES performance_reviews(id),
				rating TEXT,
				manager_comments TEXT,
				reason TEXT NOT NULL,
				actor_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
			`CREATE TRIGGER IF NOT EXISTS performance_reviews_finalized_immutable
			BEFORE UPDATE ON performance_reviews
			WHEN OLD.status = 'finalized'
			BEGIN
				SELECT RAISE(ABORT, 'finalized performance review cannot be changed');
			END`,
			// Section 5.2.
			`CREATE TABLE IF NOT EXISTS employee_relations_cases (
				id TEXT PRIMARY KEY,
				case_type TEXT NOT NULL CHECK (case_type IN ('grievance','disciplinary','mediation')),
				subject_user_id TEXT NOT NULL,
				reported_by_user_id TEXT,
				is_anonymous INTEGER NOT NULL DEFAULT 0 CHECK (is_anonymous IN (0, 1)),
				assigned_to_user_id TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('open','investigating','resolved','closed','escalated')),
				confidentiality_level TEXT NOT NULL CHECK (confidentiality_level IN ('standard','restricted')),
				summary TEXT,
				resolution_notes TEXT,
				related_employment_event_id TEXT,
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (is_anonymous = 0 OR reported_by_user_id IS NULL)
			)`,
			`CREATE INDEX IF NOT EXISTS employee_relations_cases_assignee ON employee_relations_cases(assigned_to_user_id)`,
			`CREATE INDEX IF NOT EXISTS employee_relations_cases_subject ON employee_relations_cases(subject_user_id)`,
			// Every handler a case has had; restricted cases stay visible only
			// to these people (Section 5.4).
			`CREATE TABLE IF NOT EXISTS employee_relations_case_handlers (
				case_id TEXT NOT NULL REFERENCES employee_relations_cases(id),
				user_id TEXT NOT NULL,
				assigned_at TEXT NOT NULL,
				PRIMARY KEY (case_id, user_id)
			)`,
			// Section 8: full change history for every sensitive HR record.
			`CREATE TABLE IF NOT EXISTS hr_audit_log (
				id TEXT PRIMARY KEY,
				entity_type TEXT NOT NULL,
				entity_id TEXT NOT NULL,
				action TEXT NOT NULL,
				actor_user_id TEXT NOT NULL,
				detail TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS hr_audit_log_entity ON hr_audit_log(entity_type, entity_id, created_at)`,
			`CREATE TRIGGER IF NOT EXISTS hr_audit_log_append_only_update
			BEFORE UPDATE ON hr_audit_log
			BEGIN
				SELECT RAISE(ABORT, 'hr audit log is append-only');
			END`,
			`CREATE TRIGGER IF NOT EXISTS hr_audit_log_append_only_delete
			BEFORE DELETE ON hr_audit_log
			BEGIN
				SELECT RAISE(ABORT, 'hr audit log is append-only');
			END`,
		},
		SyncFunc: syncHRSettingsDefaults,
	}
}

// syncHRSettingsDefaults inserts organization-level HR defaults without
// overwriting values an administrator already changed. See
// docs/System/hr-system.md Sections 3.2 and 4.3: probation and the rating
// scale are organization configuration, not hardcoded behavior.
func syncHRSettingsDefaults(ctx context.Context, tx *sql.Tx) error {
	defaults := map[string]string{
		hrSettingProbationRequired:    "true",
		hrSettingProbationDefaultDays: "90",
		hrSettingRatingScale:          `["1","2","3","4","5"]`,
	}
	for key, value := range defaults {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO hr_settings (key, value) VALUES (?, ?)`, key, value); err != nil {
			return err
		}
	}
	return nil
}
