package system

func salaryDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "salary.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS compensation_records (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				compensation_basis TEXT NOT NULL CHECK (compensation_basis IN (
					'hourly','daily','weekly','monthly','annual','piece_rate','project_based','contract'
				)),
				rate_amount TEXT NOT NULL,
				currency TEXT NOT NULL,
				jurisdiction_id TEXT NOT NULL,
				effective_start_date TEXT NOT NULL,
				effective_end_date TEXT,
				note TEXT NOT NULL DEFAULT '',
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				CHECK (effective_end_date IS NULL OR effective_end_date >= effective_start_date)
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS compensation_records_one_active_per_user
				ON compensation_records(user_id) WHERE effective_end_date IS NULL`,
			`CREATE INDEX IF NOT EXISTS compensation_records_user_start
				ON compensation_records(user_id, effective_start_date DESC)`,
		},
	}
}
