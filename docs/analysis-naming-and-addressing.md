# Naming and Addressing Analysis

This document analyses the system element hierarchy, identifier design, and NATS
namespace implications for Renotify. It was produced in response to concerns
that the current refinement plan does not account for the full topology of
interactions between broker, user, daemon, workspace, and execution units.

The goal is an unambiguous set of system elements with well-defined identifiers
and a clear scope of uniqueness for each identifier.

**Identifier Encoding:** All generated identifiers use Crockford Base32
(alphabet `0123456789ABCDEFGHJKMNPQRSTVWXYZ`). This encoding is
case-insensitive, excludes visually ambiguous characters (I, L, O, U), and
encodes 5 bits per character — yielding shorter IDs than hex (1.6x denser) while
remaining safe in NATS subjects, CLI output, filenames, log searches, and spoken
conversation. All examples in this document use uppercase Crockford Base32.

**Identifier Truncation:** Not all identifiers need the full 128-bit
(26-character) representation. The appropriate length depends on the population
size and collision-risk context of each identifier:

| Identifier     | Base32 Length                   | Entropy  | Population Rationale                                                                                                                                                                         |
|:---------------|:--------------------------------|:---------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `flow_id`      | 26 chars (full 128-bit UUIDv7)  | 128 bits | Highest-volume identifier. Thousands created per day across all daemons. Appears in NATS subjects where a collision would silently cross-contaminate flows. Must carry full UUID guarantees. |
| `workspace_id` | 16 chars (80 bits from SHA-256) | 80 bits  | Already a hash truncation. A single daemon manages at most tens of workspaces. 80 bits gives ~1.2 x 10^24 values — collision probability negligible at this population.                      |
| `daemon_id`    | 13 chars (65 bits from UUIDv4)  | 65 bits  | A user accumulates at most tens of daemon installations over their career. 65 bits gives ~3.7 x 10^19 values — collision risk astronomically low.                                            |
| `device_id`    | 13 chars (65 bits from UUIDv4)  | 65 bits  | A user pairs single-digit mobile devices. Same reasoning as daemon_id.                                                                                                                       |

---

## 1. Full Topology

The system supports the following nesting of elements. Each level can have a
one-to-many relationship with the level below it.

```
NATS Broker
  └── User (1..N per broker)
        └── Daemon Instance (1..N per user, across machines)
              └── Workspace (1..N per daemon)
                    └── Flow (1..N per workspace, concurrent)
```

**Concrete example:** A shared enterprise NATS broker serves two developers.
Developer `stewart` runs a daemon on both a workstation (`dn_ws01`) and a CI
server (`dn_ci01`). The workstation daemon manages two workspaces (`renotify`
and `gethos-api`), each with multiple concurrent flows (a build pipeline, an AI
agent conversation, etc.). Developer `alice` runs a single daemon with her own
set of workspaces and flows.

This topology is strictly hierarchical. Every flow belongs to exactly one
workspace, every workspace belongs to exactly one daemon, and every daemon
belongs to exactly one user within a given broker.

---

## 2. System Elements and Identifiers

### 2.1 NATS Broker

The root infrastructure element. Either embedded in a daemon (single-user,
single-machine) or shared across an organisation.

* **Identifier:** Connection URL (host:port). Not modelled as a first-class ID
  within the payload schemas — it is infrastructure configuration.
* **Uniqueness scope:** Global (DNS/IP).
* **Relevance:** Determines whether the mobile app connects to one daemon or a
  shared broker. Affects pairing.

### 2.2 User

A human developer authenticated to the NATS broker.

* **Identifier:** `username` — a short alphanumeric string (e.g., `stewart`).
* **Uniqueness scope:** Per NATS broker. Enforced by NATS auth configuration.
  Two users on the same broker cannot share a username.
* **Generation:** Configured during daemon setup. Corresponds to the NATS auth
  credential.
* **Stability:** Permanent for a given broker installation.

