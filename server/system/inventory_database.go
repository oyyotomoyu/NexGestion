package system

func inventoryDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{Name: "inventory.db", Schema: []string{
		`CREATE TABLE IF NOT EXISTS inventory_warehouses (id TEXT PRIMARY KEY, name TEXT NOT NULL, address TEXT NOT NULL DEFAULT '', status TEXT NOT NULL CHECK(status IN ('active','inactive')), created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS inventory_locations (id TEXT PRIMARY KEY, warehouse_id TEXT NOT NULL REFERENCES inventory_warehouses(id), name TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('active','inactive')), created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS inventory_items (id TEXT PRIMARY KEY, sku TEXT NOT NULL UNIQUE, name TEXT NOT NULL, unit_of_measure TEXT NOT NULL, barcode TEXT UNIQUE)`,
		`CREATE TABLE IF NOT EXISTS inventory_records (id TEXT PRIMARY KEY, kind TEXT NOT NULL, status TEXT NOT NULL, payload TEXT NOT NULL, created_at TEXT NOT NULL, created_by TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS inventory_movements (id TEXT PRIMARY KEY, item_id TEXT NOT NULL REFERENCES inventory_items(id), warehouse_id TEXT NOT NULL REFERENCES inventory_warehouses(id), location_id TEXT REFERENCES inventory_locations(id), quantity_delta INTEGER NOT NULL, movement_type TEXT NOT NULL, source_reference_id TEXT NOT NULL REFERENCES inventory_records(id), occurred_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS inventory_stock ON inventory_movements(item_id,warehouse_id,location_id)`,
		`CREATE TRIGGER IF NOT EXISTS inventory_movements_no_update BEFORE UPDATE ON inventory_movements BEGIN SELECT RAISE(ABORT,'stock ledger is append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS inventory_movements_no_delete BEFORE DELETE ON inventory_movements BEGIN SELECT RAISE(ABORT,'stock ledger is append-only'); END`,
	}, Seed: []string{`INSERT INTO inventory_warehouses VALUES ('default','Main warehouse','','active',strftime('%Y-%m-%dT%H:%M:%SZ','now'))`}}
}
