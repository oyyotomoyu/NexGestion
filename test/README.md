# NexGestion API Test Design

This directory contains API-level test scripts for NexGestion. Tests simulate user workflows through HTTP requests only; scripts must not call Go services directly or modify SQLite records.

## Source Of Truth

Each script must follow the matching system document in `docs/System`:

| Script | System document | Scope |
| --- | --- | --- |
| `user.sh` | `docs/System/login.md`, `docs/System/user-system.md` | Auth and user account lifecycle |
| `role.sh` | `docs/System/role-system.md` | Role CRUD, assignment, permission grants |
| `permissions.sh` | `docs/System/permission-system.md` | Permission catalog and grant/revoke boundaries |
| `group.sh` | `docs/System/group-system.md` | Group CRUD, hierarchy, membership, generated roles |
| `attendance.sh` | `docs/System/attendance-system.md` | Attendance clocking, leave, approvals, reports |
| `notification.sh` | `docs/System/notification-system.md` | Notification types, send/edit/hide/export |
| `report_files.sh` | `docs/System/report-files.md` | Report file list/download/delete policy |
| `logs.sh` | `docs/System/log.md` | Log query filters, limits, retention-readable output |

When implementation and documentation disagree, record the failed case in the report and update the document or implementation in a separate change.

## Levels

Every script accepts one level argument:

```bash
./test/user.sh 1
./test/user.sh 2
./test/user.sh 3
./test/all.sh 3
```

Level `1` is the basic smoke workflow. It should prove the core documented happy path works.

Level `2` includes all documented ordinary correct and error cases for that function group: valid create/list/read/edit/delete, invalid JSON, missing auth, duplicate input, unknown IDs, protected records, insufficient permission, and documented delete rules.

Level `3` includes complex user scenarios: multi-step lifecycle behavior, cross-feature effects, permission changes, ownership-driven deletion, cascading cleanup, generated records, report generation, and any workflow that requires multiple actors or ordered state transitions.

Running level `2` also runs level `1`. Running level `3` runs levels `1` and `2` first.

## Report Contract

Every test run writes reports under `report/`:

- Markdown summary: `report/<script>-level<N>-<timestamp>.md`
- Machine-readable events: `report/<script>-level<N>-<timestamp>.jsonl`

Every case must report:

| Field | Required | Description |
| --- | --- | --- |
| `case` | Yes | Stable case id |
| `level` | Yes | `1`, `2`, or `3` |
| `description` | Yes | User-facing behavior under test |
| `method` | Yes | HTTP method or workflow name |
| `path` | Yes | API path or workflow label |
| `expected` | Yes | Expected status/result |
| `actual` | Yes | Actual status/result |
| `result` | Yes | `PASS` or `FAIL` |
| `reason` | On failure | Clear failure reason |

## Environment

Scripts target a running NexGestion API server.

Required:

- `curl`
- `jq`
- `bash`

Environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `BASE_URL` | `http://localhost:8080` | API server base URL |
| `ADMIN_EMAIL` | `admin@nexgestion.local` | Initial admin login email |
| `ADMIN_PASSWORD` | `password` | Initial admin password for the target test server |

The test server should use disposable `DATABASE_DIR`, `LOG_DIR`, and `REPORT_DIR` values. API tests create and delete records, and level `3` tests intentionally exercise lifecycle side effects.

## Script Rules

- Use only API calls.
- Keep case IDs stable.
- Clean up records created by a test when the system document allows deletion.
- Do not delete protected records unless the case is verifying that deletion is rejected.
- Treat failed cleanup as a test failure.
- Never put access tokens, refresh tokens, passwords, or raw cookies in reports.
- Prefer unique names and emails with timestamps so repeated runs do not collide.

## Module Checklist

Each module script should cover:

- Authentication required for protected endpoints.
- Permission success and failure when the document defines permissions.
- Create, list, read, update, and delete operations when available.
- Validation errors for required fields, bad formats, duplicate unique fields, and unknown IDs.
- Deletion policy, including records that must not be deleted directly.
- Audit/report side effects when the document defines them.
