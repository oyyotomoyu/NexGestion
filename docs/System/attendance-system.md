# Attendance System

User-facing attendance workflow documentation belongs in [`../UserApp/attendance.md`](../UserApp/attendance.md).

## 1. Purpose

The Attendance System records whether each user is working or non-working, every daily work session, and the user's total working hours.

Every authenticated user can open the Attendance page and manage their own current-day attendance. Attendance data is stored separately in `attendance.db`.

The user may sign in and sign out multiple times during an open attendance day. Each completed pair is a work session. Overnight shifts, manual timesheets, leave requests, and payroll calculations are outside the initial scope.

## 2. Attendance Day and Timezone

An attendance day is a calendar date in the user's attendance timezone. The timezone is resolved in this order:

1. the user's configured IANA timezone, such as `America/New_York`;
2. the installation's configured business timezone; or
3. the default `Asia/Taipei`.

The server determines the attendance timezone and date. Clients must not submit or calculate the current date because a device clock or timezone may be incorrect.

All sign-in and sign-out timestamps are recorded to minute precision using server time and stored in UTC. Seconds are normalized to `00`. Each daily record stores the IANA timezone used to determine its boundaries. Responses include the timezone and may include locally formatted values.

Example:

```txt
Attendance timezone: Asia/Taipei
Attendance date:   2026-07-28
Sign-in UTC:       2026-07-28T00:57:00Z
Local sign-in:     2026-07-28T08:57:00+08:00
```

A user timezone change applies to the first attendance day created while that user is `non_working`. A continuous working session keeps its original timezone through every midnight rollover until sign-out. The change must not move existing records between dates.

## 3. Daily State Machine

Each user has at most one daily attendance record for an attendance date.

| Status | Meaning | Available action |
| --- | --- | --- |
| `non_working` | The user has no open work session | Sign in while the attendance day is open |
| `working` | The user has signed in and has not signed out | Sign out |

State transitions:

```txt
                 ┌─────────────────────────────────────┐
                 │                                     │
                 v                                     │
non_working ── Sign in ──> working ── Sign out ──> non_working
```

This cycle may repeat any number of times while the attendance day is open. The status follows the latest session:

- `non_working` means there is no open session;
- `working` means exactly one session has a sign-in time and no sign-out time; and
- a closed day with no sessions is classified as absent in reports.

Only one session may be open for a user at a time.

### 3.1 Sign In

When a user selects **Sign in**, the server:

1. resolves the current attendance date;
2. creates or locks that user's daily record;
3. verifies that its status is `non_working` and the day is still open;
4. creates a new session with `sign_in_at` set to the current server minute;
5. changes the status to `working`; and
6. records an immutable `sign_in` event.

The operation is atomic. Repeated or concurrent sign-in requests must not create more than one open session.

### 3.2 Sign Out

When a working user selects **Sign out**, the server:

1. resolves and locks the current daily record;
2. verifies that its status is `working`;
3. closes the current open session with `sign_out_at` set to the current server minute;
4. calculates that session's duration and updates the daily total;
5. changes the status to `non_working`; and
6. records an immutable `sign_out` event.

The sign-out minute must not be earlier than the sign-in minute. Session duration is the minute difference between the two timestamps. Repeated or concurrent sign-out requests must not overwrite the original sign-out time or close more than one session.

Hours are the primary reporting and UI unit. Minute totals are retained internally so time is not lost. For example, `451` accumulated minutes is reported as `7 h 31 min` and may be represented numerically as approximately `7.52` hours.

### 3.3 Midnight Rollover

Working across midnight is supported. At `00:00` in the user's attendance timezone, the server atomically splits an open session:

1. close the old day's session at the exact local-midnight boundary;
2. calculate and add the old segment to the old day's total;
3. finalize the old attendance day;
4. create the next day's attendance record with status `working`; and
5. create a continuation session whose sign-in time is that same midnight boundary.

The user remains `working`; no user interaction is required. When the user later selects **Sign out**, only the continuation session on the new attendance day is closed.

Example in `Asia/Taipei`:

```txt
Sign in:             2026-07-28 22:30
Day boundary:        2026-07-29 00:00
July 28 allocation:  1 h 30 min
July 29 continuation starts: 00:00
Sign out:            2026-07-29 03:15
July 29 allocation:  3 h 15 min
```

