# Security Information and Event Monitoring (SIEM)

## 1. Purpose

This document plans a security-monitoring layer for NexGestion: a curated view of security-relevant events across modules, basic rule-based detection, and alerting — sized for how this platform is actually operated, not a general-purpose enterprise SIEM.

NexGestion is self-hosted by small organizations that typically have no dedicated security team; the "SOC" is often one administrator. The design goal is **common-sense coverage of the platform's actual highest-risk actions**, not exhaustive log ingestion or anomaly-detection machine learning, which would be disproportionate to how this system is deployed and staffed.

This is a planning document, not a finalized schema or API contract.

## 2. Threat Model for This Deployment Shape

NexGestion's risk profile differs from a cloud SaaS product in specific ways that should drive what this document prioritizes:

- **LAN-first exposure**: the primary deployment model is a server reachable over Wi-Fi/LAN ([`architecture.md`](./architecture.md) Section 9). A compromised device already on that network is a more realistic attacker position than an internet-wide scanner — unless an operator has also port-forwarded the server to the internet, which this document should assume some operators will do despite guidance against it. Because "LAN vs. internet" changes what a realistic attacker looks like, it should also change how the rules in this document behave, not just how they are described — see Section 7.
- **Single/small admin model, no separation of duties**: the protected initial administrator has unrestricted access ([`user-system.md`](./user-system.md) Section 6). A compromised or malicious administrator account is the single highest-impact risk this platform has, and the least detectable one, since most authorization checks correctly defer to that role rather than question it.
- **High-value data at rest**: payroll and PII ([`salary-system.md`](./salary-system.md) Section 7), employee personal data ([`user-system.md`](./user-system.md) Section 3.2), and — once built — network/interface configuration ([`system-config.md`](./system-config.md) Section 5) are all present in a system that may otherwise be run with limited operational rigor (no dedicated ops team, infrequent patching, shared physical access to the host machine).
- **Low operator security maturity is the default assumption**, not the exception — controls in this document should fail toward being visible and actionable by a non-specialist, not require log-analysis expertise.

## 3. Relationship to the Existing Log System

[`log.md`](./log.md) already defines a general-purpose operational log: every request writes `info`/`warning`/`error` records with IP, user ID, and content, retained for exactly seven days.

That system is intentionally general and short-retained — fine for day-to-day debugging, but insufficient for security monitoring on two counts:

1. **Retention**: seven days is far too short to notice a slow-moving compromise or support an after-the-fact investigation. A credential-stuffing campaign, a dormant compromised account used weeks later, or a slow data-exfiltration pattern would already be gone.
2. **Signal**: the general log mixes routine `info` noise with the handful of events that actually matter for security. An administrator should not have to read every log line to notice that someone just granted themselves `permissions.assign`.

Rather than replacing the Log System, this document defines a narrower, longer-retained, alert-capable layer on top of it — the same tiered pattern the Attendance System already uses for detailed vs. aggregate records ([`attendance-system.md`](./attendance-system.md) Section 6): short-retention operational detail feeds a smaller set of durable, purpose-built security records.

## 4. Event Sources

Security-relevant events already exist, scattered across each module's own database, per this platform's existing per-module-database architecture ([`architecture.md`](./architecture.md) Section 7):

| Source | Location | Relevant events |
| --- | --- | --- |
| Log System | `log/*.log` | All `warning`/`error` entries; failed/successful logins, account locks ([`log.md`](./log.md) Section 7) |
| UserSystem | `user.db` | `login_audit_logs`, `user_audit_logs` — role/permission grants, user creation/disable, password resets ([`user-system.md`](./user-system.md) Section 4) |
| Attendance | `attendance.db` | `attendance_events` corrections by an administrator, particularly ones affecting pay-relevant totals ([`attendance-system.md`](./attendance-system.md) Section 4.3) |
| Notifications | `notification.db` | `notification_events` — organization-wide broadcast sends, which are a plausible phishing/social-engineering vector if an account is compromised ([`notification-system.md`](./notification-system.md) Section 4) |
| Salary (planned) | salary module DB | settlement configuration changes, approvals, and corrections — all `salary.settlement.*` actions ([`salary-system.md`](./salary-system.md) Sections 6.7–7) |
| System Config (planned) | `system.db` | ODM/branding changes, and especially any network/interface change ([`system-config.md`](./system-config.md) Section 5) — the single highest-risk configurable action in the platform |

