# Checkout System

## 1. Purpose

This document collects the design considerations for a future Checkout (point-of-sale) module: a fast, in-person retail transaction — scan items, take payment, done. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

NexGestion serves organizations of very different natures, and **not every organization does B2C retail**. Checkout is therefore one more optional module in the Operations System family (see [`operations-system.md`](./operations-system.md) for the shared Module Independence Principle) — a B2B-only deployment never installs it, and a retail deployment doesn't need Order, Procurement, or Production to use it.

The defining requirement is speed. Unlike [`order-system.md`](./order-system.md), which models an order's lifecycle over time (`draft` → `confirmed` → `fulfilled`), a checkout transaction is created and completed in one continuous, in-person interaction. Every design choice below — the data model, the barcode-scanning default, and the dedicated UI route (Section 3) — optimizes for that.

## 2. Data Model

### 2.1 Checkout Transactions

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable transaction ID |
| `warehouse_id` | TEXT/UUID | Yes | Warehouse/location the sale occurred at, when Inventory is installed (Section 4.1) |
| `cashier_user_id` | TEXT/UUID | Yes | Staff member who operated the checkout |
| `crm_customer_id` | TEXT/UUID | No | Optional link to a CRM Customer (Section 4.5), typically resolved by scanning a membership number |
| `status` | TEXT | Yes | `in_progress`, `completed`, `voided` |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `subtotal_amount` | DECIMAL | Yes | Sum of line amounts (Section 2.2) before any discount |
| `discount_amount` | DECIMAL | Yes | Sum of applied Checkout Discounts (Section 2.3.3); `0` when none apply |
| `total_amount` | DECIMAL | Yes | `subtotal_amount - discount_amount` — the amount actually due, and the amount Checkout Payments (Section 2.4) must sum to for the transaction to complete |
| `completed_at` | DATETIME | No | Set when the transaction is completed |

Splitting the total into `subtotal_amount` and `discount_amount` keeps the arithmetic auditable: the line total, what was taken off, and what's actually owed are three separate numbers rather than one field that silently already has a discount baked in. `payment_method` is no longer a single field here — Section 2.4 replaces it with a proper table, because a real till supports paying with more than one tender in the same transaction.

