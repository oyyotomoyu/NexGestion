# Scheduling System

## 1. Purpose

This document collects the design considerations for a future Scheduling module. It is a planning checklist, not a finalized implementation spec. No data model, API, or permission keys are finalized here; those must be confirmed in a follow-up design pass once scope is approved.

The Scheduling System lets an organization plan who should work, when they should work, and which group the work belongs to. Scheduling is centered on **groups**: a schedule is created for a department, branch, store, project team, or other group, then assigned to users who belong to or are eligible for that group.

The module may integrate with the Group System, UserSystem, Attendance System, and Salary System, but these integrations are **admin-configurable** rather than mandatory. A deployment can use Scheduling as a standalone planning calendar, or connect it to membership validation, user assignment, attendance comparison, or payroll calculation depending on operational needs.

## 2. Core Principles

- A schedule belongs to one group.
- A shift is a planned work interval inside a schedule.
- A shift may be assigned to a user, left open for later assignment, or marked as requiring a role/skill rather than a named person.
- Group membership may be enforced, warned, or ignored depending on admin configuration.
- Scheduling does not grant access permissions. Roles still define what a user may do; groups define where the scheduled work belongs.
- Historical schedules must remain stable after publication. Later group, user, salary, or attendance changes must not silently rewrite published records.
- Time calculations use IANA timezones and UTC storage, matching the Attendance System's approach.

## 3. Group-Centered Scheduling

Each schedule is scoped to exactly one group from the Group System when that integration is enabled. The group may represent:

- an organization group, such as a department, branch, store, or team;
- a project group, such as a temporary event team or cross-functional project; or
- a standalone scheduling group created inside this module if Group System integration is disabled.

Group scoping answers these questions:

- which manager or scheduler owns the schedule;
- which users are expected to appear as candidates;
- which calendar, report, or payroll bucket the shift belongs to;
- how conflicts are detected across related groups; and
- which published schedules a user sees in their own schedule view.

### 3.1 Group Integration Modes

Admin configuration determines how Scheduling uses groups:

| Mode | Behavior |
| --- | --- |
| `required` | Every schedule must reference an active Group System group. Assignments are limited to eligible group members unless an authorized override is recorded |
| `advisory` | Schedules reference Group System groups, but assigning a non-member creates a warning instead of blocking publication |
| `standalone` | Scheduling owns its own scheduling groups and does not require Group System data |

The default should be `advisory` for early deployments because it preserves useful structure without blocking organizations whose group data is incomplete.

### 3.2 Organization vs Project Groups

Both organization and project groups may be scheduled:

- organization groups are suitable for recurring operational schedules, such as store coverage or department shifts;
- project groups are suitable for temporary schedules, such as an event, installation, audit, or project sprint; and
- approval/manager routing should only use organization hierarchy when explicitly configured, consistent with Group System semantics.

Project groups must not be treated as reporting-line hierarchy. A project schedule can have owners and schedulers, but it should not inherit leave-approval behavior from the Group System.

## 4. Scheduling Model

### 4.1 Schedule Period

A schedule period is the planning container for one group and date range.

Recommended shape:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable schedule period ID |
| `group_id` | TEXT/UUID | Conditional | Group System group ID when group integration is enabled |
| `scheduling_group_id` | TEXT/UUID | Conditional | Scheduling-owned group ID when standalone mode is used |
| `name` | TEXT | Yes | Human-readable schedule name, such as `August Store Roster` |
| `timezone` | TEXT | Yes | IANA timezone used for local shift boundaries |
| `start_date` | DATE | Yes | First local date in the schedule period |
| `end_date` | DATE | Yes | Last local date in the schedule period |
| `status` | TEXT | Yes | `draft`, `published`, `locked`, `archived`, or `cancelled` |
| `created_by_user_id` | TEXT/UUID | Yes | User who created the period |
| `published_at` | DATETIME | No | UTC time when the schedule became visible to assigned users |
| `created_at` | DATETIME | Yes | UTC creation time |
| `updated_at` | DATETIME | Yes | UTC last update time |

### 4.2 Shifts

A shift is a planned work interval within a schedule period.

Recommended shape:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable shift ID |
| `schedule_period_id` | TEXT/UUID | Yes | Owning schedule period |
| `title` | TEXT | No | Optional label, such as `Opening`, `Closing`, or `Inventory Count` |
| `local_start_at` | DATETIME | Yes | Local start time in the schedule timezone |
| `local_end_at` | DATETIME | Yes | Local end time in the schedule timezone |
| `start_at_utc` | DATETIME | Yes | UTC start instant |
| `end_at_utc` | DATETIME | Yes | UTC end instant |
| `planned_minutes` | INTEGER | Yes | Planned duration calculated from UTC instants |
| `required_headcount` | INTEGER | Yes | Number of people needed; defaults to `1` |
| `assignment_mode` | TEXT | Yes | `named_users`, `open_shift`, or `requirement_only` |
| `notes` | TEXT | No | Internal scheduling notes |
| `status` | TEXT | Yes | `draft`, `published`, `cancelled`, or `completed` |
| `created_at` | DATETIME | Yes | UTC creation time |
| `updated_at` | DATETIME | Yes | UTC last update time |

