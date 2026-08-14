# Production System

## 1. Purpose

This document collects the design considerations for a future Production module: the core record of a manufacturing/assembly run — what was produced, from what, and whether it's done. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

Production is one of four modules in the Operations System family — see [`operations-system.md`](./operations-system.md) for the shared Module Independence Principle and a cross-module overview — and the one most likely to be entirely absent from a deployment, since it only applies to manufacturers or light assemblers. It is kept intentionally minimal: no routing, no multi-level bill of materials, no work-center scheduling.

## 2. Data Model

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable production order ID |
| `output_description` | TEXT | Yes | What is being produced, in plain text |
| `output_inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item for the finished good |
| `output_quantity` | DECIMAL | Yes | Quantity to produce |
| `status` | TEXT | Yes | `planned`, `in_progress`, `completed`, `cancelled` |
| `planned_date` | DATE | No | Target completion date |
| `completed_at` | DATETIME | No | Actual completion timestamp |

| Field (material used) | Type | Required | Description |
| --- | --- | --- | --- |
| `production_order_id` | TEXT/UUID | Yes | Owning production order |
| `description` | TEXT | Yes | Input material, in plain text |
| `inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item |
| `quantity_used` | DECIMAL | Yes | Quantity consumed |

Production owns its own database (`production.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. Relationship to Other Systems

### 3.1 Inventory System

A production order's `output_inventory_item_id` and each material line's `inventory_item_id` optionally link to [`inventory-system.md`](./inventory-system.md#2-data-model). Completing a production order (`in_progress` → `completed`) posts a `production_input` movement (negative) per linked material and a `production_output` movement (positive) for the linked finished good (`inventory-system.md` §3.3), when both modules are installed. Without Inventory, completion is just a record that the run happened, with no stock effect.

### 3.2 Procurement System

No direct reference. Raw materials a production order consumes may have originated from a [`procurement-system.md`](./procurement-system.md) purchase order, but that relationship is expressed only through shared Inventory stock levels, not a direct link between Production and Procurement records.

### 3.3 Order System

No direct reference. A finished good produced here may later fulfill an [`order-system.md`](./order-system.md) order, again only through Inventory's stock levels, not a direct link between Production and Order records.

### 3.4 Finance & Accounting

A completed production order may post a draft GL entry moving value from raw-material inventory to finished-goods inventory (WIP costing) (`finance-system.md` §3), when Finance is installed and inventory valuation is enabled (`inventory-system.md` §4). This is the most speculative Finance linkage in the Operations family and is deferred until inventory valuation itself is decided.

## 4. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- whether Production needs a real bill-of-materials (multi-level, with routing/operations) beyond the flat material list in Section 2;
- production costing (standard cost vs. actual cost) and its GL treatment, dependent on `inventory-system.md` §4's valuation decision;
- permission keys, to be defined per the existing catalog convention (`user-system.md` §3.4) once API design starts.
