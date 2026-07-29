package system

func attendanceDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "attendance.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS attendance_days (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				attendance_date TEXT NOT NULL,
				timezone TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('non_working', 'working')),
				total_worked_minutes INTEGER NOT NULL DEFAULT 0 CHECK (total_worked_minutes >= 0),
				requires_review INTEGER NOT NULL DEFAULT 0 CHECK (requires_review IN (0, 1)),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(user_id, attendance_date)
			)`,
			`CREATE TABLE IF NOT EXISTS attendance_sessions (
				id TEXT PRIMARY KEY,
				attendance_day_id TEXT NOT NULL REFERENCES attendance_days(id) ON DELETE CASCADE,
				sequence_number INTEGER NOT NULL CHECK (sequence_number > 0),
				continued_from_session_id TEXT REFERENCES attendance_sessions(id) ON DELETE SET NULL,
				sign_in_at TEXT NOT NULL,
				sign_out_at TEXT,
				worked_minutes INTEGER CHECK (worked_minutes >= 0),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(attendance_day_id, sequence_number)
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS attendance_one_open_session_per_day
				ON attendance_sessions(attendance_day_id) WHERE sign_out_at IS NULL`,
			`CREATE TRIGGER IF NOT EXISTS attendance_prevent_second_open_session
			BEFORE INSERT ON attendance_sessions
			WHEN NEW.sign_out_at IS NULL AND EXISTS (
				SELECT 1 FROM attendance_sessions current_session
				JOIN attendance_days current_day ON current_day.id=current_session.attendance_day_id
				JOIN attendance_days new_day ON new_day.id=NEW.attendance_day_id
				WHERE current_day.user_id=new_day.user_id AND current_session.sign_out_at IS NULL
			)
			BEGIN
				SELECT RAISE(ABORT, 'user already has an open attendance session');
			END`,
			`CREATE INDEX IF NOT EXISTS attendance_days_user_date
				ON attendance_days(user_id, attendance_date)`,
			`CREATE TABLE IF NOT EXISTS attendance_events (
				id TEXT PRIMARY KEY,
				attendance_day_id TEXT NOT NULL REFERENCES attendance_days(id) ON DELETE CASCADE,
				attendance_session_id TEXT REFERENCES attendance_sessions(id) ON DELETE SET NULL,
				user_id TEXT NOT NULL,
				actor_user_id TEXT NOT NULL,
				event_type TEXT NOT NULL CHECK (event_type IN ('sign_in', 'sign_out', 'midnight_rollover', 'mark_absent', 'correction')),
				occurred_at TEXT NOT NULL,
				previous_status TEXT,
				new_status TEXT NOT NULL,
				reason TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS attendance_events_day
				ON attendance_events(attendance_day_id, occurred_at)`,
			`CREATE TABLE IF NOT EXISTS attendance_monthly_reports (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				report_month TEXT NOT NULL,
				timezone TEXT NOT NULL,
				scheduled_work_days INTEGER NOT NULL DEFAULT 0,
				present_days INTEGER NOT NULL DEFAULT 0,
				absent_days INTEGER NOT NULL DEFAULT 0,
				incomplete_days INTEGER NOT NULL DEFAULT 0,
				worked_minutes INTEGER NOT NULL DEFAULT 0,
				worked_hours REAL NOT NULL DEFAULT 0,
				generated_at TEXT NOT NULL,
				source_updated_at TEXT NOT NULL,
				UNIQUE(user_id, report_month)
			)`,
			`CREATE TABLE IF NOT EXISTS attendance_report_exports (
				id TEXT PRIMARY KEY,
				report_month TEXT NOT NULL,
				format TEXT NOT NULL CHECK (format = 'csv'),
				relative_path TEXT NOT NULL,
				sha256 TEXT NOT NULL,
				row_count INTEGER NOT NULL CHECK (row_count >= 0),
				generated_at TEXT NOT NULL,
				source_updated_at TEXT NOT NULL,
				is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS attendance_one_active_export
				ON attendance_report_exports(report_month, format) WHERE is_active = 1`,
			`CREATE TABLE IF NOT EXISTS leave_requests (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				leave_type TEXT NOT NULL,
				leave_date TEXT NOT NULL,
				duration_type TEXT NOT NULL CHECK (duration_type IN ('hourly', 'full_day')),
				start_time TEXT,
				end_time TEXT,
				requested_minutes INTEGER NOT NULL CHECK (requested_minutes >= 60),
				reason TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (
					(duration_type='full_day' AND start_time IS NULL AND end_time IS NULL AND requested_minutes=480)
					OR (duration_type='hourly' AND start_time IS NOT NULL AND end_time IS NOT NULL)
				),
				UNIQUE(user_id, leave_date, leave_type, duration_type, start_time, end_time)
			)`,
			`CREATE INDEX IF NOT EXISTS leave_requests_user_date
				ON leave_requests(user_id, leave_date DESC)`,
			`CREATE TABLE IF NOT EXISTS leave_request_assignments (
				leave_request_id TEXT NOT NULL REFERENCES leave_requests(id) ON DELETE CASCADE,
				approver_user_id TEXT NOT NULL,
				organization_group_id TEXT,
				is_administrator_route INTEGER NOT NULL DEFAULT 0 CHECK (is_administrator_route IN (0,1)),
				assigned_at TEXT NOT NULL,
				PRIMARY KEY (leave_request_id, approver_user_id)
			)`,
			`CREATE INDEX IF NOT EXISTS leave_assignments_approver
				ON leave_request_assignments(approver_user_id, assigned_at DESC)`,
			`CREATE TABLE IF NOT EXISTS leave_request_events (
				id TEXT PRIMARY KEY,
				leave_request_id TEXT NOT NULL REFERENCES leave_requests(id) ON DELETE CASCADE,
				actor_user_id TEXT NOT NULL,
				event_type TEXT NOT NULL CHECK (event_type IN ('submitted','assigned','approved','rejected','cancelled','reassigned')),
				note TEXT NOT NULL DEFAULT '',
				occurred_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS leave_events_request
				ON leave_request_events(leave_request_id, occurred_at)`,
		},
	}
}
