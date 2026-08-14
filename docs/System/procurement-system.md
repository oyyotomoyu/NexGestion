# Procurement System

## 1. Purpose

This document collects the design considerations for a future Procurement module, covering its four core functions: **vendors**, **purchase orders**, **goods receipt**, and **vendor billing**. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

Procurement is one of four modules in the Operations System family — see [`operations-system.md`](./operations-system.md) for the shared Module Independence Principle and a cross-module overview. Procurement functions standalone: a business can track purchase orders with plain-text vendor names and no stock or finance linkage at all, and can adopt the vendor master, goods receipt, and billing pieces incrementally rather than all at once.

## 2. Data Model

### 2.1 Vendors

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable vendor ID |
| `name` | TEXT | Yes | Vendor name |
| `contact_email` | TEXT | No | |
| `contact_phone` | TEXT | No | |
| `payment_terms` | TEXT | No | Free text (e.g. "Net 30") — terms vary too widely to enumerate |
| `tax_identifier` | TEXT | No | Format varies by jurisdiction, same caveat as `salary-system.md` §5 |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

A formal vendor record is optional — see Section 2.2's `vendor_id`/`vendor_name` pair.

### 2.2 Purchase Orders

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable purchase order ID |
| `vendor_id` | TEXT/UUID | No | Optional link to Section 2.1; absent when the organization doesn't maintain a vendor record for this vendor |
| `vendor_name` | TEXT | Yes | Vendor name — copied from the linked vendor at creation time if `vendor_id` is set, or entered directly otherwise. Always populated so a PO's vendor is never lost if the vendor record is later edited or removed |
| `order_date` | DATE | Yes | Date the purchase order was placed |
| `status` | TEXT | Yes | `draft`, `approved`, `sent`, `closed`, `cancelled` |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `total_amount` | DECIMAL | Yes | Sum of line amounts |

| Field (line) | Type | Required | Description |
| --- | --- | --- | --- |
| `purchase_order_id` | TEXT/UUID | Yes | Owning purchase order |
| `description` | TEXT | Yes | What was purchased, in plain text |
| `inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item |
| `quantity` | DECIMAL | Yes | Quantity ordered |
| `unit_price` | DECIMAL | Yes | Price per unit |

`status` no longer includes `received` — whether a PO has been received or billed is derived from Sections 2.3 and 2.4, not tracked as a manually set PO field, the same append-only-ledger-over-mutable-flag principle already used for Inventory's on-hand quantity (`inventory-system.md` §2).

### 2.3 Goods Receipt

Records an actual delivery event, separate from the purchase order itself, so partial deliveries and quantity/quality discrepancies have somewhere to live.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable goods receipt ID |
| `purchase_order_id` | TEXT/UUID | Yes | PO this delivery is against |
| `received_date` | DATE | Yes | Date the delivery arrived |
| `received_by_user_id` | TEXT/UUID | Yes | Who recorded the delivery |
| `inspected_by_user_id` | TEXT/UUID | No | Who signed off on acceptance, when inspection is a separate step from receiving |
| `inspected_at` | DATETIME | No | Sign-off time |
| `note` | TEXT | No | Free-text note, e.g. reason for a discrepancy |

| Field (line) | Type | Required | Description |
| --- | --- | --- | --- |
| `goods_receipt_id` | TEXT/UUID | Yes | Owning goods receipt |
| `purchase_order_line_id` | TEXT/UUID | Yes | PO line this delivery line fulfills |
| `quantity_received` | DECIMAL | Yes | Quantity physically delivered |
| `quantity_accepted` | DECIMAL | Yes | Quantity that passed inspection |
| `quantity_rejected` | DECIMAL | No | Quantity rejected, if any |
| `rejection_reason` | TEXT | No | Required when `quantity_rejected` is greater than zero |

A PO line can be covered by multiple goods receipts over time (partial deliveries). A PO is considered fully received once the sum of `quantity_accepted` across its goods receipts meets the ordered quantity on every line — this is a derived read, not a stored status.

### 2.4 Vendor Billing

Records that the vendor has billed the organization for a PO, and whether that bill matches what was ordered and received — the operational "three-way match" step that precedes an actual payable.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable vendor invoice record ID |
| `purchase_order_id` | TEXT/UUID | Yes | PO being billed against |
| `vendor_invoice_number` | TEXT | Yes | The vendor's own invoice/reference number |
| `invoice_date` | DATE | Yes | Date on the vendor's invoice |
| `invoice_amount` | DECIMAL | Yes | Billed amount |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `match_status` | TEXT | Yes | `pending`, `matched`, `discrepancy` — comparing `invoice_amount` against the PO's `total_amount` and its accepted goods receipts |
| `finance_reference_id` | TEXT/UUID | No | Optional link to the resulting draft AP bill in Finance (Section 3.4), once created |

Procurement records that a bill exists and whether it matches; it does not decide when or how the organization pays it — that decision and the resulting money movement belong entirely to Finance (Section 3.4).

Procurement owns its own database (`procurement.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. Relationship to Other Systems

### 3.1 Inventory System

A purchase order line may carry an optional `inventory_item_id` linking to [`inventory-system.md`](./inventory-system.md#2-data-model). Recording a goods receipt line (Section 2.3) with `quantity_accepted` greater than zero creates or adds to an Inbound record in Inventory (`inventory-system.md` §2.3, §3.2) for the linked item, when both modules are installed and the line is linked; otherwise a goods receipt is just a Procurement-side record that the delivery happened, with no stock effect.

### 3.2 Production System

Procurement has no direct reference to [`production-system.md`](./production-system.md). A production order's consumed materials are drawn from Inventory's stock levels — which Procurement may have contributed to via goods receipts — not from a direct link between these two documents.

### 3.3 Order System

No direct relationship. See [`order-system.md`](./order-system.md) §3.2.

### 3.4 Finance & Accounting

A vendor invoice recorded with `match_status = matched` (Section 2.4) may create a draft Accounts Payable bill (`finance-system.md` §4), when Finance is installed — never a direct GL write, following the draft-then-post pattern used throughout Finance (`finance-system.md` §4, §8.1). Billing, not mere delivery, is the trigger: goods arriving (Section 2.3) makes stock available, but a payable only exists once the vendor has actually billed for it. This is the most directly necessary Operations→Finance link in this family, but it remains optional so Procurement still works with Finance uninstalled — an organization can track vendors, POs, receipts, and bills entirely for its own operational visibility with no Finance module present.

## 4. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- whether vendors (Section 2.1) and CRM's customers (`crm-system.md` §2.1) should eventually unify into one shared contact/party concept, or remain separately owned as they are now;
- vendor-invoice matching is currently header-level (Section 2.4); whether line-level three-way matching (PO line vs. goods receipt line vs. invoice line) is worth the added complexity;
- vendor performance tracking (on-time delivery, rejection rate) — out of scope for this core design;
- retention period for closed purchase orders, goods receipts, and vendor invoices;
- permission keys, to be defined per the existing catalog convention (`user-system.md` §3.4) once API design starts.
