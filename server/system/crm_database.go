package system

func crmDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "crm.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS crm_customer_tiers (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT,
				default_price_list_id TEXT,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_customer_tiers_status
				ON crm_customer_tiers(status)`,
			`CREATE TABLE IF NOT EXISTS crm_membership_tiers (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT,
				default_price_list_id TEXT,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_membership_tiers_status
				ON crm_membership_tiers(status)`,
			`CREATE TABLE IF NOT EXISTS crm_price_lists (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				currency TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_price_lists_status
				ON crm_price_lists(status)`,
			`CREATE TABLE IF NOT EXISTS crm_price_list_items (
				id TEXT PRIMARY KEY,
				price_list_id TEXT NOT NULL REFERENCES crm_price_lists(id) ON DELETE CASCADE,
				description TEXT NOT NULL,
				inventory_item_id TEXT,
				unit_price TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_price_list_items_price_list
				ON crm_price_list_items(price_list_id)`,
			`CREATE TABLE IF NOT EXISTS crm_customers (
				id TEXT PRIMARY KEY,
				party_type TEXT NOT NULL CHECK (party_type IN ('individual', 'organization')),
				segment TEXT NOT NULL CHECK (segment IN ('b2b', 'b2c')),
				name TEXT NOT NULL,
				contact_email TEXT,
				contact_phone TEXT,
				tax_identifier TEXT,
				tier_id TEXT REFERENCES crm_customer_tiers(id) ON DELETE SET NULL,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_customers_segment
				ON crm_customers(segment, status)`,
			`CREATE INDEX IF NOT EXISTS crm_customers_tier
				ON crm_customers(tier_id)`,
			`CREATE TABLE IF NOT EXISTS crm_memberships (
				id TEXT PRIMARY KEY,
				customer_id TEXT NOT NULL REFERENCES crm_customers(id) ON DELETE CASCADE,
				membership_tier_id TEXT NOT NULL REFERENCES crm_membership_tiers(id),
				member_number TEXT UNIQUE,
				joined_at TEXT NOT NULL,
				expires_at TEXT,
				status TEXT NOT NULL CHECK (status IN ('active', 'lapsed', 'cancelled')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_memberships_customer
				ON crm_memberships(customer_id)`,
			`CREATE INDEX IF NOT EXISTS crm_memberships_tier
				ON crm_memberships(membership_tier_id)`,
			`CREATE TABLE IF NOT EXISTS crm_points_earning_rules (
				id TEXT PRIMARY KEY,
				membership_tier_id TEXT REFERENCES crm_membership_tiers(id) ON DELETE CASCADE,
				points_per_currency_unit TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_points_earning_rules_tier
				ON crm_points_earning_rules(membership_tier_id, status)`,
			`CREATE TABLE IF NOT EXISTS crm_points_ledger (
				id TEXT PRIMARY KEY,
				customer_id TEXT NOT NULL REFERENCES crm_customers(id) ON DELETE CASCADE,
				points_delta INTEGER NOT NULL,
				entry_type TEXT NOT NULL CHECK (entry_type IN ('earned', 'redeemed', 'expired', 'adjustment')),
				source_module TEXT,
				source_reference_id TEXT,
				occurred_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS crm_points_ledger_customer
				ON crm_points_ledger(customer_id, occurred_at)`,
		},
	}
}