Overnight shifts are valid. Duration must be calculated from UTC instants so daylight-saving transitions produce the actual elapsed time.

### 4.3 Shift Assignments

Shift assignments connect a shift to a user or leave capacity open.

Recommended shape:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable assignment ID |
| `shift_id` | TEXT/UUID | Yes | Assigned shift |
| `user_id` | TEXT/UUID | No | Assigned UserSystem user; null for open or requirement-only slots |
| `assignment_status` | TEXT | Yes | `planned`, `confirmed`, `declined`, `cancelled`, or `completed` |
| `source` | TEXT | Yes | `manual`, `template`, `import`, `swap`, or `auto_scheduler` |
| `override_reason` | TEXT | No | Required when admin override bypasses configured validation |
| `created_at` | DATETIME | Yes | UTC creation time |
| `updated_at` | DATETIME | Yes | UTC last update time |

For a shift with `required_headcount > 1`, the system may create several assignment rows for the same shift. Named assignments count toward headcount; open slots remain visible until filled or cancelled.

## 5. Optional System Integrations

### 5.1 Group System

When enabled, Scheduling reads group identity, status, type, and membership from [`group-system.md`](./group-system.md).

Configurable behaviors:

- require schedules to reference active groups;
- limit assignment candidates to active group members;
- allow managers of the scheduled group to publish or approve schedules;
- warn when a user is assigned outside their group; and
- snapshot group name/type at publication for historical reports.

Scheduling must not directly modify Group System records. Membership changes belong to UserSystem/Group System, not this module.

### 5.2 UserSystem

When enabled, Scheduling reads active users and employee profile display fields from [`user-system.md`](./user-system.md).

Configurable behaviors:

- allow only active users to be assigned;
- include job title, timezone, locale, or employment status in candidate lists;
- notify assigned users when a schedule is published or changed; and
- hide disabled or terminated users from future assignments while preserving historical assignments.

A shift assignment references immutable `user_id`. It must not duplicate authentication or employee profile data.

### 5.3 Attendance System

Scheduling can be compared with actual attendance from [`attendance-system.md`](./attendance-system.md), but attendance remains the source of actual worked time.

Configurable behaviors:

- show scheduled vs actual start/end time;
- flag absence when a user had a scheduled shift but no attendance session;
- flag unscheduled work when a user worked without a shift;
- mark late arrival, early departure, or overtime relative to the planned shift; and
- require manager review before schedule variances flow to reports or payroll.

Scheduling should not replace Attendance. A published shift is a plan; attendance records what happened.

### 5.4 Salary System

Salary integration is optional and must be explicitly enabled by an administrator because organizations differ in whether schedules affect pay.

Configurable behaviors:

| Mode | Behavior |
| --- | --- |
| `none` | Schedules do not affect payroll. Salary reads Attendance or manual inputs only |
| `reference_only` | Salary can display scheduled hours alongside attendance but does not calculate from schedules |
| `planned_hours` | Salary may use published scheduled hours as an input for planning, budgeting, or fixed-shift compensation |
| `variance_review` | Salary consumes approved schedule-vs-attendance variance records after review |

If enabled, Salary must still apply its own compensation basis, jurisdiction, overtime, rounding, and settlement rules. Scheduling provides planned intervals and group context; it must not hardcode payroll calculations.

## 6. Templates and Recurrence

Scheduling should support templates to reduce repeated manual work.

Template examples:

- weekly store roster;
- weekday office coverage;
- rotating team schedule;
- project event schedule;
- seasonal or holiday staffing pattern.

Recommended template fields:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable template ID |
| `group_id` | TEXT/UUID | Conditional | Target Group System group when enabled |
| `name` | TEXT | Yes | Template name |
| `timezone` | TEXT | Yes | IANA timezone |
| `pattern_type` | TEXT | Yes | `weekly`, `fixed_cycle`, or `custom_dates` |
| `status` | TEXT | Yes | `active` or `inactive` |
| `created_at` | DATETIME | Yes | UTC creation time |
| `updated_at` | DATETIME | Yes | UTC last update time |

Applying a template creates ordinary draft shifts. Later template changes must not mutate already-created schedule periods unless an explicit regenerate action is requested.

## 7. Conflicts and Validation

The system should detect conflicts before publication:

