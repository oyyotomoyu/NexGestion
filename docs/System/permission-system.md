# Permission System

User-facing permission and access-control documentation belongs in [`../UserApp/access-control.md`](../UserApp/access-control.md).

## 1. Authorization Model

NexGestion uses role-based access control. A user may have any number of roles, and each role may grant any number of permissions.

For every authenticated API request:

1. Authentication verifies the access token and identifies the user.
2. Permission middleware reads the permission required by the route.
3. `server/apis` checks the effective permissions from every role assigned to the user.
4. If at least one role grants the required permission, the request continues.
5. Otherwise, the server returns `403 Forbidden`.

Role grants are additive. A missing permission on one role does not override a grant from another role. Groups and group membership do not contribute permissions.

The public health, login, and token-refresh endpoints are the only routes that do not perform a user permission check because no authenticated user exists at that point.

## 2. Permission Catalog

[`config/permission.json`](../../config/permission.json) is the authoritative catalog. Each definition contains:

```json
{
  "key": "roles.read",
  "module": "roles",
  "description": "View roles and their assigned permissions",
  "high_risk": false,
  "high_risk_reason": "",
  "requires_password": false
}
```

At startup the server:

- validates that the catalog is present and non-empty;
- rejects missing keys, missing modules, and duplicate keys;
- inserts new definitions into `user.db`;
- synchronizes module and description changes; and
- removes obsolete definitions and their obsolete grants.

The deployed server package includes the catalog under `server/config/permission.json`. `NEXGESTION_PERMISSION_CONFIG` may specify an explicit catalog path.

## 3. Route Registration

Every authenticated route is registered with a permission key. Router initialization fails if that key does not exist in the catalog. Therefore, adding a protected request requires both:

1. adding or selecting its permission in `config/permission.json`; and
2. declaring that key when registering the route in `server/apis/router.go`.

Current mappings:

| Requests | Permission |
| --- | --- |
| Read current user, logout, list/read users | `users.read` |
| Create/update/delete users | `users.manage` |
| List/read roles and role users | `roles.read` |
| Create/update/delete custom roles | `roles.manage` |
| Assign/remove roles from users | `roles.assign` |
| List the permission catalog | `permissions.read` |
| Grant/revoke role permissions | `permissions.assign`, plus administrator enforcement |
| List/read groups and members | `groups.read` |
| Create/update/delete groups | `groups.manage` |
| Add/update/remove group members | `groups.assign` |
| Read request and audit logs | `logs.read` |
| Read the current user's attendance | `attendance.read.self` |
| Sign the current user in and out | `attendance.clock.self` |
| Read other users' attendance | `attendance.read` |
| Correct attendance and generate reports | `attendance.manage` |
| Read and download organization attendance reports | `attendance.reports.read` |

`server/apis/access_control.go` owns the allow/deny function and permission middleware. The system package supplies role and permission data but does not authorize ordinary API requests. System services retain only domain invariants, such as protecting the Admin role. Route middleware is the mandatory authorization boundary.

`GET /api/permissions` follows the shared List API Query Standard in [`architecture.md`](./architecture.md). The response array property is `permissions`.

Permission list query definition:

| Option | Definition |
| --- | --- |
| Keyword fields | `permission_key`, `module`, `description` |
| Sort fields | `permission_key`, `module` |
| Default sort | `permission_key asc`, then `id asc` |
| Extra filters | `module` |

## 4. Role Permission Editing

The protected initial administrator can edit a custom role's permissions from the role detail screen. Each catalog permission is represented as a true/false checkbox:

- `true` creates a `role_permissions` grant;
- `false` removes that grant.

Only the initial administrator may call these grant/revoke operations. Granting another user `roles.manage` or `permissions.assign` does not let that user edit role grants. Non-admin users with `roles.read` may see the permissions assigned to a role but receive no editing controls.

Catalog entries may mark a permission as high risk with `high_risk = true`, a human-readable `high_risk_reason`, and `requires_password = true`. When the protected initial administrator grants one of these permissions to a custom role, the client must show a warning and ask for the administrator's current login password. The server verifies that password before inserting the grant. Missing or incorrect passwords return `403 Forbidden`. Revoking a high-risk permission does not require password confirmation because it reduces access.

High-risk permissions include grants that can escalate privileges, manage users or roles, export records, edit payroll/attendance/approval configuration, view broad sensitive employee data, or manage confidential HR employee-relations cases. Examples include `users.manage`, `roles.manage`, `roles.assign`, `permissions.assign`, `salary.read`, `salary.settlement.configure`, `attendance.manage`, `approvals.templates.manage`, and `hr.employee_relations.manage`.

## 5. System Access Permissions

Every documented business system has a module-level `.access` permission in the catalog, for example `hr.access`, `crm.access`, `inventory.access`, and `salary.access`. Administrators use these entries to decide whether a custom role can enter a system at all.

Function-level permissions remain separate and should be checked by APIs that read, create, update, delete, export, approve, or configure data. For example, a small organization may grant `hr.access` to a non-HR role, while still withholding sensitive function permissions until that role actually needs them.

## 6. Admin Invariants

The system Admin role has `grants_all_permissions = true`. Consequently:

- every current catalog permission evaluates to true;
- future catalog permissions automatically evaluate to true;
- the role response exposes all catalog permissions;
- the Admin role cannot be renamed, edited, deleted, reassigned, or removed; and
- individual Admin permission values cannot be granted or revoked.

Application checks and SQLite protection enforce these invariants.