If the scheduled rollover runs late or the server was offline, the next attendance read, sign-in, or sign-out request performs the same reconciliation before processing the request. The split uses the real midnight boundary, not the delayed job time. A session spanning several missed days is split once at every intervening local midnight.

When a day closes:

- a user with no sessions remains `non_working` and is classified as absent;
- completed sessions contribute to that day's total;
- an open session is rolled into the next day; and
- a rollover failure is flagged for administrator review and must never discard elapsed time.

### 3.4 Timezone and Daylight-Saving Rules

- Day boundaries are calculated using the stored IANA timezone, not a fixed UTC offset.
- Duration is calculated from UTC instants, so daylight-saving transitions produce the actual elapsed time.
- A local day may contain 23 or 25 elapsed hours.
- Ambiguous or skipped local clock times do not alter stored UTC timestamps.
- Every session segment belongs to exactly one local attendance date.

## 4. Data Model

### 4.0 Leave approval workflow

A submitted leave request begins in `pending` status. Its approver is resolved from the Group System's organization hierarchy:

- project groups never participate in leave approval;
- organization groups have five levels, with level 1 highest and level 5 lowest;
- an ordinary member's request goes first to a manager in the same organization group;
- a group manager's own request moves to the manager of the immediately higher organization level;
- missing managers are skipped by walking upward toward level 1;
- a highest-level manager, a user without an organization assignment, or a hierarchy with no eligible manager falls back to an administrator; and
- a requester can never approve their own request.

Leave request states are:

```txt
pending ── approve ──> approved
   │
   ├────── reject ───> rejected
   │
   └──── requester cancellation ───> cancelled
```

Only `pending` requests may be approved or rejected. A requester may cancel only their own pending request. Every submission, assignment, reassignment, decision, and cancellation is stored in an append-only audit event.

The approval assignment records the resolved organization group, assigned manager users or administrator fallback, assignment time, decision actor, decision time, and optional decision note. Multiple managers at the same level use first-decision-wins semantics.

Approved or pending requests for the same user must not overlap. A full day occupies the configured eight-hour working day; an hourly request occupies its exact local time interval and must be at least one hour.

