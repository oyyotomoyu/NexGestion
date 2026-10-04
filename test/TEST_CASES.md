# API Test Case Matrix

This matrix defines the intended API script coverage. A case can be implemented in a module script when the matching API exists and the setup data can be created through public or protected API calls.

## Auth And Users

| Case | Level | Expected |
| --- | --- | --- |
| Public health check | 1 | `GET /api/health` returns `200` without auth |
| Login success | 1 | Admin credentials return access token and refresh cookie |
| Current user | 1 | `GET /api/auth/me` returns authenticated admin |
| User CRUD happy path | 1 | Create, list, read, update, soft-delete normal user |
| Missing auth | 2 | Protected user endpoints return `401` |
| Invalid JSON/input | 2 | Create/update return `400` |
| Duplicate email | 2 | Create returns `409` case-insensitively |
| Unknown user | 2 | Read/update/delete return `404` |
| Protected admin delete | 2 | Delete initial administrator returns `403` |
| Bad login | 2 | Invalid credentials return `401` |
| User isolation | 3 | Updating/deleting one non-protected user does not change another |
| Session lifecycle | 3 | Refresh rotates token; logout revokes refresh token |
| Account status | 3 | Disabled/locked users cannot authenticate or call protected APIs |

## Roles

| Case | Level | Expected |
| --- | --- | --- |
| Role CRUD happy path | 1 | Create, list, read, update, delete custom role |
| Missing auth/permission | 2 | Protected endpoints return `401` or `403` |
| Invalid title | 2 | Empty title returns `400` |
| Duplicate title | 2 | Duplicate title returns `409` case-insensitively |
| Unknown role | 2 | Read/update/delete returns `404` |
| System role mutation | 2 | Admin/generated group roles cannot be edited or deleted |
| Assignment lifecycle | 3 | Assign/remove custom role from a non-protected user |
| Permission grant lifecycle | 3 | Initial administrator grants/revokes catalog permission |
| Delegated manager boundary | 3 | Non-initial admin cannot grant/revoke permissions |

## Groups

| Case | Level | Expected |
| --- | --- | --- |
| Group CRUD happy path | 1 | Create, list, read, update, delete leaf group |
| Missing auth/permission | 2 | Protected endpoints return `401` or `403` |
| Invalid group input | 2 | Missing name/type/status/level returns `400` |
| Duplicate group name | 2 | Duplicate name returns `409` |
| Unknown group | 2 | Read/update/delete returns `404` |
| Child deletion policy | 2 | Parent with child group returns `409` on delete |
| Generated role policy | 3 | Group roles cannot be deleted through Role API |
| Membership lifecycle | 3 | Add/update/remove manager/member through API |
| Owner-driven deletion | 3 | Deleting leaf group removes memberships and generated roles |
| Hierarchy cycle | 3 | Parent update creating a cycle returns `400` |

## Attendance

| Case | Level | Expected |
| --- | --- | --- |
| Today/read basics | 1 | Read today, days, leave types |
| Clock workflow | 1 | Sign in and sign out in documented order |
| Invalid clock order | 2 | Repeated sign-in/sign-out errors match document |
| Leave request basics | 2 | Create/list leave request with valid type |
| Invalid leave request | 2 | Bad dates/type return documented errors |
| Approval routing | 3 | Manager/admin approval flow follows group hierarchy rules |
| Monthly reports | 3 | Generate, list, and download monthly CSV through API |
| Correction workflow | 3 | Authorized correction changes a day and records audit/report effect |

## Notifications

| Case | Level | Expected |
| --- | --- | --- |
| Type/list basics | 1 | List notification types and visible notifications |
| Send/edit/hide own notification | 1 | Sender can create, edit, and hide allowed notification |
| Missing auth | 2 | Protected endpoints reject unauthenticated requests |
| Invalid payload | 2 | Bad type/content/time returns documented errors |
| Admin list | 2 | `notifications.manage` user can list all messages |
| Sender boundary | 3 | Non-sender cannot edit/hide another sender's notification |
| Expiry/export | 3 | Timed notifications expire and monthly CSV export works |

## Logs And Report Files

| Case | Level | Expected |
| --- | --- | --- |
| Log list basics | 1 | Authorized user can query logs |
| Log filters | 2 | Date/status/limit filters match document |
| Bad log filters | 2 | Invalid date/status/limit returns `400` |
| Report file list | 1 | Authorized user can list report files |
| Report file download/delete | 2 | Existing report file can be downloaded/deleted |
| Path traversal | 2 | Traversal attempts are rejected and cannot leave report root |
| Retention/cleanup evidence | 3 | Generated reports and logs obey documented retention boundaries |

## Security

Source: `server/apis/security.go`, `server/system/security.go`, `docs/System/SIEM.md`.

Design note: `security_settings` (deployment posture, retention) and each row in
`security_rules` are singletons, not per-run-creatable resources like a user or
role. Level 3 cases that mutate them must capture the pre-test value and
restore it before the script exits, win or lose — unlike every other module
here, rerunning this script does not get a clean slate for free.

