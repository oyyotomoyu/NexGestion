# Finance & Accounting System

## 1. Purpose

This document collects the design considerations for a future Finance & Accounting module, following the same planning-checklist convention as [`salary-system.md`](./salary-system.md) and [`hr-system.md`](./hr-system.md). No data model, API, or permission keys are finalized here; those must be defined in a follow-up design pass once scope is confirmed.

"Finance" and "accounting" are conceptually distinct functions — accounting is the system of record and compliance layer (bookkeeping, statutory filings, audit trail); finance is the planning/analysis layer built on top of it (budgeting, cash forecasting, treasury). This platform deliberately merges both into **one system** rather than two, because separating them only pays off once an organization needs dedicated FP&A tooling doing scenario planning independent of the ledger — a scale this platform is not assuming. Sections 3–9 (General Ledger through Tax) are the accounting core: the system of record that must be accurate and auditable. Sections 10–11 (Budget, Financial Reporting) are the finance layer: they read from the accounting core rather than maintaining a second set of books. If a deployment later outgrows this and needs standalone FP&A tooling, that tool should consume this system's GL as its data source rather than this system being split in two.

NexGestion is a globally distributed platform, not scoped to one country. The same principle already established for payroll in [`salary-system.md` §4](./salary-system.md#4-jurisdiction-and-localization-architecture) applies here: **no statutory tax rule, filing format, or accounting standard may be hardcoded to one jurisdiction.** Anything jurisdiction-specific (Section 9) must be modeled as a pluggable pack from the start, not retrofitted later.

This document exists because the HR System depends on it, not the other way around. [`hr-system.md`](./hr-system.md) already assumes Finance owns final settlement disbursement (via [`salary-system.md` §6.5](./salary-system.md#65-employment-changes)) and will own expense reimbursement (its Section 2, item 10) once this system exists. This document defines enough of Finance and Accounting to make those handoffs concrete and to stand on its own as a system of record.

## 2. Scope Map

| # | Area | Layer | Status |
| --- | --- | --- | --- |
| 1 | General Ledger | Accounting core | This document — Section 3 |
| 2 | Accounts Payable | Accounting core | This document — Section 4 |
| 3 | Accounts Receivable | Accounting core | This document — Section 5, conditional on whether the deployment has external revenue |
| 4 | Expense & Reimbursement | Accounting core | This document — Section 6 |
| 5 | Cash & Bank Management | Accounting core | This document — Section 7 |
| 6 | Fixed Asset Accounting | Accounting core | This document — Section 8 |
| 7 | Tax and Accounting Standards | Accounting core | This document — Section 9 |
| 8 | Budget Management | Finance layer | This document — Section 10 |
| 9 | Financial Reporting | Reads the accounting core; serves both finance analysis and statutory use | This document — Section 11 |
| 10 | Audit & Internal Controls | Cross-cutting | This document — Section 12 |

"Accounting core" sections must be accurate and auditable on their own, independent of anything in the finance layer — the finance layer (Budget, and the analysis side of Reporting) is a read-only view over the core and must never become a second source of truth for a figure the core already owns.

## 3. General Ledger

The General Ledger (GL) is the aggregation core every other module posts into. Nothing in Sections 4–10 is a standalone ledger; each produces journal entries that land here.

- **Chart of Accounts**: a configurable, hierarchical list of accounts (asset, liability, equity, revenue, expense). Must not assume a single country's standard chart — the chart itself should be a per-deployment configuration, with an optional starter template per jurisdiction pack (Section 9).
- **Journal entries**: every financial event (an AP payment, a payroll disbursement, an expense reimbursement, a depreciation run) posts one balanced journal entry (debits = credits). No module should mutate account balances directly; they only ever create journal entries.
- **Accounting periods**: a period (typically monthly) that can be open or closed. Closing a period requires its trial balance — total debits equal total credits across every account — to balance, and should follow a defined close checklist (e.g. AP, AR, and fixed-asset sub-ledgers reconciled and posted before the period locks). Once closed, no journal entry may post into it; a correction requires a new entry in an open period referencing the original, following the same non-destructive-correction principle used in `attendance-system.md` §4.3 and `salary-system.md` §6.6.
- **Accounting basis**: whether the ledger recognizes transactions on a cash or accrual basis must be a per-deployment configuration, not an assumption. Most sub-ledgers in this document (AP, AR, fixed-asset depreciation) only produce meaningful entries under accrual accounting; a cash-basis deployment posts a simplified subset.
- **Accounting standard / reporting framework**: which framework (a local GAAP, IFRS, or another) governs account classification and statement presentation is jurisdiction-dependent and must be modeled as an extension of the jurisdiction pack introduced for tax in Section 9, not a separate hardcoded assumption.
- **Multi-currency / multi-entity**: consistent with `salary-system.md`'s multi-currency requirement, the GL must support transactions in different currencies with configurable rounding, and should not assume one legal entity per deployment — an organization spanning multiple jurisdictions (already anticipated in `salary-system.md` §4) may need consolidated and per-entity views. A foreign-currency transaction also needs a translation/revaluation policy, since the resulting foreign-exchange gain or loss is itself a journal entry (this section), not a display-only rounding artifact.

Recommended shape (design note, not a finalized schema):

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable journal entry ID |
| `entry_date` | DATE | Yes | Accounting date, distinct from `created_at` |
| `period_id` | TEXT/UUID | Yes | Owning accounting period |
| `source_module` | TEXT | Yes | `salary`, `accounts_payable`, `expense`, `fixed_asset`, `manual`, etc. |
| `source_reference_id` | TEXT/UUID | No | ID of the originating record in its owning module |
| `description` | TEXT | No | Free-text memo |
| `status` | TEXT | Yes | `draft`, `posted`, `reversed` |
| `created_by_user_id` | TEXT/UUID | Yes | Who recorded the entry |

| Field (journal line) | Type | Required | Description |
| --- | --- | --- | --- |
| `entry_id` | TEXT/UUID | Yes | Owning journal entry |
| `account_id` | TEXT/UUID | Yes | Chart-of-accounts reference |
| `debit_amount` | DECIMAL | No | Mutually exclusive with `credit_amount` |
| `credit_amount` | DECIMAL | No | Mutually exclusive with `debit_amount` |
| `currency` | TEXT | Yes | ISO 4217 currency code |

## 4. Accounts Payable (AP)

Tracks money the organization owes and pays out — vendor bills today, and payroll disbursement once it hands off from Salary.

- **Vendor/payee records**: name, bank details, tax identifier (format varies by jurisdiction, same caveat as `salary-system.md` §5).
- **Bills/invoices received**: amount, due date, GL account coding, approval status.
- **Payment approval workflow**: draft → approved → disbursed, mirroring the draft-then-approve pattern already established for payroll in `salary-system.md` §6.4 — money movement should never be a single-step action anywhere in this platform.
- **Disbursement batches and bank transfer files**: `salary-system.md` §9 already specifies that payroll needs bank transfer files in bank/country-specific formats. This is the same capability AP needs for vendor payments. **Recommendation: build one bank-transfer-file generation capability in Finance, and have Salary's disbursement (§6.3, §9) call into it as a payroll-typed payment batch, rather than maintaining two separate bank-file generators.** This is the primary integration point named in Section 1.

## 5. Accounts Receivable (AR)

Tracks money owed to the organization — customer invoices and incoming payments.

This module is **conditional**: a deployment that is purely an internal operations platform (no external billing) may not need it at all. Recommend deferring AR's detailed design until a deployment with actual external revenue is scoped, rather than building it speculatively now. If needed later, it mirrors AP's shape (invoice issued → payment received → GL posting) rather than requiring new architecture.

## 6. Expense & Reimbursement

Employee-submitted expense claims — the module `hr-system.md` (Section 2, item 10) explicitly deferred to Finance.

- **Claim submission**: employee submits amount, category, receipt attachment, business justification.
- **Approval workflow**: manager approval, then Finance approval for disbursement — same draft → approve → disburse shape as Section 4.
- **Disbursement**: once approved, becomes a payment either through the AP batch mechanism (Section 4) or a dedicated reimbursement run; either way it posts a GL journal entry (Section 3).
- **Receipt storage**: reuse `template-system.md`'s uploaded-file pattern (checksum, size caps, audience/visibility) rather than inventing a second file-upload mechanism — a receipt is functionally the same kind of object (a user-uploaded file with metadata) as a template upload.
- **Integration with HR**: submission and status should surface through the future HR Employee Self-Service portal (`hr-system.md` Section 2, item 10) as the front door, while Finance remains the system of record and approver of disbursement.

## 7. Cash & Bank Management

- **Bank accounts**: the organization's own accounts (distinct from vendor/employee bank details stored in Sections 4/6), balances, currency.
- **Reconciliation**: matching bank statement lines against GL entries/payment batches; a reconciliation is itself a record (matched vs. unmatched lines), not a destructive edit to either side.
- **Cash flow visibility**: derived from GL + open AP/AR, not a separately maintained figure.

This is the module with the least unique data of its own — most of its value is a view over Sections 3, 4, and 5 plus an external bank statement import.

## 8. Fixed Asset Accounting

This is a **financial view**, distinct from and complementary to HR's asset-management module (`hr-system.md` Section 2, item 10 — "equipment issued to employees, e.g. laptops, ID badges"). The two must not be merged:

| | HR Asset Management | Finance Fixed Asset Accounting |
| --- | --- | --- |
| Answers | Who currently has which physical item? | What is this asset's book value, and how is it depreciated? |
| Trigger | Onboarding issuance / offboarding return (`hr-system.md` §3.4, §3.7) | Purchase, capitalization threshold, disposal |
| Owner | HR module | Finance module |

Recommended relationship: HR's asset record and Finance's fixed-asset record share the same physical asset via a common `asset_id`/tag, but each module owns its own fields — HR owns custodian/location/condition, Finance owns acquisition cost, depreciation method, useful life, and accumulated depreciation. A depreciation run posts a GL journal entry (Section 3) each period; it does not touch HR's custody record.

## 9. Tax and Accounting Standards

Tax and statutory accounting standards are grouped in one section because both are jurisdiction-pack concerns, even though they answer different questions — tax is what the organization owes a government; accounting standards govern how its books are kept and presented. Do not design country-specific rules here; instead, plan to **extend** the same jurisdiction pack concept payroll already uses (`salary-system.md` §4), so both are configured once per jurisdiction rather than duplicated or diverging between Salary and Finance.

### 9.1 Tax

- **Indirect tax** (VAT/GST/sales tax): rate tables, applicability by transaction type, effective-dated like every other statutory rate in `salary-system.md` §4.
- **Withholding tax on non-payroll payments** (e.g. vendor withholding): distinct from payroll withholding (already owned by Salary) but structurally the same kind of rule.
- **Statutory filings**: jurisdiction-specific export adapters, same pattern as `salary-system.md` §4's filing/export-format guidance — one adapter per filing type/authority, not a single generic export.
- **Effective-dated rates**: every tax rate/bracket carries an effective-date range, since these change on government schedules independent of software releases (identical requirement to `salary-system.md` §4's closing bullet).

### 9.2 Accounting Standards

- **Reporting framework**: the jurisdiction pack should also carry which accounting standard (a local GAAP, IFRS, or another framework) applies, since it governs account classification and statement presentation (Section 3, Section 11).
- **Chart-of-accounts starter template**: an optional per-jurisdiction-pack default chart of accounts (Section 3) that an organization may still fully customize — a starting point, not an enforced structure.
- **Accounting basis default**: a jurisdiction pack may also carry the basis (cash/accrual, Section 3) commonly required or expected in that jurisdiction, as a default rather than an enforced rule.

## 10. Budget Management

- **Budgets**: scoped to a department/group (reusing `group-system.md`'s existing organization hierarchy rather than inventing a parallel cost-center structure), a period, and optionally an account or account category.
- **Budget vs. actual**: actual figures are read from posted GL entries (Section 3) tagged with the relevant group/account — budgeting is a planning and comparison layer, not a second bookkeeping system.
- **Approval**: budget creation/revision should follow the same permission-gated, auditable pattern as every other Finance mutation (Section 12).

## 11. Financial Reporting

- **Standard statements**: P&L (income statement), balance sheet, cash flow statement, generated from GL data (Section 3).
- **Export**: reuse the existing `report-files.md` pattern (write to a temp file, checksum, atomic rename into a module subfolder under `report/finance/`) already used by Attendance and planned for Salary, rather than a new export mechanism.
- **Multi-entity/consolidated views**: needed if a deployment spans multiple legal entities (Section 3's multi-entity note).
- **Statement presentation by standard**: the required statements and their line-item presentation vary by accounting standard (Section 9.2) — IFRS and different local GAAPs group items differently. The reporting layer reads presentation rules from the jurisdiction pack rather than hardcoding one framework's layout.

## 12. Audit & Internal Controls

- **Segregation of duties**: the platform's existing role/permission system (`role-system.md`) is the enforcement mechanism — e.g. the user who creates an AP bill should not, by default, hold the permission to approve its payment. This document does not invent a new authorization model; it applies the existing one with Finance-specific permission keys (Section 13).
- **Full change history**: every GL entry, AP/expense approval, and budget revision needs a change history strong enough for external audit, matching the "who changed what, when" standard already set in `salary-system.md` §8.
- **Immutable posted entries**: a posted journal entry is never edited or deleted; corrections are new entries referencing the original (Section 3), the same non-destructive-correction principle used throughout this platform.

## 13. Permissions

Planned permission keys, to be added to `config/permission.json` when these APIs are implemented, following the same catalog convention as other modules (`user-system.md` §3.4).

Being one system does not mean one permission namespace. The accounting core (Section 2's "Accounting core" rows) and the finance layer (Budget) are granted separately, using `accounting.*` and `finance.*` prefixes, so a role can be scoped to one without the other — e.g. an accounting clerk role holding GL/AP/tax permissions with no budget-planning access, or a finance analyst role holding budget and statutory-report read access with no ability to post a journal entry or approve a vendor payment.

### 13.1 Accounting Core Permissions

| Permission | Allows |
| --- | --- |
| `accounting.gl.read` | View journal entries and account balances |
| `accounting.gl.manage` | Post manual journal entries, open/close accounting periods |
| `accounting.ap.read` | View vendor bills and payment batches |
| `accounting.ap.manage` | Create and approve vendor bills for payment |
| `accounting.ap.disburse` | Execute a payment batch / generate a bank transfer file |
| `accounting.expense.read.self` | View the current user's own expense claims |
| `accounting.expense.submit` | Submit an expense claim |
| `accounting.expense.approve` | Approve an expense claim (manager or accounting tier) |
| `accounting.assets.read` | View fixed-asset records and depreciation |
| `accounting.assets.manage` | Record acquisitions, depreciation runs, disposals |
| `accounting.tax.manage` | Configure jurisdiction tax and accounting-standard packs (Section 9) |
| `accounting.reports.read` | View and export statutory financial statements (Section 11) |

### 13.2 Finance Layer Permissions

| Permission | Allows |
| --- | --- |
| `finance.budget.read` | View budgets and budget-vs-actual |
| `finance.budget.manage` | Create and revise budgets |

The initial Admin role automatically receives every accounting and finance permission through `grants_all_permissions`, as with other modules. Every Finance & Accounting API route must declare its permission and follow the existing role-union authorization rule — a route only ever requires one key from one namespace, never a combined check across both.

## 14. Integration Points

- **Salary System**: payroll disbursement should reuse Finance's AP bank-transfer-file capability (Section 4) instead of a second implementation; every settlement run posts a GL journal entry (Section 3).
- **HR System**: expense reimbursement (Section 6) is the module `hr-system.md` deferred to Finance; HR's asset-management module and Finance's fixed-asset accounting (Section 8) share an `asset_id` but remain separately owned.
- **Group System**: budget scoping (Section 10) reuses the existing organization hierarchy rather than a parallel cost-center model.
- **Report Files**: financial statement export (Section 11) reuses the existing `report-files.md` generation pattern.
- **Template System**: expense receipt uploads (Section 6) reuse the existing `template-system.md` file-upload pattern.

## 15. Explicitly Deferred Decisions

The following require separate product decisions before implementation:

- whether Accounts Receivable (Section 5) is in scope at all for a given deployment;
- the finalized chart-of-accounts default/template strategy, and whether it ships per jurisdiction pack or fully custom per organization;
- the schema and scope boundary for a combined "jurisdiction tax and accounting-standard pack" (Section 9), and its exact relationship to `salary-system.md` §4's existing jurisdiction pack — one shared pack per jurisdiction, or two related but separate packs;
- whether Salary's disbursement (`salary-system.md` §6.3, §9) is refactored to call Finance's AP payment-batch capability, or the two remain independent until a later migration;
- multi-entity consolidation rules when one deployment spans several legal entities;
- capitalization threshold and default depreciation methods for Fixed Asset Accounting (Section 8);
- retention period for posted journal entries and financial statements;
- whether Cash & Bank reconciliation (Section 7) requires a bank-statement import format per bank/jurisdiction, similar in spirit to `salary-system.md` §4's filing/export adapters;
- the exact period-end close checklist and which sub-ledger reconciliations gate a period lock (Section 3);
- foreign-exchange revaluation/translation policy and how realized vs. unrealized FX gain or loss is booked (Section 3); and
- whether this system should ever split into separate Accounting (system of record) and Finance (FP&A) tools, and at what organizational scale that would become worth doing (Section 1).