Manager approval endpoints use record-scoped authorization:

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/attendance/leave-approvals` | List requests assigned to the authenticated manager or administrator |
| `PATCH` | `/api/attendance/leave-approvals/{id}` | Approve or reject one assigned pending request |

Decision request:

```json
{
  "decision": "approved",
  "note": "Coverage confirmed"
}
```

`decision` accepts `approved` or `rejected`. The endpoint verifies the stored assignment rather than granting organization-wide attendance access. Requests created before assignment support was introduced are resolved through the current hierarchy when an approval inbox is first loaded.

Attendance data is owned by the Attendance System and stored in `attendance.db`. User identity remains owned by UserSystem in `user.db`; attendance records reference immutable user IDs without duplicating user account data.

### 4.1 `attendance_days`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable record ID |
| `user_id` | TEXT/UUID | Yes | Immutable UserSystem user ID |
| `attendance_date` | DATE | Yes | Calendar date in the stored attendance timezone |
| `timezone` | TEXT | Yes | IANA timezone used for this day's boundaries |
| `status` | TEXT | Yes | `non_working` or `working` |
| `total_worked_minutes` | INTEGER | Yes | Sum of completed session minutes; internal exact value |
| `requires_review` | BOOLEAN | Yes | True for incomplete or corrected records needing review |
| `created_at` | DATETIME | Yes | UTC creation timestamp |
| `updated_at` | DATETIME | Yes | UTC last-update timestamp |

Constraints:

- `(user_id, attendance_date)` is unique.
- `total_worked_minutes` cannot be negative.
- `working` requires exactly one open session.
- `non_working` requires no open sessions.

### 4.2 `attendance_sessions`

Each sign-in creates a new session. A user may have multiple sessions on one attendance day.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable session ID |
| `attendance_day_id` | TEXT/UUID | Yes | Owning daily record |
| `sequence_number` | INTEGER | Yes | Session order within the day, beginning at `1` |
| `continued_from_session_id` | TEXT/UUID | No | Previous day's session when created by midnight rollover |
| `sign_in_at` | DATETIME | Yes | UTC server timestamp at minute precision |
| `sign_out_at` | DATETIME | No | UTC server timestamp at minute precision |
| `worked_minutes` | INTEGER | No | Finalized duration for this session |
| `created_at` | DATETIME | Yes | UTC creation timestamp |
| `updated_at` | DATETIME | Yes | UTC last-update timestamp |

Constraints:

- `(attendance_day_id, sequence_number)` is unique.
- Only one session per attendance day may have `sign_out_at IS NULL`.
- `worked_minutes` is `NULL` for an open session and non-negative for a closed session.
- `sign_out_at` cannot be earlier than `sign_in_at`.

### 4.3 `attendance_events`

This append-only table provides an audit trail.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable event ID |
| `attendance_day_id` | TEXT/UUID | Yes | Related daily record |
| `attendance_session_id` | TEXT/UUID | No | Related session for sign-in/sign-out events |
| `user_id` | TEXT/UUID | Yes | User affected by the event |
| `actor_user_id` | TEXT/UUID | Yes | User or administrator performing the action |
| `event_type` | TEXT | Yes | `sign_in`, `sign_out`, `mark_absent`, or `correction` |
| `occurred_at` | DATETIME | Yes | UTC server timestamp |
| `previous_status` | TEXT | No | Status before the event |
| `new_status` | TEXT | Yes | Status after the event |
| `reason` | TEXT | No | Required for administrator corrections |

Normal users cannot update or delete events.

### 4.4 `attendance_monthly_reports`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable report ID |
| `user_id` | TEXT/UUID | Yes | Report owner |
| `report_month` | TEXT | Yes | Month in `YYYY-MM` form |
| `timezone` | TEXT | Yes | Latest attendance timezone represented in the report month |
| `scheduled_work_days` | INTEGER | Yes | Expected work days under the configured calendar |
| `present_days` | INTEGER | Yes | Days with a completed sign-in |
| `absent_days` | INTEGER | Yes | Closed absent days |
| `incomplete_days` | INTEGER | Yes | Records requiring review |
| `worked_minutes` | INTEGER | Yes | Exact sum of finalized session minutes |
| `worked_hours` | DECIMAL | Yes | Primary report value, calculated as `worked_minutes / 60` |
| `generated_at` | DATETIME | Yes | Last generation time in UTC |
| `source_updated_at` | DATETIME | Yes | Latest included daily-record update |

`(user_id, report_month)` is unique.

These rows are the structured source used to generate the official monthly CSV. They are not themselves the downloadable report file.

### 4.5 `attendance_report_exports`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable export ID |
| `report_month` | TEXT | Yes | Exported month in `YYYY-MM` form |
| `format` | TEXT | Yes | Always `csv` in the initial implementation |
| `relative_path` | TEXT | Yes | Path relative to the configured report directory |
| `sha256` | TEXT | Yes | File integrity checksum |
| `row_count` | INTEGER | Yes | Number of exported user rows |
| `generated_at` | DATETIME | Yes | Successful generation time in UTC |
| `source_updated_at` | DATETIME | Yes | Latest source value included in this file |

Only the latest valid export is active for a report month. Replaced export metadata remains available for audit according to the report-retention policy.

## 5. Monthly CSV Reports

A scheduled job calculates per-user report rows after the end of every calendar month in each user's attendance timezone. Records are grouped by their stored `attendance_date`, so a later timezone change does not reclassify historical work. The organization CSV is generated after that month has closed for every eligible user's timezone.

For example, the July 2026 report is generated after `2026-08-01T00:00:00+08:00`.

Generation requirements:

- create one report for every active user who had employment or attendance activity during the month;
- include only finalized session durations in `worked_minutes`;
- count incomplete working records separately;
- produce the same result when safely rerun;
- replace or update an existing report when a reviewed daily record changes; and
- record generation failures without deleting the previous valid report.

The first version sums every completed session. Time between a sign-out and the next sign-in is non-working time and is not included. It does not calculate overtime.

Hours are the main report unit. `worked_hours` is rounded to two decimal places for report and export presentation. The UI also displays hours and remaining minutes, such as `7 h 31 min`. The database retains the source `worked_minutes` value so monthly recalculation does not accumulate decimal rounding drift.

### 5.1 CSV File Contract

Each completed month produces one organization-level CSV file containing one row per eligible user.

Default path:

```txt
reports/
└── attendance/
    └── 2026/
        └── attendance-2026-07.csv
