# General Affairs System

## 1. Purpose

This document collects the design considerations for a future General Affairs (總務) module: the internal-facing administrative work that keeps an office running — who has which piece of equipment, how someone requests office supplies, how a meeting room gets booked, and who is allowed to use the company seal on a contract. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

This document exists to close a specific gap. A prior review of the platform's coverage found that `hr-system.md` §2 explicitly deferred "Administrative & records management" (item 10) as out of scope, while `finance-system.md` §8 already assumed an "HR Asset Management" counterpart exists to pair with Finance's Fixed Asset Accounting — two documents referencing a system that was never actually designed. General Affairs is that system: Section 2.1 below is what `finance-system.md` §8 was assuming existed all along.

General Affairs is not part of the Operations System family (`operations-system.md`) — it has no goods-flow relationship to Order, Inventory, Procurement, or Production. It sits alongside HR as an internal administrative capability: HR owns who a person *is* within the organization, General Affairs owns the physical and administrative *things* that support them working there.

## 2. Data Model

### 2.1 Assets & Custody

Tracks which physical item a specific employee currently holds — a laptop, an ID badge, a company phone — distinct from and complementary to Finance's Fixed Asset Accounting (`finance-system.md` §8), exactly as that document already anticipates: General Affairs owns custodian, location, and condition; Finance owns acquisition cost, depreciation method, and accumulated depreciation. The two share a common `asset_id`/tag rather than merging into one record.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable asset ID — the shared tag Finance's Fixed Asset record references (`finance-system.md` §8) |
| `name` | TEXT | Yes | e.g. "MacBook Pro 14" #A1042", "員工識別證 #0042" |
| `category` | TEXT | No | Organization-defined, e.g. "electronics", "furniture", "access_card" |
| `status` | TEXT | Yes | `in_stock`, `issued`, `under_repair`, `lost`, `retired` |
| `current_custodian_user_id` | TEXT/UUID | No | Who currently holds it; unset when `status = in_stock` |
| `current_location` | TEXT | No | Free-text location when not issued to a person, e.g. "3F Storage" |
| `created_at` | DATETIME | Yes | Creation time in UTC |

#### 2.1.1 Asset Events

An append-only history of what happened to one asset, the same non-destructive-ledger pattern already used for Inventory's Stock Movements (`inventory-system.md` §2.6) and HR's Employment Events (`hr-system.md` §3.3) — current custody is a derived read from the latest event, not a separately trusted mutable field alone.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable event ID |
| `asset_id` | TEXT/UUID | Yes | Owning asset |
| `event_type` | TEXT | Yes | `issued`, `returned`, `transferred`, `repaired`, `lost`, `retired` |
| `user_id` | TEXT/UUID | No | The employee involved — receiving custody on `issued`/`transferred`, returning it on `returned` |
| `related_approval_request_id` | TEXT/UUID | No | Optional link to an Approval Request (`approval-system.md` §2.3), when issuance of this asset required sign-off (Section 3.1) |
| `related_hr_task_id` | TEXT/UUID | No | Optional link to an HR onboarding/offboarding checklist task (`hr-system.md` §3.4) — this is the record that task actually points at (Section 3.2) |
| `condition_note` | TEXT | No | Free-text condition at the time of this event |
| `actor_user_id` | TEXT/UUID | Yes | Who recorded the event |
| `occurred_at` | DATETIME | Yes | UTC timestamp |

### 2.2 Office Supply Requisitions

