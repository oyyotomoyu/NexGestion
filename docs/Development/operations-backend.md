# Operations backend

Order, Procurement and Production now have backend services, authenticated APIs, audit events and separate SQLite databases: `order.db`, `procurement.db`, `production.db`. Startup initializes them through `RequiredDatabases`. Order and Quote screens are available at `/orders` and `/quotes`; see [user workflow](../UserApp/orders.md). Procurement and Production screens remain separate work.

## Independence and inventory

Each module runs without Inventory, CRM or Finance. Plain descriptions and required customer/vendor name snapshots support standalone use. Optional `inventory_item_id` references have no cross-database foreign key. A linked CRM customer can supply an omitted `customer_name` when CRM is present; once saved, the name snapshot is retained. Vendor IDs resolve to active vendors in Procurement's own database.

If `inventory.db` exists, the service attaches it to its SQLite connection before opening the transaction. Accepted receipts add stock, shipped order lines remove stock, completed production runs consume materials and add finished goods, and completed checkout sales create shipped Outbound records. Unlinked lines have no stock effect. When Inventory is absent, operations finish without creating its database. An existing but broken inventory database is an error rather than a reason to silently skip stock.

Business records and stock changes commit or roll back together using SQLite attached-database transactions and the default rollback journals. Keep the default rollback journal mode for multi-file crash atomicity; this implementation does not enable WAL. Linked stock transactions validate item, active warehouse, location ownership and available stock. An inventory failure leaves the source record and stock unchanged. Every new write uses an immediate transaction to serialize competing transitions. Production consumes materials before posting finished goods, so output cannot fund its own material shortage.

Inventory provenance records include `source_module` and `source_reference_id`. Receipts point to the goods receipt; order Outbound records point to the order and retain `order_line_id` per stock line; checkout points to its transaction. Production posts `production_input`/`production_output` ledger movements with an internal provenance record referencing its production order, without a warehouse Inbound/Outbound operation.

## Authentication and permissions

All endpoints require a bearer access token. Read APIs require `orders.read`, `procurement.read` or `production.read`; mutations require the corresponding `.manage` permission. Module `.access` permissions continue to control frontend entry. Protected administrators have all catalog permissions. Mutations record the authenticated actor in database audit events and request logs.

Lists use the shared List API Query Standard: `page`, `page_size` (maximum 100), `keyword`, `sort`, `order`, optional `status` and `parent_id`. Sorts: `created_at`, `updated_at`, `status`, `name`, `total_amount`. Responses include the named array, `pagination`, `sort`, `keyword`. Parent filters select a PO's receipts/invoices or an order's shipments. Quotes past `valid_until` are derived as expired on reads and cannot be sent or accepted. The end date itself is inclusive, using the server UTC business date.

Amounts and quantities are decimal strings with at most six decimal places. Quantities must fit Inventory's signed 64-bit millionth-unit representation. Commercial quantities and production consumption/output are positive; prices may be zero. Totals are calculated with exact rational arithmetic, then rounded to the platform's six decimal places. Currency is an uppercase three-letter code. Dates are `YYYY-MM-DD`; timestamps are UTC.

Errors: 400 invalid input/query, 401 unauthenticated, 403 missing permission, 404 missing record, 409 invalid transition, duplicate reference/request, over-receipt, over-shipment or insufficient stock.

## Commercial documents

| Resource | Collection path | Array key |
| --- | --- | --- |
| Quotes | `/api/quotes` | `quotes` |
| Orders | `/api/orders` | `orders` |
| Vendors | `/api/procurement/vendors` | `vendors` |
| Purchase orders | `/api/procurement/purchase-orders` | `purchase_orders` |
| Production orders | `/api/production/orders` | `production_orders` |

Each supports GET collection, POST collection, GET `/{id}`, PUT `/{id}`. PUT replaces a draft (or planned production order), rather than patching individual fields. Vendor PUT can update name/contact/status at any time. IDs, actor, timestamps, totals and initial statuses are server controlled. Draft line IDs are regenerated on replacement; use IDs from the latest saved version for receipts and shipments.

