# NexGestion SSO Specification

This document defines the product and engineering rules for adding Single Sign-On
(SSO) login to NexGestion in the future.

NexGestion is a local-first, self-hosted business management platform. SSO should
therefore be implemented as an optional integration with external identity
providers, not as a requirement for every deployment.

## Goals

- Allow organizations to sign in with their existing identity provider.
- Keep NexGestion responsible for application roles and permissions.
- Preserve local username/password login for small deployments and fallback use.
- Support self-hosted deployments without requiring a cloud dependency.
- Keep authentication auditable, reversible, and tenant-scoped.

## Non-Goals

- NexGestion should not become a general-purpose identity provider.
- NexGestion should not manage external account passwords.
- NexGestion should not automatically trust provider-side roles as application
  permissions without explicit mapping.
- SSO should not remove local administrator recovery access.

## Supported Protocols

### Phase 1: OpenID Connect

OpenID Connect (OIDC) should be the first supported SSO protocol.

Required provider examples:

- Google Workspace
- Microsoft Entra ID
- Okta
- Auth0
- Keycloak

Required OIDC features:

- Authorization Code Flow with PKCE.
- ID token validation.
- Access token exchange through the backend only.
- Provider discovery through `.well-known/openid-configuration` when available.
- Redirect URI allowlist validation.

### Phase 2: SAML 2.0

SAML may be added later for enterprise customers that require it.

SAML support must be tenant-scoped and should not be implemented before the OIDC
integration points are stable.

## Authentication Model

SSO identifies the user. NexGestion authorizes the user.

Identity provider responsibilities:

- Authenticate the person.
- Return stable identity claims.
- Optionally return group or organization claims.

NexGestion responsibilities:

- Link external identities to local users.
- Maintain user status.
- Maintain roles, groups, and permissions.
- Enforce tenant boundaries.
- Create and rotate NexGestion session tokens.
- Record login and account-linking audit events.

## Required Data Model

The exact database schema can change, but the following concepts must exist.

### Users

Represents a NexGestion application user.

Required fields:

- `id`
- `email`
- `display_name`
- `status`
- `created_at`
- `updated_at`

### Organizations

Represents a tenant or business workspace.

Required fields:

- `id`
- `name`
- `status`
- `created_at`
- `updated_at`

### SSO Connections

Represents one configured identity provider for one organization.

Required fields:

- `id`
- `organization_id`
- `provider_type`
- `protocol`
- `issuer`
- `client_id`
- `client_secret_encrypted`
- `authorization_endpoint`
- `token_endpoint`
- `jwks_uri`
- `redirect_uri`
- `scopes`
- `claim_mapping`
- `is_enabled`
- `created_at`
- `updated_at`

`client_secret` must never be stored in plaintext.

### User Identities

Links an external identity to a NexGestion user.

Required fields:

- `id`
- `user_id`
- `organization_id`
- `sso_connection_id`
- `provider_subject`
- `provider_email`
- `provider_email_verified`
- `last_login_at`
- `created_at`
- `updated_at`

The tuple of `sso_connection_id` and `provider_subject` must be unique.

## Login Flow

1. User opens the NexGestion login page.
2. User chooses an enabled SSO provider.
3. Frontend requests an SSO authorization URL from the backend.
4. Backend creates a temporary `state` value and PKCE verifier.
5. User is redirected to the identity provider.
6. Provider redirects back to NexGestion with `code` and `state`.
7. Backend validates `state`.
8. Backend exchanges `code` for tokens.
9. Backend validates the ID token.
10. Backend finds or provisions the linked NexGestion user.
11. Backend creates the normal NexGestion session.
12. Frontend redirects the user to the dashboard.

SSO login must end with the same session model as password login.

## Account Linking

Account linking must be explicit and deterministic.

Allowed linking strategies:

- Existing verified user email matches verified provider email.
- Administrator pre-provisions the user and assigns the SSO identity.
- Just-in-time provisioning is enabled for the organization.

Default behavior:

- If no matching user exists, deny login unless just-in-time provisioning is
  enabled for that organization.
- If provider email is not verified, do not auto-link by email.
- If multiple users match, deny login and require administrator action.

## Just-In-Time Provisioning

Just-in-time provisioning should be disabled by default.

When enabled, NexGestion may create a user after successful SSO login.

Required controls:

- Organization-level enablement.
- Allowed email domains.
- Default role or group.
- Optional approval requirement before first access.
- Audit event for every created user.

## Authorization

SSO claims must not directly grant privileged NexGestion permissions.

Supported mapping:

- Provider group to NexGestion group.
- Provider email domain to organization.
- Provider claim to display name or profile metadata.

Rules:

- Local NexGestion roles remain the source of truth.
- Administrator roles must require explicit local assignment.
- Group claim synchronization must be opt-in per SSO connection.
- Removing a provider group should not delete the NexGestion user.

## API Contract

Initial API endpoints should follow the existing `/api/auth` namespace.

Required public endpoints:

- `GET /api/auth/sso/connections`
- `POST /api/auth/sso/:connectionId/start`
- `GET /api/auth/sso/:connectionId/callback`

Required administrator endpoints:

- `GET /api/admin/sso/connections`
- `POST /api/admin/sso/connections`
- `PATCH /api/admin/sso/connections/:id`
- `DELETE /api/admin/sso/connections/:id`
- `POST /api/admin/sso/connections/:id/test`

The backend must never expose provider secrets through API responses.

## Frontend Requirements

The login page should support both password login and SSO login.

Required states:

- No SSO providers configured.
- One SSO provider configured.
- Multiple SSO providers configured.
- SSO redirect in progress.
- SSO callback failure.
- Account not allowed.
- Provider unavailable.

Password login should remain available unless disabled by an administrator after
SSO recovery access has been configured.

## Session Requirements

SSO sessions should use the same NexGestion session and refresh mechanism as
password login.

Rules:

- Do not use the provider access token as the NexGestion API token.
- Do not store provider tokens in browser local storage.
- Store refresh tokens only according to the existing backend session policy.
- Logout from NexGestion must clear the NexGestion session.
- Provider global logout may be added later but is not required for MVP.

## Security Requirements

Required controls:

- Authorization Code Flow with PKCE for OIDC.
- Strict redirect URI validation.
- `state` validation for CSRF protection.
- Nonce validation for ID tokens when applicable.
- Issuer and audience validation.
- Token expiration validation.
- JWKS key rotation support.
- Encrypted client secrets.
- HTTPS required outside local development.
- Login rate limiting.
- Audit logging for login success, login failure, account linking, and SSO
  configuration changes.

Rejected behavior:

- Accepting unsigned tokens.
- Trusting tokens from unknown issuers.
- Linking accounts by unverified email.
- Sending provider client secrets to the browser.
- Granting admin permissions from provider claims without local approval.

## Local-First Deployment Rules

SSO must remain optional because many NexGestion deployments may run fully inside
a local network.

Rules:

- A deployment without internet access must still support local login.
- Local administrator recovery login must remain possible.
- SSO configuration should be exportable and importable with secrets redacted.
- OIDC providers hosted on the same local network, such as Keycloak, should be
  supported.

## Administration Requirements

Administrators should be able to:

- Enable or disable an SSO connection.
- Configure provider metadata.
- Configure allowed email domains.
- Configure default role or group for provisioned users.
- Test a connection before enabling it.
- View recent SSO login failures.
- Disconnect an external identity from a user.
- Disable password login only when at least one recovery administrator exists.

## Audit Events

The following events must be logged:

- `sso.connection.created`
- `sso.connection.updated`
- `sso.connection.deleted`
- `sso.login.started`
- `sso.login.succeeded`
- `sso.login.failed`
- `sso.identity.linked`
- `sso.identity.unlinked`
- `sso.user.provisioned`
- `sso.group_mapping.updated`

Audit records should include:

- `organization_id`
- `user_id` when known
- `sso_connection_id` when known
- source IP when available
- user agent when available
- timestamp
- failure reason when applicable

## Error Handling

User-facing errors should be short and safe.

Examples:

- `Unable to sign in with this provider.`
- `Your account is not allowed to access this organization.`
- `This SSO connection is currently disabled.`

Internal logs may include more diagnostic detail, but must not expose tokens,
authorization codes, client secrets, or full claim payloads.

## MVP Checklist

Minimum acceptable SSO release:

- OIDC Authorization Code Flow with PKCE.
- One or more SSO connections per organization.
- Login button on the login page.
- Backend callback endpoint.
- ID token validation.
- Account linking by verified email.
- Optional just-in-time provisioning.
- Normal NexGestion session creation after SSO.
- Administrator connection test.
- Audit logging.
- Local password login fallback.

## Future Enhancements

- SAML 2.0 support.
- SCIM user provisioning.
- Provider group synchronization.
- Per-organization password login disablement.
- Provider-initiated logout.
- Conditional access signals.
- MFA policy awareness.
- CLI setup for local identity providers such as Keycloak.

## Implementation Notes

- Keep SSO code behind feature flags until the full MVP checklist is complete.
- Keep protocol-specific code isolated from the core session system.
- Reuse the existing `/api/auth/me`, refresh, and logout behavior after SSO
  login succeeds.
- Add automated tests for token validation, account linking, tenant isolation,
  and disabled-provider behavior.
- Document provider setup examples only after the internal API and data model are
  stable.