An internal request for supplies or small equipment, distinct from Procurement's vendor-facing Purchase Orders (`procurement-system.md` §2.2) — this is "an employee asking for something," not "the organization ordering from a vendor." The two connect only when the request is actually fulfilled (Section 3.4).

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable requisition ID |
| `requested_by_user_id` | TEXT/UUID | Yes | Who is asking |
| `description` | TEXT | Yes | What's needed, in plain text |
| `quantity` | DECIMAL | No | Quantity requested, when applicable |
| `estimated_amount` | DECIMAL | No | Estimated cost, used for `approval-system.md` §2.2's `min_amount` step-threshold routing when the flow varies by cost |
| `status` | TEXT | Yes | `pending_approval`, `approved`, `rejected`, `fulfilled`, `cancelled` |
| `approval_request_id` | TEXT/UUID | No | Optional link to an Approval Request (`approval-system.md` §2.3); when Approval is not installed, this stays unset and the requester's manager (resolved directly per `group-system.md` §5) decides instead |
| `procurement_purchase_order_id` | TEXT/UUID | No | Optional link to the Purchase Order (`procurement-system.md` §2.2) created to fulfill this requisition, when Procurement is installed and the item was newly bought through it |
| `inventory_outbound_id` | TEXT/UUID | No | Optional link to the Outbound record (`inventory-system.md` §2.4) created when this requisition is instead fulfilled by drawing from existing warehouse stock (領料) rather than buying new — see Section 3.7 |
| `created_at` | DATETIME | Yes | Creation time in UTC |

A requisition is fulfilled by at most one of these two paths, never both — a fresh vendor purchase or a draw against existing stock. `status = fulfilled` is set manually when neither link applies (the item was handed over directly with no formal PO or stock record), or derived from the linked PO or Outbound reaching a terminal state when one exists.

### 2.3 Resource Booking

Bookable shared resources — meeting rooms, a company vehicle, a projector — and the reservations against them. Booking is self-service: any organization member holding booking permission (Section 4) picks a resource and a time directly, with no approval step by default — General Affairs' role is providing the resources and, when two people want the same slot, coordinating the conflict (Section 2.3.4), not gatekeeping every booking.

#### 2.3.1 Bookable Resources

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable resource ID |
| `name` | TEXT | Yes | e.g. "3樓大會議室", "公務車 (ABC-1234)" |
| `category` | TEXT | No | Organization-defined, e.g. "meeting_room", "vehicle", "equipment" |
| `status` | TEXT | Yes | `active`, `inactive` |

#### 2.3.2 Bookings

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable booking ID |
| `resource_id` | TEXT/UUID | Yes | The resource being booked |
| `booked_by_user_id` | TEXT/UUID | Yes | Who made the booking — the organizer, and the one who holds direct edit/cancel rights over it (Section 4) |
| `purpose` | TEXT | No | Free-text reason |
| `starts_at` | DATETIME | Yes | Booking start |
| `ends_at` | DATETIME | Yes | Booking end, must be after `starts_at` |
| `status` | TEXT | Yes | `confirmed`, `pending_coordination`, `cancelled` |

Two `confirmed` bookings for the same `resource_id` must not have overlapping `[starts_at, ends_at)` ranges — the conflict check this table exists to enforce. A new request that overlaps an existing `confirmed` booking is rejected by default; the only way past that rejection is Space Coordination (Section 2.3.4), and `pending_coordination` is exactly for that case — a conflicting request stays in that status while the conflict is being negotiated, instead of being blocked outright. This document deliberately stops at the booking primitive; a fuller meeting concept beyond invitees (Section 2.3.3) — full agenda, minutes — is a distinct, larger idea some other documents refer to as a future "Meeting System" and remains out of scope here (Section 5) — it would consume this booking primitive for its room reservation, not duplicate it.

#### 2.3.3 Booking Invitees

The organizer may invite other organization members to a booking — most commonly a meeting's attendee list.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `booking_id` | TEXT/UUID | Yes | Owning booking |
| `invited_user_id` | TEXT/UUID | Yes | The invited organization member |
| `response_status` | TEXT | Yes | `invited`, `accepted`, `declined` |
| `responded_at` | DATETIME | No | When the invitee responded |

Invitations are delivered through the existing notification-system.md capability (Section 3.6). This table tracks who is invited and whether they've responded; it does not extend Section 2.3.2's conflict check to attendee availability — this document only checks whether the *resource* is free, never whether every invitee's calendar is (Section 5).