- a shift end must be after its start;
- a shift must belong inside the schedule period, unless cross-boundary shifts are explicitly allowed;
- a user should not be assigned to overlapping shifts;
- a user should not exceed configured maximum planned hours without warning or override;
- required rest time between shifts should be configurable, not hardcoded;
- inactive users should not be assigned to future shifts;
- inactive groups should not receive new schedule periods when Group System integration is required; and
- publication should fail if required headcount is not met, unless the schedule policy allows open shifts.

Labor-law rules such as maximum daily hours, rest days, or minors' work restrictions vary by jurisdiction. They should be modeled as optional policy packs or admin-defined rules rather than fixed assumptions in core Scheduling.

## 8. Workflow

Recommended schedule lifecycle:

```txt
draft ── publish ──> published ── lock ──> locked ── archive ──> archived
  │                      │
  └──── cancel ──────────┴──── cancel ──> cancelled
```

- `draft`: schedulers can freely edit shifts and assignments.
- `published`: assigned users can view the schedule; changes require audit and optional notification.
- `locked`: the schedule is frozen for reporting, attendance comparison, or salary integration.
- `archived`: historical read-only state after the period is complete.
- `cancelled`: the schedule period is no longer active.

An administrator may configure whether publication requires approval. If approval is enabled, approver resolution may use group managers, a dedicated scheduling manager role, or a custom approval workflow. This must be configuration-driven rather than hardcoded to a specific group level.

## 9. Permissions

Planned permission keys, to be added to `config/permission.json` when Scheduling APIs are implemented:

| Permission | Allows |
| --- | --- |
| `scheduling.read.self` | View the authenticated user's own published shifts |
| `scheduling.read` | View schedules and shifts for groups the user is allowed to inspect |
| `scheduling.manage` | Create and edit schedule periods, shifts, templates, and assignments |
| `scheduling.publish` | Publish or lock schedules |
| `scheduling.approve` | Approve schedules when approval workflow is enabled |
| `scheduling.override` | Bypass configured validation with an audited reason |
| `scheduling.config.manage` | Configure integration modes, validation policy, and recurrence defaults |
| `scheduling.reports.read` | View schedule coverage, variance, and staffing reports |

The initial Admin role automatically receives every scheduling permission through `grants_all_permissions`, as with other modules. Every route must declare its required permission and follow the existing role-union authorization rule.

## 10. API Areas

Detailed endpoint contracts are deferred, but the module likely needs these API groups:

| Area | Examples |
| --- | --- |
| Config | Read/update integration modes and validation policy |
| Schedule periods | List, create, update, publish, lock, cancel, archive |
| Shifts | Create, update, cancel, bulk edit, copy |
| Assignments | Assign user, unassign user, fill open shift, decline/confirm |
| Templates | Create, update, apply, retire |
| Candidate lookup | List eligible users for a group and time window |
| Conflicts | Preview validation errors before saving or publishing |
| Reports | Coverage, planned hours, schedule-vs-attendance variance |

List endpoints should follow the shared List API Query Standard in [`architecture.md`](./architecture.md).

## 11. Reporting

Initial reports should focus on operational planning:

- coverage by group/date/time;
- planned hours by user;
- planned hours by group;
- open shifts and understaffed periods;
- schedule changes after publication;
- schedule-vs-attendance variance when Attendance integration is enabled; and
- planned labor cost estimate when Salary integration is enabled in reference or planned-hours mode.

Reports must keep group and user historical snapshots where needed so old schedules remain understandable even after a group is renamed or a user leaves.

## 12. Security and Audit

- Every publish, lock, cancel, assignment change, override, and integration-mode change must be audited.
- Schedule visibility should respect group scope when group integration is enabled.
- Users can view their own published shifts without broad schedule-management permission.
- Draft schedules may be restricted to schedulers and approvers until publication.
- Override reasons should be required when bypassing group membership, overlap, rest-time, or headcount rules.
- Published schedules should be versioned or change-tracked so users and managers can see what changed after publication.

## 13. Explicitly Deferred Decisions

The following require separate product/engineering decisions before implementation:

- whether Scheduling owns a dedicated `scheduling.db` or shares another module database;
- finalized schema for schedule periods, shifts, assignments, templates, and policy settings;
- default integration mode for Group System, UserSystem, Attendance System, and Salary System;
- whether group membership is enforced, advisory, or ignored by default;
- how schedule approval is routed when enabled;
- exact notification behavior for publish/change/cancel events;
- whether users can request shift swaps, claim open shifts, or mark availability;
- whether auto-scheduling is in scope, and what constraints it may optimize;
- policy-pack design for jurisdiction-specific scheduling/labor rules;
- how Salary consumes planned hours or variance records when integration is enabled; and
- retention rules for historical schedules and audit events.
