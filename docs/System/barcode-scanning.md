# Barcode Scanning

## 1. Purpose

This document collects the design considerations for scanning a barcode to quickly resolve an item, instead of manually searching for it. It is a planning checklist, not an implementation spec, following the same convention as [`salary-system.md`](./salary-system.md).

The first and primary consumer is Inventory's Inbound and Outbound line entry (`inventory-system.md` §2.3, §2.4), where adding a line by scanning is faster and less error-prone than picking an item from a list during physical receiving or shipping. The capability is designed to be reusable by any future module that needs code-to-record lookup, but nothing beyond Inventory consumes it yet.

Two capture sources must be supported, and they have very different technical constraints:

1. a physical **barcode scanner device** (a "barcode gun"), typically used at a fixed desktop/PC workstation; and
2. a **camera** — a PC's webcam, or a mobile device's (phone, tablet, laptop) built-in camera.

## 2. Capture Source Comparison

| | Hardware scanner | Camera |
| --- | --- | --- |
| Where it applies | Desktop/PC only | Desktop/PC webcam, and mobile devices |
| Browser permission required | None — the OS presents it as a generic keyboard | Yes — explicit camera permission per browser security rules |
| Works over plain HTTP on a LAN | Yes | **No** — see Section 3.2's secure-context constraint |
| Device selection needed | No — the OS handles it | Yes, when more than one camera exists |

Because NexGestion's default deployment model serves the app over plain HTTP to a LAN IP address (`architecture.md` §9), the hardware scanner is the more reliable option out of the box; camera scanning is a capability that only becomes available once the deployment is reachable over a secure origin (Section 3.2).

## 3. Capture Sources

### 3.1 Hardware Barcode Scanner (Desktop/PC)

A barcode scanner behaves as a keyboard-wedge HID device: it "types" the decoded value into whatever field currently has focus, character by character, far faster than a human can type, and finishes with an `Enter` or `Tab` keystroke. From the browser's perspective it is indistinguishable from a keyboard, so:

- no camera-style permission prompt is needed, and it works over plain HTTP;
- detection is a timing heuristic — a run of keystrokes with an inter-key interval below a threshold (e.g. 30–50ms), terminated by `Enter`/`Tab`, is treated as a scan rather than manual typing;
- the scanning input target must stay focused during an active Inbound/Outbound recording session, with a visible "scanner ready" indicator so staff know input will register; and
- if more than one scanner is physically connected, no special handling is needed — the OS presents all of them as generic keyboard input to whichever field is focused.

### 3.2 Camera-Based Scanning (PC Webcam and Mobile Devices)