```

CSV requirements:

- media type: `text/csv`;
- character encoding: UTF-8 with a byte-order mark for spreadsheet compatibility;
- structure: RFC 4180-compatible comma-separated fields;
- line endings: CRLF;
- first row: stable English machine-readable column names;
- decimal separator: period;
- dates: `YYYY-MM-DD`;
- timestamps: ISO 8601 with timezone offset; and
- values containing commas, quotes, or line breaks must be quoted and escaped.

Columns:

```csv
report_month,user_id,employee_number,display_name,timezone,scheduled_work_days,present_days,absent_days,incomplete_days,worked_hours,worked_hours_minutes,worked_minutes
```

Example:

```csv
report_month,user_id,employee_number,display_name,timezone,scheduled_work_days,present_days,absent_days,incomplete_days,worked_hours,worked_hours_minutes,worked_minutes
2026-07,52d6d9dd-66a3-41fe-ac8a-66c73a2414e9,1042,Lin Mei,Asia/Taipei,23,22,1,0,173.52,173:31,10411
```

`worked_hours` is the primary report value. `worked_hours_minutes` preserves a human-readable hour-and-minute representation, and `worked_minutes` preserves an exact machine-readable total.

The generator writes to a temporary file, flushes it, calculates its checksum, and atomically renames it to the final path. A failed generation must not replace the previous valid CSV.

## 6. Retention

Detailed attendance data is retained for six rolling calendar months:

- `attendance_days`;
- `attendance_sessions`;
- `attendance_events`; and
- operational generation logs tied only to those detailed records.

The cleanup job runs after successful monthly report generation. It deletes detailed records only when:

1. they are older than six calendar months;
2. their monthly report was generated successfully; and
3. the official CSV was generated and its checksum verified; and
4. they do not require review.

Monthly aggregate rows and CSV report files are retained after detailed records expire so historical attendance summaries remain available. Their final legal retention period should be configured according to the organization's employment and payroll obligations.

Cleanup must run in transactions and write an audit record containing the affected month and record counts.

## 7. Permissions

These permission keys are planned and must be added to `config/permission.json` when the Attendance APIs are implemented:

| Permission | Allows |
| --- | --- |
| `attendance.read.self` | View the current user's daily and monthly attendance |
| `attendance.clock.self` | Sign the current user in and out |
| `attendance.read` | View attendance for other users |
| `attendance.manage` | Correct records and resolve incomplete days |
| `attendance.reports.read` | View organization-level monthly reports |

The initial Admin role automatically receives every attendance permission through `grants_all_permissions`.

Every Attendance API route must declare its permission in `server/apis/router.go`. Request authorization follows the existing role-union rule: access passes when any assigned role grants the required permission.

## 8. API Contract

All endpoints require authentication and request-level permission middleware.

| Method | Path | Permission | Description |
| --- | --- | --- | --- |
| `GET` | `/api/attendance/today` | `attendance.read.self` | Read the current user's state |
| `POST` | `/api/attendance/today/sign-in` | `attendance.clock.self` | Sign in |
| `POST` | `/api/attendance/today/sign-out` | `attendance.clock.self` | Sign out |
| `GET` | `/api/attendance/days` | `attendance.read.self` | List the current user's retained daily records |
| `GET` | `/api/attendance/monthly/{month}` | `attendance.read.self` | Read the current user's monthly report |
| `GET` | `/api/attendance/users/{userId}/days` | `attendance.read` | List another user's daily records |
| `GET` | `/api/attendance/reports/{month}` | `attendance.reports.read` | Read an organization monthly report |
| `POST` | `/api/attendance/reports/{month}/generate` | `attendance.manage` | Generate or refresh the official monthly report |
| `GET` | `/api/attendance/reports/{month}/csv` | `attendance.reports.read` | Download the official monthly CSV |
| `PATCH` | `/api/attendance/days/{id}` | `attendance.manage` | Correct a daily record with a reason |

Sign-in and sign-out requests contain no client timestamp:

```json
{}
```

Current-day response:

```json
{
  "id": "208a888b-2c5f-4570-8f37-c0bb39129768",
  "user_id": "52d6d9dd-66a3-41fe-ac8a-66c73a2414e9",
  "attendance_date": "2026-07-28",
  "timezone": "Asia/Taipei",
  "status": "working",
  "worked_hours": 2.5,
  "worked_minutes": 150,
  "sessions": [
    {
      "id": "3ae60ef0-a039-49e6-8187-2c3d0a567272",
      "sequence_number": 1,
      "sign_in_at": "2026-07-28T00:57:00Z",
      "sign_out_at": "2026-07-28T03:27:00Z",
      "worked_minutes": 150
    },
    {
      "id": "72894861-23fe-49b8-b9d1-e73af35bc716",
      "sequence_number": 2,
      "continued_from_session_id": null,
      "sign_in_at": "2026-07-28T04:10:00Z",
      "sign_out_at": null,
      "worked_minutes": null
    }
  ],
  "requires_review": false
}
```

`worked_hours` and `worked_minutes` include completed sessions only. The current open session is added after sign-out.

Expected errors:

| Status | Condition |
| --- | --- |
| `400 Bad Request` | Invalid month or correction |
| `401 Unauthorized` | Missing or invalid authentication |
| `403 Forbidden` | Missing route permission |
| `404 Not Found` | Requested attendance record does not exist |
| `409 Conflict` | Action conflicts with the current attendance status |

API errors should use stable error codes so the UI can translate them:

```json
{
  "code": "attendance_already_signed_in",
  "error": "attendance has already been signed in"
}
```

## 9. Attendance Page

The authenticated navigation includes an **Attendance** destination visible to users who have an attendance permission.

The mobile and desktop page contains:

1. today's date and the attendance timezone;
2. the current attendance status;
3. every recorded session's sign-in and sign-out minute;
4. accumulated working time with hours as the main unit and minutes shown for precision;
5. one primary action button; and
6. recent daily records and available monthly reports.

Users with `attendance.reports.read` can download the official CSV for a completed month. Report files are served through the authorized API and are not exposed as unrestricted static files.

Button behavior:

| Status | Button |
| --- | --- |
| `non_working` during an open attendance day | Primary **Sign in** button |
| `working` | Destructive or emphasized **Sign out** button |
| `non_working` after the attendance day closes | No attendance action |

After each sign-out, the status returns to `non_working` and the button changes back to **Sign in**, allowing another work session during the same day.

The client refreshes today's record after every successful action. While a request is in progress, the action is disabled to prevent double submission.

The UI must not optimistically change attendance state before the server confirms the transition. All labels, statuses, errors, dates, and accessibility text require English, Traditional Chinese, and Japanese translations.

On phone layouts:

- the status and action appear before history;
- the action button spans the available width;
- daily history uses labeled cards rather than a wide table; and
- timestamps and durations remain readable without horizontal page scrolling.

## 10. Security and Audit

- The authenticated user ID comes from verified access-token claims, never from a self-service request body.
- The server supplies every attendance timestamp.
- Sign-in and sign-out are transactional and idempotent with respect to the current status and open session.
- Successful transitions, rejected transitions, permission denials, corrections, report generation, and cleanup are logged.
- A correction records the administrator, previous values, new values, reason, and time.
- Attendance APIs never expose password, token, or unrelated employee data.
- Database backups must include `attendance.db` and preserve monthly reports.
- Backups must also include the configured `reports/attendance` directory and verify CSV checksums.

## 11. Operational Jobs

The server owns three recurring jobs:

| Job | Schedule | Responsibility |
| --- | --- | --- |
| Midnight rollover | At midnight in each active attendance timezone | Finalize the old day and continue open sessions on the new day |
| Monthly report | After the month closes in each applicable timezone | Generate or refresh monthly aggregates |
| Retention cleanup | After report success | Remove eligible details older than six months |

Jobs must use a database lock or lease so multiple server processes cannot process the same period concurrently.

On startup, the system detects missed rollovers and reports and safely catches up. Attendance request handlers also reconcile an affected user's missed midnight boundaries before returning or changing state.

## 12. Explicitly Deferred Decisions

The following require separate product decisions before implementation:

- employee-specific schedules and weekends;
- public holidays;
- late arrival and early departure rules;
- formal paid/unpaid break classification;
- geolocation, IP, or device restrictions;
- leave and business trips;
- overtime approval;
- payroll export;
- the final retention period for monthly reports; and
- administrator correction workflow and approval requirements.

These features must extend the event history rather than overwrite original attendance evidence.
