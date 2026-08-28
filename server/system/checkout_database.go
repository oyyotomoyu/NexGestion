package system

func checkoutDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "checkout.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS checkout_transactions (
				id TEXT PRIMARY KEY,
				warehouse_id TEXT NOT NULL,
				cashier_user_id TEXT NOT NULL,
				crm_customer_id TEXT,
				status TEXT NOT NULL CHECK (status IN ('in_progress', 'completed', 'voided')),
				currency TEXT NOT NULL,
				subtotal_amount TEXT NOT NULL,
				discount_amount TEXT NOT NULL,
				total_amount TEXT NOT NULL,
				completed_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS checkout_transactions_created
				ON checkout_transactions(created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS checkout_transactions_cashier
				ON checkout_transactions(cashier_user_id, created_at DESC)`,
			`CREATE TABLE IF NOT EXISTS checkout_transaction_lines (
				id TEXT PRIMARY KEY,
				checkout_transaction_id TEXT NOT NULL REFERENCES checkout_transactions(id) ON DELETE CASCADE,
				description TEXT NOT NULL,
				inventory_item_id TEXT,
				quantity TEXT NOT NULL,
				unit_price TEXT NOT NULL,
				line_amount TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS checkout_transaction_lines_transaction
				ON checkout_transaction_lines(checkout_transaction_id)`,
			`CREATE TABLE IF NOT EXISTS checkout_promotion_rules (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				discount_type TEXT NOT NULL CHECK (discount_type IN ('percentage', 'fixed_amount')),
				discount_value TEXT NOT NULL,
				scope TEXT NOT NULL CHECK (scope IN ('transaction', 'item')),
				inventory_item_id TEXT,
				min_subtotal_amount TEXT,
				membership_tier_id TEXT,
				starts_at TEXT,
				ends_at TEXT,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS checkout_promotion_rules_status
				ON checkout_promotion_rules(status, starts_at, ends_at)`,
			`CREATE TABLE IF NOT EXISTS checkout_coupons (
				id TEXT PRIMARY KEY,
				code TEXT NOT NULL UNIQUE,
				coupon_type TEXT NOT NULL CHECK (coupon_type IN ('discount', 'voucher')),
				discount_type TEXT CHECK (discount_type IN ('percentage', 'fixed_amount')),
				discount_value TEXT,
				value_amount TEXT,
				usage_limit INTEGER,
				redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
				starts_at TEXT,
				ends_at TEXT,
				status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (
					(coupon_type = 'discount' AND discount_type IS NOT NULL AND discount_value IS NOT NULL AND value_amount IS NULL)
					OR (coupon_type = 'voucher' AND discount_type IS NULL AND discount_value IS NULL AND value_amount IS NOT NULL)
				)
			)`,
			`CREATE INDEX IF NOT EXISTS checkout_coupons_status
				ON checkout_coupons(status, starts_at, ends_at)`,
			`CREATE TABLE IF NOT EXISTS checkout_discounts (
				id TEXT PRIMARY KEY,
				checkout_transaction_id TEXT NOT NULL REFERENCES checkout_transactions(id) ON DELETE CASCADE,
				source_type TEXT NOT NULL CHECK (source_type IN ('promotion', 'coupon', 'points')),
				source_reference_id TEXT NOT NULL,
				amount TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS checkout_discounts_transaction
				ON checkout_discounts(checkout_transaction_id)`,
			`CREATE TABLE IF NOT EXISTS checkout_payments (
				id TEXT PRIMARY KEY,
				checkout_transaction_id TEXT NOT NULL REFERENCES checkout_transactions(id) ON DELETE CASCADE,
				method TEXT NOT NULL CHECK (method IN ('cash', 'card', 'mobile_payment', 'voucher', 'crypto')),
				amount TEXT NOT NULL,
				reference TEXT,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS checkout_payments_transaction
				ON checkout_payments(checkout_transaction_id)`,
		},
	}
}