Except vendors, documents support `PATCH /{id}/status` with `{ "status": "confirmed" }` (using a valid target status for that resource).

Every resource, including receipts, shipments and invoices below, supports `GET /{id}/events`: paginated, append-only audit history, sortable by `occurred_at` or `action`, keyword matching action/actor. It uses the `events` response array and does not accept status/parent filters.

Quote or order example:

```json
{
  "customer_name": "Customer Ltd",
  "customer_id": "optional-crm-id",
  "quote_date": "2026-10-19",
  "valid_until": "2026-11-19",
  "currency": "TWD",
  "lines": [{ "description": "Widget", "inventory_item_id": "optional-item-id", "quantity": "4", "unit_price": "12.5" }]
}
```

For a direct order, use `order_date` in place of `quote_date`/`valid_until`.

Quote transitions: draft → sent → accepted/rejected. Acceptance creates a draft order with copied lines/prices and `source_quote_id`, and saves `resulting_order_id` on the quote in the same transaction. Retrying acceptance returns the same quote/order link. Quotes have no stock effect.

Order transitions: draft → confirmed/cancelled; confirmed → cancelled when nothing has shipped. Fulfillment occurs through shipment APIs, not a direct status PATCH. Accepted-quote orders begin in draft and require confirmation. Confirmed orders cannot be edited. Partial shipment leaves the order confirmed; shipping every line changes it to fulfilled atomically.

### Order shipment

`POST /api/orders/{id}/shipments`:

```json
{
  "request_key": "shipment-unique-client-reference",
  "warehouse_id": "default",
  "shipped_date": "2026-10-19",
  "tracking_reference": "carrier-reference",
  "lines": [{ "order_line_id": "saved-order-line-id", "location_id": "", "quantity": "2" }]
}
```

Shipments represent goods/services actually fulfilled and are immediately posted. Quantities cannot exceed the remaining ordered quantity, including duplicate lines in one request. For nonstock service lines, shipping records fulfillment without touching inventory.

`POST /api/orders/{id}/fulfill` with `{ "warehouse_id": "default" }` ships all remaining quantities in one transaction. Optional `location_id` supplies the stock location for all automatically generated lines. Use explicit partial shipment lines when different items need different locations. GET `/api/order-shipments` and `/{id}` read shipments.

## Procurement

Vendor fields: `name`, optional `contact_email`, `contact_phone`, `payment_terms`, `tax_identifier`, `status` (`active` default or `inactive`).

Purchase orders use `vendor_name` or `vendor_id`, `order_date`, `currency` and commercial `lines` as above. Vendor IDs snapshot the vendor name. Transitions: draft → approved/cancelled; approved → sent/cancelled; sent → closed/cancelled. Closing requires full accepted receipt. Cancellation is forbidden once a receipt or invoice exists. Receipt completeness is derived from accepted quantities, not a mutable received status.

### Goods receipt

`POST /api/procurement/purchase-orders/{id}/receipts`, against a sent PO:

```json
{
  "request_key": "delivery-unique-client-reference",
  "warehouse_id": "default",
  "received_date": "2026-10-19",
  "note": "One unit damaged",
  "lines": [{
    "purchase_order_line_id": "saved-po-line-id",
    "quantity_received": "3",
    "quantity_accepted": "2",
    "quantity_rejected": "1",
    "rejection_reason": "Damaged",
    "location_id": ""
  }]
}
```

Received must equal accepted plus rejected; rejection requires a reason. The authenticated recorder is the receiver and inspector in this single-step acceptance workflow. Partial receipts are supported; cumulative accepted quantities may not exceed the order. Only accepted, inventory-linked quantities add stock. GET `/api/procurement/goods-receipts` and `/{id}` read records.

### Vendor invoice

`POST /api/procurement/purchase-orders/{id}/invoices` against a sent/closed PO:

```json
{ "vendor_invoice_number": "INV-100", "invoice_date": "2026-10-19", "invoice_amount": "50", "currency": "TWD" }
```

