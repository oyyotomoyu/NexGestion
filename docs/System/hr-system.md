# HR System

## 1. Purpose

This document collects the design considerations for the parts of core HR work that are not already covered by an existing system document. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md). No data model, API, or permission keys are finalized here; those must be defined in a follow-up design pass once scope is confirmed.

HR responsibility is split across several documents rather than one monolithic module, because most of it is already owned by existing systems. This document only designs the pieces that have no owner yet: **Employment Lifecycle** (Section 3), **Performance Management** (Section 4), and **Employee Relations case management** (Section 5). Everything else is either delegated to an existing document, deferred, or explicitly out of scope (Section 2).

Regulatory/legal compliance is intentionally left out of every section below. Where a jurisdiction-specific rule would normally apply (e.g. mandatory grievance procedures, statutory disciplinary process, works-council involvement), treat it the same way [`salary-system.md`](./salary-system.md#4-jurisdiction-and-localization-architecture) treats payroll law: a future pluggable jurisdiction concern, not something to hardcode now.

## 2. Scope Map

The ten areas of HR responsibility, and which document owns each:

| # | Responsibility | Owner | Status |
| --- | --- | --- | --- |
| 1 | Recruitment & hiring | Out of scope. A future Meeting System will handle interview scheduling; job postings and candidate tracking are not part of this platform. This document begins at the point a hiring decision creates a user (Section 3.2). | Deferred |
| 2 | Onboarding & offboarding | This document — Section 3 | New |
| 3 | Compensation & benefits | [`salary-system.md`](./salary-system.md) | Existing, reference only |
| 4 | Attendance & leave | [`attendance-system.md`](./attendance-system.md) | Existing, reference only |
| 5 | Performance management | This document — Section 4 | New |
| 6 | Training & development | Out of scope for now | Deferred |
| 7 | Organization & workforce planning | [`group-system.md`](./group-system.md) (structure/hierarchy) and [`role-system.md`](./role-system.md) (what a user may do); `employee_profiles.job_title` in [`user-system.md`](./user-system.md#32-employee_profiles) | Existing, reference only |
| 8 | Employee relations | This document — Section 5 (grievance, disciplinary, mediation). Satisfaction surveys and employee activities are deferred to a future Form System. | New (partial) |
| 9 | Regulatory compliance | Out of scope; see Section 1 | Deferred |
| 10 | Administrative & records management | [`general-affairs-system.md`](./general-affairs-system.md) — asset custody/issuance in particular (Section 3.2 of that document); this document's onboarding/offboarding checklist tasks (Section 3.4) link to it | Existing, reference only |

## 3. Employment Lifecycle

### 3.1 Relationship to UserSystem

Onboarding does not introduce a separate "candidate" or "new hire" record. It begins with the same `users` + `employee_profiles` creation already defined in [`user-system.md`](./user-system.md#3-recommended-core-tables), performed by whoever holds `users.manage`. This document adds the HR-specific state machine and history layer on top of that record; it does not duplicate or replace it.

### 3.2 Employment Status State Machine

`employee_profiles.employment_status` (user-system.md §3.2) already allows "`active`, `on_leave`, `terminated`, or another defined state." This document formally defines the additional states needed for onboarding and offboarding:

```txt
probation ── review passed ──> active ── on_leave/return ──> active
    │                              │
    │                              ├── suspended ──> active or terminated
    │                              │
    └── review failed ─────────────┴──> terminated
```

| Status | Meaning |
| --- | --- |
| `probation` | Default status set on `hire_date`; the employee is under initial evaluation |
| `active` | Regular employment, probation passed or not applicable |
| `on_leave` | Existing status from user-system.md; unaffected by this document |
| `suspended` | Temporary loss of active-duty status arising from an Employee Relations case (Section 5); does not itself terminate employment |
| `terminated` | Existing status from user-system.md; set on offboarding (Section 3.4) |

Whether an organization requires a probation period, and its default length, should be organization-level configuration rather than a hardcoded duration — some jurisdictions/organizations skip probation entirely. When probation is skipped, an employee record can be created directly in `active` status.

At most one status is active per employee at a time; status changes are timestamped and become part of the employment history described in Section 3.3, not a silent field overwrite.

### 3.3 Employment Events (History Layer)

Group membership changes (`user_groups.left_at`) and role reassignment already have their own audit trail in [`group-system.md`](./group-system.md) and [`role-system.md`](./role-system.md). What is missing is an HR-facing narrative: *why* a change happened, tied to one employee's career history. This document proposes one append-only table for that purpose, owned by the HR module's own database and referencing the immutable `user_id` — never duplicating User/Group/Role data.

Recommended shape (design note, not a finalized schema):

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable event ID |
| `user_id` | TEXT/UUID | Yes | Reference to `users.id` in UserSystem |
| `event_type` | TEXT | Yes | `onboarded`, `probation_passed`, `probation_extended`, `transferred`, `promoted`, `suspended`, `reinstated`, `terminated`, `rehired` |
| `effective_date` | DATE | Yes | Date the change takes effect |
| `previous_employment_status` | TEXT | No | Status before this event, when applicable |
| `new_employment_status` | TEXT | No | Status after this event, when applicable |
| `related_group_id` | TEXT/UUID | No | Group involved in a transfer, for reference only — ownership stays with `group-system.md` |
| `related_case_id` | TEXT/UUID | No | Employee Relations case that triggered this event (Section 5), when applicable |
| `reason` | TEXT | No | Free-text HR note |
| `actor_user_id` | TEXT/UUID | Yes | User who recorded the event |
| `created_at` | DATETIME | Yes | Creation time in UTC |

This table is a record of *what happened and why* for HR purposes; it is never the source of truth for current group membership, role assignment, or login access — those remain owned by UserSystem/Group/Role.

### 3.4 Onboarding

1. A hiring decision (outside this system's scope, Section 2 item 1) results in someone with `users.manage` creating the user and `employee_profile` per user-system.md §3.
2. `employment_status` is set to `probation` (or `active` if the organization has no probation policy) with `hire_date` set.
3. An `onboarded` employment event is recorded.
4. An onboarding checklist tracks the surrounding admin tasks (equipment issuance, document collection, account provisioning). This document keeps the task list generic and unopinionated rather than a fixed set of onboarding steps — a task that involves issuing physical equipment links to a General Affairs Asset Event (`general-affairs-system.md` §2.1.1) via `related_hr_task_id`, when that module is installed:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable task ID |
| `user_id` | TEXT/UUID | Yes | Employee the task belongs to |
| `title` | TEXT | Yes | Free-text task description |
| `status` | TEXT | Yes | `pending`, `in_progress`, `done`, `skipped` |
| `assigned_to_user_id` | TEXT/UUID | No | Person responsible for completing the task |
| `due_date` | DATE | No | Target completion date |
| `completed_at` | DATETIME | No | Completion timestamp |

The same table is reused for offboarding tasks (Section 3.6) — an onboarding/offboarding checklist is the same kind of record either way, only the task titles differ.

### 3.5 Probation Review

At the end of the probation period, a reviewer (typically the employee's manager, resolved the same way as leave approval in group-system.md §5) records the outcome:

- **Passed** — `employment_status` moves to `active`; a `probation_passed` event is recorded. This is a natural point to trigger the first Performance Management cycle (Section 4) if the organization runs one, but the two are not required to coincide.
- **Extended** — probation continues with a new end date; a `probation_extended` event is recorded.
- **Failed** — `employment_status` moves to `terminated`, following the offboarding flow in Section 3.6.

### 3.6 Internal Transfers, Promotions

Per the existing design principle in user-system.md §5 ("role controls what a user may do; group describes where the user belongs"), a transfer or promotion is executed entirely through the existing Group and Role APIs — updating `user_groups` membership (group-system.md §7) and `user_roles`/`employee_profiles.job_title` — never through a parallel HR-owned membership table. This document's only addition is recording a `transferred` or `promoted` employment event (Section 3.3) alongside that change, so HR retains a career-history narrative that the Group/Role APIs themselves don't capture (they record *what* changed, this records *why*).

### 3.7 Offboarding

1. Termination is initiated by someone holding the HR employment-management permission (Section 6).
2. `employment_status` moves to `terminated`, `termination_date` is set on `employee_profiles`, and a `terminated` employment event is recorded, including its reason (resignation, dismissal, probation failure, contract end, etc.) and, if applicable, a `related_case_id` linking to the Employee Relations case that led to it (Section 5).
3. Login access is disabled per the existing UserSystem disable/soft-delete pattern (user-system.md §5) — the user account itself is never deleted, preserving historical ownership of past records.
4. Group memberships are ended via the existing `user_groups.left_at` mechanism (group-system.md §3.2); this document does not introduce a separate removal path.
5. An offboarding checklist (same task model as Section 3.4) tracks equipment return and other admin steps.
6. Final settlement is out of this document's scope — it is already covered by [`salary-system.md` §6.5](./salary-system.md#65-employment-changes).

## 4. Performance Management

### 4.1 Performance Cycle

A performance cycle is a named, dated review period (e.g. annual, semi-annual, quarterly). Cadence must be organization-configurable, not hardcoded, consistent with this platform's general-purpose design principle.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable cycle ID |
| `name` | TEXT | Yes | e.g. "2026 H1 Review" |
| `start_date` | DATE | Yes | Cycle start |
| `end_date` | DATE | Yes | Cycle end |
| `status` | TEXT | Yes | `draft`, `open`, `closed` |

### 4.2 Goals

Goals may be set by the employee, the manager, or both, and are scoped to one cycle.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable goal ID |
| `user_id` | TEXT/UUID | Yes | Goal owner |
| `cycle_id` | TEXT/UUID | Yes | Owning performance cycle |
| `title` | TEXT | Yes | Goal statement |
| `description` | TEXT | No | Supporting detail |
| `weight` | DECIMAL | No | Relative weight toward the overall review, if the organization uses weighted goals |
| `status` | TEXT | Yes | `draft`, `active`, `completed`, `cancelled` |
| `created_by_user_id` | TEXT/UUID | Yes | Who created the goal (self or manager) |

### 4.3 Review Workflow

```txt
draft ── submit ──> self_review ── manager review ──> manager_review ── finalize ──> finalized
```

- `draft` — goals and cycle are open, no review submitted yet.
- `self_review` — employee has submitted their self-assessment.
- `manager_review` — manager has submitted their assessment; the platform does not assume a fixed number of review levels beyond self + manager, since a skip-level or calibration step is an organizational choice.
- `finalized` — review is closed and visible to the employee; further changes require a new correction record rather than overwriting the finalized result, consistent with the correction pattern already used in attendance-system.md §4.3 and salary-system.md §6.6.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable review ID |
| `user_id` | TEXT/UUID | Yes | Employee being reviewed |
| `cycle_id` | TEXT/UUID | Yes | Owning performance cycle |
| `reviewer_user_id` | TEXT/UUID | No | Manager conducting the review; null while in `self_review` |
| `status` | TEXT | Yes | `draft`, `self_review`, `manager_review`, `finalized` |
| `rating` | TEXT/DECIMAL | No | Overall outcome, on an organization-configurable scale — this platform must not hardcode a specific rating scale (e.g. 1–5 numeric vs. descriptive tiers) |
| `self_comments` | TEXT | No | Employee's self-assessment narrative |
| `manager_comments` | TEXT | No | Manager's assessment narrative |
| `finalized_at` | DATETIME | No | When the review became visible to the employee |

### 4.4 Relationship to Compensation

A finalized review's rating may inform a raise or bonus decision, but this document does not define an automatic calculation from rating to pay. Consistent with salary-system.md §2.3, any resulting rate change is entered as a new effective-dated compensation-basis record by whoever holds `salary.settlement.configure` — Performance Management only supplies the human-readable justification, never a direct write into Salary data.

### 4.5 Visibility

- An employee can always see their own goals and, once finalized, their own review.
- A manager can see and edit reviews for people who report to them, resolved the same way as leave/manager assignment in group-system.md §5.
- Broader HR visibility (e.g. an HR business partner viewing reviews outside their own reporting line) is a separate, higher-privilege permission (Section 6).

## 5. Employee Relations Case Management

This section digitizes grievance handling, disciplinary records, and dispute mediation. Satisfaction surveys and employee engagement activities are explicitly excluded — those are deferred to a future Form System, and this document does not attempt to design that system.

### 5.1 Case Types

| Type | Description |
| --- | --- |
| `grievance` | An employee-raised complaint (about a colleague, manager, policy, or working condition) |
| `disciplinary` | An HR- or manager-initiated record of a policy violation and its consequence |
| `mediation` | A dispute between two or more employees requiring HR-facilitated resolution |

### 5.2 Case Model

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable case ID |
| `case_type` | TEXT | Yes | `grievance`, `disciplinary`, or `mediation` |
| `subject_user_id` | TEXT/UUID | Yes | Employee the case is primarily about |
| `reported_by_user_id` | TEXT/UUID | No | Reporter, when not anonymous — anonymous reporting should be a supported option for `grievance` cases |
| `assigned_to_user_id` | TEXT/UUID | Yes | HR handler responsible for the case |
| `status` | TEXT | Yes | `open`, `investigating`, `resolved`, `closed`, `escalated` |
| `confidentiality_level` | TEXT | Yes | Controls visibility beyond the assigned handler; at minimum distinguishes standard HR access from a restricted/escalated tier |
| `summary` | TEXT | No | Free-text description |
| `resolution_notes` | TEXT | No | Outcome, recorded on `resolved`/`closed` |
| `related_employment_event_id` | TEXT/UUID | No | Link to Section 3.3 when the case results in a suspension or termination |
| `created_at` | DATETIME | Yes | Creation time in UTC |
| `updated_at` | DATETIME | Yes | Last update time in UTC |

### 5.3 Workflow

```txt
open ── assign/investigate ──> investigating ── resolve ──> resolved ── close ──> closed
                                     │
                                     └── escalate ──> escalated (reassigned to a higher HR role)
```

A `disciplinary` case that results in a suspension or termination creates the corresponding employment event (Section 3.3) and links back to it via `related_employment_event_id`, keeping the disciplinary rationale and the employment-status change auditable together without merging the two tables.

### 5.4 Confidentiality

Employee Relations case data requires the strictest access control of any HR data in this document — stricter than the "stricter than most modules" standard already set for salary data in salary-system.md §8, because a case may involve allegations against the subject's own manager. Visibility must never be inferred from the Group/Role hierarchy (e.g. a subject's manager must not automatically see a case about them); it is scoped only to the assigned handler and holders of the broader case-visibility permission (Section 6).

## 6. Permissions

Permission keys in `config/permission.json` follow the same catalog convention as other modules (user-system.md §3.4):

| Permission | Allows |
| --- | --- |
| `hr.access` | Access the HR System |
| `hr.employment.read.self` | View the current user's own employment history and status |
| `hr.employment.read` | View employment history and status for other employees |
| `hr.employment.manage` | Record onboarding, probation outcomes, transfers, suspensions, and termination (Section 3) |
| `hr.tasks.manage` | Create and update onboarding/offboarding checklist tasks (Section 3.4) |
| `hr.performance.read.self` | View the current user's own goals and finalized reviews |
| `hr.performance.read` | View goals and reviews for other employees, scoped to the reporting line (Section 4.5) |
| `hr.performance.cycles.manage` | Create and configure performance cycles (Section 4.1) |
| `hr.performance.review` | Submit a manager review for a direct or indirect report |
| `hr.employee_relations.read.self` | View the status of a case the current user reported or is the subject of, excluding restricted details |
| `hr.employee_relations.manage` | Create, investigate, and resolve Employee Relations cases (Section 5) |
| `hr.employee_relations.read` | View cases beyond one's own assigned cases; a narrower, higher-trust grant than most `.read` permissions elsewhere in the platform, given Section 5.4 |

The initial Admin role automatically receives every HR permission through `grants_all_permissions`, as with other modules. Every HR API route must declare its permission and follow the existing role-union authorization rule.

## 7. Integration Points

- **UserSystem**: onboarding creates the `users` + `employee_profile` record; offboarding disables login and sets `employment_status = terminated` (Sections 3.4, 3.7).
- **Group System / Role System**: transfers and promotions are executed entirely through existing membership and role-assignment APIs; this document only adds the narrative history layer on top (Section 3.6).
- **Salary System**: probation completion or a performance outcome may lead to a new compensation-basis record (salary-system.md §2.3); termination triggers final settlement (salary-system.md §6.5); this document never writes salary data directly.
- **Attendance System**: unaffected; `on_leave` status ownership and leave approval remain as defined in attendance-system.md and group-system.md.
- **Notification System**: onboarding/offboarding task assignments, review-cycle reminders, and case-status updates should reuse the existing notification-system.md patterns. Employee Relations case content must follow a pointer-only notification style (salary-system.md §10.2's recommended default), never embedding case detail in the notification or email body.
- **General Affairs System**: equipment issuance/return steps in the onboarding/offboarding checklist (Section 3.4, 3.7) link to that module's Asset Events (`general-affairs-system.md` §2.1.1, §3.2) via `related_hr_task_id`; this document does not track physical asset custody itself.
- **Future Meeting System**: will own interview scheduling for recruitment (Section 2, item 1); it would consume `general-affairs-system.md` §2.3's resource-booking primitive for room reservation rather than duplicating it, once it exists — out of scope here beyond noting the boundary.
- **Future Form System**: will own satisfaction surveys and employee engagement activities (Section 2, item 8) — out of scope here beyond noting the boundary. `general-affairs-system.md` §2.5's Polls capability is a narrower, lighter-weight statistics tool built for General Affairs' own ad-hoc headcounts and is deliberately not positioned as this Form System; whether the two ever converge is unresolved.

## 8. Security and Audit

- Employment events, performance reviews, and Employee Relations cases are all sensitive HR data requiring full change history (who changed what, when), consistent with the audit principle in salary-system.md §8.
- Employee Relations case data (Section 5.4) requires the strictest visibility scoping in this document and must never be inferred from Group/Role hierarchy.
- A finalized performance review is corrected via a new recorded correction, never a silent overwrite (Section 4.3).

## 9. Explicitly Deferred Decisions

The following require separate product decisions before implementation:

- the finalized schema and database ownership boundary for `employment_events`, onboarding/offboarding tasks, performance cycles/goals/reviews, and Employee Relations cases (all data models above are design notes, not final schemas);
- default probation duration and whether it is organization-wide or role/group-specific;
- the organization-configurable performance rating scale referenced in Section 4.3;
- whether performance review levels beyond self + manager (e.g. calibration, skip-level) are supported, and how;
- the confidentiality-tier model for Employee Relations cases (Section 5.2) beyond a minimal standard/restricted distinction;
- anonymous grievance reporting mechanics, if supported;
- retention period for employment events, performance records, and closed Employee Relations cases;
- how the future Meeting System (recruitment interviews) and Form System (surveys/engagement) will integrate with employee identity once they exist; and
- jurisdiction-specific legal requirements for disciplinary process, grievance handling, or works-council involvement, deliberately excluded per Section 1.
