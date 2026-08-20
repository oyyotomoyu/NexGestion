# Approval System

## 1. Purpose

This document collects the design considerations for a future Approval (簽核) module: a generic, reusable engine for routing a request through one or more human sign-offs before it takes effect. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

The platform already has several approval flows, each hand-built inside the module that needed it: leave approval's manager-resolution algorithm (`group-system.md` §5), probation review (`hr-system.md` §3.5), and the draft → approved → disbursed pattern used by Accounts Payable and Expense reimbursement (`finance-system.md` §4, §6). Every one of those is a **module-specific state machine** — useful, but not reusable, and each new approval need up to now has meant designing another one from scratch.

This document does not replace any of those. It exists so that **future** approval needs — starting with General Affairs (`general-affairs-system.md` §3.1), the first real consumer — have a shared engine to build on instead of hand-rolling another bespoke workflow. Whether any existing flow (leave, probation, AP, expense) ever migrates onto this engine is an open decision (Section 7), not something this document assumes.

## 2. Data Model

### 2.1 Approval Flow Templates

An organization-defined, reusable sequence of steps for one kind of request. A request type (e.g. "office supply requisition," "seal usage") is configured once as a template and then instantiated for every actual request of that type.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable template ID |
| `name` | TEXT | Yes | Organization-defined name, e.g. "辦公用品請購簽核", "用印申請簽核" |
| `request_type` | TEXT | Yes | Identifies which kind of request this template applies to, e.g. `general_affairs.supply_requisition`, `general_affairs.seal_usage` — a dotted `module.request` convention, mirroring the `source_module` convention already used for journal entries (`finance-system.md` §3) and stock movements (`inventory-system.md` §2.6) |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

### 2.2 Approval Step Templates

The ordered steps within one Flow Template. Steps run strictly in sequence for v1 — step 2 does not become eligible until step 1 reaches a decision.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable step template ID |
| `flow_template_id` | TEXT/UUID | Yes | Owning Flow Template |
| `step_order` | INTEGER | Yes | Position in the sequence, starting at `1` |
| `approver_type` | TEXT | Yes | `specific_user`, `role`, `requester_manager`, `group_manager` |
| `approver_user_id` | TEXT/UUID | No | Required when `approver_type = specific_user` |
| `approver_role_id` | TEXT/UUID | No | Required when `approver_type = role` — any holder of the role is eligible, same all-eligible-decide-once semantics as group-system.md §5's multi-manager case |
| `approver_group_id` | TEXT/UUID | No | Required when `approver_type = group_manager` — resolves to that specific group's manager(s), rather than walking the hierarchy |
| `min_amount` | DECIMAL | No | Step only applies once the request's amount (when the request type carries one) reaches this threshold — e.g. a purchase requisition under $5,000 skips the second-level approver entirely |

`approver_type = requester_manager` reuses `group-system.md` §5's Manager Resolution algorithm exactly — start at the requester's primary organization group, walk up until an eligible manager is found, fall back to an administrator if none exists. This is deliberate reuse, not a re-implementation: the algorithm, its "never routed downward, sideways, or through a project group" rule, and its administrator-fallback behavior all carry over unchanged.

### 2.3 Approval Requests

The instantiated request — one row per actual thing awaiting sign-off.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable request ID |
| `flow_template_id` | TEXT/UUID | Yes | The template this request was instantiated from |
| `source_module` | TEXT | Yes | The owning module, e.g. `general_affairs` |
| `source_reference_id` | TEXT/UUID | Yes | The actual record awaiting approval in its owning module (a supply requisition, a seal usage request) — this table never duplicates that record's content, only routes its approval |
| `requested_by_user_id` | TEXT/UUID | Yes | Who submitted the request |
| `status` | TEXT | Yes | `pending`, `approved`, `rejected`, `cancelled`, `requires_assignment` — the last reusing group-system.md §5's exact term for "no eligible approver could be resolved" |
| `current_step_order` | INTEGER | Yes | Which Approval Step (Section 2.4) is currently awaiting a decision |
| `created_at` | DATETIME | Yes | Creation time in UTC |
| `completed_at` | DATETIME | No | Set when `status` reaches `approved`, `rejected`, or `cancelled` |

