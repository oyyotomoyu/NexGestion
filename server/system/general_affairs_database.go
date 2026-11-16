package system

// General Affairs owns general_affairs.db (docs/System/general-affairs-system.md
// Section 2, "one module, one database"). Timestamps are stored as RFC3339 UTC
// strings ("2006-01-02T15:04:05Z") so that string comparison is chronological;
// the booking overlap triggers below rely on that.

func generalAffairsDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "general_affairs.db",
		Schema: []string{
			// 2.1 Assets & Custody
			`CREATE TABLE IF NOT EXISTS ga_assets (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				category TEXT,
				status TEXT NOT NULL CHECK (status IN ('in_stock', 'issued', 'under_repair', 'lost', 'retired')),
				current_custodian_user_id TEXT,
				current_location TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK ((status = 'issued') = (current_custodian_user_id IS NOT NULL))
			)`,
			`CREATE INDEX IF NOT EXISTS ga_assets_custodian ON ga_assets(current_custodian_user_id)`,
			`CREATE TABLE IF NOT EXISTS ga_asset_events (
				id TEXT PRIMARY KEY,
				asset_id TEXT NOT NULL REFERENCES ga_assets(id),
				event_type TEXT NOT NULL CHECK (event_type IN ('created', 'issued', 'returned', 'transferred', 'repaired', 'lost', 'retired')),
				user_id TEXT,
				location TEXT,
				related_approval_request_id TEXT,
				related_hr_task_id TEXT,
				condition_note TEXT,
				actor_user_id TEXT NOT NULL,
				occurred_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS ga_asset_events_asset ON ga_asset_events(asset_id, occurred_at)`,
			`CREATE TRIGGER IF NOT EXISTS ga_asset_events_append_only_update BEFORE UPDATE ON ga_asset_events
			BEGIN SELECT RAISE(ABORT, 'asset events are append-only'); END`,
			`CREATE TRIGGER IF NOT EXISTS ga_asset_events_append_only_delete BEFORE DELETE ON ga_asset_events
			BEGIN SELECT RAISE(ABORT, 'asset events are append-only'); END`,

			// 2.2 Office Supply Requisitions
			`CREATE TABLE IF NOT EXISTS ga_requisitions (
				id TEXT PRIMARY KEY,
				requested_by_user_id TEXT NOT NULL,
				description TEXT NOT NULL,
				quantity TEXT,
				estimated_amount TEXT,
				status TEXT NOT NULL CHECK (status IN ('pending_approval', 'approved', 'rejected', 'fulfilled', 'cancelled')),
				approval_request_id TEXT,
				procurement_purchase_order_id TEXT,
				inventory_outbound_id TEXT,
				decided_by_user_id TEXT,
				decided_at TEXT,
				decision_comment TEXT,
				fulfilled_by_user_id TEXT,
				fulfilled_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (procurement_purchase_order_id IS NULL OR inventory_outbound_id IS NULL)
			)`,
			`CREATE INDEX IF NOT EXISTS ga_requisitions_requester ON ga_requisitions(requested_by_user_id, created_at)`,
			`CREATE INDEX IF NOT EXISTS ga_requisitions_status ON ga_requisitions(status, created_at)`,

			// Section 3.1 fallback: who may decide a request when no Approval
			// flow template is configured for its request type.
			`CREATE TABLE IF NOT EXISTS ga_fallback_approvers (
				subject_type TEXT NOT NULL CHECK (subject_type IN ('requisition', 'seal_usage')),
				subject_id TEXT NOT NULL,
				user_id TEXT NOT NULL,
				PRIMARY KEY (subject_type, subject_id, user_id)
			)`,
			`CREATE INDEX IF NOT EXISTS ga_fallback_approvers_user ON ga_fallback_approvers(user_id, subject_type)`,

			// 2.3 Resource Booking & Item Borrowing
			`CREATE TABLE IF NOT EXISTS ga_resources (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				category TEXT,
				requires_return INTEGER NOT NULL DEFAULT 0 CHECK (requires_return IN (0, 1)),
				status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS ga_bookings (
				id TEXT PRIMARY KEY,
				resource_id TEXT NOT NULL REFERENCES ga_resources(id),
				booked_by_user_id TEXT NOT NULL,
				purpose TEXT,
				starts_at TEXT NOT NULL,
				ends_at TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('confirmed', 'pending_coordination', 'cancelled')),
				returned_at TEXT,
				return_recorded_by_user_id TEXT,
				overdue_notified_at TEXT,
				created_by_user_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (ends_at > starts_at)
			)`,
			`CREATE INDEX IF NOT EXISTS ga_bookings_resource_time ON ga_bookings(resource_id, status, starts_at)`,
			`CREATE INDEX IF NOT EXISTS ga_bookings_booker ON ga_bookings(booked_by_user_id, starts_at)`,
			// Section 2.3.2: two confirmed bookings of one resource never overlap
			// on [starts_at, ends_at). Enforced in the database so concurrent
			// requests cannot both win.
			`CREATE TRIGGER IF NOT EXISTS ga_bookings_no_overlap_insert BEFORE INSERT ON ga_bookings
			WHEN NEW.status = 'confirmed' AND EXISTS (
				SELECT 1 FROM ga_bookings b WHERE b.resource_id = NEW.resource_id AND b.status = 'confirmed'
					AND b.starts_at < NEW.ends_at AND NEW.starts_at < b.ends_at)
			BEGIN SELECT RAISE(ABORT, 'ga_booking_overlap'); END`,
			`CREATE TRIGGER IF NOT EXISTS ga_bookings_no_overlap_update BEFORE UPDATE OF status, resource_id, starts_at, ends_at ON ga_bookings
			WHEN NEW.status = 'confirmed' AND EXISTS (
				SELECT 1 FROM ga_bookings b WHERE b.id <> NEW.id AND b.resource_id = NEW.resource_id AND b.status = 'confirmed'
					AND b.starts_at < NEW.ends_at AND NEW.starts_at < b.ends_at)
			BEGIN SELECT RAISE(ABORT, 'ga_booking_overlap'); END`,
			`CREATE TABLE IF NOT EXISTS ga_booking_invitees (
				booking_id TEXT NOT NULL REFERENCES ga_bookings(id) ON DELETE CASCADE,
				invited_user_id TEXT NOT NULL,
				response_status TEXT NOT NULL DEFAULT 'invited' CHECK (response_status IN ('invited', 'accepted', 'declined')),
				responded_at TEXT,
				PRIMARY KEY (booking_id, invited_user_id)
			)`,
			`CREATE INDEX IF NOT EXISTS ga_booking_invitees_user ON ga_booking_invitees(invited_user_id)`,
			`CREATE TABLE IF NOT EXISTS ga_coordination_cases (
				id TEXT PRIMARY KEY,
				requesting_booking_id TEXT NOT NULL REFERENCES ga_bookings(id),
				conflicting_booking_id TEXT NOT NULL REFERENCES ga_bookings(id),
				reason TEXT,
				initiated_by_user_id TEXT NOT NULL,
				original_booker_decision TEXT NOT NULL DEFAULT 'pending' CHECK (original_booker_decision IN ('pending', 'accept', 'decline')),
				decided_at TEXT,
				status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS ga_coordination_one_open_case
				ON ga_coordination_cases(requesting_booking_id) WHERE status = 'pending'`,

			// 2.4 Seal & Stamp Management
			`CREATE TABLE IF NOT EXISTS ga_seals (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				custodian_user_id TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'available' CHECK (status IN ('available', 'in_use', 'lost')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS ga_seal_requests (
				id TEXT PRIMARY KEY,
				seal_id TEXT NOT NULL REFERENCES ga_seals(id),
				requested_by_user_id TEXT NOT NULL,
				document_reference TEXT NOT NULL,
				requested_use_date TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('pending_approval', 'approved', 'rejected', 'used', 'cancelled')),
				approval_request_id TEXT,
				decided_by_user_id TEXT,
				decided_at TEXT,
				decision_comment TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS ga_seal_requests_requester ON ga_seal_requests(requested_by_user_id, created_at)`,
			`CREATE TABLE IF NOT EXISTS ga_seal_usage_log (
				id TEXT PRIMARY KEY,
				seal_usage_request_id TEXT NOT NULL UNIQUE REFERENCES ga_seal_requests(id),
				seal_id TEXT NOT NULL REFERENCES ga_seals(id),
				used_by_user_id TEXT NOT NULL,
				used_at TEXT NOT NULL,
				returned_at TEXT,
				recorded_by_user_id TEXT NOT NULL
			)`,
			// The log is append-only except for stamping returned_at once.
			`CREATE TRIGGER IF NOT EXISTS ga_seal_usage_log_guard BEFORE UPDATE ON ga_seal_usage_log
			WHEN OLD.returned_at IS NOT NULL OR NEW.id <> OLD.id OR NEW.seal_usage_request_id <> OLD.seal_usage_request_id
				OR NEW.seal_id <> OLD.seal_id OR NEW.used_by_user_id <> OLD.used_by_user_id OR NEW.used_at <> OLD.used_at
			BEGIN SELECT RAISE(ABORT, 'seal usage log is append-only'); END`,
			`CREATE TRIGGER IF NOT EXISTS ga_seal_usage_log_no_delete BEFORE DELETE ON ga_seal_usage_log
			BEGIN SELECT RAISE(ABORT, 'seal usage log is append-only'); END`,

			// 2.5 Polls & Statistics, 2.6 Group Meal Ordering
			`CREATE TABLE IF NOT EXISTS ga_polls (
				id TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				description TEXT,
				poll_type TEXT NOT NULL CHECK (poll_type IN ('general', 'food_order')),
				allow_multiple INTEGER NOT NULL DEFAULT 0 CHECK (allow_multiple IN (0, 1)),
				created_by_user_id TEXT NOT NULL,
				target_scope TEXT NOT NULL CHECK (target_scope IN ('organization', 'group')),
				target_group_id TEXT,
				status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'closed')),
				closes_at TEXT,
				settlement_method TEXT CHECK (settlement_method IN ('none', 'finance_expense', 'salary_deduction')),
				finance_expense_claim_id TEXT,
				settled_at TEXT,
				settled_by_user_id TEXT,
				closed_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK ((target_scope = 'group') = (target_group_id IS NOT NULL))
			)`,
			`CREATE INDEX IF NOT EXISTS ga_polls_status ON ga_polls(status, closes_at)`,
			`CREATE TABLE IF NOT EXISTS ga_poll_options (
				id TEXT PRIMARY KEY,
				poll_id TEXT NOT NULL REFERENCES ga_polls(id) ON DELETE CASCADE,
				label TEXT NOT NULL,
				unit_amount TEXT,
				sort_order INTEGER NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS ga_poll_options_poll ON ga_poll_options(poll_id, sort_order)`,
			`CREATE TABLE IF NOT EXISTS ga_poll_responses (
				id TEXT PRIMARY KEY,
				poll_id TEXT NOT NULL REFERENCES ga_polls(id) ON DELETE CASCADE,
				responded_by_user_id TEXT NOT NULL,
				participating INTEGER NOT NULL CHECK (participating IN (0, 1)),
				responded_at TEXT NOT NULL,
				UNIQUE (poll_id, responded_by_user_id)
			)`,
			`CREATE TABLE IF NOT EXISTS ga_poll_response_selections (
				poll_response_id TEXT NOT NULL REFERENCES ga_poll_responses(id) ON DELETE CASCADE,
				option_id TEXT NOT NULL REFERENCES ga_poll_options(id) ON DELETE CASCADE,
				quantity TEXT NOT NULL,
				PRIMARY KEY (poll_response_id, option_id)
			)`,
			`CREATE TABLE IF NOT EXISTS ga_salary_deduction_requests (
				id TEXT PRIMARY KEY,
				poll_id TEXT NOT NULL REFERENCES ga_polls(id),
				poll_response_id TEXT NOT NULL UNIQUE REFERENCES ga_poll_responses(id),
				user_id TEXT NOT NULL,
				amount TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'applied', 'cancelled')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS ga_salary_deductions_user ON ga_salary_deduction_requests(user_id, status)`,
		},
	}
}
