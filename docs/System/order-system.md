# Order System

## 1. Purpose

This document collects the design considerations for a future Order module, covering its four core functions: **customers**, **quotes**, **orders**, and **shipment**. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

Order is one of four modules in the Operations System family — see [`operations-system.md`](./operations-system.md) for the shared Module Independence Principle and a cross-module overview. Order needs no other module installed to function: a service business can run it with plain-text line items and no stock, inventory, or finance linkage at all.

## 2. Data Model

### 2.1 Customer Reference

Order does not own customer identity itself — that moved to its own module, [`crm-system.md`](./crm-system.md) §2.1, once it became clear customer relationships needed B2B/B2C tiering and membership handling beyond what Order should own (see `crm-system.md` §1 for why CRM is separate).

Every place below that references "a customer" (Sections 2.2, 2.3) uses the same optional-link-plus-snapshot pattern already established for vendors (`procurement-system.md` §2.1–2.2) and inventory items: a `customer_id` field that optionally links to a CRM Customer when CRM is installed, alongside a required `customer_name` snapshot so the record never loses its customer even without a formal CRM record, without CRM installed at all, or if the linked CRM record is later edited or removed.

### 2.2 Quotes

A quote is a price proposal that hasn't committed to anything yet — it never touches Inventory or Finance (Section 3). It only becomes real demand once accepted and converted into an order.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable quote ID |
| `customer_id` | TEXT/UUID | No | Optional link to a CRM Customer (Section 2.1, `crm-system.md` §2.1) |
| `customer_name` | TEXT | Yes | Copied from the linked customer at creation time if `customer_id` is set, or entered directly otherwise |
| `quote_date` | DATE | Yes | Date the quote was issued |
| `valid_until` | DATE | No | Date the quote expires; once passed without acceptance the quote is treated as `expired` |
| `status` | TEXT | Yes | `draft`, `sent`, `accepted`, `rejected`, `expired` |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `total_amount` | DECIMAL | Yes | Sum of line amounts |
| `resulting_order_id` | TEXT/UUID | No | The order created when this quote was accepted (Section 2.3) |

| Field (line) | Type | Required | Description |
| --- | --- | --- | --- |
| `quote_id` | TEXT/UUID | Yes | Owning quote |
| `description` | TEXT | Yes | What was quoted, in plain text |
| `inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item |
| `quantity` | DECIMAL | Yes | Quantity quoted |
| `unit_price` | DECIMAL | Yes | Price per unit |

Accepting a quote (`status` → `accepted`) atomically creates an order (Section 2.3) with lines copied from the quote, and sets `resulting_order_id`. There is no separate, later "convert" step — a quote is either not yet accepted, or accepted and already an order.

### 2.3 Orders

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable order ID |
| `customer_id` | TEXT/UUID | No | Optional link to a CRM Customer (Section 2.1, `crm-system.md` §2.1) |
| `customer_name` | TEXT | Yes | Copied from the linked customer at creation time if `customer_id` is set, or entered directly otherwise. Always populated so an order's customer is never lost if the customer record is later edited or removed |
| `source_quote_id` | TEXT/UUID | No | The quote this order was created from (Section 2.2), when applicable |
| `order_date` | DATE | Yes | Date the order was placed |
| `status` | TEXT | Yes | `draft`, `confirmed`, `fulfilled`, `cancelled` |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `total_amount` | DECIMAL | Yes | Sum of line amounts |

| Field (line) | Type | Required | Description |
| --- | --- | --- | --- |
| `order_id` | TEXT/UUID | Yes | Owning order |
| `description` | TEXT | Yes | What was ordered, in plain text |
| `inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item; absent when Inventory is not installed or the line isn't a stocked item |
| `quantity` | DECIMAL | Yes | Quantity ordered |
| `unit_price` | DECIMAL | Yes | Price per unit |

Order owns its own database (`order.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. Relationship to Other Systems

### 3.1 Inventory System

An order line may carry an optional `inventory_item_id` linking to [`inventory-system.md`](./inventory-system.md#2-data-model). When present and Inventory is installed, fulfilling the order is expressed as one or more Outbound (shipment) records against it in Inventory (`inventory-system.md` §2.4, §3.1) — supporting partial shipment rather than a single all-or-nothing deduction. When the reference is absent, or Inventory is not installed, fulfillment is purely a status change with no stock effect.

A quote (Section 2.2) never posts to Inventory, even when its lines carry `inventory_item_id` — nothing is reserved or deducted until it's accepted and becomes an order.

### 3.2 Procurement and Production Systems

Order has no direct link to [`procurement-system.md`](./procurement-system.md) or [`production-system.md`](./production-system.md). Any relationship between customer demand and manufacturing or purchasing runs through Inventory's stock levels, not through a direct reference between these documents.

### 3.3 CRM System

`customer_id` on a Quote or an Order (Section 2.1) is an optional link to [`crm-system.md`](./crm-system.md) §2.1. When installed, CRM also supplies the customer's tier and default price list (`crm-system.md` §2.2, §2.4), which may pre-fill a quote's line pricing — the line's own `unit_price` remains editable regardless. Without CRM installed, Order works exactly as it always has, on `customer_name` alone.

### 3.4 Finance & Accounting

A confirmed or fulfilled order may create a draft Accounts Receivable invoice (`finance-system.md` §5), when the Finance module and its AR component are installed — never a direct GL write, following the draft-then-post pattern used throughout Finance (`finance-system.md` §4, §8.1).

**Note:** `finance-system.md` §5 currently marks Accounts Receivable as fully conditional/deferred, reasoning that a deployment with no external billing may not need it. Enabling Order in a deployment that sells to customers changes that: it will need AR to turn a fulfilled order into an actual invoice and payment record. This document does not change `finance-system.md`'s status for AR; it is flagged as an open decision (Section 4) to revisit together once both modules are implemented.

A quote (Section 2.2) never creates a Finance document either — there's nothing to bill for until it becomes an order.

## 4. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- backorder and partial-fulfillment handling;
- quote versioning/revisions — today an edit just changes the quote in place; whether a sent quote should instead be immutable once sent, with a new revision superseding it, isn't decided;
- whether an accepted quote's line prices are protected from later catalog price changes before the resulting order is fulfilled — this also depends on how `crm-system.md` §4 resolves tier/price-list override rules;
- automatic expiration of quotes past `valid_until` — whether this is a scheduled job or a derived read at query time, matching how Inventory derives on-hand quantity rather than storing it (`inventory-system.md` §2.6);
- revisiting `finance-system.md` §5's Accounts Receivable status now that Order can depend on it (Section 3.4);
- permission keys, to be defined per the existing catalog convention (`user-system.md` §3.4) once API design starts.