This module already spans read access to several of these; it does not own them. A security event is a **reference to** the originating record (module, table, record ID, timestamp), not a duplicated copy of the full business record — avoiding a second source of truth for the same fact.

## 5. Security Event Aggregation

Given local-first, single-box deployment and modest data volume, a distributed log-streaming pipeline (Kafka-style ingestion, external log shippers) is not proportionate. A lightweight, in-process aggregator is:

- each module emits a security event by calling a shared function at the moment an event in Section 4's table already happens (the same call site that writes the module's own audit record), rather than a separate process scraping other databases after the fact;
- the aggregator writes a normalized record into a dedicated store (e.g. a `security_events` table, plausibly living alongside the Log System or in its own small database) containing: timestamp, event type, severity, actor user ID, source IP, affected module/record reference, and a short human-readable summary;
- this keeps the platform's existing per-module-database boundaries intact ([`architecture.md`](./architecture.md) Section 7) — modules still own their detailed data; this table only holds the security-relevant subset needed for cross-module monitoring.

## 6. Detection Rules

Kept deliberately simple and rule-based — thresholds and pattern matches an administrator can understand at a glance, not statistical anomaly detection, which this platform has neither the data volume nor the operating expertise to tune or trust.

Minimum rule set, informed by Section 2's threat model:

| Rule | Trigger | Why it matters here |
| --- | --- | --- |
| Brute-force login | N failed logins for one account or from one IP within a short window | Directly detectable from existing `login_audit_logs`; currently only locks the account ([`login.md`](./login.md)) without alerting anyone |
| Privilege escalation | A role gains `permissions.assign`, `roles.manage`, or any `*.manage`/`*.configure` permission | The clearest signal of an administrator account being misused, since this platform has no separation of duties (Section 2) |
| Network/interface change | Any `system_config.network.manage` action | The single action most capable of causing outage or exposing the server outside the LAN ([`system-config.md`](./system-config.md) Section 5) |
| Off-hours privileged action | An administrative action (role grant, salary approval, network change) outside the organization's configured working hours | Cheap to compute from Attendance's existing working-hours concept ([`attendance-system.md`](./attendance-system.md)), and a common real-world compromise indicator |
| Mass export/download | A single user downloading an unusually large number of report files or records in a short window ([`report-files.md`](./report-files.md)) | The most likely shape of a data-exfiltration attempt given how much PII/payroll data this platform holds |
| New-device/new-IP privileged login | A user holding any `*.manage` permission signs in from an IP or device not seen for that account before | Cheap signal, high value against credential reuse from a breached account |

Rules should be configurable (threshold values, on/off) by an administrator rather than hardcoded, but the rule catalog itself should ship with sane defaults — most self-hosted operators will never tune this and should still be protected out of the box.

## 7. Network-Environment-Adaptive Response

Section 2 already flags LAN-first exposure as a defining trait of this platform's threat model, but "LAN vs. internet" is not just a fact about the deployment — it should actively change how Section 6's rules behave. A brute-force threshold that is reasonable for a handful of employees on office Wi-Fi is far too loose once the same server is reachable from the open internet, and a rule tuned for internet exposure would generate constant false alarms on a LAN-only deployment where every device is already inside the trust boundary. One fixed rule set cannot serve both cases well.

### 7.1 Classifying network origin

Every security event already carries a source IP (Section 5). That IP can be cheaply classified without any new infrastructure:

- **Private/loopback** — RFC1918 ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`) and loopback — consistent with the LAN/Wi-Fi deployment model `architecture.md` Section 9 describes as primary.
- **Public** — anything else. On a deployment that is genuinely LAN-only, a public source IP should not be reachable at all; its presence in an event is itself a signal, not just a classification (see 7.2).

### 7.2 Deployment posture as an explicit, auditable setting

Rather than inferring intent from traffic alone, the administrator should declare a **deployment posture** — `lan_only` (default) or `internet_exposed` — as a setting alongside the network/interface configuration in [`system-config.md`](./system-config.md) Section 5. This makes the operator's intent explicit instead of guessed, and gives the detection layer something authoritative to check reality against:

| Posture | Expected traffic | Adaptive behavior |
| --- | --- | --- |
| `lan_only` (default) | Private-range source IPs only | A privileged action (role grant, network change, mass export, or any event on Section 6's table) from a **public** source IP is treated as high-severity regardless of its own threshold, because it should be structurally impossible — it means either an undisclosed port-forward, a NAT/router misconfiguration, or a compromised perimeter device relaying traffic in. This is a stronger signal than any threshold-based rule in Section 6, since it is a binary violation of the declared deployment shape rather than a matter of degree. |
| `internet_exposed` | Mix of public and private source IPs | Public-origin traffic is expected, so it does not trigger the `lan_only` violation above, but Section 6's numeric thresholds (brute-force attempt count, mass-export size, off-hours window) should default to materially stricter values than under `lan_only` — an internet-facing login form faces credential-stuffing and scanning traffic a LAN deployment never sees, per Section 2's "an operator may port-forward despite guidance against it." |

Toggling this setting is itself a security-relevant configuration change: it should generate a security event (Section 4) at the same severity as a network/interface change (Section 6), since it directly changes what "normal" means for every other rule in this document, and a silent switch to `internet_exposed` would quietly widen the platform's attack surface without the audit trail an administrator would expect for a decision of that weight.

### 7.3 Known blind spot: VPN and reverse-proxy origins

A user connecting through a company VPN or behind a reverse proxy/load balancer will present a source IP that is either the VPN concentrator's private address or the proxy's address — not the true client IP — unless the platform is explicitly configured to trust a forwarding header (e.g. `X-Forwarded-For`) from a known, trusted upstream. Trusting such a header from an untrusted or unconfigured source is itself a spoofing risk, so this cannot default to "on." Until a trusted-proxy configuration exists, traffic arriving this way will misclassify as LAN-origin even when the actual user is remote, silently defeating 7.2's `lan_only` violation check. This is called out explicitly rather than left implicit so it isn't mistaken for coverage this design does not yet provide (tracked in Section 13).

## 8. Alerting and Delivery

Reuse the platform's existing Notification System rather than building a second delivery mechanism, but treat prompt email delivery as a requirement for a security incident specifically, not an optional extra that waits on another module's timeline.

- **In-app**: a triggered rule always sends an in-app notification scoped to users holding `security.alerts.read`, using the existing `notifications` + user-scoped `notification_audiences` tables ([`notification-system.md`](./notification-system.md) Section 4). This fires unconditionally and is the fallback of last resort if email is unset or fails (Section 8.2).
- **Email**: a security incident needs to reach someone even when nobody has the app open — an administrator does not watch an in-app inbox in real time the way they would watch an inbox they already check on their phone. NexGestion has no outbound-email capability today; [`salary-system.md`](./salary-system.md) Section 10.3 scopes the same underlying gap (SMTP/transactional-email integration, sender identity, retry handling) for payslip notices. Building that transport is a shared platform dependency, not something this document or the Salary System should each solve independently — but the *requirement* that a triggered incident emails the relevant people promptly is not itself deferred; it is the core of what makes this an alerting layer rather than a log an administrator has to remember to check. A security alert should never be silently absorbed into the same in-app inbox as routine business notices without visual distinction — reuse the existing `urgent`/`important` notification types ([`notification-system.md`](./notification-system.md) Section 2) rather than inventing a parallel severity scheme.

### 8.1 Recipient configuration

Hardcoding "every `security.alerts.read` holder gets every alert" does not fit every deployment — a one-administrator installation wants every alert in one inbox; a slightly larger one may want routine alerts kept internal but a critical incident (privilege escalation, a network change, an unexpected WAN-origin privileged action per Section 7.2) escalated to someone outside the day-to-day admin, such as an external IT contact who has no reason to hold a NexGestion login at all.

- **Default**: every user holding `security.alerts.read` receives every alert by both channels, mirroring who already sees it in-app.
- **Override**: an administrator can attach one or more additional email recipients to a severity tier or to an individual rule (e.g. "always email owner@example.com on privilege escalation, regardless of who else holds the permission"). Recipients must be allowed to be a bare email address, not only an existing platform user — the person who most needs to hear about an intrusion is often outside the system being intruded upon.
- **Per-recipient channel toggle** (in-app only / email only / both), so someone already paged another way isn't double-notified, and so a free-standing email recipient (who has no in-app account to receive an in-app notification anyway) isn't offered a channel that can't apply to them.

### 8.2 Timeliness and failure handling

- Email should be attempted synchronously at the moment the security event is recorded (Section 5), not batched into a periodic digest — a brute-force alert seen six hours later has already lost most of its value.
- A failed send (SMTP unreachable, bad credentials) must never prevent the in-app notification from landing — email is additive, never a single point of failure for whether an alert reaches anyone.
- A failed email attempt is itself worth surfacing back to the administrator (Section 9.5), since a self-hosted operator has no ops team watching a dead-letter queue on their behalf; a silently-broken email channel is worse than no email channel, because it creates false confidence that alerts are being delivered.

## 9. Configuration Surface: SIEM Settings Screen

Every "should be configurable" in Sections 6–8 needs one visible place an administrator actually does it, the same way [`system-config.md`](./system-config.md) Section 7 gives its own settings a dedicated screen rather than leaving configuration as undiscoverable API-only behavior.

A **SIEM Settings** screen, visible only to users holding `security.rules.manage` or the new `security.alerts.manage` (Section 12), contains:

1. **Detection Rules** — one row per rule in Section 6's table: on/off toggle and editable thresholds (count, window, size), with the shipped default shown alongside so an administrator can see what they've changed.
2. **Deployment Posture** — the `lan_only` / `internet_exposed` toggle from Section 7.2, with a plain-language explanation of what changes on switching (e.g. "a privileged action from a public IP will stop being treated as abnormal on its own; brute-force and export thresholds tighten automatically").
3. **Alert Recipients & Channels** — the per-severity/per-rule recipient list from Section 8.1: add a platform user (not restricted to existing `security.alerts.read` holders) or a bare email address, and toggle in-app/email/both per entry.
4. **Test Alert** — a "send test alert" action that exercises the real delivery path end-to-end (in-app and email) without waiting for a real incident, so email delivery can be confirmed working before it's needed.
5. **Alert History & Delivery Status** — the security-event timeline (`security.events.read`), with each entry showing whether its email attempt succeeded, failed, or was skipped (e.g. no recipient configured), directly surfacing Section 8.2's failure handling instead of leaving it invisible.

This screen is itself high-privilege for the same reason Section 12 already flags for the underlying permissions: being able to silence a rule or redirect who gets notified is functionally equivalent to disabling detection for whoever does it. Whether this lives as its own top-level destination or as a tab inside System Config's existing Settings surface ([`system-config.md`](./system-config.md) Section 7) is an open question — see Section 13.

## 10. Retention

Security events need a materially longer retention window than the Log System's seven-day cap (Section 3), because the value of this layer is specifically noticing what a short-retention operational log has already discarded.

- A default retention on the order of months, not days — the exact figure is an open decision (Section 13), and should probably default longer than the Attendance System's six-month detail retention ([`attendance-system.md`](./attendance-system.md) Section 6), since a security investigation typically starts only after a compromise is already suspected, at which point the triggering event may be old.
- Retention should be configurable per deployment, similar in spirit to how [`salary-system.md`](./salary-system.md) Section 4 treats statutory retention as jurisdiction-configurable, since some organizations may be subject to external record-keeping requirements this platform cannot assume.
- Security events are explicitly exempt from the seven-day cap that governs the general Log System (Section 3); they are a distinct, purpose-built record, not a longer-lived copy of the same table.

## 11. Integrity

Because this platform is self-hosted with direct filesystem access by whoever operates the host machine, a sufficiently privileged attacker (or a malicious administrator, per Section 2's threat model) could edit or delete log evidence after the fact. Full tamper-proof logging (write-once storage, external log shipping, blockchain-style chaining) is disproportionate for this platform's scale and audience, but doing nothing leaves the highest-risk actor in Section 2 able to erase their own trail without detection.

A proportionate middle ground: each security-event record includes a hash of the previous record (a simple rolling hash chain per period, similar in spirit to the daily-file structure already used by the Log System). This does not prevent tampering, but it makes tampering **detectable** — a broken chain is evidence something was altered or deleted — at negligible implementation cost, which matches this document's "common sense over completeness" framing.

## 12. Permissions

Planned permission keys, added to `config/permission.json` following the existing catalog convention ([`user-system.md`](./user-system.md) Section 3.4):

| Permission | Allows |
| --- | --- |
| `security.events.read` | View the security-event timeline and its detection-rule configuration |
| `security.alerts.read` | Receive security alert notifications (Section 8) |
| `security.rules.manage` | Enable/disable and tune detection-rule thresholds and deployment posture (Section 6, Section 7.2) |
| `security.alerts.manage` | Configure who receives alerts and through which channel — recipients, severity/rule routing, in-app vs. email (Section 8.1, Section 9) |

Access to this module is itself sensitive — a user who can see which behaviors are monitored, silence a rule, or redirect who gets notified can evade detection or intercept its warning — so it should sit behind the same high-privilege bar as `system_config.network.manage` ([`system-config.md`](./system-config.md) Section 6), and every grant or revocation of these four permissions should itself generate a security event (Section 4), since watching who can watch the watchers is the one blind spot this design would otherwise have.

## 13. Explicitly Deferred Decisions

- exact retention window for security events, and whether it differs by event severity;
- storage location — a dedicated `security.db`, or a table inside the existing Log System's storage;
- whether the SIEM Settings screen (Section 9) is its own top-level destination or a tab inside System Config's existing Settings surface ([`system-config.md`](./system-config.md) Section 7);
- who owns building the outbound-email transport (SMTP/transactional-email integration) that both this document (Section 8) and [`salary-system.md`](./salary-system.md) Section 10.3 depend on, and which module lands it first;
- whether a free-standing (non-platform-user) email recipient added under Section 8.1 needs an address-verification step before it starts receiving real alerts, to avoid silently leaking incident details to a mistyped address;
- template and localization design for security-alert emails, and whether they follow the same per-user `locale` pattern used elsewhere ([`user-system.md`](./user-system.md)) or default to a single admin-facing language;
- whether off-hours detection depends on the Attendance System exposing a working-hours concept it does not yet formally define;
- whether this module should support exporting to an external SIEM (e.g. syslog/CEF) for organizations large enough to already run one, versus staying self-contained;
- how "new device" is defined for the new-device/new-IP login rule (Section 6) given no device-fingerprinting mechanism exists elsewhere in the platform today;
- whether/when to add a trusted-reverse-proxy configuration (e.g. a `X-Forwarded-For` allowlist tied to a known proxy IP) so VPN/proxy-fronted deployments can be classified correctly instead of hitting the blind spot in Section 7.3; and
- the exact stricter default thresholds Section 7.2 uses under `internet_exposed` posture, and whether they scale automatically or require the administrator to re-tune them after switching posture.