| Case | Level | Expected |
| --- | --- | --- |
| Read basics | 1 | Admin can read settings, list rules, list events, list recipients |
| Rule catalog defaults | 1 | Rule list includes `brute_force_login`, `privilege_escalation`, `mass_export`, `new_device_privileged_login` (enabled) and `network_change`, `off_hours_privileged_action` (shipped disabled — their event sources don't exist yet) |
| Test alert happy path | 1 | `POST /api/security/test-alert` returns `200` with `event_type: "test_alert"` |
| Recipient add/remove happy path | 1 | Admin can add an email recipient and remove it |
| Missing auth | 2 | All nine endpoints reject an unauthenticated request with `401` |
| Invalid JSON/unknown fields | 2 | Rule and settings `PATCH` reject malformed JSON and unknown fields with `400` |
| Invalid rule config | 2 | `PATCH rules/{key}` with a `config_json` value that isn't valid JSON returns `400` without writing |
| Unknown rule key | 2 | `PATCH rules/{key}` for an undefined key returns `404` |
| Invalid settings values | 2 | `deployment_posture` outside `lan_only`/`internet_exposed`, or `retention_days <= 0`, returns `400` without writing |
| Invalid recipient input | 2 | Recipient create rejects: both `user_id` and `email` set, neither set, unknown `scope_type`, empty `scope_value`, unknown `channel` — all `400` |
| Unknown recipient | 2 | Deleting an unknown recipient id returns `404` |
| Invalid list query | 2 | Events list rejects non-numeric `page`/`page_size`, negative `page_size`, `page_size` over the maximum, and an unknown `sort`/`order` value with `400`. Note: `page_size=0` is *not* an error — it is silently treated as unset and falls back to the default page size, the same convention `page=0` uses; a case asserting this is expected `200` behavior, not a `400` row |
| Permission boundaries | 2 | `security.events.read` alone can read events/rules/settings but gets `403` on every rules/settings/recipients/test-alert write; `security.rules.manage` alone gets `403` reading events/rules/settings (it does not imply `events.read`) and `403` on recipients/test-alert; `security.alerts.manage` alone can manage recipients and send a test alert but gets `403` reading events/rules/settings and `403` updating rules/settings. The three write/read permissions are independent — holding one grants nothing from the others |
| Password-gated grant | 2 | `security.rules.manage` and `security.alerts.manage` are `high_risk`+`requires_password` in `config/permission.json`; granting either via `PUT /api/roles/{id}/permissions/{permissionId}` with a missing or wrong `current_password` returns `403` before the grant takes effect. (This gate exists for every high-risk permission, not just Security's two, but nothing in `role.sh`/`permissions.sh` currently exercises it because none of the permissions they grant are high-risk — this module is the first real coverage of that path.) |
| Permission catalog pagination | 2 | `GET /api/permissions` defaults to `page_size=20` against a 95+ entry catalog; any setup helper that looks up a permission id must filter by `?keyword=` (or otherwise paginate) rather than scanning only the first page — `security.events.read`/`security.rules.manage`/`security.alerts.manage` do not fall on page 1, so an unfiltered lookup silently fails. `role.sh` and `attendance.sh` use the same unfiltered-lookup pattern and only work today because the permissions they look up (`users.read`, `attendance.*`) happen to sort onto page 1 — this is latent breakage waiting for the catalog to reorder, not something specific to Security |
| Deployment posture event | 3 | Switching `deployment_posture` to `internet_exposed` records a `deployment_posture_changed` event at `critical` severity; switching back to `lan_only` records one at `warning` severity (SIEM.md 7.2) |
| Rule enable/disable round trip | 3 | Toggling an inert rule (`network_change`, ships disabled with no event source yet, so toggling it cannot affect any other rule's evaluation) and its `config_json` persists and is restored to its original value |
| Failed-login visibility | 3 | Repeated failed logins against one (unique, run-scoped) email are each recorded as `login_failed` events with `record_ref` equal to that email, retrievable via `?keyword=` |
| Recipient full lifecycle | 3 | Create a rule-scoped email recipient, confirm it appears in the list, delete it, and confirm a second delete of the same id returns `404` (deletion is not idempotent) |
| Brute-force/mass-export exact threshold | 3 | Covered by `server/system/security_test.go` (`TestBruteForceLoginRuleFiresAtThreshold`), not by an HTTP script: `countRecentSecurityEvents` matches by `actor_user_id OR source_ip OR record_ref`, and every HTTP test client shares one source IP, so an HTTP-level script cannot assert an *exact* firing threshold without also being affected by whatever else recently hit that same server from that same IP. A shell script may assert that repeated failures are recorded as events; it must not assert the alert fires at exactly N |
| Privilege-escalation rule | 3 | Granting any `*.manage`/`*.configure` permission (or `permissions.assign`/`roles.manage`) to a role records a `permission_granted` event with that `PermissionKey` and fires the rule; granting a non-escalation permission (e.g. `security.events.read`) still records `permission_granted` but does not fire the rule. Covered in `server/system/security_test.go`; an HTTP-level case only needs to confirm the event's `event_type`/`permission_key`, not the alert side-effect |
| **Doc/implementation gap: revoke does not log** | 3 (FAIL expected) | SIEM.md Section 12: "every grant **or revocation** of these four permissions should itself generate a security event." `setRolePermission` (`server/apis/access_control.go`) only calls `RecordEvent` when `grant` is `true` — revoking `security.rules.manage`/`security.alerts.manage`/etc. from a role produces no security event at all. Record this as a failing case against the doc rather than silently testing only the grant half |
| Alert delivery channel status | 3 | `sendAlertEmail` (`server/system/security.go`) is a stub that always returns `errEmailTransportNotConfigured`; every email-channel delivery attempt is recorded with `status: "skipped"`, never `"sent"`, until SMTP transport lands (SIEM.md 13, shared with `salary-system.md` 10.3). A case asserting `"skipped"` today will need updating the day email transport ships — do not assert `"sent"` prematurely |
| Hash chain integrity | 3 | Each event's `prev_hash` equals the immediately preceding event's `hash` (SIEM.md Section 11); covered directly in `server/system/security_test.go` (`TestRecordEventChainsHashes`) since it requires reading a column (`prev_hash`) the HTTP API never exposes |
| Retention maintenance | 3 (Go-test only) | `RunMaintenance` deletes events past `retention_days` but runs on an hourly ticker from `main.go`, not an HTTP endpoint — not reachable from a shell script at all; stays covered by `server/system/security_test.go` (`TestRunMaintenanceDeletesEventsPastRetention`) |
| `lan_only` violation forcing critical severity | 3 (Go-test only) | A public source IP under `lan_only` posture forces `severity: "critical"` regardless of the event's own severity (SIEM.md 7.2) — not practically reachable from a local HTTP test client, whose source IP is always private/loopback; stays covered by `server/system/security_test.go` (`TestRecordEventForcesCriticalOnLANOnlyViolation`) |

## Templates

Source: `server/apis/templates.go`, `server/system/templates.go`, `docs/System/template-system.md`.

| Case | Level | Expected |
| --- | --- | --- |
| Template CRUD happy path | 1 | Upload, list, download, and delete own template file |
| Storage usage basics | 1 | `templates.manage` caller reads current usage vs configured caps |
| Missing auth/permission | 2 | Protected endpoints return `401` or `403` for `templates.read`/`templates.upload`/`templates.manage` |
| Invalid upload input | 2 | Missing file, missing audiences, malformed `audiences` JSON, or incomplete/unknown audience scope returns `400` |
| Oversized file | 2 | Upload exceeding `template_file_max_bytes` returns `413` |
| Storage limit exceeded | 2 | Upload that would push total usage over `template_storage_max_bytes` returns `507` and is not persisted |
| Duplicate filename allowed | 2 | Uploading an identical filename twice succeeds with two distinct ids; no uniqueness rule applies to `original_filename` (only the UUID-based `stored_path` is unique) |
| Unknown template | 2 | Download/delete of an unknown id returns `404` |
| Visibility-restricted download | 2 | Caller holding `templates.read` but outside the file's audience returns `403` on download |
| Delete ownership boundary | 2 | `DELETE` has no permission middleware (only `authenticated`) — ownership vs. `templates.manage` is enforced inside the service. Non-owner without `templates.manage` deleting another user's upload returns `403`; uploader deletes own file; `templates.manage` deletes any file |
| Audience-scoped visibility | 3 | Organization/group/role/user audiences each resolve to the correct visible subset for different callers' roles and groups |
| Manager bypass | 3 | `templates.manage` caller lists and downloads every file regardless of audience, bypassing visibility filtering |
| Storage lifecycle accounting | 3 | Sequential uploads and deletes keep `used_bytes`/`file_count` from `GET /api/templates/storage` consistent with actual files on disk |

## Salary

Source: `server/apis/salary.go`, `server/system/salary.go`, `docs/System/salary-system.md`.

| Case | Level | Expected |
| --- | --- | --- |
| Compensation record basics | 1 | Admin creates initial compensation record for an employee; employee reads own current record and history via `/me`; admin reads the same employee's current record and history via `/employees/{userId}` |
| Missing auth | 2 | All five compensation-record endpoints return `401` without a valid access token |
| Insufficient permission | 2 | Caller without `salary.read.self` cannot call `/me` endpoints; caller without `salary.read` cannot call `/employees/{userId}` read endpoints; caller without `salary.settlement.configure` cannot create a record; all return `403` |
| Invalid JSON/input | 2 | Create returns `400` for malformed JSON body, unknown `compensation_basis`, non-numeric or negative `rate_amount`, zero `rate_amount`, non-3-letter `currency`, empty `jurisdiction_id`, and `effective_start_date` not formatted as `YYYY-MM-DD` |
| Unknown employee | 2 | Create, list, and current-record reads against an unknown `userId` return `404` |
| No active record yet | 2 | Current-record read for an employee with no compensation record returns `404`; list for the same employee returns an empty page, not an error |
| Overlapping effective date | 2 | Create with `effective_start_date` on or before the active record's `effective_start_date` returns `409` and leaves the active record unchanged |
| List filters | 2 | `active=true`/`active=false` and `compensation_basis` filters on list match the documented record set; invalid `page`/`page_size` returns `400` |
| **Doc/implementation gap: undocumented permission** | 2 (FAIL expected) | `config/permission.json` defines `salary.access` (module `salary`) with no matching entry in `docs/System/salary-system.md` Section 7's permission table, and no route in `router.go` checks it — a caller granted only `salary.access` still gets `403` on every salary endpoint. Record this as a failing case against the doc |
| Rate history/effective-dating lifecycle | 3 | Creating a second record after the first closes the first record's `effective_end_date` to the day before the new record's start date, leaves exactly one active (open-ended) record, and list/current reflect the full ordered history |
| Compensation confidentiality boundary | 3 | An employee holding only `salary.read.self` can read their own `/me` records but receives `403` on another employee's `/employees/{userId}` records even when given that employee's exact ID |
| Permission separation | 3 | A role granted only `salary.settlement.configure` can create records but cannot list/read them (`403`); a role granted only `salary.read` can list/read but cannot create (`403`) |

## Checkout

Source: `server/apis/checkout.go`, `server/system/checkout.go`, `docs/System/checkout-system.md`.

| Case | Level | Expected |
| --- | --- | --- |
| Transaction lifecycle happy path | 1 | Create transaction, add line, add payment equal to total, complete; `GET` list/read reflect `completed` status and recalculated totals |
| Scan basics | 1 | `GET /api/checkout/scan` resolves a known coupon code and returns `not_recognized` for an unmatched code |
| Coupon and promotion rule creation | 1 | `checkout.manage` user creates a valid discount coupon, a valid voucher coupon, and a valid promotion rule |
| Missing auth | 2 | All `/api/checkout/*` endpoints return `401` without a token |
| Insufficient permission | 2 | `checkout.read`-only user gets `403` on sell/manage endpoints; `checkout.sell`-only user gets `403` creating coupons/promotion rules; `checkout.manage`-only user gets `403` on transaction endpoints |
| Invalid transaction input | 2 | Missing `warehouse_id` or malformed `currency` returns `400` |
| Invalid line input | 2 | Zero/negative `quantity`, or non-numeric `quantity`/`unit_price` returns `400` |
| Invalid discount input | 2 | Missing `source_reference_id`, non-positive `amount`, or unknown `source_type` returns `400` |
| Invalid payment input | 2 | Unknown `method` or non-positive `amount` returns `400` |
| Invalid coupon input | 2 | Missing `code`, bad `coupon_type`, a discount coupon missing `discount_type`/`discount_value`, a voucher with non-positive `value_amount`, or `usage_limit <= 0` returns `400` |
| Invalid promotion rule input | 2 | Bad `discount_type`, bad `scope`, or non-numeric `discount_value`/`min_subtotal_amount` returns `400` |
| Unknown transaction | 2 | Read, add line/discount/payment, complete, and void on an unknown transaction id all return `404` |
| Mutating a completed transaction | 2 | Add line/discount/payment to a `completed` transaction returns `409` |
| Mutating a voided transaction | 2 | Add line/discount/payment to a `voided` transaction returns `409` |
| Double completion | 2 | Completing an already-`completed` transaction returns `409` |
| Completing a voided transaction | 2 | `complete` on a `voided` transaction returns `409` |
| Double void | 2 | Voiding an already-`voided` transaction returns `409` |
| Voiding a completed transaction | 2 | `void` on a `completed` transaction returns `409` (no post-completion reversal exists) |
| Insufficient payment on complete | 2 | Completing with payments summing to less or more than `total_amount` returns `400`, not `409` |
| **Doc/implementation gap: scan resolves coupons only** | 2 (FAIL expected) | `checkout-system.md` Section 4.4 documents scan resolution against Inventory items, CRM membership numbers, and Coupon codes, in that order; `CheckoutService.ResolveScanCode` only ever queries `checkout_coupons` — a known Inventory barcode or CRM membership number returns `not_recognized` instead of the documented line-add/customer-link effect |
| **Doc/implementation gap: duplicate coupon code returns 500** | 2 (FAIL expected) | `code` is documented unique (Section 2.3.2) and is DB-enforced unique, but `CreateCoupon` never pre-checks it; a duplicate `code` trips the SQLite `UNIQUE` constraint and falls through the default branch of `writeCheckoutError`, returning `500` instead of `400`/`409` |
| **Doc/implementation gap: discount reference not validated** | 2 (FAIL expected) | Section 3 says a redeemed coupon is validated (active, within window, under usage limit) before being applied; `AddDiscount` performs no lookup against `checkout_coupons`/`checkout_promotion_rules` at all — any `source_reference_id` and positive `amount` is accepted even for a nonexistent/inactive/expired coupon, and `redeemed_count` is never incremented anywhere |
| Split-tender completion | 3 | Add multiple lines, add a discount, settle the discounted total across separate cash and card payment calls, and confirm `complete` only succeeds once the sum exactly matches `total_amount` |
| Discount exceeds subtotal floors at zero | 3 | A discount amount larger than `subtotal_amount` leaves `total_amount` at `0` rather than negative, and `complete` only requires payments summing to `0` |
| Void mid-lifecycle lockout | 3 | Void an in-progress transaction that already has lines added, then confirm further line/discount/payment additions and `complete` all return `409` |
| Member points on completion | 3 | Completing a transaction with `crm_customer_id` set posts earned points to that customer's Points Ledger via CRM; completing without a linked customer skips the posting |
| Role separation across sell and manage | 3 | A `checkout.sell`-only cashier can run the full transaction lifecycle but cannot create coupons/promotion rules; a `checkout.manage`-only user can create coupons/promotion rules but cannot operate transactions |

## Approvals

Source: `server/apis/approvals.go`, `server/system/approval.go`, `docs/System/approval-system.md`.

| Case | Level | Expected |
| --- | --- | --- |
| Flow template CRUD happy path | 1 | Create, list, read, update, delete a flow template (with steps and notification targets) using `approvals.templates.manage` |
| Single-step submit-and-approve happy path | 1 | Submit a request against an active single-step template; the assigned approver decides `approved` and the request reaches `approved` |
| Self-service visibility basics | 1 | Requester reads own request via `GET /api/approvals/requests/{id}` and sees it under `/me/requests`; the assignee sees it under `/me/assignments` |
| Missing auth | 2 | All `/api/approvals/*` endpoints return `401` without a valid access token |
| Insufficient permission | 2 | Each permission-gated endpoint returns `403` when the caller lacks its specific permission (`approvals.templates.manage`, `approvals.read`, `approvals.read.self`, `approvals.decide`, `approvals.reassign`) |
| Invalid JSON/input | 2 | Malformed body on template create/update, submit, decide, and reassign returns `400` |
| Invalid template fields | 2 | Missing `name`, a `request_type` not matching the `module.request` pattern, zero steps, an unknown `approver_type`, or a missing type-specific field (`approver_user_id`/`approver_role_id`/`approver_group_id`) returns `400` |
| Invalid flow template status | 2 | `PATCH` with `status` other than `active`/`inactive` returns `400` |
| Unknown template | 2 | Read/update/delete an unknown template ID returns `404` |
| Invalid template approver references | 2 | A step or notification target referencing an unknown user, role, or group returns `404` |
| Template deletion policy | 2 | Deleting a template referenced by any approval request — including fully completed ones — returns `409`; deleting an unreferenced template succeeds |
| Unknown request | 2 | Get/cancel/decide/reassign on an unknown request ID returns `404` |
| Submit against unknown/inactive template | 2 | Submitting with an unknown `flow_template_id` returns `404`; submitting against an `inactive` template returns `400` |
| Invalid submit input | 2 | Missing `flow_template_id`/`source_module`/`source_reference_id`, or a malformed `amount`, returns `400` |
| Invalid decision value | 2 | `decide` with a value other than `approved`/`rejected` returns `409` (`approval_decision_invalid`), not `400` |
| Decide a non-assigned step | 2 | A caller who holds `approvals.decide` but is not in the current step's assignee list gets a `409` conflict, not `200` |
| Decide an already-terminal request | 2 | `decide` on a request whose status is already `approved`, `rejected`, or `cancelled` returns `409` |
| Cancel someone else's request | 2 | A non-requester holding `approvals.read.self` calling `cancel` on another user's request returns `403` |
| Cancel a non-cancellable request | 2 | Cancelling a request that already has any recorded step decision, or that is already terminal, returns `409` |
| Cancel a requires_assignment request | 2 | Cancelling succeeds while status is `requires_assignment` (no decision has been recorded yet), same as `pending` |
| Reassign a request not requiring assignment | 2 | `reassign` on a `pending`, `approved`, `rejected`, or `cancelled` request returns `409` |
| Reassign with invalid assignees | 2 | Reassigning to an unknown user ID returns `404`; reassigning with an empty list or only the requester's own ID returns `400` |
| Non-owner/non-assignee read masking | 2 | A user who is neither the requester nor an assignee, and lacks `approvals.read`, gets `404` (not `403`) from `GET /api/approvals/requests/{id}`, masking the record's existence |
| **Doc/implementation gap: decide permission scoping** | 2 (FAIL expected) | `docs/System/approval-system.md` Section 5 describes `approvals.decide` as "record-scoped... not a blanket grant," but the router grants it as an ordinary blanket role permission and `DecideRequest` rejects a non-assigned holder with a `409` conflict, not a `403` permission error. Assert the actual `409` and flag the doc's "permission" framing as misleading |
| **Doc/implementation gap: undocumented `approvals.access`** | 2 (FAIL expected) | `config/permission.json` defines `approvals.access`, but `approval-system.md` Section 5's permission table lists only `approvals.templates.manage`, `approvals.read.self`, `approvals.decide`, `approvals.read`, `approvals.reassign` — `approvals.access` is absent from the doc and no backend route requires it |
| Sequential step advancement | 3 | In a multi-step template, step 2 is not generated/assigned until step 1 is approved; the request stays `pending` with `current_step_order` incremented and the step 2 approver(s) notified |
| Rejection ends the flow | 3 | Rejecting any step immediately sets the whole request to `rejected` regardless of remaining steps; no further step is generated and no "send back for revision" path exists |
| min_amount step skipping | 3 | A step whose `min_amount` the request's `amount` doesn't meet is recorded `skipped` with no assignee; if every remaining step is skipped the request completes `approved` with zero decisions recorded |
| Self-approval exclusion | 3 | A `specific_user` step whose configured approver equals the requester resolves to no assignee and the request lands in `requires_assignment` instead of auto-assigning the requester |
| Admin reassignment unblocks requires_assignment | 3 | `approvals.reassign` on a `requires_assignment` request assigns approver(s) to the stuck step and flips status back to `pending`; the newly assigned approver can then successfully `decide` it |
| Multi-approver first-decision-wins | 3 | For a `role` or multi-manager (`requester_manager`/`group_manager`) step with several eligible approvers, the first decision completes the step; a second assignee's later decision attempt on the same step returns `409`, never a second success |
| Cancel blocked after prior step decided | 3 | Once step 1 is approved and the request has advanced to step 2, the requester can no longer cancel the request even though its status is still `pending` |
| requester_manager routing | 3 | A `requester_manager` step resolves via group-system.md Section 5's Manager Resolution algorithm exactly as leave approval does — walking up the hierarchy and falling back to an administrator when no manager exists |
| Completion notifications | 3 | Reaching `approved`/`rejected` notifies the requester and every Completion Notification Target whose `notify_on` matches that outcome; a target configured only for the other outcome receives nothing |
| Full lifecycle audit trail | 3 | A completed multi-step request's final `GET` shows every step's `decision`, `decided_by_user_id`, and `decided_at`, with skipped steps correctly marked `skipped` and never carrying a decider |

## Finance

Source: `server/apis/finance.go`, `server/system/finance.go`, `docs/System/finance-system.md`.

| Case | Level | Expected |
| --- | --- | --- |
| Account and period happy path | 1 | Create a finance account and an open accounting period, then list and read both |
| Journal entry happy path | 1 | Create a balanced draft journal entry in an open period, then post it |
| Vendor and AP bill happy path | 1 | Create a vendor, create a draft AP bill for it, then approve the bill |
| Missing auth | 2 | All `/api/finance/*` endpoints return `401` without a valid session |
| Insufficient permission | 2 | GL endpoints reject callers lacking `accounting.gl.read`/`accounting.gl.manage`; AP endpoints reject callers lacking `accounting.ap.read`/`accounting.ap.manage`, both with `403` |
| Invalid account/period input | 2 | Account with blank code/name or unknown `account_type`, or period with a non-`YYYY-MM-DD` date or `end_date` before `start_date`, returns `400` |
| Invalid journal entry input | 2 | Entry with fewer than two lines, a malformed `entry_date`, blank `period_id`/`source_module`, a line carrying both or neither of debit/credit, a non-positive or malformed amount, or an invalid currency code returns `400` |
| Unbalanced journal entry | 2 | Entry whose summed debit lines do not equal summed credit lines returns `400` ("debits must equal credits"), even when every individual line is otherwise valid |
| Unknown period on journal entry | 2 | Creating a journal entry against a nonexistent `period_id` returns `404` |
| Journal entry date outside period range | 2 | Entry dated before the period's `start_date` or after its `end_date` returns `409`, even though the referenced period is open |
| Journal entry against closed period | 2 | Creating a journal entry in a closed period, or posting a draft entry whose period has since been closed, returns `409` |
| Re-post already-posted entry | 2 | Posting a journal entry that is already `posted` (or `reversed`) returns `409` |
| Re-close already-closed period | 2 | Closing a period that is already `closed` returns `409` |
| Unknown journal entry / period IDs | 2 | Reading, posting, or closing a nonexistent ID returns `404` |
| Invalid vendor/AP bill input | 2 | Vendor with a blank name, or bill with a blank vendor ID/bill number, an invalid `bill_date`/`due_date`, a bad currency code, or a non-positive `total_amount`, returns `400` |
| Re-approve already-approved bill | 2 | Approving an AP bill that is already `approved` returns `409` |
| **Doc/implementation gap: dangling FK returns 500** | 2 (FAIL expected) | `account_id`/`vendor_id`/`period_id` are DB-level `REFERENCES` but are not pre-validated in the service layer; a journal line or AP bill referencing a nonexistent account/vendor ID fails at the database foreign-key level and surfaces as `500`, not a handled `400`/`404`. Approving a nonexistent AP bill ID itself does correctly return `404` |
| **Doc/implementation gap: period close skips trial-balance check** | 2 (FAIL expected) | `finance-system.md` Section 3 requires a period's trial balance to balance, with sub-ledgers reconciled and posted, before it can close; `FinanceService.ClosePeriod` only checks the period's own `status == "open"` and performs no balance or posted-entries check, so a period holding draft, unbalanced, or unposted entries closes successfully anyway |
| GL lifecycle across a period close | 3 | A draft entry created in an open period remains postable right up until the period closes; once closed, posting that same still-draft entry fails with `409` while `GET` continues to return it unchanged |
| AP bill approval boundary | 3 | A bill approved once cannot be approved again, and its `journal_entry_id` link (when supplied at creation) persists unchanged through the approval transition |
| Account hierarchy | 3 | A child account created with a valid `parent_account_id` is created and read back with the parent linkage intact |
| **Doc/implementation gap: doc describes itself as pre-implementation** | 3 (FAIL expected) | `finance-system.md` is written and labeled as a pre-implementation design document ("No data model, API, or permission keys are finalized here" — Section 1) describing a future module, yet GL accounts, periods, journal entries, vendors, and AP bills are already implemented with live routes using exactly the permission keys the document lists in its own Section 13 — the document's stated status does not match the shipped implementation |

## CRM

Source: `server/apis/crm.go`, `server/system/crm.go`, `docs/System/crm-system.md`. Largest module (26 routes across 7 sub-resources) — grouped by sub-resource below.

### Customers

| Case | Level | Expected |
| --- | --- | --- |
| Customer CRUD happy path | 1 | Create (`individual`/`b2c` and `organization`/`b2b`), list, read, update customer |
| Points balance/ledger basics | 1 | `GET .../points/balance` and `GET .../points/ledger` return zero balance and an empty ledger for a new customer |
| Missing auth | 2 | Protected customer and points endpoints return `401` |
| Insufficient permission | 2 | A `crm.read`-only user gets `403` on create/update; a user without `crm.read`/`crm.manage` gets `403` on read |
| Invalid customer input | 2 | Missing/invalid `party_type`, `segment`, empty `name`, or invalid `status` returns `400` |
| Invalid JSON | 2 | Malformed request body returns `400` |
| Unknown customer tier on create/update | 2 | Non-existent `tier_id` returns `400` (not `404`) — the tier reference is optional and validated inline as bad input |
| Unknown customer | 2 | Read/update/points-balance/points-ledger on unknown `id` return `404` |
| Invalid list query | 2 | Unknown filter key or bad pagination/sort param on any CRM list endpoint returns `400` (shared pattern across all CRM sub-resources) |
| No delete endpoint | 2 | Customers are deactivated via `status=inactive`, never deleted; there is no `DELETE` route to protect |
| **Doc disagreement: tier assignable regardless of segment** | 2 (FAIL expected) | `crm-system.md` Section 2.2 frames `tier_id` as "for `b2b` pricing," but `CreateCustomer`/`UpdateCustomer` never check `segment` — a `tier_id` can be set on a `b2c` customer too |
| Customer isolation | 3 | Updating one customer's tier/status does not change another customer's record |
| Tier reassignment lifecycle | 3 | Assign a tier via `tier_id`, then clear it with `tier_id: ""`; both changes are reflected on subsequent `GET` |

### Customer Tiers (B2B)

| Case | Level | Expected |
| --- | --- | --- |
| Customer Tier CRUD happy path | 1 | Create, list, read, update a tier, optionally with `default_price_list_id` |
| Missing auth/permission | 2 | Protected endpoints return `401`/`403` per `crm.read`/`crm.manage` |
| Invalid tier input | 2 | Empty `name` or invalid `status` returns `400` |
| Invalid JSON | 2 | Malformed body returns `400` |
| Unknown default price list | 2 | Non-existent `default_price_list_id` returns `400` |
| Unknown tier | 2 | Read/update unknown `id` returns `404` |
| Duplicate tier name allowed | 2 | Creating a second tier with the same `name` succeeds (`201`) — `crm_customer_tiers.name` has no `UNIQUE` constraint, unlike Roles/Groups' name/title uniqueness |
| No delete endpoint | 2 | Tiers are deactivated via `status=inactive`; no deletion to protect |
| Tier assignment lifecycle | 3 | Create a tier, assign it to a customer via `tier_id`, update the tier's `default_price_list_id`, confirm the customer's `tier_id` reference is unaffected by the tier edit |

### Membership Tiers (B2C)

| Case | Level | Expected |
| --- | --- | --- |
| Membership Tier CRUD happy path | 1 | Create, list, read, update a tier |
| Missing auth/permission | 2 | Protected endpoints return `401`/`403` |
| Invalid tier input | 2 | Empty `name` or invalid `status` returns `400` |
| Invalid JSON | 2 | Malformed body returns `400` |
| Unknown default price list | 2 | Non-existent `default_price_list_id` returns `400` |
| Unknown tier | 2 | Read/update unknown `id` returns `404` |
| Duplicate tier name allowed | 2 | Creating a second tier with the same `name` succeeds — no unique constraint on `name` |
| No delete endpoint | 2 | Tiers are deactivated via `status`; no deletion to protect |
| Referenced-tier reuse | 3 | A membership tier referenced by a membership and by a points-earning rule can still be read/updated; deactivating it does not cascade to those dependents (no cascade logic exists) |

### Memberships

| Case | Level | Expected |
| --- | --- | --- |
| Membership CRUD happy path | 1 | Create a membership for an existing customer + membership tier, list, read, update |
| Resolve by member number | 1 | `GET /api/crm/memberships/resolve?member_number=...` returns the matching membership |
| Missing auth/permission | 2 | Protected endpoints return `401`/`403` |
| Invalid membership input | 2 | Missing `customer_id`, `membership_tier_id`, or `joined_at`, or an invalid `status`, returns `400` |
| Invalid JSON | 2 | Malformed body returns `400` |
| Unknown customer or tier on create | 2 | Non-existent `customer_id` or `membership_tier_id` returns `404` (required-reference lookup), unlike the `400` returned for the optional tier references elsewhere |
| Unknown membership | 2 | Read/update unknown `id` returns `404` |
| **Doc/convention disagreement: duplicate member number** | 2 (FAIL expected) | Creating or updating a membership with a `member_number` already in use returns `400 crm_invalid_input`, not `409` — the service maps the DB's `UNIQUE` violation to `ErrCRMInvalid` instead of a conflict, unlike email/title duplicates elsewhere in the system |
| Unresolvable member number | 2 | `resolve` with an unknown `member_number` returns `404`; missing/empty `member_number` returns `400` |
| No date format validation | 2 | `joined_at`/`expires_at` accept any non-empty string — a malformed date (e.g. `"not-a-date"`) is stored as-is and does **not** return `400`, unlike Attendance's date validation |
| Clearing required tier reference | 2 | `PATCH` with `membership_tier_id: ""` looks up tier id `""` and returns `404` (not `400`), since the field is required, not nullable, in the schema |
| No delete endpoint | 2 | Memberships are retired via `status=lapsed`/`cancelled`, never deleted |
| **Doc disagreement: membership allowed for b2b customer** | 2 (FAIL expected) | `crm-system.md` Section 2.3 scopes Memberships to B2C, but `CreateMembership` never checks the customer's `segment` — a membership can be created against a `b2b` customer too |
| Membership + resolve lifecycle | 3 | Create a customer, create a membership with a member number, resolve it, update its tier/status, re-resolve and confirm the change is visible |
| Multiple memberships per customer | 3 | A customer can hold more than one membership record (no uniqueness on `customer_id`); listing with a `customer_id` filter returns all of them |

### Price Lists

| Case | Level | Expected |
| --- | --- | --- |
| Price List CRUD happy path | 1 | Create a list with an ISO currency code, list, read, update |
| Add price list item | 1 | `POST /{id}/items` appends an item and returns the price list with `items` populated |
| Missing auth/permission | 2 | Protected endpoints return `401`/`403` |
| Invalid price list input | 2 | Empty `name` or a malformed `currency` (not a 3-letter ISO code) returns `400` |
| Invalid item input | 2 | Empty `description` or a non-numeric/negative `unit_price` returns `400` |
| Invalid JSON | 2 | Malformed body returns `400` |
| Unknown price list | 2 | Read/update/add-item on unknown `id` returns `404` |
| Duplicate price list name allowed | 2 | Creating a second price list with the same `name` succeeds — no unique constraint on `name` |
| No effective-date range | 2 | Price lists carry no start/end date anywhere in this implementation (nor in Section 2.4); "overlapping date range" conflicts cannot occur and are out of scope for this script |
| No delete endpoint | 2 | Price lists/items are deactivated via `status`; no deletion to protect |
| Price list reuse across tiers | 3 | The same price list can be set as `default_price_list_id` on both a Customer Tier and a Membership Tier simultaneously |
| Adding items touches `updated_at` | 3 | Adding an item bumps the price list's `updated_at` without changing its own `name`/`currency`/`status` |

### Points Earning Rules

| Case | Level | Expected |
| --- | --- | --- |
| Points Earning Rule CRUD happy path | 1 | Create a default (no `membership_tier_id`) rule and a tier-specific rule, list, update |
| Missing auth/permission | 2 | Protected endpoints return `401`/`403` |
| Invalid rule input | 2 | Non-numeric/negative `points_per_currency_unit` or invalid `status` returns `400` |
| Invalid JSON | 2 | Malformed body returns `400` |
| Unknown membership tier | 2 | Non-existent `membership_tier_id` returns `404` |
| Unknown rule | 2 | Update of an unknown `id` returns `404` (there is no single-rule `GET`, only list/create/update) |
| Multiple active rules per tier allowed | 2 | Creating two `active` rules for the same `membership_tier_id` (or two tier-unset default rules) both succeed — no uniqueness/conflict check exists, so rate resolution depends on which row an unordered `LIMIT 1` query returns |
| Rate resolution untestable via this API | 3 | Section 2.5.1's tier-specific-vs-default resolution (`resolveEarningRate`/`EarnPointsForCheckout`) has no direct CRM HTTP endpoint — it only runs inside Checkout. This script can verify rules are stored/listed correctly but cannot verify a purchase resolves the documented rate without exercising Checkout |

### Loyalty Points Ledger

| Case | Level | Expected |
| --- | --- | --- |
| Post ledger entry happy path | 1 | `POST /api/crm/points-ledger` with `entry_type=earned` and a positive `points_delta` returns `201`; balance and ledger reflect it |
| Missing auth/permission | 2 | `401` without auth; `403` for a user holding `crm.read`/`crm.manage` but lacking `crm.points.manage`, which gates this route separately |
| Invalid ledger input | 2 | Missing `customer_id`, `points_delta=0`, or an `entry_type` outside `earned`/`redeemed`/`expired`/`adjustment` returns `400` |
| Invalid JSON | 2 | Malformed body returns `400` |
| Unknown customer | 2 | Posting against a non-existent `customer_id` returns `404` |
| No edit/delete endpoint | 2 | The ledger is append-only — no edit or delete route exists, matching the doc's non-destructive design (Section 2.5) |
| **Doc disagreement: entry type / sign mismatch accepted** | 2 (FAIL expected) | Section 2.5 states `points_delta` is "positive for points earned, negative for points redeemed or expired," but the service only checks `points_delta != 0` and a valid `entry_type` — a negative delta with `entry_type=earned` (or vice versa) is accepted and posted as-is |
| **Doc disagreement: redeeming more than the balance is allowed** | 2 (FAIL expected) | Posting `entry_type=redeemed` with a magnitude larger than the current balance succeeds and drives the balance negative — there is no balance check before insert, despite the doc's framing of redemption as spending down an existing balance |
| Ledger filter by entry_type | 2 | `GET .../points/ledger?entry_type=earned` returns only matching rows |
| Balance equals sum of ledger | 3 | After posting `earned`, `redeemed`, `expired`, and `adjustment` rows, `GET .../points/balance` equals the arithmetic sum of every `points_delta` returned by `GET .../points/ledger` |
| Manual adjustment lifecycle | 3 | An `adjustment` entry with no `source_module`/`source_reference_id` posts successfully and is distinguishable from `checkout`-sourced rows in the ledger |
