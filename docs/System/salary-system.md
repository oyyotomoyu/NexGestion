# Salary System

## 1. Purpose

This document collects the design considerations for a future Salary (Payroll) module. It is a planning checklist, not an implementation spec. No data model, API, or permission keys are finalized here; those must be defined in a follow-up design pass once scope is confirmed.

NexGestion is a globally distributed open-source platform, not a product scoped to a single country. Payroll and labor law vary significantly by jurisdiction, so **no statutory rule, rate, form, or workflow step may be hardcoded to one country's law**. Every legal/regulatory behavior described below must be modeled as jurisdiction-specific configuration that can be added, versioned, and swapped without changing core calculation code.

The Salary System is expected to integrate with the existing Attendance System (see [`attendance-system.md`](./attendance-system.md)) for worked hours, overtime, and leave, and with the User, Organization, and Group systems for employee and organizational data.

## 2. Compensation Basis

Every employee is assigned exactly **one** compensation basis at a time, together with the pay rate for that unit (e.g. an hourly rate, a daily rate, a monthly salary amount). An employee is paid on hourly terms *or* monthly terms *or* project terms, never several bases simultaneously.

### 2.1 Compensation Basis Values

| Basis | Rate unit | Description |
| --- | --- | --- |
| `hourly` | per hour | Pay = hours worked × hourly rate, sourced from Attendance |
| `daily` | per day | Pay = days worked × daily rate |
| `weekly` | per week | Fixed weekly rate |
| `monthly` | per month | Fixed monthly salary; converting to a daily-equivalent rate for proration uses the applicable jurisdiction pack's formula (see Section 4) |
| `annual` | per year | Salary expressed as an annual figure; actual disbursement still follows the pay-cycle configuration in Section 6 rather than a single yearly payment |
| `piece_rate` | per unit produced | Pay = units completed × rate per unit; must support a minimum-wage top-up rule where the jurisdiction requires it |
| `project_based` | per project/milestone | Fixed fee tied to a defined deliverable, optionally split into milestone payments |
| `contract` | per contract term | Fixed periodic fee for availability/service (retainer); typically outside standard payroll withholding — see the worker-classification note in Section 4 |

`weekly`, `biweekly`, and `semimonthly` schedules described in Section 6 are **pay-cycle** settings, not compensation-basis values. An employee on a `monthly` basis can still be disbursed on a semimonthly schedule. Compensation basis defines how the rate is quoted; pay cycle defines how often money moves.

### 2.2 Data Ownership