### 2.3 Daemon Instance

A running `renotify daemon` process. A user may operate daemons on multiple
machines (e.g., a laptop and a CI server), each managing different workspaces.

* **Identifier:** `daemon_id` — a persistent, auto-generated opaque ID.
* **Format:** `dn_` prefix + 13 Crockford Base32 characters (65 bits truncated
  from UUIDv4) (e.g., `dn_3G2K7V9WNFQ4J`).
* **Uniqueness scope:** Global. UUIDv4 generation ensures no collisions across
  brokers, users, or machines.
* **Generation:** Created on first daemon startup. Persisted in the XDG state
  directory (e.g., `~/.local/state/renotify/daemon_id`). Survives daemon
  restarts.
* **Stability:** Permanent per installation. Reinstalling or wiping state
  generates a new ID.

The daemon instance ID is critical because it anchors workspace identity (see
2.4) and allows the mobile app to distinguish between "the same project on two
different machines."

### 2.4 Workspace

A project directory from which automation pipelines originate. The
human-readable name (e.g., `renotify`) is the directory basename and will
collide across users, machines, and even across daemons on the same machine
(e.g., `~/projects/renotify` vs `/opt/builds/renotify`).

* **Display name:** Directory basename (e.g., `renotify`). NOT unique. Used for
  UI display only.
* **Identifier:** `workspace_id` — a deterministic hash derived from the daemon
  instance and absolute path.
* **Derivation:** `ws_` prefix + first 16 Crockford Base32 characters (80 bits)
  of `SHA-256(daemon_id + "|" + absolute_path)`.
* **Uniqueness scope:** Global. Because `daemon_id` is globally unique and the
  absolute path is unique per filesystem, the composite hash is globally unique.
* **Examples:**
  * `stewart`'s laptop daemon `dn_3G2K7V9WNFQ4J` with path
    `/home/stewart/projects/renotify` → `ws_5MBJR1HXNP3KQ8DW`
  * `stewart`'s CI daemon `dn_9F4HN2TCRWK6Y` with path `/opt/builds/renotify` →
    `ws_R7CV4WFQE2NM1KGX` (different ID, same display name)
* **Generation:** Deterministic — the same daemon + path always
  produces the same ID. See "Workspace Discovery" below for how
  each caller obtains the workspace_id.
* **Stability:** Stable as long as the daemon_id and path don't
  change.

#### Workspace Discovery

Two paths exist for workspace_id computation, depending on the
caller. In both cases, workspaces are created implicitly on first
use — no pre-registration is required.

**CLI path (local computation).** The CLI process computes
`workspace_id` locally before publishing to NATS:

1. Read `daemon_id` from `$XDG_STATE_HOME/renotify/daemon_id`.
2. Get the current working directory via `os.Getwd()`.
3. Compute `SHA-256(daemon_id + "|" + abs_path)`, truncate to
   80 bits, encode as Crockford Base32 with `ws_` prefix.
4. Derive `display_name` from `path.Base(abs_path)`.
5. Include both values in the `FlowLifecycleEvent` and
   `NotificationRequest` payloads.

The daemon receives the `workspace_id` via its JetStream
consumers and caches the mapping (workspace_id → display_name,
abs_path) if it has not seen this workspace before.

**MCP path (daemon computation).** The MCP agent provides the
absolute workspace path in the `register_flow` tool call (the
`workspace_path` field). The daemon computes `workspace_id`
using the same formula, caches the mapping, and returns the
computed `workspace_id` in the `RegisterFlowResult`. The agent
does not need to know the hash formula or the `daemon_id`.

**Fallback behaviour.** When the CLI runs outside a recognisable
project directory (e.g., `/tmp`, `$HOME`), it uses the current
working directory as-is. There is no project-detection heuristic
(no `.git` check, no marker file). The workspace is simply "the
directory where the command was invoked." This keeps the logic
deterministic and avoids magic.

