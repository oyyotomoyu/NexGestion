# Template Library

## 1. Purpose

A shared library of organization template files — for example 會員資料填寫表 or 新進人員資料表 — that qualifying users upload once and everyone with access can find and download. This is separate from [`report-files.md`](./report-files.md), which holds system-generated report output rather than user-uploaded shared documents.

Any file type is accepted; there is no extension/MIME allow-list, only the size limits in section 3.

## 2. Permission Model

| Permission | Grants |
| --- | --- |
| `templates.read` | Browse and download template files visible to the current user |
| `templates.upload` | Upload new template files and set their visible roles/groups |
| `templates.manage` | View, download, and delete **all** template files regardless of visibility; manage storage settings |

Delete is not a separate permission key. It follows the same ownership convention as `NotificationService.Hide` in [`notification-system.md`](./notification-system.md):

- **Own upload**: the uploader (`uploaded_by_user_id == caller`) may delete it using their existing `templates.upload` permission — no extra grant needed.
- **Any file**: requires `templates.manage` (administrator tier — the Admin role's `grants_all_permissions` satisfies this automatically).

Visibility is audience-based, matching `notification_audiences`: an uploader assigns one or more audiences (organization-wide, a group, a role, or a specific user) when uploading. A user with `templates.read` only sees files whose audience matches their own roles/groups. `templates.manage` bypasses audience filtering entirely.

## 3. Size Limits

Stored as key/value rows in `system_settings` so an administrator can tune them without a redeploy.

| Setting | Default | Rationale |
| --- | --- | --- |
| `template_file_max_bytes` | 20 MB per file | Covers Word/Excel/PDF forms even with an embedded logo or scanned signature; typical templates are under 1 MB, so this is headroom, not a squeeze. |
| `template_storage_max_bytes` | 500 MB total | At a realistic ~1–2 MB average, this covers several hundred templates — well beyond what a shared form library needs. |

Enforcement:

- **Upload**: rejected if the file exceeds `template_file_max_bytes` (`template_file_too_large`, HTTP 413), or if `current total + new file size` would exceed `template_storage_max_bytes` (`template_storage_limit_exceeded`, HTTP 507).
- **Download**: not size-gated. A file already accepted under the cap stays downloadable even if the org is later at/over the total limit — the limit blocks new uploads, not access to what's already there.

Uploaded files are stored under the project `template/` directory (mirroring `report/`), named by generated id plus original extension; the original filename, content type, and a SHA-256 checksum are kept in the database row.

## 4. Database

New `template.db`, mirroring the `notification_audiences` shape:

- `template_files`: id, original filename, stored path, content type, size, SHA-256 checksum, description, uploader, timestamps. Delete is a hard delete — there is no archive/soft-delete state — removing both the row and the file on disk.
- `template_file_audiences`: destination scope per file — organization, group, role, or user. Same `scope` CHECK-constraint pattern as `notification_audiences`.

Role, group, and user ids are stored as text, same as `notification.db`, because those records live in `user.db`.

## 5. API

All routes require authentication.

| Method | Path | Permission | Description |
| --- | --- | --- | --- |
| `GET` | `/api/templates` | `templates.read` | List files visible to the caller (admin/`templates.manage` sees all) |
| `GET` | `/api/templates/{id}/download` | `templates.read` | Download a file visible to the caller, or any file with `templates.manage` |
| `POST` | `/api/templates` | `templates.upload` | Upload a file (`multipart/form-data`: `file`, `description`, `audiences` as a JSON array) |
| `DELETE` | `/api/templates/{id}` | Authenticated; owner or `templates.manage` | Delete a file — enforced inside the service, same ownership pattern as `POST /api/notifications/{id}/hide` |
| `GET` | `/api/templates/storage` | `templates.manage` | Current usage vs. configured caps |

`GET /api/templates` follows the shared List API Query Standard in [`architecture.md`](./architecture.md#61-list-api-query-standard).

Template list query definition:

| Option | Definition |
| --- | --- |
| Response array | `templates` |
| Keyword fields | `original_filename`, `description` |
| Sort fields | `original_filename`, `size_bytes`, `created_at`, `updated_at` |
| Default sort | `created_at desc` |
| Extra filters | `uploaded_by_user_id`, `audience_scope` |

Download responses set `Content-Disposition` with both an ASCII-sanitized fallback and an RFC 5987 `filename*=UTF-8''...` value, since template names are commonly non-ASCII (e.g. 會員資料填寫表.docx).

## 6. UI Rule

- Regular users see only the download list, filtered to files their roles/groups can access; no upload/delete controls unless they hold `templates.upload`.
- Uploaders see an upload form requiring at least one audience selection, plus delete on their own uploads.
- Administrators (`templates.manage`) see every file regardless of audience, plus delete-any and a storage usage view (used vs. `template_storage_max_bytes`).

A client API module exists at `client/src/requests/templates` (list, upload, download URL, delete, storage usage), following the same shape as `client/src/requests/reports`. As with `report-files.md`, the module is wired but no page consumes it yet.