A requester can never be resolved as their own approver at any step, the same rule group-system.md §5 already enforces for leave.

### 2.4 Approval Steps (Instances)

The step-by-step decision record for one Approval Request, snapshotted from the Flow Template at submission time.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable step instance ID |
| `approval_request_id` | TEXT/UUID | Yes | Owning request |
| `step_order` | INTEGER | Yes | Position, copied from the Step Template |
| `assigned_user_ids` | TEXT/UUID[] | Yes | The resolved eligible approver(s) for this step, snapshotted at the moment this step became current — a later change to group membership, role assignment, or hierarchy never silently alters who can decide an already-assigned step, the same snapshot principle group-system.md §5 already applies to leave |
| `decision` | TEXT | Yes | `pending`, `approved`, `rejected`, `skipped` — `skipped` covers a step whose `min_amount` threshold (Section 2.2) wasn't met |
| `decided_by_user_id` | TEXT/UUID | No | Which of the assigned approvers actually decided; set once `decision` leaves `pending` |
| `decided_at` | DATETIME | No | Decision timestamp |
| `comment` | TEXT | No | Optional note from the approver |

When several users are assigned to one step (a `role` or multi-manager `requester_manager` resolution), the first decision completes the step and any later attempt is rejected as a state conflict — identical semantics to group-system.md §5's "first valid decision completes it."

### 2.5 Completion Notification Targets

