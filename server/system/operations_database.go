package system

// Each operations module owns its database; common storage only shares code.
func operationsDatabaseSpec(name string) DatabaseSpec {
	return DatabaseSpec{Name: name, Schema: []string{
		`CREATE TABLE IF NOT EXISTS operation_documents (id TEXT PRIMARY KEY, kind TEXT NOT NULL, status TEXT NOT NULL, parent_id TEXT, request_key TEXT UNIQUE, payload TEXT NOT NULL CHECK(json_valid(payload)), created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS operation_documents_kind ON operation_documents(kind,created_at)`,
		`CREATE INDEX IF NOT EXISTS operation_documents_parent ON operation_documents(parent_id,kind)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS operation_invoice_number ON operation_documents(parent_id,json_extract(payload,'$.vendor_invoice_number')) WHERE kind='vendor_invoice'`,
		`CREATE TABLE IF NOT EXISTS operation_events (id TEXT PRIMARY KEY, document_id TEXT NOT NULL REFERENCES operation_documents(id), actor_user_id TEXT NOT NULL, action TEXT NOT NULL, occurred_at TEXT NOT NULL)`,
		`CREATE TRIGGER IF NOT EXISTS operation_events_no_update BEFORE UPDATE ON operation_events BEGIN SELECT RAISE(ABORT,'operations audit is append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS operation_events_no_delete BEFORE DELETE ON operation_events BEGIN SELECT RAISE(ABORT,'operations audit is append-only'); END`,
	}}
}
