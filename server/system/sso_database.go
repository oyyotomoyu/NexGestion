package system

import (
	"context"
	"database/sql"
	"time"
)

// DefaultOrganizationID is the single tenant every deployment starts with.
// NexGestion is single-tenant today; the organization concept exists so SSO
// connections and linked identities are already tenant-scoped (SSO.md
// "Required Data Model") when multi-tenancy arrives.
const DefaultOrganizationID = "default"

func ssoDatabaseSpec() DatabaseSpec {
	return DatabaseSpec{
		Name: "sso.db",
		Schema: []string{
			`CREATE TABLE IF NOT EXISTS organizations (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS sso_connections (
				id TEXT PRIMARY KEY,
				organization_id TEXT NOT NULL REFERENCES organizations(id),
				name TEXT NOT NULL,
				provider_type TEXT NOT NULL CHECK (provider_type IN ('google', 'microsoft', 'okta', 'auth0', 'keycloak', 'generic')),
				protocol TEXT NOT NULL CHECK (protocol IN ('oidc')),
				issuer TEXT NOT NULL,
				client_id TEXT NOT NULL,
				client_secret_encrypted TEXT,
				token_endpoint_auth_method TEXT NOT NULL DEFAULT 'client_secret_basic'
					CHECK (token_endpoint_auth_method IN ('client_secret_basic', 'client_secret_post', 'none')),
				authorization_endpoint TEXT NOT NULL,
				token_endpoint TEXT NOT NULL,
				jwks_uri TEXT NOT NULL,
				redirect_uri TEXT NOT NULL,
				scopes TEXT NOT NULL,
				claim_mapping TEXT NOT NULL,
				allowed_email_domains TEXT NOT NULL DEFAULT '[]',
				jit_enabled INTEGER NOT NULL DEFAULT 0 CHECK (jit_enabled IN (0, 1)),
				jit_require_approval INTEGER NOT NULL DEFAULT 0 CHECK (jit_require_approval IN (0, 1)),
				jit_default_role_id TEXT,
				jit_default_group_id TEXT,
				group_sync_enabled INTEGER NOT NULL DEFAULT 0 CHECK (group_sync_enabled IN (0, 1)),
				group_mapping TEXT NOT NULL DEFAULT '{}',
				is_enabled INTEGER NOT NULL DEFAULT 0 CHECK (is_enabled IN (0, 1)),
				last_tested_at TEXT,
				last_test_ok INTEGER CHECK (last_test_ok IN (0, 1)),
				last_test_error TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS sso_connections_organization ON sso_connections(organization_id)`,
			`CREATE TABLE IF NOT EXISTS sso_user_identities (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				organization_id TEXT NOT NULL REFERENCES organizations(id),
				sso_connection_id TEXT NOT NULL REFERENCES sso_connections(id) ON DELETE CASCADE,
				provider_subject TEXT NOT NULL,
				provider_email TEXT,
				provider_email_verified INTEGER NOT NULL DEFAULT 0 CHECK (provider_email_verified IN (0, 1)),
				last_login_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE (sso_connection_id, provider_subject),
				UNIQUE (sso_connection_id, user_id)
			)`,
			`CREATE INDEX IF NOT EXISTS sso_user_identities_user ON sso_user_identities(user_id)`,
			`CREATE TABLE IF NOT EXISTS sso_login_states (
				state_hash TEXT PRIMARY KEY,
				sso_connection_id TEXT NOT NULL REFERENCES sso_connections(id) ON DELETE CASCADE,
				code_verifier TEXT NOT NULL,
				nonce TEXT NOT NULL,
				return_to TEXT NOT NULL,
				ip_address TEXT,
				created_at TEXT NOT NULL,
				expires_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS sso_login_states_expires ON sso_login_states(expires_at)`,
			`CREATE TABLE IF NOT EXISTS sso_audit_events (
				id TEXT PRIMARY KEY,
				event_type TEXT NOT NULL,
				organization_id TEXT,
				user_id TEXT,
				sso_connection_id TEXT,
				source_ip TEXT,
				user_agent TEXT,
				failure_reason TEXT,
				detail TEXT,
				occurred_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS sso_audit_events_type_time ON sso_audit_events(event_type, occurred_at)`,
			`CREATE INDEX IF NOT EXISTS sso_audit_events_time ON sso_audit_events(occurred_at)`,
		},
		SyncFunc: syncDefaultOrganization,
	}
}

func syncDefaultOrganization(ctx context.Context, tx *sql.Tx) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO organizations (id, name, status, created_at, updated_at)
		VALUES (?, 'Default organization', 'active', ?, ?)`, DefaultOrganizationID, now, now)
	return err
}
