# Inventory System

## 1. Purpose

This document collects the design considerations for a future Inventory module, covering its four core functions: **warehouses**, **stock**, **outbound (shipping)**, and **inbound (receiving)**. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

Inventory is one of four modules in the Operations System family — see [`operations-system.md`](./operations-system.md) for the shared Module Independence Principle and a cross-module overview. Inventory has no required dependency on any other module: a single default warehouse plus manual inbound/outbound/adjustment records alone make it a usable standalone stock tracker.

Inventory owns the physical **execution** side of receiving and shipping — which warehouse, which items, which quantities actually moved. The upstream *business* decision that triggers that execution belongs to whichever module requested it: [`procurement-system.md`](./procurement-system.md) owns the decision that a vendor delivery is accepted (Goods Receipt, `procurement-system.md` §2.3); [`order-system.md`](./order-system.md) owns the decision that a customer order is being fulfilled. Inventory's Inbound and Outbound records (Sections 2.3, 2.4) are the warehouse-side counterpart to those decisions, not a duplicate of them.

## 2. Data Model

### 2.1 Warehouses

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable warehouse ID |
| `name` | TEXT | Yes | Warehouse/location name |
| `address` | TEXT | No | Optional physical address |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

A deployment with only one physical location still creates exactly one warehouse row — there is no special "no warehouse" mode, so every stock movement always has an unambiguous location.

Not every organization has only one warehouse, and not every warehouse is a single undifferentiated space. A retail business might run a storefront display, a backroom stockroom, and one or more offsite warehouses as separate rows here; a factory might keep the same item split across several warehouses, each further divided into zones. **Storage Locations** are how a zone within a warehouse is represented, entirely user-defined — the system imposes no fixed taxonomy of location kinds:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable location ID |
| `warehouse_id` | TEXT/UUID | Yes | Owning warehouse |
| `name` | TEXT | Yes | Location name, freely defined by the organization — e.g. "Display Floor", "Backroom", "Zone A" |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

A location always belongs to exactly one warehouse — there is no cross-warehouse location and no further nesting below a location (Section 4). Defining locations is entirely optional: a deployment that doesn't need zone-level tracking simply never creates any, and every Inbound/Outbound/Transfer line and stock movement (Sections 2.3–2.6) that would otherwise reference one just leaves it unset.

### 2.2 Items

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable item ID |
| `sku` | TEXT | Yes | Unique stock-keeping code |
| `name` | TEXT | Yes | Item name |
| `unit_of_measure` | TEXT | Yes | e.g. `piece`, `kg`, `box` |
| `barcode` | TEXT | No | Scannable value printed on the item or its label, when different from `sku`; nullable but unique when set |

A scan resolves against `barcode` first, then falls back to `sku`, so both a vendor-printed barcode and an internally generated SKU label work. See [`barcode-scanning.md`](./barcode-scanning.md) for the scanning capability itself — item lookup by code is the only piece of that design that lives here.

### 2.3 Inbound (入庫)

Records goods actually being received into a warehouse. A line can be added either by picking an item manually or by scanning its barcode (`barcode-scanning.md` §4) — each successful scan adds one unit to a new or existing line for the matched item. Per `barcode-scanning.md` §6, the Inbound recording screen must be its own dedicated route with its own nav entry, not a step embedded in another screen.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable inbound record ID |
| `warehouse_id` | TEXT/UUID | Yes | Warehouse the goods entered |
| `source_module` | TEXT | No | `procurement`, `production`, or `NULL` for a manual inbound (initial stock load, found inventory) |
| `source_reference_id` | TEXT/UUID | No | The triggering record — a goods receipt (`procurement-system.md` §2.3) or production order — when `source_module` is set |
| `received_date` | DATE | Yes | Date goods entered the warehouse |
| `received_by_user_id` | TEXT/UUID | Yes | Who recorded the inbound |
| `note` | TEXT | No | Free-text note |

