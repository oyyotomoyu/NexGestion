# Operations System

## 1. Purpose

This document is the index for five independently documented operational modules:

- [`order-system.md`](./order-system.md)
- [`inventory-system.md`](./inventory-system.md)
- [`procurement-system.md`](./procurement-system.md)
- [`production-system.md`](./production-system.md)
- [`checkout-system.md`](./checkout-system.md)

Each module has its own data model and its own "Relationship to Other Systems" section. This document does not repeat that detail; it defines the rule that governs how they may reference each other (Section 2), a scope map (Section 3), a bird's-eye view of how they connect (Section 4), and their shared relationship to [`finance-system.md`](./finance-system.md) (Section 5).

## 2. Module Independence Principle

NexGestion serves small and medium businesses of very different natures. A pure service company may need Order but never Inventory or Production. A trading company may need Procurement and Inventory but never Production. A light manufacturer may need all four. A retail business may need Checkout and Inventory but never Order, Procurement, or Production — and, symmetrically, a B2B-only business never installs Checkout at all. **No module in this family may require another module in this family — or the Finance module — to be installed in order to function.**

Concretely:

- A reference from one module to another (e.g. an order line pointing at an inventory item) is always an **optional** field, never a required foreign key. When the referenced module is not installed or the reference is absent, the referencing module falls back to a plain description/amount and keeps working exactly as if the other module didn't exist.
- Each module owns its own database, matching the existing one-module-one-database convention (`architecture.md` §7 — `user.db`, `attendance.db`, `notification.db`, `template.db`). A module can be entirely absent from a deployment without touching another module's schema or data.
- A module never blocks its own core action while waiting on another module — e.g. confirming an order must not fail because Inventory is uninstalled; it simply skips the step that module would have handled.
- The same rule applies to Finance & Accounting: every module here can run in an "operations-only" mode with zero financial documents generated, for a deployment that doesn't use NexGestion's Finance module at all (Section 5).

## 3. Scope Map

| Module | Core question it answers | Typically needed by | Document |
| --- | --- | --- | --- |
| Order | What did a customer ask for, and has it been fulfilled? | Any business selling a product or service | [`order-system.md`](./order-system.md) |
| Inventory | How much of each item do we have, and where? | Product/goods-based businesses | [`inventory-system.md`](./inventory-system.md) |
| Procurement | What have we ordered from suppliers, and has it arrived? | Businesses that buy goods for resale or for production | [`procurement-system.md`](./procurement-system.md) |
| Production | What are we manufacturing, and is it done? | Manufacturers/light assemblers only | [`production-system.md`](./production-system.md) |
| Checkout | Ring up an in-person sale and take payment, fast | B2C/retail businesses only | [`checkout-system.md`](./checkout-system.md) |

## 4. Cross-Module Relationship Overview

A natural goods flow runs Procurement → Inventory → Production → Inventory → Order/Checkout, but no deployment is assumed to use the full chain (Section 2). Every connection below is an optional field that links through Inventory — Order, Procurement, Production, and Checkout never reference each other directly:

```txt
Procurement ──(receipt)──▶ Inventory ◀──(issue)── Order, Checkout
                             ▲   │
              (production_input)│(production_output)
                             │   ▼
                          Production
```

| From module | To module | Linking field | Detailed in |
| --- | --- | --- | --- |
| Order | Inventory | `inventory_item_id` on an order line | `order-system.md` §3.1, `inventory-system.md` §3.1 |
| Procurement | Inventory | `inventory_item_id` on a purchase order line | `procurement-system.md` §3.1, `inventory-system.md` §3.2 |
| Production | Inventory | `inventory_item_id` on material lines and on the output | `production-system.md` §3.1, `inventory-system.md` §3.3 |
| Checkout | Inventory | `inventory_item_id` on a checkout line | `checkout-system.md` §4.1, `inventory-system.md` §3.4 |

Order and Checkout both consume Inventory the same way — an `issue`/Outbound movement — but have no direct relationship to each other or to Procurement/Production in any of the five documents; any connection between customer demand, purchasing, manufacturing, and retail sale is expressed only through Inventory's stock levels.

## 5. Relationship to Finance & Accounting

Operations modules never write directly to the General Ledger or to account balances. Each one only ever creates a **draft** financial document — the same draft-before-posting shape already used throughout this platform (`salary-system.md` §6.4, `finance-system.md` §4). Finance's own approval and posting workflow decides what happens next. This keeps the dependency one-directional and optional: Finance can read from Operations when both are installed, but Operations never needs Finance to be installed to do its own job.

| Module | Business event | Finance linkage (only when Finance is installed) | Detailed in |
| --- | --- | --- | --- |
| Order | Order confirmed or fulfilled | May create a draft Accounts Receivable invoice (`finance-system.md` §5) | `order-system.md` §3.4 |
| Procurement | Vendor invoice recorded and matched | May create a draft Accounts Payable bill (`finance-system.md` §4) | `procurement-system.md` §3.4 |
| Inventory | Any stock movement (inbound, outbound, transfer, adjustment) | May post a draft GL journal entry if inventory is valued as a balance-sheet asset (`finance-system.md` §3) | `inventory-system.md` §3.5 |
| Production | Production order completed | May post a draft GL entry moving value from raw-material to finished-goods inventory (WIP costing) | `production-system.md` §3.4 |
| Checkout | Transaction completed | May post a draft GL entry directly, recognizing revenue against cash/card received — bypasses Accounts Receivable entirely, since a checkout sale is already paid | `checkout-system.md` §4.3 |

`finance-system.md` §5 currently marks Accounts Receivable as fully conditional/deferred. Enabling Order in a deployment that sells to customers changes that calculus — see `order-system.md` §3.4 for the open decision this creates. Checkout never touches AR at all, regardless of that decision, since nothing remains outstanding once a sale completes.

## 6. Explicitly Deferred Decisions

Module-specific deferred decisions live in each module's own document. The following are cross-cutting and affect more than one document:

- the shared customer/vendor "party" concept question raised in `order-system.md` §4 and `procurement-system.md` §4 (both now have their own record — Customers and Vendors respectively — the open question is only whether they should unify into one shared concept);
- inventory valuation and GL posting (`inventory-system.md` §4), which also gates Production's WIP-costing linkage (`production-system.md` §3.4);
- revisiting `finance-system.md` §5's Accounts Receivable status now that Order can depend on it (`order-system.md` §3.4).