The invoice number is unique within the PO. Matching is at the whole-PO level: every invoice remains pending until cumulative billed amount equals PO total and all items are accepted; mismatched currencies or excess cumulative billing produce discrepancy. Every invoice on the PO then shares that match status. Matching is recalculated when invoices or receipts are recorded. This supports staged billing while deliberately leaving line-level invoice matching deferred. GET `/api/procurement/vendor-invoices` and `/{id}` read records. No AP bill/payment or GL posting is created automatically.

## Production

POST `/api/production/orders`:

```json
{
  "output_description": "Assembly",
  "output_inventory_item_id": "optional-finished-item-id",
  "output_quantity": "1",
  "warehouse_id": "default",
  "location_id": "",
  "planned_date": "2026-10-20",
  "materials": [{ "description": "Raw material", "inventory_item_id": "optional-raw-item-id", "location_id": "", "quantity_used": "2" }]
}
```

Transitions: planned → in_progress/cancelled; in_progress → completed/cancelled. Completion sets `completed_at` and posts linked material/output movements atomically. A retry or concurrent second completion cannot post stock twice. Production is intentionally flat: no multilevel BOM, routings, work centers or costing.

## Checkout changes

Completion now deducts linked stock atomically with the sale's completed status. Insufficient stock keeps the sale in progress. Voiding an in-progress cart does not move stock. Repeating completion is rejected. Checkout totals and payments now sum decimal amounts exactly without SQLite REAL casts.

The existing scan API now resolves Inventory barcode/SKU first, CRM membership number second, coupon third. It returns `type: item` plus `item`, `type: membership` plus `membership`, the existing coupon result, or `not_recognized`. Lookup does not mutate a cart, price items, redeem coupons or activate promotions automatically.

## Retry behavior and remaining scope

Supply a unique `request_key` for receipts and shipments, scoped to the owning module database (up to 200 characters). A reused key returns 409 and rolls back all changes. Quote acceptance is idempotent; other repeated final transitions return 409. Without a request key, submitting a second partial receipt/shipment is a distinct business event, subject to remaining-quantity checks.

Remaining scope includes frontend screens for Procurement and Production, separate receiving/inspection approval stages, price-list defaults, finance AP/AR/valuation integration, returns and reversals, line-level invoice matching, and advanced production planning. Checkout's pre-existing loyalty-points award remains a separate CRM transaction after sale completion; failures there do not roll back a completed sale. Checkout promotion evaluation and coupon redemption rules remain separate existing work.

Verification: `go test ./...` in `server`; `pnpm build` in `client` for the permission-catalog UI change. Tests cover absent optional modules, quote acceptance/expiry, receipts/rejections/duplicate references, invoice matching, partial shipments, rollback, concurrent production completion, checkout stock/scan integration and API authentication/permissions/query validation.


## Order UI additions

`GET /api/orders` and `GET /api/orders/{id}` include derived `fulfilled_quantity` and `remaining_quantity` on every order line. They sum the order's shipment records and do not maintain a second mutable quantity counter. Draft replacements discard client-supplied derived quantities.

PUT document requests may include `expected_updated_at`; status PATCH requests accept it alongside `status`; shipment and fulfill inputs accept it as well. The token is checked inside the same transaction as the mutation. A mismatch returns 409 with `code: operations_stale_document`. Omitting the token preserves the existing API behavior. The Order/Quote UI always supplies it. Updated timestamps advance monotonically per document, including when the injected clock has not advanced. Retried quote acceptance without a token returns the stored timestamp and order reference unchanged.

`GET /api/orders/{id}/shipping-options` requires `orders.manage` and returns `inventory_available`, active `warehouses` and their active `locations`. It validates the order exists and exposes no stock balances or item catalog. Missing Inventory returns false and empty arrays. Fulfillment still revalidates all references and available stock in the write transaction.

The UI uses a stable `request_key` for each shipment form, paginated order/quote lists, paginated shipments/audit events, and exact BigInt decimal totals. Run `pnpm test:orders` in `client` for the decimal edge-case checks.