Who else, beyond the requester, should be told once a request reaches a final decision — the people the outcome hands off to, not just the people who decided it. Approving a material requisition doesn't just need the requester to know; it needs the warehouse team to actually go pull the stock (`general-affairs-system.md` §3.7 is a concrete example of this).

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable target ID |
| `flow_template_id` | TEXT/UUID | Yes | Owning Flow Template |
| `target_type` | TEXT | Yes | `specific_user`, `role`, `group_manager` — the same resolution vocabulary as Section 2.2's `approver_type`, reused rather than inventing a second one |
| `target_user_id` / `target_role_id` / `target_group_id` | TEXT/UUID | No | Whichever field matches `target_type` |
| `notify_on` | TEXT | Yes | `approved`, `rejected`, `both` — most coordinating-personnel targets only care about `approved` (the warehouse doesn't need to hear about a rejected requisition), but a template may notify on rejection too |

Completion Notification Targets never gate the request's own approval — they fire only after `status` (Section 2.3) already reached a terminal outcome, and this module only ever notifies; resolved recipients still take whatever action they need to take (e.g. recording an Inventory Outbound) through their own module's normal path, never through a write this module makes on their behalf, keeping with this platform's Module Independence Principle (`operations-system.md` §2).

## 3. Request Lifecycle

```txt
(submitted) ── flow_template resolved, step 1 assigned ──> pending
   pending ── step approved, more steps remain ──> pending (next step)
   pending ── final step approved ──> approved
   pending ── any step rejected ──> rejected
   pending ── requester cancels before any decision ──> cancelled
   pending ── no eligible approver resolved at any step ──> requires_assignment
```

A rejection at any step ends the whole request — there is no "send back for revision" loop in this design; a rejected request must be resubmitted as a new one if the requester wants to try again (Section 7). `requires_assignment` mirrors group-system.md §5's fallback: an administrator can manually assign an approver to unblock the request, and that reassignment must be audited the same way.

Reaching `approved` or `rejected` notifies two audiences, not one: the requester always learns the outcome, and every Completion Notification Target (Section 2.5) matching that outcome's `notify_on` is notified alongside them — a material requisition reaching `approved`, for instance, tells both the requester and the warehouse role configured to fulfill it.

## 4. Relationship to Other Systems

### 4.1 General Affairs System

`general-affairs-system.md` §3.1 is the first consumer: supply requisitions, resource bookings above a configurable threshold, and seal usage requests each carry an optional `approval_request_id`. When Approval is not installed, General Affairs falls back to a single-approver default — the requester's manager, resolved directly via group-system.md §5 — rather than requiring this module.

### 4.2 Group System

`approver_type = requester_manager` and `approver_type = group_manager` (Section 2.2) both resolve through group-system.md's existing hierarchy and generated Manager roles (`group-system.md` §3.3, §5) — this document adds no parallel approver-resolution logic of its own for those two cases, only routes to it.

### 4.3 Notification System

Each time an Approval Step becomes current, the assigned approver(s) should be notified via the existing notification-system.md capability, and the requester notified on the request's final decision. Following the pointer-only notification convention already recommended for sensitive content (`salary-system.md` §10.2, reused by `hr-system.md` §7 for Employee Relations cases), the notification should link to the request rather than embedding its detail in the message body, since a request's `source_reference_id` may point at content (e.g. a seal usage request naming a contract) the notification channel shouldn't carry directly.

### 4.4 Existing Ad-Hoc Approval Flows

Leave approval (`group-system.md` §5), probation review (`hr-system.md` §3.5), and Finance's AP/Expense approval (`finance-system.md` §4, §6) are unaffected by this document and continue to own their own state machines. No migration is assumed or required (Section 7).

## 5. Permissions

Planned permission keys, to be added to `config/permission.json` when these APIs are implemented, following the same catalog convention as other modules (`user-system.md` §3.4).

| Permission | Allows |
| --- | --- |
| `approvals.templates.manage` | Create and configure Flow Templates, their steps, and their Completion Notification Targets (Section 2.1, 2.2, 2.5) |
| `approvals.read.self` | View the status of a request the current user submitted or is an assigned approver on |
| `approvals.decide` | Approve or reject a step the current user is assigned to — record-scoped, resolved per-request the same way group-system.md §5 scopes leave-decision eligibility, not a blanket grant |
| `approvals.read` | View approval requests beyond one's own submissions/assignments (an administrative/audit view) |
| `approvals.reassign` | Manually assign an approver to a `requires_assignment` request (Section 3) |

The initial Admin role automatically receives every approval permission through `grants_all_permissions`, as with other modules.

## 6. Security and Audit

- Every decision (`approved`, `rejected`, `skipped`) and every reassignment must be logged with actor, timestamp, and reason where applicable, consistent with the audit principle already established in `salary-system.md` §8 and `hr-system.md` §8.
- A decided Approval Step is never edited after the fact — a mistaken decision is corrected by cancelling the request and resubmitting, not by silently rewriting `decision`, the same non-destructive-correction principle used throughout this platform.

## 7. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- whether any existing ad-hoc approval flow (leave, probation, AP, expense) ever migrates onto this engine, and if so, in what order — this document does not assume any migration;
- parallel/branching routing (e.g. two approvers must both sign off on one step, or routing splits on a condition beyond the single `min_amount` threshold in Section 2.2) — v1 is strictly sequential, single-approver-per-step;
- a "return for revision" loop — today a rejection ends the request outright, with no partial-edit-and-resubmit path;
- delegation (a user designating someone else to decide on their behalf while away, e.g. during `on_leave` per `attendance-system.md`) — not modeled;
- SLA/escalation timers (auto-escalating a step that sits pending too long);
- legally-binding e-signature (cryptographic signing, timestamping, non-repudiation beyond an authenticated actor + timestamp record) — Section 2.4's `decided_by_user_id`/`decided_at` is an internal audit record, not a signature product;
- whether a Completion Notification Target's resolved recipients (Section 2.5) are snapshotted at fire time the same way Section 2.4's `assigned_user_ids` are, or re-resolved live — not decided;
- retention period for completed Approval Requests and Steps;
- permission keys beyond Section 5's first pass, to be finalized once API design starts.
