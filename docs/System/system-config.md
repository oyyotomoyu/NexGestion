# System Config

## 1. Purpose

This document defines an administrative "System Config" feature that lets an authorized user view and edit selected configuration surfaces from the UI, without a code change or redeploy. It covers three surfaces, each with different edit semantics and risk:

1. `config/*.json` catalog files (e.g. [`leave-types.json`](../../config/leave-types.json));
2. `odm/` branding files (theme colors, images) described in [`ui-architecture.md`](../UiDesign/ui-architecture.md) Sections 12–13; and
3. system/network settings (e.g. bind address, port) — not yet built, planned as a follow-up.

This is a planning document, not a finalized schema or API contract.

## 2. Editability Is Not Uniform

Not every file under `config/` is safe to expose through a generic editor. The distinguishing question is whether the file's entries are pure data, or whether each entry is coupled to a specific line of backend code.

| Surface | Example | Editable via UI? | Why |
| --- | --- | --- | --- |
| Business catalog | `config/leave-types.json` | Yes, full CRUD | Entries are organization-defined data with no 1:1 backend route coupling |
| Route-coupled catalog | `config/permission.json` | Read-only | Per [`permission-system.md`](./permission-system.md) Section 3, a route requires **both** a catalog key **and** a matching `server/apis/router.go` registration. A key added only through this UI protects nothing; a key removed while its route is still registered breaks that route's authorization check |
| ODM branding | `odm/theme.json`, `odm/img/*` | Yes, structured editing | Explicitly designed for customization ([`ui-architecture.md`](../UiDesign/ui-architecture.md) Section 12), already schema/format constrained |
| Network/interface settings | bind address, port | Yes, behind a guarded apply flow | High risk: a wrong value can cut off the admin's own access to the UI needed to fix it (Section 5) |

`config/permission.json` still appears in this UI as a **read-only reference view** — an administrator assigning role permissions benefits from seeing the full catalog in the same place — but it carries no create/edit/delete controls here. That editing surface, if ever built, is a separate "developer adds a route + catalog key" workflow, not a runtime admin action.

## 3. Business Catalog Config

Applies to files like `config/leave-types.json`: flat lists of `{ key, label }` (or similar) entries with no direct code coupling.

- Editing uses structured forms bound to each file's known schema — an "add entry" / "edit label" / "reorder" / "delete entry" UI — not a raw JSON textarea. A malformed file would fail the same startup validation used for `permission.json` ([`permission-system.md`](./permission-system.md) Section 2), so free-text JSON editing is the wrong shape for this UI.
- **Runtime source of truth**: the platform's existing pattern (per `permission-system.md` Section 2) is file → validated at startup → synced into the database → runtime reads the database. Following that same direction, an edit made through this UI should write through to the catalog's database-backed table (the runtime source of truth), not rewrite the on-disk JSON file directly while the server is running. The on-disk file remains the deploy-time seed/default used to initialize a fresh installation or restore defaults. This needs confirming as the intended direction before implementation — see Section 7.
- **Deleting an entry still referenced elsewhere** (e.g. a `leave_type` key already used by existing leave requests, or branched on by jurisdiction-specific paid/unpaid handling in the future Salary System — see [`salary-system.md`](./salary-system.md) Section 3) must not silently orphan those references. Either block deletion while references exist, or support a "retire" state that keeps the key valid for historical records but hides it from new selections.
- Audit trail for every change (who, what, when), consistent with every other module in this platform.

## 4. ODM Branding

Applies to `odm/theme.json` and `odm/img/*`, described in [`ui-architecture.md`](../UiDesign/ui-architecture.md) Sections 12–13.

### 4.1 Theme colors

- Editable through one control per token (color picker), matching the fixed token set `NexColor` already validates (`primary`, `secondary`, `text`, `heading`, … `danger`, `info`) — not a generic key-value JSON editor, since an arbitrary new key would not be consumed by any component.
- Show a live preview before saving.
- **Open question**: `ui-architecture.md` Section 12 describes `odm/theme.json` as letting "an ODM build... replace it without changing component code," which reads as a **build-time** asset baked into the client bundle, not something the backend serves and the client re-reads at runtime. If that is correct, an admin editing colors through this UI would change the file on disk, but the browser would not reflect it until the client is rebuilt/redeployed — which conflicts with an "edit and see it immediately" expectation. This must be resolved as an explicit architecture decision before building the screen: either (a) theme tokens become a runtime-fetched config the client loads on startup, or (b) the editor is scoped as "prepare an ODM build," with the UI clearly stating that a rebuild/restart is required to apply the change.

### 4.2 Branding images