| Field (line) | Type | Required | Description |
| --- | --- | --- | --- |
| `inbound_id` | TEXT/UUID | Yes | Owning inbound record |
| `item_id` | TEXT/UUID | Yes | Item received |
| `location_id` | TEXT/UUID | No | Storage location (Section 2.1) the item was put away into, when the warehouse uses zone-level tracking; must belong to the record's `warehouse_id` |
| `quantity` | DECIMAL | Yes | Quantity received into stock |

A manual inbound (no `source_module`) is how Inventory works standalone — an organization can record starting stock or a found item without Procurement installed.

### 2.4 Outbound / Shipment (出貨)

Records goods actually leaving a warehouse. As with Inbound, a line can be added by scanning the item's barcode instead of picking it manually (`barcode-scanning.md` §4), and this screen is likewise required to be its own dedicated route (`barcode-scanning.md` §6).

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable outbound record ID |
| `warehouse_id` | TEXT/UUID | Yes | Warehouse the goods left |
| `source_module` | TEXT | No | `order`, or `NULL` for a manual outbound |
| `source_reference_id` | TEXT/UUID | No | The order being fulfilled, when `source_module = order` |
| `shipped_date` | DATE | Yes | Date goods left the warehouse |
| `shipped_by_user_id` | TEXT/UUID | Yes | Who recorded the outbound |
| `tracking_reference` | TEXT | No | Carrier tracking number or delivery reference |
| `status` | TEXT | Yes | `pending`, `packed`, `shipped`, `cancelled` |
| `note` | TEXT | No | Free-text note |

| Field (line) | Type | Required | Description |
| --- | --- | --- | --- |
| `outbound_id` | TEXT/UUID | Yes | Owning outbound record |
| `item_id` | TEXT/UUID | Yes | Item shipped |
| `location_id` | TEXT/UUID | No | Storage location (Section 2.1) the item was picked from, when the warehouse uses zone-level tracking; must belong to the record's `warehouse_id` |
| `quantity` | DECIMAL | Yes | Quantity shipped |
| `order_line_id` | TEXT/UUID | No | The order line this shipment line fulfills, when applicable |

One order can be covered by multiple outbound records over time (partial shipment), the same pattern Procurement uses for partial deliveries against one PO (`procurement-system.md` §2.3).

### 2.5 Stock Transfers

Moves stock between two warehouses, or between two locations within the same warehouse (re-zoning). This is Inventory's own operation — it has no upstream trigger from another module.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable transfer ID |
| `item_id` | TEXT/UUID | Yes | Item being moved |
| `from_warehouse_id` | TEXT/UUID | Yes | Source warehouse |
| `from_location_id` | TEXT/UUID | No | Source location (Section 2.1) within `from_warehouse_id`, when used |
| `to_warehouse_id` | TEXT/UUID | Yes | Destination warehouse |
| `to_location_id` | TEXT/UUID | No | Destination location (Section 2.1) within `to_warehouse_id`, when used |
| `quantity` | DECIMAL | Yes | Quantity moved |
| `transferred_by_user_id` | TEXT/UUID | Yes | Who recorded the transfer |
| `transferred_at` | DATETIME | Yes | UTC timestamp |

The source and destination must differ: either the warehouses differ, or — when it's the same warehouse — the locations differ, so a transfer always actually moves something.

### 2.6 Stock Movements

The underlying, append-only ledger. Every Inbound line, Outbound line, Stock Transfer, and manual adjustment posts one or more rows here; nothing else changes stock.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable movement ID |
| `item_id` | TEXT/UUID | Yes | Item affected |
| `warehouse_id` | TEXT/UUID | Yes | Warehouse affected |
| `location_id` | TEXT/UUID | No | Storage location (Section 2.1) within `warehouse_id`, when the source record carried one |
| `quantity_delta` | DECIMAL | Yes | Positive for stock in, negative for stock out |
| `movement_type` | TEXT | Yes | `receipt`, `issue`, `transfer_in`, `transfer_out`, `production_input`, `production_output`, `adjustment` |
| `source_type` | TEXT | No | `inbound`, `outbound`, `transfer`, `production`, or `NULL` for a direct manual adjustment |
| `source_reference_id` | TEXT/UUID | No | ID of the Section 2.3/2.4/2.5 record, or the production order, that generated this movement |
| `occurred_at` | DATETIME | Yes | UTC timestamp |