### 2.5 Flow (renamed from "Session")

A single, time-bounded execution of a pipeline or agent conversation within a
workspace. This is the unit of work that the human developer monitors, responds
to, and can interject into.

**Why rename from "Session":**

The term "session" is overloaded in the surrounding ecosystem:
* **MCP sessions** — protocol-level connections between an agent and the MCP
  server.
* **AI agent sessions** — the conversational context of an AI assistant (e.g., a
  Claude Code conversation).
* **HTTP sessions** — web authentication contexts.
* **NATS connections** — sometimes informally called sessions.

Using "session" for Renotify's execution unit creates ambiguity: "the session
timed out" could mean the MCP connection dropped, the agent conversation ended,
or the pipeline's Renotify flow expired. Renaming to **flow** eliminates this
ambiguity.

A **flow** is:
* Created when a CLI command (`renotify post`, `renotify ask`) or MCP tool
  invocation begins.
* Identified by a globally unique `flow_id`.
* Scoped to a single workspace on a single daemon.
* Terminated explicitly (by the originating process) or reaped for staleness (by
  the daemon).

* **Identifier:** `flow_id` — a globally unique opaque ID.
* **Format:** `fl_` prefix + 26 Crockford Base32 characters encoding a full
  128-bit UUIDv7 (e.g., `fl_0R3FABM6NQKJ71XWCD4PG9V2HE`). The full 128 bits are
  retained because flow IDs are the highest-volume identifier and appear in NATS
  subjects where a collision would silently cross-contaminate flows. The UUIDv7
  time component provides natural chronological sorting.
* **Uniqueness scope:** Global. No two flows anywhere in the system will share
  an ID, regardless of user, daemon, workspace, or broker.
* **Generation:** Created by the CLI or MCP server at the moment a flow begins.
  NOT derived from the agent's own session ID.
* **Stability:** Immutable for the lifetime of the flow.

The global uniqueness of `flow_id` is the key design lever. Because it is
globally unique, the NATS subject hierarchy does not need to encode the full
containment path (daemon, workspace) — the flow ID alone is sufficient to route
messages unambiguously.

---

## 3. Identifier Summary Table

| Element             | Identifier        | Format                       | Uniqueness Scope | Generation                                    | Stable Across Restarts   |
|:--------------------|:------------------|:-----------------------------|:-----------------|:----------------------------------------------|:-------------------------|
| NATS Broker         | Connection URL    | `host:port`                  | Global (DNS/IP)  | Infrastructure config                         | Yes                      |
| User                | `username`        | Alphanumeric string          | Per broker       | Configured                                    | Yes                      |
| Daemon Instance     | `daemon_id`       | `dn_` + 13 Base32 (65 bits)  | Global           | Auto-generated, persisted                     | Yes                      |
| Workspace           | `workspace_id`    | `ws_` + 16 Base32 (80 bits)  | Global           | Deterministic hash                            | Yes (same daemon + path) |
| Workspace (display) | `display_name`    | Directory basename           | NOT unique       | Derived from path                             | Yes                      |
| Flow                | `flow_id`         | `fl_` + 26 Base32 (128 bits) | Global           | Generated at flow start                       | N/A (single-use)         |
| Notification        | `notification_id` | `ntf_` + 16 Base32 (80 bits) | Global           | Generated per notification (UUIDv7 truncated) | N/A (single-use)         |
| Mobile Client       | `device_id`       | `mb_` + 13 Base32 (65 bits)  | Per user         | Generated at pairing                          | Yes                      |

---

## 4. NATS Namespace Redesign

### 4.1 Current Design (problematic)

```
resystems.renotify.user.{username}.workspace.{workspace}.session.{session_id}.{event_type}
```

Problems:
1. Encodes the full hierarchy in the subject, making subjects long and rigid.
2. Uses `workspace` as a bare name (collides across users/machines).
3. Uses `session` which is terminologically ambiguous.
4. Doesn't account for daemon instances — a user with two daemons and the same
   workspace name causes collisions.

