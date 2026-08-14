# User App Documentation

## Purpose

This folder describes how users operate NexGestion from the frontend. Keep user-facing workflows, screen behavior, labels, and tutorial-ready instructions here.

System design, database tables, API contracts, permission enforcement, and backend invariants belong under `docs/System`.

If a `docs/System` file mentions frontend behavior, copy the user-facing part into this folder so future user tutorial PDF or document generation can follow the markdown here.

## Documents

- [Notifications](./notifications.md)
- [Attendance](./attendance.md)
- [Access Control](./access-control.md)
- [Report Files](./report-files.md)
- [Barcode Scanning](./barcode-scanning.md)

## Writing Rules

- Write from the user's point of view.
- Describe what the user can see and do in the app.
- Mention required permissions only when they affect visible screens or available actions.
- Avoid database table names, internal service names, route middleware, and implementation-only details.
- Keep wording suitable for future tutorial export.