### 2.2 Checkout Transaction Lines

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `checkout_transaction_id` | TEXT/UUID | Yes | Owning transaction |
| `description` | TEXT | Yes | What was sold, in plain text |
| `inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item — populated automatically when the line was added by a barcode scan (Section 4.4) |
| `quantity` | DECIMAL | Yes | Quantity sold |
| `unit_price` | DECIMAL | Yes | Price per unit — pre-filled from the linked customer's membership tier price list when `crm_customer_id` is set (Section 4.5), same override-not-enforce pattern as `crm-system.md` §2.4 |

Line amounts sum to `subtotal_amount` (Section 2.1); nothing at the line level knows about discounts or tender — those live one level up, transaction-wide.

### 2.3 Discounts: Promotion Rules & Coupons

Two distinct ways a discount reaches a transaction: a **Promotion Rule** applies itself automatically whenever its conditions are met, with no action from the cashier or customer; a **Coupon** must be deliberately redeemed — scanned or keyed in — by a specific code. Both ultimately produce the same thing, a Checkout Discount (Section 2.3.3) that reduces `total_amount`.

#### 2.3.1 Promotion Rules

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable rule ID |
| `name` | TEXT | Yes | Organization-defined name, e.g. "夏季特賣", "會員週五九折" |
| `discount_type` | TEXT | Yes | `percentage`, `fixed_amount` |
| `discount_value` | DECIMAL | Yes | Percentage (e.g. `10` for 10%) or fixed currency amount, per `discount_type` |
| `scope` | TEXT | Yes | `transaction` (applies to `subtotal_amount`) or `item` (applies to matching lines only) |
| `inventory_item_id` | TEXT/UUID | No | When `scope = item`, restricts the rule to one item; unset means every item qualifies |
| `min_subtotal_amount` | DECIMAL | No | Rule only qualifies once `subtotal_amount` reaches this threshold, e.g. "spend $1000, get $100 off" |
| `membership_tier_id` | TEXT/UUID | No | Optional link to a CRM Membership Tier (`crm-system.md` §2.3.1) — restricts the rule to customers of that tier; requires `crm_customer_id` to be set on the transaction (Section 4.5) |
| `starts_at` / `ends_at` | DATETIME | No | Optional validity window |
| `status` | TEXT | Yes | `active`, `inactive` |

A rule with no `membership_tier_id` applies to every customer, member or not — tier restriction is opt-in, not the default. Evaluating which active rules qualify for the current cart happens at checkout time; how conflicting or stacked rules resolve is an open decision (Section 5).

#### 2.3.2 Coupons

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable coupon ID |
| `code` | TEXT | Yes | The redeemable code — typed in, or scanned when printed as a barcode (Section 4.4); unique |
| `coupon_type` | TEXT | Yes | `discount` — reduces `total_amount`, like a Promotion Rule; or `voucher` — a stored monetary value redeemed as a tender line instead (Section 2.4), functioning like a gift certificate |
| `discount_type` | TEXT | No | `percentage`, `fixed_amount`; required when `coupon_type = discount` |
| `discount_value` | DECIMAL | No | Required when `coupon_type = discount` |
| `value_amount` | DECIMAL | No | The redeemable monetary value; required when `coupon_type = voucher` |
| `usage_limit` | INTEGER | No | Maximum number of redemptions across all customers; unset means unlimited |
| `redeemed_count` | INTEGER | Yes | Running count of redemptions so far, checked against `usage_limit` |
| `starts_at` / `ends_at` | DATETIME | No | Optional validity window |
| `status` | TEXT | Yes | `active`, `inactive` |

`coupon_type = discount` (a percentage- or amount-off code) and `coupon_type = voucher` (a fixed-value certificate that pays for part of the purchase, like store credit) are kept as one table because they share every field except how their value is expressed and where it lands — one produces a Checkout Discount line, the other a Checkout Payment line. A voucher-type coupon whose `value_amount` exceeds `total_amount` is a change-making question this document doesn't resolve (Section 5).

#### 2.3.3 Checkout Discounts

The applied instances — what actually got taken off this specific transaction, and why.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `checkout_transaction_id` | TEXT/UUID | Yes | Owning transaction |
| `source_type` | TEXT | Yes | `promotion`, `coupon`, `points` (`crm-system.md` §2.5) |
| `source_reference_id` | TEXT/UUID | Yes | The Promotion Rule (2.3.1), discount-type Coupon (2.3.2), or points redemption ledger row (`crm-system.md` §2.5) applied |
| `amount` | DECIMAL | Yes | The resolved discount amount for this transaction — a `percentage` rule/coupon is evaluated once at apply time and stored as a concrete currency amount here, not recomputed later |

Checkout Discount rows sum to `discount_amount` (Section 2.1). Storing the resolved `amount` rather than re-deriving it from the rule every time means a later edit to a Promotion Rule's percentage never silently changes the discount on a transaction that already applied it — the same non-destructive, snapshot-the-value-at-the-time principle used when an Order line copies `customer_name` (`order-system.md` §2.1) rather than re-reading the live customer record.

### 2.4 Checkout Payments

A transaction may be settled with more than one tender — cash for most of it, a card for the remainder, is the standard "split tender" case any real register supports.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `checkout_transaction_id` | TEXT/UUID | Yes | Owning transaction |
| `method` | TEXT | Yes | `cash`, `card`, `mobile_payment`, `voucher`, `crypto` |
| `amount` | DECIMAL | Yes | Amount tendered by this method |
| `reference` | TEXT | No | Method-specific reference — a card authorization code, a mobile-payment transaction ID, the redeemed voucher-type Coupon's `id` (Section 2.3.2) when `method = voucher`, or a blockchain transaction hash when `method = crypto`. Free-text/nullable here because the actual payment-processor integration behind each method isn't decided (Section 5) |

Checkout Payment rows must sum to exactly `total_amount` (Section 2.1) for a transaction to move `in_progress` → `completed` — that equality is the completion gate, replacing the old single `payment_method` field's implicit "the whole total was paid one way" assumption. `voucher` here is specifically a redeemed voucher-type Coupon (Section 2.3.2); a discount-type coupon never appears in this table, because it isn't tender — it already reduced `total_amount` via Section 2.3.3.

Checkout owns its own database (`checkout.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. UI Architecture: Dedicated Checkout Route

