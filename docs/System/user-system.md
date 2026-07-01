# UserSystem Design

## 1. Purpose

UserSystem owns user identity, authentication, authorization, and organization membership in NexGestion.

All UserSystem data is stored in a dedicated SQLite database named `user.db`. Other modules may reference a user by its immutable `user_id`, but must not duplicate or directly modify UserSystem records.

Each NexGestion installation manages exactly one organization. That organization may contain multiple departments, branches, or teams represented by groups.

This document defines the initial data model direction. It is not yet a finalized database migration.

## 2. User And Employee Are Different Concepts

A **user** is an account that can sign in to NexGestion. An **employee** is a person employed by an organization.

- In the initial scope, every employee record has one user account and signs in with email.
- Keeping account and employee data in separate tables still allows service or external accounts to be supported later without inventing employee data for them.
- Authentication fields belong to the user account.
- HR-only fields belong to an employee profile and should not be required by UserSystem unless an HR module is installed.

The first version keeps both the user account and its employee profile in `user.db`. A future HR module can extend employee information in its own database while referencing the same `user_id` or `employee_id`.

## 3. Recommended Core Tables

### 3.1 `users`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable internal identifier; used by other modules |
| `display_name` | TEXT | Yes | Name shown in the UI |
| `email` | TEXT | Yes | Unique sign-in email; trim whitespace and normalize before storage |
| `password_hash` | TEXT | Yes | Password hash only; never store a plaintext password |
| `status` | TEXT | Yes | `pending`, `active`, `disabled`, or `locked` |
| `locale` | TEXT | No | Preferred UI language, such as `CHT` |
| `timezone` | TEXT | No | Preferred IANA timezone, such as `Asia/Taipei` |
| `must_change_password` | BOOLEAN | Yes | Requires a password change after initial or administrative reset |
| `failed_login_count` | INTEGER | Yes | Consecutive failed sign-in attempts |
| `locked_until` | DATETIME | No | Temporary account lock expiration |
| `last_login_at` | DATETIME | No | Last successful sign-in time |
| `password_changed_at` | DATETIME | No | Last password update time |
| `created_at` | DATETIME | Yes | Creation time in UTC |
| `updated_at` | DATETIME | Yes | Last update time in UTC |
| `deleted_at` | DATETIME | No | Soft deletion time; preserves historical references |

`email` is the only login identifier in the initial version. Its uniqueness must be case-insensitive, so addresses such as `User@example.com` and `user@example.com` cannot create separate accounts. The database must enforce this rule with a unique index or collation in addition to API validation.

### 3.2 `employee_profiles`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable employee profile identifier |
| `user_id` | TEXT/UUID | Yes | Unique one-to-one reference to `users.id` |
| `employee_number` | TEXT | Yes | Organization-visible employee number, unique within an organization |
| `legal_name` | TEXT | No | Legal or payroll name when different from display name |
| `preferred_name` | TEXT | No | Preferred workplace name |
| `work_email` | TEXT | No | Work contact email; may differ from the login email |
| `work_phone` | TEXT | No | Work contact number |
| `job_title` | TEXT | No | Human-readable job title |
| `employment_status` | TEXT | Yes | `active`, `on_leave`, `terminated`, or another defined state |
| `hire_date` | DATE | No | Employment start date |
| `termination_date` | DATE | No | Employment end date |
| `manager_employee_id` | TEXT/UUID | No | Reference to the employee's manager |
| `created_at` | DATETIME | Yes | Creation time in UTC |
| `updated_at` | DATETIME | Yes | Last update time in UTC |

Sensitive HR data such as salary, bank account, identity document number, home address, birth date, and emergency contacts should not be added to the core table. Those fields require stricter permissions and should belong to a dedicated HR module.

### 3.3 Roles And Permissions

Roles and permissions are many-to-many relationships rather than columns on `users`.

- `roles`: role identity, name, description, and system/custom marker
- `permissions`: stable permission key, module, and description
- `user_roles`: assigns one or more roles to a user
- `role_permissions`: grants permissions to a role

Example permission keys include `users.read`, `users.manage`, and `inventory.write`.

A user may hold multiple roles at the same time. Effective permissions are the union of permissions granted by all active roles assigned to that user.

### 3.4 Groups

Groups represent organization membership, not access levels. Examples include a department, branch, store, project team, or working group.

- `groups`: group identity, name, type, parent group, and status
- `user_groups`: user membership, optional title, and membership dates

A user may belong to multiple groups. If group membership later grants permissions, that behavior should be modeled explicitly instead of treating group names as roles.

## 4. Supporting Tables

The following tables are recommended as authentication features are introduced:

- `sessions`: active login sessions or refresh tokens, stored as hashes
- `password_reset_tokens`: short-lived, single-use reset token hashes
- `login_audit_logs`: successful and failed login events
- `user_audit_logs`: administrative changes to users, roles, and groups
- `mfa_methods`: optional multi-factor authentication configuration

Secrets and raw session or reset tokens must never be stored directly.

## 5. Key Design Decisions

- Use an immutable UUID `id` for database relationships; do not use `employee_number` or email as a foreign key.
- One installation represents one organization; no `organization_id` is required in every UserSystem table.
- Keep normalized email, employee number, role name, and permission key unique.
- Require email for every employee account and use it as the login identifier.
- Store timestamps in UTC and convert them for display using the user's timezone.
- Prefer disabling or soft-deleting users over physical deletion so business records keep valid ownership history.
- Password hashing should use Argon2id or bcrypt with per-password salts and an upgradeable hash format.
- Role controls what a user may do; group describes where the user belongs.

## 6. Initial Administrator

When `user.db` is first created, UserSystem must create an initial administrator with:

- Employee number: `0`
- System role: `Administrator`
- Account status: `active`
- Permission behavior: full system access
- Password behavior: require a password change after the initial sign-in
- Deletion behavior: both physical deletion and soft deletion are blocked at the database level

Employee number `0` is reserved and cannot be assigned to another employee. The administrator email may be supplied through `NEXGESTION_ADMIN_EMAIL` and defaults to `admin@nexgestion.local`. The initial password may be supplied through `NEXGESTION_ADMIN_PASSWORD`; when omitted, a cryptographically random temporary password is generated and printed once in the server startup log. Only its bcrypt hash is stored in `user.db`.

## 7. Initial Scope Recommendation

The first implementation should include:

1. `users`
2. `employee_profiles`
3. `roles`
4. `permissions`
5. `user_roles`
6. `role_permissions`
7. `groups`
8. `user_groups`

Sessions, password reset, MFA, and audit tables can be added with their corresponding features, although login auditing should be introduced early.

## 8. Confirmed Requirements

- One installation manages one organization.
- An organization may contain multiple groups.
- Every employee account signs in using a required, unique email address.
- A user may have multiple roles, and roles determine permissions.
- The initial administrator uses reserved employee number `0`.

## 9. Open Questions

- Should a future installer or first-run web page replace environment variables for initial administrator setup?
- Can roles be limited to a specific group, or are all roles organization-wide?
- Can one employee belong to multiple groups?
- Are departments, branches, and project teams all group types?