`production_input`/`production_output` movements are posted directly by a completed production order (`production-system.md` §3.1) — Production already has its own header record for what was consumed and produced, so no separate Inbound/Outbound record wraps it here. `adjustment` remains a single-line, headerless entry for manual corrections and stock counts.

Current on-hand quantity is always derived from the sum of movements, never a separately maintained mutable counter, so it can always be reconciled against its history — the same non-destructive, append-only principle used for `attendance_sessions` (`attendance-system.md` §4.2) and journal entries (`finance-system.md` §3). The aggregation level depends on what's asked: filtering by `item_id` and `warehouse_id` gives the warehouse-wide total regardless of location; adding `location_id` to the filter gives the quantity in one specific zone. A deployment that never sets `location_id` simply always gets the warehouse-wide answer.

Inventory owns its own database (`inventory.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. Relationship to Other Systems

### 3.1 Order System

An [`order-system.md`](./order-system.md) order line may carry an optional `inventory_item_id`. Fulfilling an order is expressed here as one or more Outbound records (Section 2.4) against that order — supporting partial shipment — rather than a single bare stock deduction. When an order line has no `inventory_item_id`, or Inventory is not installed, fulfillment is purely an Order-side status change with no Inventory record at all.

### 3.2 Procurement System

A [`procurement-system.md`](./procurement-system.md) purchase order line may carry an optional `inventory_item_id`. Recording a goods receipt line (`procurement-system.md` §2.3) with an accepted quantity creates or adds to an Inbound record here (Section 2.3), referencing the goods receipt via `source_module = procurement`. When the line isn't linked, or Inventory is not installed, the goods receipt is just a Procurement-side record with no stock effect.

### 3.3 Production System

[`production-system.md`](./production-system.md) production orders may reference Inventory items for both consumed materials and finished output. Completing a production order posts `production_input` and `production_output` movements directly (Section 2.6) — Production's own record already carries the detail Inbound/Outbound would otherwise provide, so no separate Section 2.3/2.4 record is created for it.

### 3.4 Checkout System

A [`checkout-system.md`](./checkout-system.md) transaction line may carry an optional `inventory_item_id`, the same as an Order line. Completing a checkout transaction creates an Outbound record here (Section 2.4) already in `shipped` status — a retail sale hands goods to the customer immediately, so it skips the `pending`/`packed` staging an Order-driven shipment normally goes through (`checkout-system.md` §4.1). When a line has no `inventory_item_id`, or Inventory is not installed, completing a sale is purely a Checkout-side record with no stock effect.

### 3.5 Finance & Accounting

Stock movements (Section 2.6) may optionally post a draft GL journal entry if the organization values inventory as a balance-sheet asset (`finance-system.md` §3), following the same draft-then-post pattern used throughout Finance. This is an open decision (Section 4) — Inventory functions fully as a quantity ledger with zero GL linkage until valuation is decided.

## 4. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- inventory valuation method (FIFO, weighted average, or none) and whether/when stock movements post a GL journal entry (Section 3.4) — this decision also gates Production's WIP-costing linkage (`production-system.md` §3.4);
- whether Inbound/Outbound need their own pick/pack/QC sub-workflow beyond the single `status` field in Section 2.4, or whether that belongs to a future dedicated warehouse-operations module;
- batch/cycle stock-count support, versus today's single-line manual `adjustment`;
- transfer approval workflow (Section 2.5) for organizations that want sign-off before moving stock between warehouses;
- whether Storage Locations (Section 2.1) ever need a second level of nesting (e.g. zone → shelf → bin) — today a location belongs directly to a warehouse with no further subdivision;
- serial/lot/batch tracking;
- permission keys, to be defined per the existing catalog convention (`user-system.md` §3.4) once API design starts.