Efficiency is the whole point of this module, so Checkout does not live inside the standard multi-section authenticated app shell used by the rest of NexGestion. This is also the reference case for a platform-wide rule: every screen that uses barcode scanning must be its own dedicated route with its own nav entry, not a step embedded in a larger screen (`barcode-scanning.md` §6) — Checkout just has the tightest speed requirement of any module that rule currently applies to.

- It is reachable at a dedicated top-level route (e.g. `/checkout`), using its own minimal layout — no sidebar, no multi-section navigation chrome — distinct from the shared application shell described in `architecture.md` §4 (`src/layouts`: "shared application shells, sidebar, header, and auth layouts"). That document should gain this as a second, purpose-built layout kind rather than a variant of the standard one.
- A user holding checkout permission should be able to reach `/checkout` immediately after signing in and land on a scan-ready screen — not navigate through a dashboard or settings tree first.
- The scan input auto-refocuses after every line and after completing a transaction, so a cashier can keep scanning back-to-back items, and back-to-back customers, without touching the mouse or keyboard between them.
- The screen defaults to `NexBarcodeScanner`'s hardware mode (`barcode-scanning.md` §5) rather than camera mode: a hardware scanner works over plain HTTP with no permission prompt and no per-scan decode latency (`barcode-scanning.md` §2), which matters when speed is the requirement being designed for. Camera mode stays available as a fallback for a workstation with no scanner attached, but is not the default path here the way it might be for occasional Inbound/Outbound use in Inventory. A printed Coupon code (Section 2.3.2) scans through the same input as an item or a membership number — the resolver (Section 4.4) just needs to know which kind of code it decoded.
- Applicable Promotion Rules (Section 2.3.1) are evaluated and shown automatically as the cart changes — a cashier never manually invokes them. Redeeming a Coupon (Section 2.3.2) is a deliberate step: scan or type the code, see it validated (still active, within its window, under its usage limit) and its effect previewed before it's applied.
- A dedicated **closing screen** is where a transaction moves from `in_progress` to `completed`: it shows `total_amount` after discounts, accepts one or more Checkout Payments (Section 2.4) against it with a running remaining-balance total as each tender is entered, and only allows completion once tendered equals due. When a linked member is present (Section 4.5), the same screen surfaces their points balance and lets a redemption reduce `total_amount` before tendering.
- Voiding an in-progress transaction and starting a new one must be reachable without leaving this route — a cashier should never need to navigate away from `/checkout` mid-shift.

## 4. Relationship to Other Systems

### 4.1 Inventory System