Camera scanning uses the browser's camera through `getUserMedia`, decoded with the native `BarcodeDetector` API where the browser supports it, falling back to a bundled JS decoding library where it doesn't (Section 6's deferred decision).

**Secure-context constraint:** browsers block `getUserMedia` entirely on an insecure origin, with `localhost` as the only exception. NexGestion's local-first deployment model — a server on a LAN IP, reached over plain HTTP from other employees' devices (`architecture.md` §9) — is exactly the case browsers block. Camera scanning therefore only works when the deployment is reachable over HTTPS (a reverse proxy or a local TLS certificate) or accessed directly on the same machine via `localhost`. This must be surfaced to the user as a clear, actionable state rather than a silent failure or a missing button — see the "Camera Scanning May Not Be Available" section of [`../UserApp/barcode-scanning.md`](../UserApp/barcode-scanning.md).

Behavior once available:

- **Desktop/PC**: after permission is granted, enumerate `videoinput` devices (`navigator.mediaDevices.enumerateDevices()`) and let the user pick one — a PC may have more than one camera (a built-in webcam plus an external one).
- **Mobile** (phone, tablet, or a laptop with a single built-in camera): default to the rear-facing camera (`facingMode: "environment"`), since that's the one pointed at a physical barcode.
- Permission is requested only on an explicit user action (tapping/clicking a "Start camera scan" control) — never automatically on page load, both because browsers require a user gesture for some permission prompts and because silently requesting camera access would be surprising.
- The decode loop runs against live video frames until a valid code is recognized, fires the result once, and stops the `MediaStream` immediately afterward so the camera indicator light turns off and the device isn't held open longer than needed.

## 4. Resolving a Scan to an Item

A scanned code is matched against `inventory_items.barcode` first, then against `sku` (`inventory-system.md` §2.2), so both a vendor-printed barcode and an internally generated SKU label work as scan targets.

- **Match found**: add one unit to a new or existing line for that item on the current Inbound or Outbound record (`inventory-system.md` §2.3, §2.4). The user can still adjust the quantity manually afterward — repeated scanning of the same code is the expected way to reach a larger quantity.
- **No match found**: show an inline "barcode not recognized" state. The system does not auto-create an item from an unrecognized scan; the user is offered a link to manual item search/selection instead, keeping catalog creation a deliberate, permissioned action rather than a side effect of a bad scan.

## 5. Frontend Component

A reusable scanning component (planned name `NexBarcodeScanner`, matching the existing `NexButton`/`NexInput`/`NexSelect` naming convention in `client/src/components`) wraps both capture sources behind one interface:

- **Mode**: `hardware`, `camera`, or `auto` — `auto` shows a small mode switcher on the same screen, since a warehouse desktop workstation might have a scanner gun attached while a mobile device only ever has a camera.
- **Scan callback**: fires once per successfully decoded code, regardless of which capture source produced it — the consumer (e.g. an Inbound/Outbound line-entry screen) doesn't need to know which mode was active.
- **Hardware mode** renders a focused capture zone with a visible "scanner ready" indicator; it doesn't need its own permission or error states, per Section 3.1.
- **Camera mode** renders a live video preview, a device selector when more than one camera is available, a start/stop control, and an explicit error state covering: permission denied, no camera present, and insecure context (Section 3.2) — each with its own actionable message rather than one generic failure.
- The component stops its `MediaStream` on unmount or when scanning is manually stopped, so the camera is never left open longer than the user intended.

## 6. UI Architecture: Every Scanning Screen Is Its Own Route

Any screen that includes barcode scanning — hardware or camera — must be reachable as its own dedicated route with its own entry point in the app's navigation, never embedded as a modal, a step inside a larger multi-purpose screen, or a tab buried within an unrelated page. This is a platform-wide rule that every scanning-enabled screen follows, not a one-off decision made for any single module:

- A dedicated route keeps the scan input focused and ready without competing with unrelated page state, and lets a user jump straight to a scanning task — via a bookmark, a pinned nav item, or a direct link right after login — instead of navigating through an unrelated parent screen first.
- Each such route gets its own independent tab/page in the app's navigation, so operating it never requires first opening a different, larger screen and finding scanning as a sub-feature buried inside it.
- This generalizes the reasoning already applied to Checkout's dedicated `/checkout` route (`checkout-system.md` §3): scanning is a time-sensitive, repetitive action, and every extra click or unrelated bit of page state between "open the screen" and "scan" works against that.

Current and planned applications of this rule:

| Screen | Module | Route status |
| --- | --- | --- |
| Checkout | `checkout-system.md` §3 | Dedicated route + own minimal layout, specified |
| Inbound recording | `inventory-system.md` §2.3 | Needs its own dedicated route, per this rule (not yet detailed further) |
| Outbound recording | `inventory-system.md` §2.4 | Needs its own dedicated route, per this rule (not yet detailed further) |

Any future screen that adopts `NexBarcodeScanner` (Section 5) inherits this requirement by default. A screen that embeds scanning without its own route and nav entry is a deviation that needs explicit justification, not the norm.

## 7. Explicitly Deferred Decisions

- which JS barcode-decoding fallback library to use where `BarcodeDetector` isn't supported (browser support, particularly Safari's, has historically been inconsistent);
- whether a local HTTPS/TLS setup should become part of the installer (`architecture.md` Phase 4) specifically to unblock camera scanning for local-first LAN deployments, rather than leaving it as an operator-configured reverse proxy;
- batch-scan mode (scanning several different items into one open Inbound/Outbound record without returning to a list between scans each time) — this document assumes continuous scanning within one open recording session, but the exact interaction flow isn't finalized;
- which barcode/QR symbologies are officially supported — likely whatever `BarcodeDetector`/the fallback library supports rather than a curated subset, but not finalized;
- permission keys — scanning is an input method for an already-permissioned action (recording an Inbound/Outbound), so it likely needs no permission key of its own beyond `inventory-system.md` §4's existing recording permission, but this should be confirmed when that catalog is defined.
