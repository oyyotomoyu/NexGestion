# CRM System

## 1. Purpose

This document collects the design considerations for a future CRM (Customer Relationship Management) module: the authoritative record of who a customer is, what tier they're in, and — for B2C — what membership they hold. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

NexGestion serves organizations that may be B2B, B2C, or both at once, and those two shapes of customer relationship are different enough that CRM needs to model them side by side rather than assume one. CRM is a standalone module, not a table owned by Order — [`order-system.md`](./order-system.md) and [`checkout-system.md`](./checkout-system.md) both consume it optionally, following the same Module Independence Principle as the Operations System family (`operations-system.md` §2): neither Order nor Checkout requires CRM to be installed, and CRM itself requires neither of them.

CRM is scoped to **customers only**. Procurement's Vendors (`procurement-system.md` §2.1) remain a separate, locally-owned concept — whether the two should ever unify into one shared "party" model is a standing open question (Section 4), not something this document resolves by assumption.

## 2. Data Model

### 2.1 Customers

The unified customer record. A customer can be a B2B business account or a B2C member, and either can be one person or a group.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable customer ID |
| `party_type` | TEXT | Yes | `individual`, `organization` — whether this customer is one person or a group/company |
| `segment` | TEXT | Yes | `b2b`, `b2c` — which tier system applies (Section 2.2 vs. 2.3) |
| `name` | TEXT | Yes | Person's name, or organization/group name |
| `contact_email` | TEXT | No | |
| `contact_phone` | TEXT | No | |
| `tax_identifier` | TEXT | No | Format varies by jurisdiction, same caveat as `salary-system.md` §5; typically only set for `b2b` customers |
| `tier_id` | TEXT/UUID | No | Optional link to a Customer Tier (Section 2.2), for `b2b` pricing |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

A deployment serving only one segment simply never creates customers of the other kind. `party_type = organization` under `segment = b2c` is deliberate — a family plan or a small club is a group, but it's still a consumer relationship, not a business account.

### 2.2 Customer Tiers (B2B)

Organization-defined tiers a `b2b` customer can be assigned to, used to determine which price list applies to their quotes. Names and count are entirely up to the organization — nothing here hardcodes a fixed set like "Gold/Silver/Bronze."

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable tier ID |
| `name` | TEXT | Yes | Organization-defined name, e.g. "Distributor", "Preferred", "Standard" |
| `description` | TEXT | No | |
| `default_price_list_id` | TEXT/UUID | No | Optional link to Section 2.4 |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

### 2.3 Memberships (B2C)

A B2C-specific enrollment record, kept separate from Customer Tiers because a retail membership program has its own join/renew lifecycle — a member number, a join date, an expiry — that a B2B account tier assignment doesn't need. The holder of a membership (`customer_id`) can be an individual or a group, per Section 2.1's `party_type`.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable membership ID |
| `customer_id` | TEXT/UUID | Yes | The holder — links to Section 2.1; `party_type` may be `individual` or `organization` |
| `membership_tier_id` | TEXT/UUID | Yes | Link to Section 2.3.1 |
| `member_number` | TEXT | No | Organization-issued card/ID number — the value scanned at Checkout (Section 3.2) |
| `joined_at` | DATE | Yes | Enrollment date |
| `expires_at` | DATE | No | Expiration date, when the program isn't open-ended |
| `status` | TEXT | Yes | `active`, `lapsed`, `cancelled` |

#### 2.3.1 Membership Tiers

Organization-defined loyalty levels (e.g. "一般會員", "銀卡會員", "金卡會員"), structurally parallel to Customer Tiers but scoped to `b2c`.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable membership tier ID |
| `name` | TEXT | Yes | Organization-defined name |
| `description` | TEXT | No | |
| `default_price_list_id` | TEXT/UUID | No | Optional link to Section 2.4 |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

### 2.4 Price Lists

Shared by both tier systems — a Customer Tier and a Membership Tier each optionally point at one.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable price list ID |
| `name` | TEXT | Yes | |
| `currency` | TEXT | Yes | ISO 4217 currency code |
| `status` | TEXT | Yes | `active`, `inactive` |
| `created_at` | DATETIME | Yes | Creation time in UTC |

| Field (item) | Type | Required | Description |
| --- | --- | --- | --- |
| `price_list_id` | TEXT/UUID | Yes | Owning price list |
| `description` | TEXT | Yes | What the price applies to, in plain text |
| `inventory_item_id` | TEXT/UUID | No | Optional link to an Inventory item, the same optional-link pattern used throughout this platform |
| `unit_price` | DECIMAL | Yes | |

A tier's default price list is a **default, not an enforced constraint** — a Quote or Checkout line's own `unit_price` remains directly editable regardless of what the customer's tier suggests.

### 2.5 Loyalty Points (B2C)

