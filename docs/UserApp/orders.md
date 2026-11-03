# Quotes and orders

Open **Orders** (`/orders`) or **Quotes** (`/quotes`) from the sidebar. You need `orders.access` and `orders.read`; `orders.manage` also enables creation, editing and fulfillment. Administrators have access automatically.

## Quote to order

1. Choose **New quote**, enter the customer name, quote date, currency and line items. Optionally add a validity date and notes.
2. Save the draft. You can edit a draft before sending it.
3. Choose **Send quote**. Sent quotes can be accepted or rejected; expired quotes cannot be accepted.
4. Choose **Accept and create order**. The backend creates one draft order with the quote's customer and price snapshots. **View resulting order** opens it. Retrying acceptance does not create another order.
5. Confirm the resulting order when it is ready for fulfillment.

For a direct sale, **New order** creates a draft without a quote. Customer names can be entered directly, so CRM is optional. Saved CRM references are retained when editing an existing linked document.

## Line items and stock

Every line needs a description, positive quantity and nonnegative unit price. Totals use exact decimal arithmetic and the backend validates them. Quantities and prices accept up to six decimal places.

Users with `inventory.read` can optionally select stock items. Unlinked lines are suitable for services and do not move stock. An existing stock link is preserved when an editor lacks Inventory read permission. Inventory lookup failures do not prevent creating an unlinked order.

## Partial shipments

1. Open a confirmed order and choose **Record shipment**.
2. Enter the date and optional tracking reference or note.
3. For stock-linked lines, select the warehouse and storage locations. Warehouse/location choices are available to order operators without exposing stock balances or the full inventory catalog.
4. Enter each quantity to ship now. A zero leaves the line for a later shipment. Quantities cannot exceed the remaining ordered amount.
5. Record the shipment. This immediately fulfills those quantities and deducts linked stock in one transaction. If stock is insufficient, nothing is shipped or deducted.

The detail page shows ordered, shipped and remaining quantities per line. Partial shipment leaves an order confirmed; shipping all remaining quantities marks it fulfilled. The shipment history includes dates, quantities, tracking references and notes.

If Inventory is absent, fulfillment still works as an order record with no stock effect. The core workflow supports services without warehouses or other installed business modules.

## Editing, cancellation and history

Drafts can be replaced; sent quotes and confirmed orders are no longer editable. Unshipped orders can be cancelled. After any shipment, cancellation is blocked; returns/reversals are outside this workflow.

Every change records the actor and action in paginated history. List screens support search, status filters, sorting and pagination. Shipment history has separate pagination.

The app sends the document's last-update token when editing, changing status or shipping. If another operator changes the record first, the backend rejects the outdated request. Reload the latest record and reapply your change. Shipment retries reuse the same request reference to prevent duplicate deductions; after an uncertain network response, reload and check shipment history before attempting a new shipment.

## Running with backend data

Production builds use the real API. For development, run the Go server and start the client with `VITE_USE_BACKEND=true pnpm dev`. Sign in with a real account. These screens intentionally do not use in-memory order fixtures.

Finance invoicing, automatic CRM price-list defaults, returns, quote revisions and advanced fulfillment approvals remain separate work.