Per [`user-system.md`](./user-system.md#L69) (Section 3.2), salary data must not be added to `employee_profiles` — it belongs to a dedicated module. Compensation basis and rate are Salary System data: they live in the salary module's own database and reference the immutable `user_id`, never duplicated into `user.db`.

### 2.3 Rate History

A rate is not a single mutable value on the employee record. Raises, basis changes (e.g. moving from hourly to monthly on promotion), and contract renewals must be recorded as new effective-dated entries, keeping prior rates intact for historical payroll runs and audit. At any given date, exactly one compensation-basis record is active per employee.

Recommended shape (design note, not a finalized schema):

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable record ID |
| `user_id` | TEXT/UUID | Yes | Reference to `users.id` in UserSystem |
| `compensation_basis` | TEXT | Yes | One of the values in Section 2.1 |
| `rate_amount` | DECIMAL | Yes | Amount per the basis's rate unit |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `jurisdiction_id` | TEXT/UUID | Yes | Applicable jurisdiction pack, per Section 4 |
| `effective_start_date` | DATE | Yes | First date this rate applies |
| `effective_end_date` | DATE | No | Last date this rate applies; null means currently active |
| `created_at` | DATETIME | Yes | Creation time in UTC |

Constraints:

- For a given `user_id`, effective date ranges must not overlap.
- At most one record per `user_id` may have a null `effective_end_date` (the current basis).
- Changing an employee's basis or rate creates a new record and closes the previous one; it never overwrites history.

## 3. Salary Structure

- **Base pay and allowances**: base salary, position allowance, meal allowance, transportation allowance, full-attendance bonus, and which of these are fixed versus conditional.
- **Overtime pay**: multipliers that differ by day type (weekday, rest day, statutory holiday) and by jurisdiction; night-shift allowance. Applies primarily to `hourly` and `daily` bases; salaried (`monthly`/`annual`) bases may be exempt depending on jurisdiction.
- **Statutory extra payments**: many jurisdictions mandate payments with no equivalent elsewhere — e.g. 13th/14th-month pay (common across Latin America, the Philippines, and parts of Europe), Eid/festival bonuses, or profit-sharing. These must be optional, jurisdiction-scoped structure items, not universal fields.
- **Bonuses and commissions**: year-end bonus, performance bonus, sales commission, and whether these run on a separate calculation cycle from the regular pay defined by the employee's compensation basis.
- **Deductions**: lateness/early-leave deductions, unpaid leave, salary advances or loan repayments.

## 4. Jurisdiction and Localization Architecture

Because the platform targets any country, statutory and regulatory behavior must be built as a **pluggable "jurisdiction pack"** rather than embedded logic. Each pack encapsulates everything specific to one country or sub-national region:

- **Contribution schemes**: the number, structure, and naming of mandatory social/health/pension schemes differ completely by country (e.g. Taiwan's three separate labor insurance, health insurance, and labor pension schemes vs. a single combined payroll tax elsewhere). Model this as an arbitrary list of named schemes per jurisdiction, each with its own employer/employee rate, contribution cap or floor, and effective-date range — not a fixed set of fields.
- **Income tax withholding**: some jurisdictions require periodic withholding plus a mandatory annual reconciliation filing; others withhold in near real time and need no annual filing for standard employees. This must be an optional step per jurisdiction, not an assumed universal flow.
- **Minimum wage**: not universal — several countries (e.g. the Nordic states) have no statutory minimum wage and rely on collective bargaining instead. Minimum-wage validation must be an opt-in rule per jurisdiction, and where present, must support sub-national variation (state/province/city minimums) and apply correctly across every compensation basis in Section 2 (e.g. piece-rate top-up, hourly floor, prorated monthly floor).
- **Worker classification**: whether a given working relationship is even subject to payroll law (employee vs. independent contractor vs. other statuses) is defined differently by each jurisdiction and changes which rules apply at all. The `contract` compensation basis (Section 2.1) is the most likely to fall outside standard payroll withholding, but this must be confirmed per jurisdiction, not assumed.
- **Pay frequency norms**: monthly is not universal — biweekly and semimonthly cycles are standard in some countries. Pay-cycle configuration must not assume monthly as the only supported cadence.
- **Currency and rounding**: multi-currency support is a core requirement, including per-currency rounding and minor-unit rules, not an optional extra.
- **Statutory payslip content**: the legally required fields on a payslip differ by jurisdiction; the payslip template must be configurable per jurisdiction rather than fixed.
- **Filing/export formats**: government submission formats (tax authority, social-insurance bureau, statistics agencies) are jurisdiction-specific and numerous; these should be implemented as separate export adapters, not a single generic export.
- **Effective-dated rates**: every statutory rate, bracket, or table must carry an effective-date range, since these change on government schedules (often annually) independent of software releases and old records must still calculate correctly for the period they belonged to.

A single organization or deployment may span multiple jurisdictions (e.g. a company with staff in more than one country), so an employee's applicable jurisdiction pack must be assignable per employee or per employment record, not fixed once per installation.

## 5. Integration with Other Modules

- **Attendance / Leave**: automatically import overtime hours, leave days, and unpaid-leave days from the Attendance System, expressed in units the jurisdiction pack can interpret (e.g. overtime multipliers by day type). Primarily relevant to `hourly` and `daily` compensation bases.
- **Employee data**: bank account, national/tax identifier (format varies by country), hire and termination dates (affects prorated calculation for partial periods).
- **Organization / Team**: different departments, job levels, or legal entities may have different salary rules, jurisdictions, or currencies simultaneously.
- **General Affairs**: a settled group-meal-order poll (`general-affairs-system.md` §2.6) may produce per-employee Salary Deduction Requests (`general-affairs-system.md` §2.6.1) as one instance of the "salary advances or loan repayments" ad-hoc deduction category (Section 3) — General Affairs only ever produces the request; whether and how a settlement run here actually consumes and applies it is unresolved (Section 12).

## 6. Settlement (Payroll Run) Mechanism

Settlement is the process that turns attendance data and compensation basis into an actual calculated amount for a period, then produces both a per-employee and an organization-wide output.

### 6.1 Attendance as the Calculation Input

The settlement engine reads attendance status from the existing Attendance System (see [`attendance-system.md`](./attendance-system.md)) rather than duplicating it. It is the primary input for `hourly` and `daily` compensation bases, and a secondary input (unpaid leave, absence deductions) even for `monthly`/`annual` bases.

`attendance_monthly_reports` (attendance-system.md Section 4.4) is a **monthly-only** aggregate. A settlement period that is not month-aligned (daily or weekly settlement — see Section 6.2) cannot rely on that aggregate alone and must be able to read day-level `attendance_days`/`attendance_sessions` data directly for the exact settlement window. This dependency should be confirmed with the Attendance System's read API before implementation.

### 6.2 Settlement Frequency Rules

Each employee's settlement frequency is configured per employee (or defaulted per compensation basis), independent of but constrained by their compensation basis (Section 2):

- The settlement period must not be **finer** than the compensation basis's own rate unit — e.g. a `monthly` basis cannot be settled weekly, because there is no defined sub-monthly rate to slice without inventing a proration rule.
- A basis with a finer natural unit may be **aggregated up** to a coarser settlement period — e.g. a `daily` basis supports daily, weekly, or monthly settlement by summing daily earnings across the period.
- A configurable maximum settlement interval should exist (commonly capped at one month), since many jurisdictions legally require wages to be paid at least once a month. This cap belongs in the jurisdiction pack (Section 4), not hardcoded.

| Compensation basis | Natural unit | Allowed settlement frequencies (example) |
| --- | --- | --- |
| `hourly` | hour | daily, weekly, biweekly, semimonthly, monthly |
| `daily` | day | daily, weekly, biweekly, semimonthly, monthly |
| `weekly` | week | weekly, biweekly, monthly |
| `monthly` | month | monthly only |
| `annual` | year | monthly, or per the configured pay cycle — never disbursed as a single annual payment |
| `piece_rate` | unit produced | tied to completion, or aggregated on the same daily/weekly/monthly options as `daily` |
| `project_based` | project/milestone | tied to milestone completion, not a calendar frequency |
| `contract` | contract term | per contract terms, not a calendar frequency |

This table is illustrative. The authoritative allowed-frequency rule per basis should be encoded as configuration rather than hardcoded branches, so a jurisdiction pack or an individual organization can adjust it (e.g. a country that legally mandates weekly pay for daily-rate workers).

### 6.3 Settlement Run Outputs

Each completed settlement run for a period produces two levels of output, mirroring the pattern already used by the Attendance monthly report (attendance-system.md Sections 4.4–4.5):

1. **Per-employee settlement detail** — one record per employee per settlement period containing the full calculation breakdown (base pay, overtime, allowances, bonuses, statutory contributions, tax withholding, deductions, net pay) and the attendance data window it was derived from. This is the source for that employee's payslip (Section 9).
2. **Organization-wide settlement batch** — one export per settlement run aggregating every included employee's totals for that run, generated the same way Attendance produces its monthly CSV: written to a temporary file, checksummed, and atomically finalized. A failed generation must not replace the previous valid batch.

Because employees can be on different settlement frequencies at the same time (e.g. some `monthly`, some `weekly`), an organization-wide settlement run is scoped to "every employee whose settlement period closes on this date," not to a single calendar boundary shared by the whole organization.

### 6.4 Approval Flow

Draft calculation → review/approval by manager or finance → disbursement, rather than direct payout. Because settlement periods close on different dates per employee frequency (Section 6.2), approval is scoped per settlement run, not a single organization-wide event.

### 6.5 Employment Changes

Prorated final-period pay for terminated employees, severance pay (rules vary widely by jurisdiction), payout of unused leave where legally required.

### 6.6 Recalculation and Corrections

How an error is recalculated after the fact, with a recorded correction history rather than overwriting the original record. A correction must reference the specific settlement record it corrects, similar to the Attendance System's correction events, rather than silently reissuing a new run.

### 6.7 Automated Execution

Configuring settlement (compensation basis, settlement frequency, jurisdiction assignment — Sections 2 and 6.2) is a high-privilege action, gated by a dedicated permission (Section 7). By convention this is expected to sit with an HR-manager-equivalent role, but the platform enforces it purely through the permission system, not a hardcoded job title — the deploying organization decides which role(s) actually hold that permission, consistent with the "role controls what a user may do" principle in [`user-system.md`](./user-system.md) Section 5.

Once configured, no further manual action should be needed to produce a settlement:

- a scheduled job, analogous to the Attendance System's operational jobs ([`attendance-system.md`](./attendance-system.md) Section 11), triggers a settlement run automatically for every employee whose settlement period closes on that date (Sections 6.2–6.3), without HR manually starting each run;
- automation covers calculation through the draft ready for review (the first step of Section 6.4); the approval gate before disbursement remains, since removing human review of money movement is a financial-control decision, not HR busywork — an organization may optionally configure auto-approval under a defined policy threshold, but that must be an explicit opt-in, not the default; and
- a failed or missed scheduled run must not fail silently — it should be retried and/or surfaced to whoever holds the settlement-configuration/manage permission, and the server should detect and safely catch up missed runs on startup, mirroring the Attendance System's reconciliation behavior.

The goal is that HR effort is required only to configure the rules once (and to review/approve exceptions or corrections), not to operate the calculation itself.

## 7. Permissions

These permission keys are planned and must be added to `config/permission.json` when the Salary APIs are implemented, following the same catalog convention as other modules ([`user-system.md`](./user-system.md) Section 3.4).

| Permission | Allows |
| --- | --- |
| `salary.read.self` | View the current user's own settlement details and payslips |
| `salary.read` | View settlement details for other employees |
| `salary.settlement.configure` | Configure compensation basis, settlement frequency, and jurisdiction assignment (Sections 2, 4, 6.2) — high-privilege, typically HR-manager-equivalent |
| `salary.settlement.approve` | Approve a settlement run before disbursement (Section 6.4) |
| `salary.settlement.manage` | Manually trigger, override, or recalculate a settlement run, including corrections (Section 6.6) |
| `salary.reports.read` | View organization-wide settlement batches and reports (Section 6.3, item 2) |
| `salary.export` | Download bank transfer files and statutory export formats (Section 9) |

The initial Admin role automatically receives every salary permission through `grants_all_permissions`, as with other modules. Every Salary API route must declare its permission and follow the existing role-union authorization rule: access passes when any assigned role grants the required permission.

## 8. Security and Audit

- Salary data is highly sensitive and requires **stricter access control** than most other modules (who can view whose salary; visibility boundaries between HR, finance, and managers).
- Full change history for every adjustment (who changed what, and when), including compensation-basis and rate changes (Section 2.3).
- Given the local-first deployment model, encrypted storage must be considered for PII such as bank account numbers and national identifiers.
- Data-residency and privacy obligations differ by jurisdiction (e.g. GDPR in the EU, and equivalent laws elsewhere); since a self-hosted deployment may itself be located in a different jurisdiction than its employees, this needs explicit design attention rather than an assumption that local hosting satisfies every jurisdiction's requirements.

## 9. Output and External Reporting

- **Payslips**: PDF generation using the jurisdiction-specific template, self-service viewing/download by employees, localized language and date/number formatting.
- **Reports**: export formats for accounting/bookkeeping and for statutory filings, implemented as jurisdiction-specific adapters (see Section 4).
- **Bank transfer files**: batch payment files in the format required by the receiving bank; formats vary by country and by bank.

## 10. Individual Salary Notification

When a per-employee settlement detail (Section 6.3, item 1) becomes available to that employee, the system notifies them through two channels simultaneously: an in-app notification and an email to their registered address.

### 10.1 Trigger Timing

Notification must fire only after a settlement run has passed approval (Section 6.4), never at draft/calculation stage, so an employee is never notified of a figure that management has not yet confirmed. This also lets a correction (Section 6.6) happen before disclosure instead of requiring a retraction notice.

### 10.2 Content Scope

The notification content is the net-pay summary derived from the per-employee settlement detail: gross pay (base + bonuses + allowances) minus statutory contributions/tax and other deductions, equal to net pay, for the settlement period.

Salary data requires stricter access control than most modules (Section 8). The existing Notification System (see [`notification-system.md`](./notification-system.md)) stores message content in a shared `notification.db` readable by anyone with `notifications.manage`, and an email, once sent, sits in an inbox entirely outside the platform's access control. Putting exact payroll figures into either channel's stored/transmitted body would undermine that stricter-access principle. This needs a product decision between two options:

- **Pointer-only** (recommended default): the notification/email states that a new payslip is available for a given period and links to the permission-scoped payslip endpoint; figures are retrieved only through authenticated, authorized access.
- **Figures included**: the notification/email embeds the full breakdown. This may be acceptable for a fully self-hosted, single-organization deployment where the operator explicitly accepts the tradeoff, but should be a configuration toggle, not the default, given the platform's general-purpose/global distribution.

### 10.3 Delivery Channels

- **In-app**: reuse the existing Notification System's `notifications` + `notification_audiences` (user-scoped audience) + `notification_deliveries` tables; likely the `info` type, or a new dedicated type if salary notices need distinct handling or their own permission gate.
- **Email**: NexGestion has no documented email-delivery mechanism today — [`login.md`](./login.md) and the rest of the auth flow are in-app only. Sending payslip email notices requires a new capability (outbound SMTP or a transactional-email integration, configurable per self-hosted installation) that does not yet exist in the platform. This should be scoped as its own piece of work, including sender identity, retry/failure handling, and localized templates matching the employee's `locale` (`user-system.md`, `users.locale`).

### 10.4 Failure Handling

Email delivery can fail (bad address, SMTP outage) independently of the in-app notification. A failed email must not block or roll back the settlement run itself; it should be retried and/or surfaced to an administrator, while the in-app notification remains a reliable fallback since it does not depend on external delivery.

## 11. Other Considerations

- Translation/localization of all UI, payslips, and notifications, consistent with the rest of the platform's multi-language requirement.
- Simulation/what-if mode to preview the impact of a raise or basis change before applying it.

## 12. Explicitly Deferred Decisions

The following require separate product decisions before implementation:

- which jurisdiction packs to build and ship first, and the process for the community to contribute new ones;
- the schema for a "jurisdiction pack" (contribution schemes, tax rules, minimum-wage rules, payslip template, export adapters) referenced throughout Section 4;
- the finalized schema and table boundaries for compensation-basis records (Section 2.3), including which module database owns them;
- how an employee or employment record is assigned to a jurisdiction, and how multi-jurisdiction organizations are modeled;
- approval workflow depth (single approver vs. multi-level);
- the auto-approval opt-in policy referenced in Section 6.7, and what threshold/conditions an organization could configure;
- payslip and report file retention period, and whether retention rules themselves are jurisdiction-specific;
- integration scope with Attendance System overtime/leave data;
- whether the Attendance System exposes a day-level read API suitable for non-monthly settlement windows (Section 6.1), or whether Salary needs its own aggregation path;
- pointer-only vs. figures-included content for salary notifications (Section 10.2), and whether it is configurable per organization;
- the email-delivery mechanism for the platform (Section 10.3) — outbound SMTP configuration, sender identity, and retry policy — since none exists today;
- the schema and consumption mechanism for ad-hoc, non-recurring deductions (Section 3's "salary advances or loan repayments") — including how a settlement run picks up an external request such as General Affairs' Salary Deduction Requests (`general-affairs-system.md` §2.6.1, §3.8) and marks it applied; and
- encryption and data-residency approach for sensitive PII fields.
