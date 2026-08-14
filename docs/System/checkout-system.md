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
| `payment_method` | TEXT | No | `cash`, `card`, `other`; required once `status = completed` |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `total_amount` | DECIMAL | Yes | Sum of line amounts |
| `completed_at` | DATETIME | No | Set when the transaction is completed |

| Field (line) | Type | Required | Description |
| --- | --- | --- | --- |
| `checkout_transaction_id` | TEXT/UUID | Yes | Owning transaction |
| `description` | TEXT | Yes | What was sold, in plain text |
| `inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item — populated automatically when the line was added by a barcode scan (Section 4.4) |
| `quantity` | DECIMAL | Yes | Quantity sold |
| `unit_price` | DECIMAL | Yes | Price per unit |

Checkout owns its own database (`checkout.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. UI Architecture: Dedicated Checkout Route

Efficiency is the whole point of this module, so Checkout does not live inside the standard multi-section authenticated app shell used by the rest of NexGestion. This is also the reference case for a platform-wide rule: every screen that uses barcode scanning must be its own dedicated route with its own nav entry, not a step embedded in a larger screen (`barcode-scanning.md` §6) — Checkout just has the tightest speed requirement of any module that rule currently applies to.

- It is reachable at a dedicated top-level route (e.g. `/checkout`), using its own minimal layout — no sidebar, no multi-section navigation chrome — distinct from the shared application shell described in `architecture.md` §4 (`src/layouts`: "shared application shells, sidebar, header, and auth layouts"). That document should gain this as a second, purpose-built layout kind rather than a variant of the standard one.
- A user holding checkout permission should be able to reach `/checkout` immediately after signing in and land on a scan-ready screen — not navigate through a dashboard or settings tree first.
- The scan input auto-refocuses after every line and after completing a transaction, so a cashier can keep scanning back-to-back items, and back-to-back customers, without touching the mouse or keyboard between them.
- The screen defaults to `NexBarcodeScanner`'s hardware mode (`barcode-scanning.md` §5) rather than camera mode: a hardware scanner works over plain HTTP with no permission prompt and no per-scan decode latency (`barcode-scanning.md` §2), which matters when speed is the requirement being designed for. Camera mode stays available as a fallback for a workstation with no scanner attached, but is not the default path here the way it might be for occasional Inbound/Outbound use in Inventory.
- Voiding an in-progress transaction and starting a new one must be reachable without leaving this route — a cashier should never need to navigate away from `/checkout` mid-shift.

## 4. Relationship to Other Systems

### 4.1 Inventory System

A checkout line may carry an optional `inventory_item_id` linking to [`inventory-system.md`](./inventory-system.md#2-data-model). Completing a transaction (`in_progress` → `completed`) creates an Outbound record (`inventory-system.md` §2.4) at the transaction's `warehouse_id`, already in `shipped` status — a checkout sale hands goods to the customer immediately, so it skips the `pending`/`packed` staging an Order-driven shipment normally goes through. When a line has no `inventory_item_id`, or Inventory is not installed, completing a sale is purely a Checkout-side record with no stock effect — a service-only retail business (e.g. a salon) can use Checkout for payment without ever installing Inventory.

### 4.2 Order, Procurement, and Production Systems

No direct relationship, consistent with the rest of the family (`operations-system.md` §4) — any connection between a checkout sale and purchasing or manufacturing runs only through Inventory's stock levels, never a direct reference between these documents.

### 4.3 Finance & Accounting

A completed checkout transaction is already paid, so — unlike a fulfilled Order (`order-system.md` §3.4) — it never creates a draft Accounts Receivable invoice. There is nothing left to collect. Instead it may post a draft GL journal entry directly (`finance-system.md` §3) recognizing revenue against cash/card received, when Finance is installed. This is the one Operations→Finance linkage in this family that bypasses Accounts Receivable entirely; it remains optional so Checkout still works with Finance uninstalled.

### 4.4 Barcode Scanning

Checkout is the primary reason speed matters for barcode scanning (`barcode-scanning.md` §1). Section 3 above covers the UI-level default; at the data level, a scanned code resolves to an item exactly as Inventory's Inbound/Outbound does (`barcode-scanning.md` §4) — a match adds one unit to a new or existing line on the current transaction.

### 4.5 CRM System

`crm_customer_id` (Section 2.1) is an optional link to [`crm-system.md`](./crm-system.md) §2.1, typically populated by scanning a membership number (`crm-system.md` §2.3) with the same barcode-scanning capability used for items (Section 4.4) — a membership card is just another scannable code. When present, the linked membership's tier may inform member pricing for the transaction. Checkout remains fully anonymous by default; a walk-in sale with no scanned membership never touches CRM at all, even when CRM is installed.

## 5. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- cash-drawer/register session management (opening/closing a till, end-of-shift cash reconciliation) — not modeled here;
- returns and refunds after a transaction has completed — only an in-progress `voided` state exists today, not a post-completion reversal;
- receipt printing/format;
- payment-method detail beyond a bare enum (e.g. an actual card-payment integration) — this document only records which method was used, not how it was processed;
- customer identification at checkout — anonymous by default; an optional `crm_customer_id` for membership lookup is now specified (Section 4.5, `crm-system.md` §3.2), but full customer identification (e.g. for non-member B2C loyalty capture) beyond that isn't decided;
- the exact `client/src/layouts` implementation for the dedicated route in Section 3 — the requirement is documented here, not the component code;
- permission keys, to be defined per the existing catalog convention (`user-system.md` §3.4) once API design starts.
