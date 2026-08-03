# Report Files

## 1. Purpose

Generated CSV files are stored under the project `report/` directory. The server creates this directory at startup when it does not exist.

Module exports should write into a module subfolder, such as:

- `report/attendance`
- `report/notifications`
- future module folders under `report/<module>`

## 2. Admin API

All report file APIs require `reports.manage`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/reports/files` | List every file under `report/` |
| `GET` | `/api/reports/files/{path...}` | Download one report file |
| `DELETE` | `/api/reports/files/{path...}` | Delete one report file |

The file path is always relative to `report/`. Absolute paths and parent-directory traversal are rejected.

## 3. UI Rule

Only administrator users should see report file management. The UI should let an administrator:

- review generated report files;
- download a file to their device; and
- delete a file from the server.