#### 2.3.4 Space Coordination

A booking request that conflicts with an existing `confirmed` booking on the same resource doesn't have to be a dead end. General Affairs can raise it as a coordination case instead — covering both an organization-priority event that needs a slot someone else already holds, and ordinary manual mediation when two people want the same room and General Affairs can help them work it out.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable coordination case ID |
| `requesting_booking_id` | TEXT/UUID | Yes | The new booking wanting the slot; stays `pending_coordination` (Section 2.3.2) while this case is open |
| `conflicting_booking_id` | TEXT/UUID | Yes | The existing `confirmed` booking occupying the same resource/time |
| `reason` | TEXT | No | Free-text context, e.g. "配合公司尾牙活動" |
| `initiated_by_user_id` | TEXT/UUID | Yes | Who raised the case — typically a General Affairs staff member mediating a conflict; whether the requester can also trigger one directly on submission is an open decision (Section 5) |
| `original_booker_decision` | TEXT | Yes | `pending`, `accept`, `decline` — the existing booker's response |
| `decided_at` | DATETIME | No | When the original booker responded |
| `status` | TEXT | Yes | `pending`, `accepted`, `declined`, `cancelled` |

The original booker is notified of the pending case — including by email, per Section 5's noted gap — and can accept or decline giving up their slot:

- **`accepted`**: the system performs the swap atomically — `conflicting_booking_id` moves to `cancelled`, `requesting_booking_id` moves to `confirmed`. From that point on, no further cooperation from the original booker is needed; the requester (`requesting_booking_id`'s own `booked_by_user_id`) edits or cancels the now-confirmed booking directly through the normal path (Section 2.3.2, Section 4) like any other booking they own — the swap is a one-time event, not an ongoing joint arrangement.
- **`declined`**: `requesting_booking_id` stays unconfirmed. This document doesn't model an automatic escalation past a decline — General Affairs and the requester resolve it manually, e.g. by finding a different slot (Section 5).

Space Coordination is a bilateral negotiation between two specific people over one resource conflict, not a multi-step sign-off chain, so it does not route through [`approval-system.md`](./approval-system.md) — it's modeled directly here rather than reusing that engine.

### 2.4 Seal & Stamp Management

Tracks the organization's physical seals/stamps (公司大小章) and who is authorized to use one on a given document — the highest-risk category of General Affairs data, since a seal legally binds the organization the moment it's applied to a contract.

#### 2.4.1 Seals

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable seal ID |
| `name` | TEXT | Yes | e.g. "公司大章", "財務專用章" |
| `custodian_user_id` | TEXT/UUID | Yes | Who physically holds the seal when it isn't in active use |
| `status` | TEXT | Yes | `available`, `in_use`, `lost` |

#### 2.4.2 Seal Usage Requests

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable request ID |
| `seal_id` | TEXT/UUID | Yes | The seal requested |
| `requested_by_user_id` | TEXT/UUID | Yes | Who is asking to use it |
| `document_reference` | TEXT | Yes | Free-text description of what's being stamped, e.g. a contract name/number |
| `requested_use_date` | DATE | Yes | When the seal is needed |
| `status` | TEXT | Yes | `pending_approval`, `approved`, `rejected`, `used`, `cancelled` |
| `approval_request_id` | TEXT/UUID | No | Optional link to an Approval Request (`approval-system.md` §2.3, Section 3.1); given the legal risk, an organization is expected to always route this through a real approval flow rather than the single-manager fallback, though this document doesn't hard-enforce that choice |

#### 2.4.3 Seal Usage Log

An append-only record of actual use, separate from the request — a request captures intent and approval; this captures what actually happened.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable log entry ID |
| `seal_usage_request_id` | TEXT/UUID | Yes | The approved request this use fulfills |
| `used_by_user_id` | TEXT/UUID | Yes | Who physically applied the seal |
| `used_at` | DATETIME | Yes | When it was applied |
| `returned_at` | DATETIME | No | When the seal was returned to its custodian |

### 2.5 Polls & Statistics (問卷統計)

A generic poll/survey capability for the ad-hoc headcounts and quick statistics General Affairs regularly needs — "who's coming to the year-end party," "does everyone want the new badge design" — targeted at either the whole organization or one specific group. It also doubles as the foundation for Group Meal Ordering (Section 2.6), which is really a poll with a price tag on each option.

#### 2.5.1 Polls

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable poll ID |
| `title` | TEXT | Yes | e.g. "尾牙意願調查", "今日團訂 - 便當" |
| `description` | TEXT | No | Free-text detail |
| `poll_type` | TEXT | Yes | `general`, `food_order` — `food_order` is what unlocks the settlement fields below and Section 2.6; a `general` poll ignores them entirely |
| `created_by_user_id` | TEXT/UUID | Yes | Who created the poll |
| `target_scope` | TEXT | Yes | `organization`, `group` — deliberately narrower than `notification_audiences`' four-way scope (`notification-system.md` §4), since this document's requirement is specifically "whole company or one specific group," not per-role or per-user targeting |
| `target_group_id` | TEXT/UUID | No | Required when `target_scope = group`; the group whose active members (`group-system.md` §3.2) can respond |
| `status` | TEXT | Yes | `draft`, `open`, `closed` |
| `closes_at` | DATETIME | No | Response deadline; whether an `open` poll past this moment is closed by a scheduled job or treated as closed on read is the same open question already left unresolved for Quote expiration (`order-system.md` §4) |
| `settlement_method` | TEXT | No | `none`, `finance_expense`, `salary_deduction` — only meaningful when `poll_type = food_order` (Section 2.6); `none` means a `food_order` poll is headcount-only, with amounts tracked but no money actually moved anywhere |
| `finance_expense_claim_id` | TEXT/UUID | No | Optional link to a Finance Expense claim (`finance-system.md` §6, Section 3.3), created when the poll is settled with `settlement_method = finance_expense` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

#### 2.5.2 Poll Options

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable option ID |
| `poll_id` | TEXT/UUID | Yes | Owning poll |
| `label` | TEXT | Yes | e.g. "雞腿飯", "同意" |
| `unit_amount` | DECIMAL | No | Price of one unit of this option; populated for `poll_type = food_order`, null/ignored for `general` |
| `sort_order` | INTEGER | Yes | Display order |

#### 2.5.3 Poll Responses

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable response ID |
| `poll_id` | TEXT/UUID | Yes | Owning poll |
| `responded_by_user_id` | TEXT/UUID | Yes | Who responded |
| `participating` | BOOLEAN | Yes | Whether this person is opting in at all — a food order's basic yes/no signal; a `general` poll's submitted response is always `true` |
| `responded_at` | DATETIME | Yes | UTC timestamp; a response may be edited up until `closes_at`, updating this field |

At most one response per `(poll_id, responded_by_user_id)` — resubmitting updates the existing response rather than creating a second one.

#### 2.5.4 Poll Response Selections

The line items within one response — which option(s), and how many.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `poll_response_id` | TEXT/UUID | Yes | Owning response |
| `option_id` | TEXT/UUID | Yes | The selected option (Section 2.5.2) |
| `quantity` | DECIMAL | Yes | Defaults to `1`; supports ordering more than one of the same item |

A response's line total is `Σ (quantity × option.unit_amount)`; a poll's grand total is the sum of every response's line total — the figure Section 2.6's settlement uses. For a `general` poll, `unit_amount` is unset everywhere, so these totals are always zero and simply unused.

### 2.6 Group Meal Ordering (團訂餐點)

Settlement for a `food_order` poll (Section 2.5.1) once it closes. Two distinct money paths, chosen per poll via `settlement_method`, matching the two ways an organization typically pays for a group order: the company covers it as a business expense, or each participant pays for their own portion through payroll.

- **`finance_expense`**: the poll's grand total (Section 2.5.4) becomes a single Expense claim (`finance-system.md` §6) submitted by the poll's `created_by_user_id`, with the poll's `title` as the claim's business justification — one lump-sum claim covering everyone who participated, not a per-person charge. `finance_expense_claim_id` (Section 2.5.1) records the link once created.
- **`salary_deduction`**: each participating response is charged individually, recorded below (Section 2.6.1) rather than pooled into one claim, since the food order is being paid for by each person, not by the organization.

#### 2.6.1 Salary Deduction Requests

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable request ID |
| `poll_id` | TEXT/UUID | Yes | Owning poll |
| `poll_response_id` | TEXT/UUID | Yes | The response this deduction charges for |
| `user_id` | TEXT/UUID | Yes | Who is being charged — same person as the response's `responded_by_user_id`, kept explicit here as this table's own Salary-facing reference |
| `amount` | DECIMAL | Yes | The response's line total (Section 2.5.4) at the moment the poll was settled |
| `status` | TEXT | Yes | `pending`, `applied`, `cancelled` — `applied` once a Salary settlement run has actually included it as a deduction |
| `created_at` | DATETIME | Yes | Creation time in UTC |

General Affairs only ever produces a `pending` request here — it never writes to Salary data directly, the same never-write-directly boundary HR already keeps with Salary (`hr-system.md` §4.4). Exactly how and when a Salary settlement run (`salary-system.md` §6) picks up a `pending` row and marks it `applied` — as one instance of the "salary advances or loan repayments" ad-hoc deduction category `salary-system.md` §3 already names but doesn't yet schema — is an open decision on both documents (Section 5 here; `salary-system.md` §12).

General Affairs owns its own database (`general_affairs.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. Relationship to Other Systems

### 3.1 Approval System

Supply Requisitions (Section 2.2), Seal Usage Requests (Section 2.4.2), and — optionally, above a configurable duration or for high-demand resources — Bookings (Section 2.3.2) each carry an optional `approval_request_id` linking to [`approval-system.md`](./approval-system.md) §2.3. When Approval is not installed, General Affairs falls back to a single-approver default: the requester's manager, resolved directly per `group-system.md` §5, exactly the pattern leave approval already uses. This is the first real consumer of the Approval System's generic engine (`approval-system.md` §4.1).

### 3.2 HR System

Asset Events (Section 2.1.1) are what an HR onboarding/offboarding checklist task (`hr-system.md` §3.4, reused for offboarding per §3.7) should actually point at via `related_hr_task_id` — HR's task tracks *that* equipment needs to be issued or returned as one step in a checklist; General Affairs' Asset Events record *which* physical asset, in what condition, and when. This resolves the gap `hr-system.md` §2 item 10 previously left open by deferring administrative/records management entirely.

### 3.3 Finance & Accounting

Assets (Section 2.1) share their `id` as a common tag with Finance's Fixed Asset Accounting record (`finance-system.md` §8) — General Affairs owns custodian/location/condition, Finance owns acquisition cost, depreciation method, useful life, and accumulated depreciation, exactly as `finance-system.md` §8 already specifies for its "HR Asset Management" counterpart. A depreciation run posts a GL journal entry on the Finance side; it never touches an Asset Event here. Aside from that indirect Procurement path (Section 3.4), General Affairs has exactly one direct Finance linkage: settling a `food_order` poll with `settlement_method = finance_expense` (Section 2.6) submits an Expense claim (`finance-system.md` §6) — still a draft submission into Finance's own approval flow, never a direct GL write, consistent with the draft-then-post pattern used throughout this platform (`finance-system.md` §4, §8.1).

### 3.4 Procurement System

An Office Supply Requisition (Section 2.2) may carry an optional `procurement_purchase_order_id` linking to [`procurement-system.md`](./procurement-system.md) §2.2, when Procurement is installed and the requested item is sourced through a formal vendor PO rather than fulfilled directly (e.g. from an existing supply cabinet). Once linked, the requisition's fulfillment status can be derived from the PO's own lifecycle (goods receipt, Section 2.3 of that document) instead of being set manually. General Affairs never talks to a vendor directly — Procurement remains the only module that owns vendor relationships.

### 3.5 Group System

`approver_type = requester_manager` fallback (Section 3.1) and every "who is this person's manager" question in this document resolve through `group-system.md` §5's existing Manager Resolution algorithm — General Affairs adds no parallel resolution logic of its own. A Poll's `target_scope = group` (Section 2.5.1) similarly resolves eligible respondents from `group-system.md` §3.2's active membership of `target_group_id` — General Affairs does not maintain a second membership list.

### 3.6 Notification System

Booking confirmations (Section 2.3.2), requisition/seal-request decisions, and asset issuance/return should reuse the existing `notification-system.md` capability rather than a bespoke messaging path. So should a Poll opening and an approaching `closes_at` deadline (Section 2.5.1) — delivered to the resolved audience (the whole organization, or `target_group_id`'s members per Section 3.5), the same audience-delivery mechanism `notification-system.md` §4 already provides.

### 3.7 Inventory System

An Office Supply Requisition (Section 2.2) fulfilled by drawing from existing warehouse stock, rather than a fresh vendor purchase, carries an optional `inventory_outbound_id` linking to [`inventory-system.md`](./inventory-system.md) §2.4. General Affairs never writes that Outbound record itself — the flow is: the requisition's Flow Template (`approval-system.md` §2.1) configures a warehouse-facing role as a Completion Notification Target (`approval-system.md` §2.5), so approval reaching `approved` notifies the warehouse team to actually pull the stock; whoever does that records the Outbound through Inventory's own recording screen (`inventory-system.md` §2.3–§2.4) exactly as they would for any other outbound movement; the requisition is then linked to that record. This is the same optional-link-plus-fallback pattern used everywhere goods physically move in this platform (e.g. `checkout-system.md` §4.1) — General Affairs stays fully usable with Inventory uninstalled, just without the stock-linked fulfillment path.

### 3.8 Salary System

Settling a `food_order` poll with `settlement_method = salary_deduction` (Section 2.6) creates one Salary Deduction Request (Section 2.6.1) per participating response. General Affairs never writes to `salary-system.md` data — it only produces a `pending` request; whether and how a Salary settlement run (`salary-system.md` §6) consumes it is an open decision on both sides (Section 5, `salary-system.md` §12). General Affairs works fully with Salary uninstalled, simply leaving `salary_deduction` unavailable as a settlement method — `finance_expense` and `none` remain usable regardless.

## 4. Permissions

Planned permission keys, to be added to `config/permission.json` when these APIs are implemented, following the same catalog convention as other modules (`user-system.md` §3.4).

| Permission | Allows |
| --- | --- |
| `general_affairs.assets.read.self` | View assets currently issued to the current user |
| `general_affairs.assets.read` | View all assets and their custody history |
| `general_affairs.assets.manage` | Record asset creation, issuance, transfer, return, loss, and retirement (Section 2.1.1) |
| `general_affairs.requisitions.submit` | Submit an Office Supply Requisition (Section 2.2) |
| `general_affairs.requisitions.read.self` | View the status of the current user's own requisitions |
| `general_affairs.requisitions.manage` | View all requisitions and mark them fulfilled |
| `general_affairs.bookings.read` | View resource availability and bookings |
| `general_affairs.bookings.book` | Create a booking for oneself, invite attendees (Section 2.3.3), and edit/cancel a booking one owns — including responding `accept`/`decline` to a Space Coordination case (Section 2.3.4) raised against a booking one owns, the same "own record" exception pattern already used for template uploads (`template-system.md` §2) |
| `general_affairs.bookings.manage` | Manage Bookable Resources (Section 2.3.1) and edit/cancel any booking regardless of owner |
| `general_affairs.bookings.coordinate` | Initiate a Space Coordination case (Section 2.3.4) |
| `general_affairs.seals.request` | Submit a Seal Usage Request (Section 2.4.2) |
| `general_affairs.seals.manage` | Manage seal records (Section 2.4.1), approve usage outside the Approval System fallback, and record actual use (Section 2.4.3) |
| `general_affairs.polls.create.organization` | Create a poll targeting the whole organization (Section 2.5.1) |
| `general_affairs.polls.create.group` | Create a poll targeting a specific group |
| `general_affairs.polls.respond` | Submit or edit one's own response to a poll visible to them (Section 2.5.3) |
| `general_affairs.polls.read` | View a poll's aggregate results and individual responses |
| `general_affairs.polls.settle` | Close a `food_order` poll and trigger its settlement (Section 2.6) — creating the Finance expense claim or the per-person Salary Deduction Requests, a distinct and higher-trust action from merely creating the poll |

The initial Admin role automatically receives every General Affairs permission through `grants_all_permissions`, as with other modules.

## 5. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- mail and document receiving/dispatch (收發文) tracking — not modeled in this document;
- contract lifecycle management beyond the point where a seal is applied (Section 2.4) — drafting, versioning, and counterparty tracking are out of scope;
- a full Meeting System (agenda, minutes, attendee RSVP) — Section 2.3 only provides the underlying resource-booking primitive it would eventually consume;
- recurring bookings (e.g. a standing weekly meeting) — Section 2.3.2 only models single, discrete bookings today;
- fleet/vehicle-specific details (mileage, fuel, maintenance scheduling) beyond treating a vehicle as one more Bookable Resource;
- whether Bookings (Section 2.3.2) always require approval or only conditionally, and what that condition is (duration, resource category, or organization-wide policy) — separate from Space Coordination (Section 2.3.4), which handles conflicts on already-self-service bookings;
- email delivery for Space Coordination notices (Section 2.3.4) — `notification-system.md` is an in-app channel only today; no SMTP/email-sending capability is designed anywhere in this platform yet, so how "send an email to the existing booker" is actually implemented isn't decided;
- attendee availability/conflict checking (Section 2.3.3) — only the resource itself is conflict-checked (Section 2.3.2), never whether an invitee is free;
- whether a requester can trigger a Space Coordination case directly on submission (Section 2.3.4), or only a General Affairs staff member can initiate one;
- what happens after a Space Coordination `decline` (Section 2.3.4) beyond "resolve it manually" — no automatic re-proposal or escalation is modeled;
- retention period for closed requisitions, completed bookings, and seal usage logs;
- exactly how and when a Salary settlement run consumes a `pending` Salary Deduction Request (Section 2.6.1) and marks it `applied` — not specified on either this document or `salary-system.md`;
- whether closing a `food_order` poll auto-triggers settlement (Section 2.6), or always requires an explicit `general_affairs.polls.settle` action;
- whether a Poll Response (Section 2.5.3) can still be edited, and whether that recomputes an already-settled poll's totals, once `status` has moved to `closed`;
- whether Polls (Section 2.5) are meant to grow into — or stay deliberately narrower than — the "Future Form System" referenced in `hr-system.md` §5, §7 for satisfaction surveys and engagement activities; this document treats Polls as a lighter, General-Affairs-scoped statistics tool, not a general-purpose form builder;
- currency handling for `unit_amount` (Section 2.5.2) — assumed implicitly single-currency per poll, with no explicit `currency` field modeled;
- whether `target_scope = group` (Section 2.5.1) is restricted to groups the creator manages, mirroring `notification-system.md`'s narrower `own_group` send scope, or open to any active group via `general_affairs.polls.create.group`;
- permission keys beyond Section 4's first pass, to be finalized once API design starts.