### 4.2 Proposed Design

Since `flow_id` is globally unique, the subject hierarchy only needs `username`
(for NATS auth/ACL scoping) and `flow_id` (for message routing). All other
hierarchy (daemon, workspace) is carried as payload metadata, not as subject
segments.

**Flow-scoped subjects (JetStream pub/sub):**

```
resystems.renotify.{username}.flow.{flow_id}.request      NotificationRequest
resystems.renotify.{username}.flow.{flow_id}.response      NotificationResponse
resystems.renotify.{username}.flow.{flow_id}.lifecycle     FlowLifecycleEvent
resystems.renotify.{username}.flow.{flow_id}.interject     InterjectionCommand
```

**Daemon-scoped subjects (Core NATS pub/sub):**

```
resystems.renotify.{username}.daemon.{daemon_id}.heartbeat DaemonHeartbeat
```

**Service subjects (Core NATS request-reply):**

```
resystems.renotify.{username}.svc.flows                    ActiveFlowsQuery / ActiveFlowsResult
resystems.renotify.{username}.svc.history                  HistoryQueryRequest / HistoryQueryResult
```

### 4.3 Subscription Patterns

| Subscriber             | Subject Pattern                                         | Purpose                                              |
|:-----------------------|:--------------------------------------------------------|:-----------------------------------------------------|
| Mobile app             | `resystems.renotify.{username}.>`                       | Receives all traffic for this user                   |
| Daemon (flow registry) | `resystems.renotify.{username}.flow.*.lifecycle`        | Maintains the active flow registry                   |
| Daemon (interjections) | `resystems.renotify.{username}.flow.*.interject`        | Routes interjections to the correct flow             |
| Daemon (service)       | `resystems.renotify.{username}.svc.>`                   | Serves query endpoints                               |
| CLI (`ask`, blocking)  | `resystems.renotify.{username}.flow.{flow_id}.response` | Waits for the human's decision on this specific flow |

### 4.4 Benefits Over Current Design

1. **Shorter, flatter subjects.**
   `resystems.renotify.stewart.flow.fl_0R3FABM6NQKJ71XWCD4PG9V2HE.request` vs
   `resystems.renotify.user.stewart.workspace.renotify.session.ses_deploy_9c2b.request`.
   The flow ID is longer (29 chars with prefix) but the subject has fewer
   segments and no collision-prone workspace name.
2. **No workspace name collisions.** Workspace identity is in the payload, not
   the subject. The subject uses the globally unique flow ID.
3. **No daemon ambiguity.** Multiple daemons publish to the same namespace; the
   heartbeat and flow lifecycle payloads carry the daemon_id for the app to
   group correctly.
4. **Simpler ACLs.** NATS auth only needs to scope on
   `resystems.renotify.{username}.>` — one rule per user.
5. **Extensible.** Adding new event types (e.g., `progress`, `log`) just adds a
   new terminal segment under `flow.{flow_id}.*` without restructuring the
   hierarchy.

---

## 5. New Payload: DaemonHeartbeat

With hierarchy removed from subjects, a new heartbeat payload carries the
structural context the mobile app needs to build its dashboard. The daemon
publishes this periodically (e.g., every 30 seconds) and on significant state
changes (workspace added/removed, flow started/ended).

```go
type WorkspaceInfo struct {
    WorkspaceID string   `json:"workspace_id"`
    DisplayName string   `json:"display_name"`
    AbsPath     string   `json:"abs_path"`
    ActiveFlows []string `json:"active_flows"`
}

type DaemonHeartbeat struct {
    DaemonID   string          `json:"daemon_id"`
    Username   string          `json:"username"`
    Hostname   string          `json:"hostname"`
    Workspaces []WorkspaceInfo `json:"workspaces"`
    Timestamp  time.Time       `json:"timestamp"`
}
```

