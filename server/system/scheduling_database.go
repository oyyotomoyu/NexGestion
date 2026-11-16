package system

func schedulingDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "scheduling.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS scheduling_settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS schedule_periods (
				id TEXT PRIMARY KEY,
				group_id TEXT NOT NULL,
				group_name TEXT NOT NULL,
				group_type TEXT NOT NULL,
				name TEXT NOT NULL,
				timezone TEXT NOT NULL,
				start_date TEXT NOT NULL,
				end_date TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'locked', 'archived', 'cancelled')),
				created_by_user_id TEXT NOT NULL,
				published_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (end_date >= start_date)
			)`,
			`CREATE INDEX IF NOT EXISTS schedule_periods_group_dates
				ON schedule_periods(group_id, start_date)`,
			`CREATE TABLE IF NOT EXISTS schedule_shifts (
				id TEXT PRIMARY KEY,
				schedule_period_id TEXT NOT NULL REFERENCES schedule_periods(id) ON DELETE CASCADE,
				title TEXT,
				local_start_at TEXT NOT NULL,
				local_end_at TEXT NOT NULL,
				start_at_utc TEXT NOT NULL,
				end_at_utc TEXT NOT NULL,
				planned_minutes INTEGER NOT NULL CHECK (planned_minutes > 0),
				required_headcount INTEGER NOT NULL DEFAULT 1 CHECK (required_headcount >= 1),
				assignment_mode TEXT NOT NULL CHECK (assignment_mode IN ('named_users', 'open_shift', 'requirement_only')),
				notes TEXT,
				status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'cancelled', 'completed')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (end_at_utc > start_at_utc)
			)`,
			`CREATE INDEX IF NOT EXISTS schedule_shifts_period_start
				ON schedule_shifts(schedule_period_id, start_at_utc)`,
			`CREATE TABLE IF NOT EXISTS schedule_assignments (
				id TEXT PRIMARY KEY,
				shift_id TEXT NOT NULL REFERENCES schedule_shifts(id) ON DELETE CASCADE,
				user_id TEXT NOT NULL,
				assignment_status TEXT NOT NULL DEFAULT 'planned' CHECK (assignment_status IN ('planned', 'confirmed', 'declined', 'cancelled', 'completed')),
				source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'template', 'import', 'swap', 'auto_scheduler')),
				override_reason TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS schedule_assignments_one_active_per_user
				ON schedule_assignments(shift_id, user_id) WHERE assignment_status IN ('planned', 'confirmed')`,
			`CREATE INDEX IF NOT EXISTS schedule_assignments_user
				ON schedule_assignments(user_id, assignment_status)`,
			`CREATE TABLE IF NOT EXISTS scheduling_events (
				id TEXT PRIMARY KEY,
				schedule_period_id TEXT REFERENCES schedule_periods(id) ON DELETE CASCADE,
				shift_id TEXT,
				assignment_id TEXT,
				actor_user_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				reason TEXT,
				detail TEXT,
				occurred_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS scheduling_events_period
				ON scheduling_events(schedule_period_id, occurred_at)`,
		},
	}
}
