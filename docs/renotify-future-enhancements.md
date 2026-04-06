# Renotify — Future Enhancements

This document tracks enhancements, verification gaps, and deferred
work identified during development. Items are grouped by category
and cross-referenced to the requirements and decisions in
[renotify-refinements.md][refinements].

None of these items are blocking — the system operates correctly
within normal usage. They are recorded here so they are not
forgotten when priorities shift.

---

## 1. Implementation Gaps

### 1.1 Per-Flow Notification Rate Limiting

**Requirement:** [R-CLI-16][r-cli-16]

The config scaffolding is in place (`RateLimitConfig.NotificationsPerMinute`,
default 60), the error code (`rate_limited`) is defined, and exit code 4 is
allocated. However, no code path enforces the limit — notifications are
accepted unconditionally.

**Approach:** Daemon-side enforcement in the `svc.insert-request` handler
within the registry subsystem. A sliding-window counter keyed by `flow_id`
(in-memory `map[string][]time.Time`) rejects requests exceeding the
configured rate, publishing an `ErrorResponse` with `code: "rate_limited"`
on the flow's `.response` subject. NATS-side enforcement is not viable —
JetStream retention bounds cap stored messages, not ingestion rate.

**Scope:** Small. One guard clause in the insert-request handler plus
a sliding-window helper.

### 1.2 Stale Flow Reaping Grace Period

**Requirement:** [R-CLI-18][r-cli-18]

The reaping mechanism works correctly. The default grace period in
`config.go` is 15 minutes, while the requirement and analysis document
specify 5 minutes. The longer default was likely an intentional adjustment
— 5 minutes proved too aggressive for MCP agent flows where tool calls
may be spaced several minutes apart.

**Action:** Update the requirement statement and
[analysis-configuration-schema.md][config-schema] to reflect the
15-minute default, noting the value is configurable. This is a
documentation fix, not a code change.

---

## 2. Verification Gaps

### 2.1 MVP Scale Bounds

**Requirement:** [R-SYS-01][r-sys-01]

[R-SYS-01][r-sys-01] specifies minimum thresholds the system must
support:

| Bound                      | Status                          |
|:---------------------------|:--------------------------------|
| 20 concurrent active flows | Likely supported, not tested    |
| 10 notifications/sec       | Likely supported, not tested    |
| 10,000 history records     | Likely supported, not tested    |
| 64 KB max payload          | Enforced via NATS `MaxMsgSize`  |

SQLite and memory-backed JetStream can comfortably handle these
volumes, but there are no tests validating the claim.

**Action:** Add a benchmark or integration test that registers 20
concurrent flows, sustains 10 notifications/second, and inserts
10,000 history records to verify query correctness and performance.

---

## 3. Deferred Security Enhancements

**Requirement:** [R-SEC-03][r-sec-03]

The following were explicitly deferred during MVP scoping:

- **Fine-grained per-workspace mobile permissions** — currently all
  paired devices see all notifications across all workspaces.
- **Automatic token rotation** — tokens are static until manually
  revoked via `renotify revoke`.

Note: multi-device pairing was originally listed in [R-SEC-03][r-sec-03]
as deferred but has since been implemented ([D-68][d-68],
[R-MOB-11][r-mob-11]).

---

## 4. Open-Source Preparation

### 4.1 Credential Review

Before making the repository public, audit for any committed
secrets, API keys, or internal hostnames that should be removed
from history.

[refinements]: renotify-refinements.md
[config-schema]: analysis-configuration-schema.md
[r-cli-16]: renotify-refinements.md#r-cli-16-notification-rate-limiting
[r-cli-18]: renotify-refinements.md#r-cli-18-stale-flow-reaping
[r-sys-01]: renotify-refinements.md#r-sys-01-mvp-scale-bounds
[r-sec-03]: renotify-refinements.md#r-sec-03-post-mvp-security-deferral
[r-mob-11]: renotify-refinements.md#r-mob-11-multi-device-support
[d-68]: renotify-refinements.md#design-decision-register