```json
{
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "username": "stewart",
  "hostname": "dev-laptop",
  "workspaces": [
    {
      "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
      "display_name": "renotify",
      "abs_path": "/home/stewart/projects/renotify",
      "active_flows": ["fl_0R3FABM6NQKJ71XWCD4PG9V2HE", "fl_0R3FABM7TP2XE89YWCGKN4QJ5V"]
    },
    {
      "workspace_id": "ws_R7CV4WFQE2NM1KGX",
      "display_name": "gethos-api",
      "abs_path": "/home/stewart/projects/gethos-api",
      "active_flows": []
    }
  ],
  "timestamp": "2026-03-27T14:00:00Z"
}
```

The mobile app builds its dashboard from heartbeats: grouping flows by
workspace, workspaces by daemon, and daemons by hostname. It no longer relies on
subject parsing for this structure.

---

## 6. Impact on Existing Payloads

### 6.1 FlowLifecycleEvent (was SessionLifecycleEvent)

Now carries `daemon_id` and `workspace_id` to link the flow to its structural
context:

```go
type FlowLifecycleEvent struct {
    FlowID      string            `json:"flow_id"`
    DaemonID    string            `json:"daemon_id"`
    WorkspaceID string            `json:"workspace_id"`
    Status      FlowStatus        `json:"status"`
    Label       string            `json:"label,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    Timestamp   time.Time         `json:"timestamp"`
}
```

### 6.2 NotificationRequest

Replaces `workspace` and `session_id` with the new identifiers:

```go
type NotificationRequest struct {
    ID            string         `json:"id"`
    FlowID        string         `json:"flow_id"`
    DaemonID      string         `json:"daemon_id"`
    WorkspaceID   string         `json:"workspace_id"`
    Title         string         `json:"title"`
    Body          string         `json:"body,omitempty"`
    ResponseTypes []ResponseType `json:"response_types"`
    Priority      Priority       `json:"priority"`
    Source        string         `json:"source"`
    Actions       []string       `json:"actions,omitempty"`
    TimeoutSec    int            `json:"timeout_sec,omitempty"`
    Timestamp     time.Time      `json:"timestamp"`
}
```

Note: `daemon_id` and `workspace_id` are included in the payload (not just the
subject) so that the mobile app and history ledger can attribute the
notification without reverse-lookups. These are denormalised from the heartbeat
for self-contained records.

### 6.3 InterjectionCommand

Targets a flow by its globally unique ID. No longer needs a workspace field in
the subject or payload for routing — the flow_id is sufficient:

```go
type InterjectionCommand struct {
    FlowID    string             `json:"flow_id"`
    Action    InterjectionAction `json:"action"`
    Context   string             `json:"context,omitempty"`
    Timestamp time.Time          `json:"timestamp"`
}
```

### 6.4 ActiveFlowsQuery / ActiveFlowsResult (was ActiveSessionsQuery/Result)

Renamed. The query may optionally filter by `workspace_id` or `daemon_id`:

```go
type ActiveFlowsQuery struct {
    DaemonID    string `json:"daemon_id,omitempty"`
    WorkspaceID string `json:"workspace_id,omitempty"`
}

