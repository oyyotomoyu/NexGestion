package system

func financeDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "finance.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS finance_accounts (
				id TEXT PRIMARY KEY,
				code TEXT NOT NULL UNIQUE,
				name TEXT NOT NULL,
				account_type TEXT NOT NULL CHECK (account_type IN ('asset', 'liability', 'equity', 'revenue', 'expense')),
				parent_account_id TEXT REFERENCES finance_accounts(id),
				currency TEXT,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS finance_accounts_type ON finance_accounts(account_type, code)`,
			`CREATE TABLE IF NOT EXISTS accounting_periods (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				start_date TEXT NOT NULL,
				end_date TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('open', 'closed')),
				closed_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (end_date >= start_date)
			)`,
			`CREATE INDEX IF NOT EXISTS accounting_periods_dates ON accounting_periods(start_date, end_date)`,
			`CREATE TABLE IF NOT EXISTS journal_entries (
				id TEXT PRIMARY KEY,
				entry_date TEXT NOT NULL,
				period_id TEXT NOT NULL REFERENCES accounting_periods(id),
				source_module TEXT NOT NULL,
				source_reference_id TEXT,
				description TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL CHECK (status IN ('draft', 'posted', 'reversed')),
				created_by_user_id TEXT NOT NULL,
				posted_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS journal_entries_period ON journal_entries(period_id, entry_date DESC)`,
			`CREATE INDEX IF NOT EXISTS journal_entries_source ON journal_entries(source_module, source_reference_id)`,
			`CREATE TABLE IF NOT EXISTS journal_entry_lines (
				id TEXT PRIMARY KEY,
				entry_id TEXT NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
				account_id TEXT NOT NULL REFERENCES finance_accounts(id),
				debit_amount TEXT,
				credit_amount TEXT,
				currency TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				CHECK (
					(debit_amount IS NOT NULL AND credit_amount IS NULL)
					OR (debit_amount IS NULL AND credit_amount IS NOT NULL)
				)
			)`,
			`CREATE INDEX IF NOT EXISTS journal_entry_lines_entry ON journal_entry_lines(entry_id)`,
			`CREATE TABLE IF NOT EXISTS finance_vendors (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				bank_details TEXT NOT NULL DEFAULT '',
				tax_identifier TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS ap_bills (
				id TEXT PRIMARY KEY,
				vendor_id TEXT NOT NULL REFERENCES finance_vendors(id),
				bill_number TEXT NOT NULL,
				bill_date TEXT NOT NULL,
				due_date TEXT NOT NULL,
				currency TEXT NOT NULL,
				total_amount TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('draft', 'approved', 'disbursed', 'voided')),
				journal_entry_id TEXT REFERENCES journal_entries(id),
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS ap_bills_vendor ON ap_bills(vendor_id, due_date)`,
			`CREATE TABLE IF NOT EXISTS ap_payment_batches (
				id TEXT PRIMARY KEY,
				batch_type TEXT NOT NULL CHECK (batch_type IN ('vendor', 'payroll', 'reimbursement')),
				currency TEXT NOT NULL,
				total_amount TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('draft', 'approved', 'disbursed', 'voided')),
				created_by_user_id TEXT NOT NULL,
				disbursed_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS ap_payment_batch_items (
				id TEXT PRIMARY KEY,
				batch_id TEXT NOT NULL REFERENCES ap_payment_batches(id) ON DELETE CASCADE,
				ap_bill_id TEXT REFERENCES ap_bills(id),
				payee_name TEXT NOT NULL,
				amount TEXT NOT NULL,
				bank_reference TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL
			)`,
		},
	}
}