A points balance a `b2c` customer accrues from purchases and can redeem against a future one. Modeled as an append-only ledger with a derived balance, the same non-destructive pattern already used for Inventory's Stock Movements (`inventory-system.md` §2.6) and Finance's journal entries (`finance-system.md` §3) — current balance is always the sum of ledger rows, never a separately maintained mutable counter, so it can always be reconciled against its history.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable ledger entry ID |
| `customer_id` | TEXT/UUID | Yes | Links to Section 2.1; only meaningful for `segment = b2c` customers |
| `points_delta` | INTEGER | Yes | Positive for points earned, negative for points redeemed or expired |
| `entry_type` | TEXT | Yes | `earned`, `redeemed`, `expired`, `adjustment` |
| `source_module` | TEXT | No | `checkout`, or `NULL` for a manual adjustment |
| `source_reference_id` | TEXT/UUID | No | The Checkout transaction (`checkout-system.md` §2.1) that earned or redeemed these points, when `source_module = checkout` |
| `occurred_at` | DATETIME | Yes | UTC timestamp |

A customer's current points balance is the sum of `points_delta` across their rows. `entry_type = earned` and `entry_type = redeemed` both originate from Checkout (Section 3.2); `expired` and `adjustment` are CRM-side operations with no Checkout counterpart — a lapsed points balance or a customer-service correction, respectively.

#### 2.5.1 Points Earning Rules

Organization-defined accrual configuration — how many points a purchase earns, and whether that rate varies by membership tier.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | TEXT/UUID | Yes | Immutable rule ID |
| `membership_tier_id` | TEXT/UUID | No | Optional link to Section 2.3.1 — a tier-specific earn rate; unset is the default rate applied when no tier-specific rule matches |
| `points_per_currency_unit` | DECIMAL | Yes | e.g. `1` point per `$1` spent, or `0.5` for a slower-earning base tier |
| `status` | TEXT | Yes | `active`, `inactive` |

A higher-tier member earning points faster than a base member is expressed as two rows here — one with `membership_tier_id` set to the higher tier and a larger `points_per_currency_unit`, one unset as the fallback — rather than a hardcoded tier-multiplier table.

CRM owns its own database (`crm.db`), matching the one-module-one-database convention already used across the platform (`architecture.md` §7).

## 3. Relationship to Other Systems

### 3.1 Order System

Order's `customer_id`/`customer_name` pair (`order-system.md` §2.3, and Quotes at §2.2) becomes an optional link to a CRM Customer (Section 2.1) when CRM is installed, with `customer_name` remaining the required snapshot fallback exactly as before — Order still works with zero CRM installed, falling back to a plain-text customer name the same way it always has. When a Quote is created for a linked customer, the customer's tier and its default price list (Section 2.2, 2.4) may pre-fill line pricing; the quote's own line `unit_price` remains editable regardless.

### 3.2 Checkout System

A checkout transaction (`checkout-system.md` §2.1) may carry an optional `crm_customer_id`, typically populated by scanning a membership number (Section 2.3) with the same barcode-scanning capability already used for items and coupons (`barcode-scanning.md`, `checkout-system.md` §4.4) — a membership card is just another scannable code resolving to a record, the same resolution pattern used for items (`inventory-system.md` §2.2). Checkout remains fully anonymous by default (`checkout-system.md` §5); this is a pure enrichment, not a requirement. Three effects when a member is linked:

- **Member pricing**: the linked membership's tier may inform member pricing at checkout (`checkout-system.md` §4.5), from Section 2.4's price lists.
- **Points earning**: completing a transaction posts one `earned` row to Section 2.5's ledger, sized by Section 2.5.1's rule for the customer's tier, against the transaction's `total_amount` (post-discount — points accrue on what was actually paid, not the pre-discount subtotal).
- **Points redemption**: before completing a transaction, the customer may redeem existing points as a Checkout Discount (`checkout-system.md` §2.3.3, `source_type = points`), posting a matching `redeemed` row here. The currency value of one redeemed point is an open decision (Section 4).

### 3.3 Procurement System

No relationship. CRM is scoped to customers; Procurement's Vendors (`procurement-system.md` §2.1) remain separately owned. See Section 4 for the open question of whether the two should ever unify.

## 4. Explicitly Deferred Decisions

- the finalized schema and API contract (the shapes in Section 2 are design notes, not a final schema);
- the actual discount/pricing computation from a tier's price list beyond "it's a default" — override rules, approval for exceptions, and how conflicting tier/price-list assignments are resolved aren't decided;
- points redemption conversion rate (how much one point is worth when redeemed as a Checkout Discount, `checkout-system.md` §2.3.3) and any minimum-redemption threshold;
- points expiration policy — whether earned points ever lapse, and if so on what schedule (a fixed period from `occurred_at`, a program-wide annual reset, or never); `entry_type = expired` (Section 2.5) is modeled but not yet scheduled anywhere;
- whether Points Earning Rules (Section 2.5.1) can vary by item/category, not just by membership tier;
- whether an `organization`-type membership (Section 2.3) implies individual members underneath it (e.g. a family plan's individual family members each getting their own card), or stays a single group-level record;
- whether Procurement's Vendors (`procurement-system.md` §2.1) should eventually unify with CRM's Customer model into one shared party concept;
- membership renewal/lapse automation, and whether `expires_at` passing is a scheduled job or a derived read at query time, matching how Inventory derives on-hand quantity rather than storing it (`inventory-system.md` §2.6);
- permission keys, to be defined per the existing catalog convention (`user-system.md` §3.4) once API design starts.