A checkout line may carry an optional `inventory_item_id` linking to [`inventory-system.md`](./inventory-system.md#2-data-model). Completing a transaction (`in_progress` → `completed`) creates an Outbound record (`inventory-system.md` §2.4) at the transaction's `warehouse_id`, already in `shipped` status — a checkout sale hands goods to the customer immediately, so it skips the `pending`/`packed` staging an Order-driven shipment normally goes through. When a line has no `inventory_item_id`, or Inventory is not installed, completing a sale is purely a Checkout-side record with no stock effect — a service-only retail business (e.g. a salon) can use Checkout for payment without ever installing Inventory.

### 4.2 Order, Procurement, and Production Systems

No direct relationship, consistent with the rest of the family (`operations-system.md` §4) — any connection between a checkout sale and purchasing or manufacturing runs only through Inventory's stock levels, never a direct reference between these documents.

### 4.3 Finance & Accounting

A completed checkout transaction is already paid, so — unlike a fulfilled Order (`order-system.md` §3.4) — it never creates a draft Accounts Receivable invoice. There is nothing left to collect. Instead it may post a draft GL journal entry directly (`finance-system.md` §3) when Finance is installed, revenue recognized net of discount:

- one credit line to a revenue account for `subtotal_amount` (Section 2.1);
- one debit line to a contra-revenue/discount account for `discount_amount`, when non-zero;
- one debit line per Checkout Payment row (Section 2.4) to the account matching its `method` — e.g. a Cash account, a Card Clearing account pending settlement, a Mobile Payment Clearing account, a Voucher Liability account (redeeming a voucher-type Coupon settles a liability the organization already recognized when it sold or issued that coupon, not new revenue), and a Crypto Holdings account.

Which account each `method` and discount type maps to is organization-configured against the Chart of Accounts (`finance-system.md` §3), not hardcoded here. This is still the one Operations→Finance linkage in this family that bypasses Accounts Receivable entirely; it remains optional so Checkout still works with Finance uninstalled.

### 4.4 Barcode Scanning

Checkout is the primary reason speed matters for barcode scanning (`barcode-scanning.md` §1). Section 3 above covers the UI-level default; at the data level, a scanned code resolves against three targets in order: an Inventory item (`barcode-scanning.md` §4, adds a line), a CRM membership number (Section 4.5, links the customer), or a Coupon `code` (Section 2.3.2, redeems it) — whichever matches. A code matching none of the three surfaces the same "not recognized" state `barcode-scanning.md` §4 already defines for items.

### 4.5 CRM System

`crm_customer_id` (Section 2.1) is an optional link to [`crm-system.md`](./crm-system.md) §2.1, typically populated by scanning a membership number (`crm-system.md` §2.3) with the same barcode-scanning capability used for items and coupons (Section 4.4) — a membership card is just another scannable code. Checkout remains fully anonymous by default; a walk-in sale with no scanned membership never touches CRM at all, even when CRM is installed. Two effects once a member is linked:

- **Member pricing**: the linked membership's tier and its default price list (`crm-system.md` §2.3.1, §2.4) pre-fill each line's `unit_price` (Section 2.2) the same way a B2B Quote line is pre-filled from a Customer Tier (`order-system.md` §3.3) — a default, not an enforced constraint; the cashier can still override it. A Promotion Rule restricted to a `membership_tier_id` (Section 2.3.1) only qualifies once this link resolves the customer's tier.
- **Points**: completing a transaction with `crm_customer_id` set posts to that customer's Points Ledger (`crm-system.md` §2.5) — earning points on the amount actually paid (`total_amount`, after discount), and optionally redeeming existing points as a further Checkout Discount (Section 2.3.3, `source_type = points`) before tendering. See `crm-system.md` §2.5 for the accrual/redemption model and §3.2 for the full relationship.

## 5. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- cash-drawer/register session management (opening/closing a till, end-of-shift cash reconciliation) — not modeled here;
- returns and refunds after a transaction has completed — only an in-progress `voided` state exists today, not a post-completion reversal, and a refund would need to reverse Checkout Payments, Checkout Discounts, points earned/redeemed, and any posted GL entry together;
- receipt printing/format;
- payment-method detail beyond Section 2.4's bare fields — actual processor integration for card, mobile payment, and crypto (authorization flow, settlement timing, webhook/callback handling) isn't designed; `reference` is a free-text placeholder for whatever each integration eventually needs;
- how conflicting or multiple qualifying Promotion Rules (Section 2.3.1) stack or exclude each other on one transaction — today's model doesn't limit how many can apply at once;
- change-making when a voucher-type Coupon's `value_amount` exceeds `total_amount`, or when cash tendered exceeds the amount due;
- coupon issuance and distribution (how a Coupon in Section 2.3.2 comes to exist and reach a customer — bulk-generated codes, a marketing campaign, a customer-service credit) — this document only covers redemption;
- customer identification at checkout — anonymous by default; an optional `crm_customer_id` for membership lookup is now specified (Section 4.5, `crm-system.md` §3.2), but full customer identification (e.g. for non-member B2C loyalty capture) beyond that isn't decided;
- the exact `client/src/layouts` implementation for the dedicated route in Section 3 — the requirement is documented here, not the component code;
- permission keys, to be defined per the existing catalog convention (`user-system.md` §3.4) once API design starts.