- Upload flow enforcing the exact filenames, dimensions, and formats already specified in `ui-architecture.md` Section 13 (`login-logo.svg` at `240×56` viewBox, `language-icon.svg` at `24×24`, `favicon.ico` at `32×32`). Reject a non-conforming upload with a clear validation message instead of silently accepting a broken asset.
- Same build-time-vs-runtime question as Section 4.1 applies to images imported through the `@odm/img/...` alias.

### 4.3 Product / organization name

`odm/theme.json` today has no field for a display name — only a `name` preset identifier (currently `"nexgestion-default"`, which reads as a theme-preset id, not a user-facing label) and `colors`. Making the organization/product name editable requires:

- adding a new field to the ODM schema (e.g. `productName` in `theme.json`, or a new `odm/branding.json`); and
- auditing and re-wiring every place the platform currently hardcodes "NexGestion" (page title, login screen, generated email templates such as the salary notification in [`salary-system.md`](./salary-system.md) Section 10) to read from that field instead.

This is a larger sweep than color/image editing and should be scoped as its own task rather than assumed to be a small addition alongside Section 4.1–4.2.

## 5. System / Network Settings (Planned Follow-Up)

Not yet built — noted here as the next config surface to add. Candidate settings: server bind address/interface, port, and possibly a public/external URL if one is ever used in generated content (e.g. links inside the salary notification email in [`salary-system.md`](./salary-system.md) Section 10.3).

- **Storage**: `system.db` ("Core application settings and system metadata," [`architecture.md`](./architecture.md) Section 7) is the natural home — a live, validate-then-apply value, not deploy-time seed data like Sections 3–4.
- This category needs materially stronger safeguards than catalogs or branding, because a wrong value can cut off the admin's own access to the interface used to fix it:
  - validate that the requested bind address/port is actually available on the host (e.g. attempt a test bind) before committing, rather than accepting any string;
  - never let the connection currently in use be invalidated without an explicit confirmation step; keep the previous known-good value recoverable — for example, auto-revert if the new interface doesn't come up within a timeout, or require a second confirmation reached through the new address before making it permanent;
  - a change likely requires restarting the listener, not just a database write; the apply flow (immediate rebind vs. "restart required" with a clear notice) needs to be decided during implementation, not assumed; and
  - this is the highest-risk action described in this document and should sit behind the strictest permission gate — most likely reserved for the protected initial administrator only, the same way `permissions.assign` is administrator-only in [`permission-system.md`](./permission-system.md) Section 4.

## 6. Permissions

Planned permission keys, to be added to `config/permission.json` when these APIs are implemented, following the existing catalog convention ([`user-system.md`](./user-system.md) Section 3.4):

| Permission | Allows |
| --- | --- |
| `system_config.read` | View business catalogs, ODM branding, and system settings (read-only, including the reference view of `permission.json`) |
| `system_config.catalogs.manage` | Edit business catalog config, e.g. leave types (Section 3) |
| `system_config.branding.manage` | Edit ODM theme colors and branding images (Section 4) |
| `system_config.network.manage` | Edit network/interface settings (Section 5) — high risk, recommend administrator-only regardless of role assignment |

The initial Admin role automatically receives every `system_config.*` permission through `grants_all_permissions`, as with other modules.

## 7. UI Page

A **Settings → System Config** destination, visible only to users holding at least one `system_config.*` permission, split into sections a user only sees if they hold the matching permission:

1. **Business Catalogs** — one panel per known catalog (e.g. Leave Types), each a form-driven list editor (Section 3), plus a read-only **Permissions** reference panel listing `permission.json`'s current catalog for context.
2. **Branding** — theme color tokens with live preview, and image upload slots for each ODM asset with inline dimension/format validation (Section 4).
3. **Network** *(planned, not in the first version)* — interface/port settings behind the guarded apply flow in Section 5.

Consistent with the rest of the platform, editors here use structured forms rather than raw JSON text areas, and every change is attributable (who, what, when) through the same audit pattern used elsewhere.

## 8. Explicitly Deferred Decisions

The following require separate product/engineering decisions before implementation:

- runtime source of truth for editable catalogs — database write-through vs. direct file rewrite (Section 3);
- whether `odm/theme.json` and `odm/img/*` are build-time or runtime assets, and what "apply" means for a color or image change (Section 4.1–4.2);
- schema and location for an editable product/organization display name, and the scope of hardcoded-name references it must replace (Section 4.3);
- the full list of network/interface settings in scope, and the exact validate/apply/rollback flow (Section 5);
- whether `permission.json` should ever become admin-editable (e.g. a future "custom permission" feature) or stay permanently code-owned; and
- retention/safety rule for deleting a business-catalog entry still referenced by historical records (Section 3).