type ActiveFlowsResult struct {
    Flows []FlowLifecycleEvent `json:"flows"`
}
```

### 6.5 Shared Types

Renamed enums:
* `SessionStatus` → `FlowStatus` (values: `active`, `completed`, `failed`)
* `SessionActive/Completed/Failed` → `FlowActive/FlowCompleted/FlowFailed`

### 6.6 Other Payloads (unchanged structurally)

* **NotificationResponse** — correlates via `request_id`, unchanged.
* **ProvisioningPayload** — broker connection details, unchanged.
* **HistoryQueryRequest / HistoryQueryResult** — filter fields rename from
  `session_id` to `flow_id`.
* **ErrorResponse** — correlates via `correlation_id`, unchanged.
* **DecisionResource** — correlates via `request_id`, unchanged.

---

## 7. Impact on Pairing and Provisioning

### 7.1 Two Broker Deployment Models

The embedded NATS broker and the shared enterprise broker are both first-class
deployment models — they are not distinguished as "simple" vs "production." Each
serves a distinct operational context:

* **Embedded broker:** The daemon runs its own NATS server. Ideal for a solo
  developer working independently (e.g., at home, on a personal machine, or
  disconnected from enterprise infrastructure). The mobile app connects directly
  to the daemon. Pairing provisions the daemon's host IP, port, token, and cert
  fingerprint.
* **Shared broker:** An organisation operates a centralised NATS broker.
  Multiple developers and daemons connect as clients. The mobile app connects to
  the shared broker. Pairing provisions the broker's address, token, and cert
  fingerprint.

### 7.2 Pairing Scenarios

| Scenario                        | Broker   | Pairing Behaviour                                           |
|:--------------------------------|:---------|:------------------------------------------------------------|
| Single daemon, solo developer   | Embedded | One pairing, mobile connects to daemon directly             |
| Single daemon, enterprise       | Shared   | One pairing to the shared broker                            |
| Multiple daemons, enterprise    | Shared   | One pairing to the shared broker, all daemons publish there |
| Multiple daemons, each embedded | Embedded | Multiple pairings needed (one per daemon)                   |

The provisioning payload is the same in all cases — it provides the connection
target (whether that is an embedded daemon or a shared broker), an auth token,
and a TLS cert fingerprint. The mobile app does not need to know whether it is
connecting to an embedded or shared broker; the protocol and subject namespace
are identical in both models. The heartbeat payloads from connected daemons
provide the structural context (which daemons, workspaces, and flows exist).

The multi-daemon embedded scenario (multiple pairings) is functional but
ergonomically limited; developers in that situation are better served by a
shared broker. No `mode` field in the ProvisioningPayload is needed — the mobile
app behaviour is the same regardless of broker topology.

---

## 8. Terminology Rename Summary

| Current Term            | Proposed Term        | Reason                                                            |
|:------------------------|:---------------------|:------------------------------------------------------------------|
| Session                 | Flow                 | Avoids collision with MCP sessions, agent sessions, HTTP sessions |
| session_id              | flow_id              | Follows from above                                                |
| SessionLifecycleEvent   | FlowLifecycleEvent   | Follows from above                                                |
| SessionStatus           | FlowStatus           | Follows from above                                                |
| register_session        | register_flow        | MCP tool name                                                     |
| terminate_session       | terminate_flow       | MCP tool name                                                     |
| ActiveSessionsQuery     | ActiveFlowsQuery     | Follows from above                                                |
| ActiveSessionsResult    | ActiveFlowsResult    | Follows from above                                                |
| Active Session Registry | Active Flow Registry | Daemon component name                                             |
| Workspace View          | Workspace View       | **Unchanged** — this is a UI label, not a data model term         |
| Workspaces Dashboard    | Workspaces Dashboard | **Unchanged** — still groups by workspace, now using workspace_id |

---

## 9. Resolved Design Decisions

2. **Heartbeat interval: 30 seconds.** This is a standard interval that balances
   bandwidth with dashboard freshness. The daemon also publishes an immediate
   heartbeat on significant state changes (flow started/ended, workspace
   added/removed), so the 30-second cadence is a staleness backstop rather than
   the primary update mechanism.

3. **Denormalised `daemon_id` and `workspace_id` in flow payloads.**
   `NotificationRequest`, `FlowLifecycleEvent`, and other per-flow payloads
   carry `daemon_id` and `workspace_id` directly, duplicating context available
   from the heartbeat. This denormalisation is deliberate: it makes each payload
   self-contained so the mobile app and history ledger can attribute a
   notification without requiring a heartbeat lookup or maintaining join state.
   The trade-off is marginally larger payloads (two short ID fields), which is
   negligible relative to the title/body content.
