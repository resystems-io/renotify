# Payload Schema Analysis

This document defines the JSON payload schemas for all message types in the
Renotify system. It serves as the authoritative reference for Go struct
definitions and JSON wire formats.

For the system element hierarchy, identifier design, and NATS namespace that
these payloads operate within, see the [Naming & Addressing
Analysis](analysis-naming-and-addressing.md).

## Conventions

* JSON field names use `snake_case`.
* Timestamps are RFC 3339 strings (Go `time.Time`).
* Optional fields carry the `omitempty` struct tag and are omitted from JSON
  when absent.
* The `ProvisioningPayload` uses single-character keys to minimise QR code
  density per R-API-08.
* All generated identifiers use Crockford Base32 encoding. See the Naming &
  Addressing Analysis for format details and truncation rationale.
* `daemon_id` and `workspace_id` are denormalised into per-flow payloads
  (NotificationRequest, FlowLifecycleEvent) so that each record is
  self-contained. The mobile app and history ledger can attribute a notification
  without requiring a heartbeat lookup or maintaining join state.

---

## Payload Enumeration

Each payload's **Transport** identifies the delivery mechanism and its
**Direction** identifies the logical actor-to-actor flow:

* **NATS JetStream** — pub/sub with ephemeral in-memory buffering.
* **NATS Pub/Sub** — plain Core NATS pub/sub without JetStream
  persistence.
* **NATS Request-Reply** — synchronous Core NATS query/response.
* **MCP Tool** — Model Context Protocol tool call (agent invokes
  daemon via stdio transport).
* **MCP Resource** — Model Context Protocol dynamic resource read.
* **Offline (QR)** — out-of-band provisioning via terminal QR code.
* **Any (contextual)** — transport depends on the originating
  request.

| Payload Name | Transport | Direction | Requirement Cross-Ref | ConOps Workflow | Description |
| :--- | :--- | :--- | :--- | :--- | :--- |
| [**NotificationRequest**](#notificationrequest) | NATS JetStream | CLI/Agent -> App | R-API-01, N-01 | W2, W3 | The core domain model representing an interrupt or alert. Contains the title, body, priority, source, and the type of response required. |
| [**NotificationResponse**](#notificationresponse) | NATS JetStream | App -> CLI/Agent | R-API-02, N-03 | W3 | The human decision. Correlates to a `NotificationRequest` ID, capturing the selected action or free-form text input alongside the decision timestamp. |
| [**FlowLifecycleEvent**](#flowlifecycleevent) | NATS JetStream | CLI/Agent -> Daemon | R-API-10, N-04 | W3, W5 | A structured event indicating the birth or death of a distinct pipeline flow. Used by the daemon to maintain the active flow registry. |
| [**RegisterFlowRequest/Result**](#register_flow-mcp-tool) | MCP Tool | Agent -> Daemon | R-CLI-08 | W3, W5 | MCP tool to begin a new flow. Daemon generates `flow_id` and publishes `FlowLifecycleEvent`. |
| [**RefreshFlowRequest/Result**](#refresh_flow-mcp-tool) | MCP Tool | Agent -> Daemon | R-CLI-08 | W5 | MCP tool to signal continued activity and update flow label/metadata. Resets stale reaping timer. |
| [**TerminateFlowRequest/Result**](#terminate_flow-mcp-tool) | MCP Tool | Agent -> Daemon | R-CLI-08 | W3, W5 | MCP tool to end a flow with `completed` or `failed` status. |
| [**PostNotificationRequest/Result**](#post-mcp-tool) | MCP Tool | Agent -> Daemon | R-CLI-08 | W2 | MCP tool for fire-and-forget notification within an active flow. Daemon fills system fields from flow context. |
| [**AskNotificationRequest/Result**](#ask-mcp-tool) | MCP Tool | Agent -> Daemon | R-CLI-08, R-CLI-10 | W3 | MCP tool for non-blocking interactive prompt. Returns `resource_uri` for async `DecisionResource` polling. |
| [**ProvisioningPayload**](#provisioningpayload) | Offline (QR) | CLI -> App | R-API-08, N-01 | W1 | The secure handshake payload containing the target IP, port, auth token, and required TLS certificate fingerprints in a minified JSON map. |
| [**InterjectionCommand**](#interjectioncommand) | NATS JetStream | App -> Daemon/Agent | R-API-09, N-05 | W5 | An asynchronous, unprompted control signal emitted by the user targeting a specific flow by its globally unique flow_id. |
| [**DaemonHeartbeat**](#daemonheartbeat) | NATS Pub/Sub | Daemon -> App | R-CLI-14 | W5 | Periodic structural context (daemon identity, hostname, workspaces, active flows) enabling the mobile dashboard to group and display the system hierarchy. |
| [**ActiveFlowsQuery**](#activeflowsquery) | NATS Request-Reply | App -> Daemon | R-CLI-14, R-MOB-09 | W5 | Core NATS query sent by the Android app to list all currently running flows across the host. |
| [**ActiveFlowsResult**](#activeflowsresult) | NATS Request-Reply | Daemon -> App | R-CLI-14, R-MOB-09 | W5 | The daemon's reply containing the array of currently active `FlowLifecycleEvent` contexts. |
| [**HistoryQueryRequest**](#historyqueryrequest) | NATS Request-Reply | App -> Daemon | R-CLI-13, R-MOB-07 | W4 | Core NATS query sent by the Android app requesting the historical ledger of past notifications and decisions. |
| [**HistoryQueryResult**](#historyqueryresult) | NATS Request-Reply | Daemon -> App | R-CLI-13, R-MOB-07 | W4 | The daemon's structured payload wrapping the requested SQLite history records to be rendered native on the device. |
| [**ErrorResponse**](#errorresponse) | Any (contextual) | Daemon -> Caller | R-API-11, N-04 | W2, W3, W4, W5 | A generic error envelope returned when any request fails at the daemon or broker level. Contains a correlation ID, error code, human-readable message, and timestamp. |
| [**DecisionResource**](#decisionresource) | MCP Resource | Daemon -> Agent | R-CLI-10 | W3 | The MCP dynamic resource exposing a decision result that agents read after receiving the `notifications/resources/updated` notification. |
| [**InterjectionResource**](#interjectionresource) | MCP Resource | Daemon -> Agent | R-API-09, R-CLI-10 | W5 | The MCP dynamic resource exposing the most recent interjection for a flow, enabling agents to react to out-of-band stop/note signals from the mobile user. |

---

## Shared Types

```go
// ResponseType indicates what kind of human feedback a notification expects.
type ResponseType string

const (
	ResponseNone    ResponseType = "none"    // fire-and-forget, no response expected
	ResponseBoolean ResponseType = "boolean" // binary yes/no decision
	ResponseChoice  ResponseType = "choice"  // selection from a list of actions
	ResponseText    ResponseType = "text"    // free-form text input
)

// Priority controls how prominently the Android app renders the notification.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

// FlowStatus represents the current lifecycle state of a pipeline flow.
type FlowStatus string

const (
	FlowActive    FlowStatus = "active"
	FlowCompleted FlowStatus = "completed"
	FlowFailed    FlowStatus = "failed"
)

// InterjectionAction is the type of proactive control signal from the mobile client.
type InterjectionAction string

const (
	InterjectionStop  InterjectionAction = "stop"
	InterjectionPause InterjectionAction = "pause"
	InterjectionNote  InterjectionAction = "note"
)
```

---

## Payload Definitions

### NotificationRequest

The core domain payload representing an interrupt or alert sent from a CLI
command or AI agent to the Android app. The `response_types` field is an array
of `ResponseType` values indicating which kinds of feedback the notification
accepts. For fire-and-forget notifications (`response_types: ["none"]`), the
`actions` and `timeout_sec` fields are omitted. For blocking prompts, `actions`
lists the available choices (when `"choice"` is present) and
`timeout_sec` communicates the timeout to the daemon, which is the
sole enforcer (R-CLI-17). The daemon starts a server-side timer
from the moment it receives the request; on expiry it publishes
an `ErrorResponse` (`code: "timeout"`) to the `.response` subject.
See [NATS Transport Design](analysis-nats-transport-design.md)
Section 3.3. Multi-modal requests (e.g., `["boolean", "text"]`)
allow the user to provide more than one form of feedback simultaneously. The
`daemon_id` and `workspace_id` fields are denormalised from the heartbeat so
that each notification record is self-contained.

The `id` field uses the `ntf_` prefix + 16 Crockford Base32
characters (80 bits from UUIDv7, truncated). Generated by the CLI
process (for `renotify post`/`ask`) or the daemon (for MCP `post`/
`ask` tools) before publishing to NATS. See [Naming &
Addressing](analysis-naming-and-addressing.md) Section 3.

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

Fire-and-forget notification (via `renotify post`):

```json
{
  "id": "ntf_0R3FABM6NQKJ71XW",
  "flow_id": "fl_0R3FABM6NQKJ71XWCD4PG9V2HE",
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "title": "Build complete",
  "body": "All 42 tests passed in 3m12s.",
  "response_types": ["none"],
  "priority": "normal",
  "source": "ci/build-pipeline",
  "timestamp": "2026-03-27T10:15:00Z"
}
```

Blocking choice prompt (via `renotify ask`):

```json
{
  "id": "ntf_4H7DCRW2VNPK9FMJ",
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "title": "Deploy to production?",
  "body": "Image sha256:ab12cd34 is ready. 3 migrations pending.",
  "response_types": ["choice"],
  "priority": "high",
  "source": "cd/deploy-pipeline",
  "actions": ["Approve", "Reject", "Defer"],
  "timeout_sec": 300,
  "timestamp": "2026-03-27T14:30:00Z"
}
```

Multi-modal prompt (boolean with optional text explanation):

```json
{
  "id": "ntf_8X1GBEQ5STNZ3KWR",
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "title": "Proceed with database migration?",
  "body": "3 pending migrations on prod. Rollback requires manual intervention.",
  "response_types": ["boolean", "text"],
  "priority": "high",
  "source": "cd/deploy-pipeline",
  "timeout_sec": 600,
  "timestamp": "2026-03-27T14:30:00Z"
}
```

### NotificationResponse

The human decision returned from the Android app, correlated to a
`NotificationRequest` by `request_id`. The three response fields are orthogonal
and all optional (`omitempty`): `accepted` carries the boolean decision,
`action` carries the selected choice, and `text` carries free-form input. For
multi-modal requests, more than one field may be populated simultaneously (e.g.,
`accepted: false` with an explanatory `text`). The caller inspects whichever
fields are present.

```go
type NotificationResponse struct {
	RequestID string    `json:"request_id"`
	Accepted  *bool     `json:"accepted,omitempty"`
	Action    string    `json:"action,omitempty"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
```

Boolean approval:

```json
{
  "request_id": "ntf_8X1GBEQ5STNZ3KWR",
  "accepted": true,
  "timestamp": "2026-03-27T14:32:15Z"
}
```

Boolean rejection with explanation (multi-modal response):

```json
{
  "request_id": "ntf_8X1GBEQ5STNZ3KWR",
  "accepted": false,
  "text": "Wait for the security audit to close.",
  "timestamp": "2026-03-27T14:33:42Z"
}
```

Choice selection:

```json
{
  "request_id": "ntf_4H7DCRW2VNPK9FMJ",
  "action": "Approve",
  "timestamp": "2026-03-27T14:32:15Z"
}
```

Free-form text response:

```json
{
  "request_id": "ntf_4H7DCRW2VNPK9FMJ",
  "text": "Hold off until the hotfix lands on main.",
  "timestamp": "2026-03-27T14:33:42Z"
}
```

### FlowLifecycleEvent

A structured event marking the start or end of a pipeline flow. Published by the
CLI or MCP agent when a flow begins (`active`) and when it terminates
(`completed` or `failed`). The daemon consumes these to maintain the active flow
registry (R-CLI-14). The `daemon_id` and `workspace_id` fields link the flow to
its structural context. The optional `label` provides a human-readable name for
display on the Android dashboard, and `metadata` carries arbitrary key-value
context.

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

Flow registration:

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "status": "active",
  "label": "Production Deploy",
  "metadata": {
    "branch": "main",
    "commit": "e2e2c55"
  },
  "timestamp": "2026-03-27T14:00:00Z"
}
```

Flow completion:

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "status": "completed",
  "timestamp": "2026-03-27T14:45:00Z"
}
```

### Flow Lifecycle Management

Flow state changes are communicated via `FlowLifecycleEvent` messages
on the NATS lifecycle subject (see [NATS Transport
Design](analysis-nats-transport-design.md) Section 1). Two distinct
paths exist for managing flow lifecycle, depending on the caller:

**CLI path (implicit).** Each CLI command manages its own flow
automatically. `renotify post` and `renotify ask` generate a
`flow_id`, publish a `FlowLifecycleEvent` with `status: active`,
perform their operation, and publish `completed` or `failed` on
exit. One flow per command invocation. The flow is an internal
implementation detail — the user does not interact with flow
management directly. See [NATS Transport
Design](analysis-nats-transport-design.md) Section 8.6 for the
full CLI connection sequence.

**MCP path (explicit).** An AI agent manages flow lifecycle via
dedicated MCP tool calls. This allows a long-lived flow spanning
multiple `post` and `ask` operations within a single agent
conversation. The agent calls `register_flow` to begin, performs
its work, and calls `terminate_flow` to end. The daemon generates
the `flow_id` and publishes the underlying `FlowLifecycleEvent`
on the agent's behalf.

| Operation | CLI Path | MCP Path |
| :--- | :--- | :--- |
| Start a flow | Implicit: CLI generates `flow_id` and publishes `FlowLifecycleEvent` (`active`) | Explicit: agent calls `register_flow` tool; daemon generates `flow_id` and publishes event |
| Send fire-and-forget | `renotify post` (flow_id set internally) | Agent calls `post` tool with `flow_id`; daemon returns `notification_id` |
| Send blocking prompt | `renotify ask` (blocks until response or timeout) | Agent calls `ask` tool (non-blocking); daemon returns `notification_id` + `resource_uri`; agent reads `DecisionResource` asynchronously via `notifications/resources/updated` |
| Signal progress | N/A (CLI flows are short-lived) | Agent calls `refresh_flow` with optional label/metadata update; resets reaping timer |
| End a flow | Implicit: CLI publishes `FlowLifecycleEvent` (`completed` or `failed`) on exit | Explicit: agent calls `terminate_flow` tool; daemon publishes event |
| Stale reaping | Daemon detects CLI process termination (5-min grace, R-CLI-18) | Daemon detects absence of any tool call referencing the flow (5-min grace, R-CLI-18) |

#### `register_flow` MCP Tool

Called by an AI agent to begin a new flow. The daemon generates
the `flow_id`, publishes a `FlowLifecycleEvent` with
`status: active`, adds the flow to the active registry, and
returns the generated identifier to the agent.

**Input parameters:**

```go
type RegisterFlowRequest struct {
	WorkspaceID string            `json:"workspace_id"`
	Label       string            `json:"label,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
```

- `workspace_id` (required) — the workspace this flow belongs to.
  Must match a workspace known to the daemon (present in the
  daemon's heartbeat). The daemon rejects unknown workspace IDs
  with error code `not_found`.
- `label` (optional) — human-readable name for the flow displayed
  on the Android dashboard (e.g., "Code Review", "Deploy Pipeline").
- `metadata` (optional) — arbitrary key-value context attached to
  the `FlowLifecycleEvent` (e.g., branch, commit, agent name).

**Output:**

```go
type RegisterFlowResult struct {
	FlowID    string    `json:"flow_id"`
	Timestamp time.Time `json:"timestamp"`
}
```

The daemon generates the `flow_id` (UUIDv7, Crockford Base32
with `fl_` prefix) and returns it. The agent must use this
`flow_id` in all subsequent `post`, `ask`, and `terminate_flow`
calls within this flow.

**Exemplar — request:**

```json
{
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "label": "Production Deploy",
  "metadata": {
    "branch": "main",
    "commit": "e2e2c55"
  }
}
```

**Exemplar — result:**

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "timestamp": "2026-03-27T14:00:00Z"
}
```

**Error conditions:**

| Error Code | Condition |
| :--- | :--- |
| `not_found` | The `workspace_id` does not match any workspace known to the daemon. |
| `rate_limited` | The agent has exceeded the maximum number of concurrent active flows (R-SYS-01: 20). |
| `internal` | Unexpected daemon-side failure during flow registration. |

Errors are returned as MCP tool error responses. The daemon does
not publish a `FlowLifecycleEvent` when registration fails.

#### `terminate_flow` MCP Tool

Called by an AI agent to end a flow. The daemon publishes a
`FlowLifecycleEvent` with the specified status, removes the flow
from the active registry, and confirms termination to the agent.

**Input parameters:**

```go
type TerminateFlowRequest struct {
	FlowID string     `json:"flow_id"`
	Status FlowStatus `json:"status"`
}
```

- `flow_id` (required) — the flow to terminate. Must be an active
  flow registered by this agent.
- `status` (required) — the terminal state: `"completed"` for
  successful conclusion or `"failed"` for abnormal termination.
  The value `"active"` is rejected as invalid.

**Output:**

```go
type TerminateFlowResult struct {
	FlowID    string    `json:"flow_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}
```

**Exemplar — request (successful completion):**

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "status": "completed"
}
```

**Exemplar — result:**

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "status": "completed",
  "timestamp": "2026-03-27T14:45:00Z"
}
```

**Error conditions:**

| Error Code | Condition |
| :--- | :--- |
| `not_found` | The `flow_id` is not in the active flow registry (already terminated, reaped, or never registered). |
| `internal` | Unexpected daemon-side failure during termination. |

#### `refresh_flow` MCP Tool

Called by an AI agent to signal continued activity on a
long-running flow. Resets the stale reaping timer and optionally
updates the flow's display label and metadata on the mobile
dashboard. The daemon publishes a `FlowLifecycleEvent` with
`status: active` and the updated fields. Agents should call this
between major work steps during tasks that may exceed the
5-minute reaping grace period.

**Input parameters:**

```go
type RefreshFlowRequest struct {
	FlowID   string            `json:"flow_id"`
	Label    string            `json:"label,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
```

- `flow_id` (required) — the active flow to refresh.
- `label` (optional) — updated display name for the mobile
  dashboard (e.g., "Running tests 14/42"). Overwrites the
  prior label.
- `metadata` (optional) — updated key-value context. Merged
  with existing metadata: new keys are added, existing keys
  are overwritten, keys absent from the update are retained.

**Output:**

```go
type RefreshFlowResult struct {
	FlowID    string    `json:"flow_id"`
	Timestamp time.Time `json:"timestamp"`
}
```

**Exemplar — request:**

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "label": "Analysing 12 files..."
}
```

**Exemplar — result:**

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "timestamp": "2026-03-27T14:20:00Z"
}
```

**Error conditions:**

| Error Code | Condition |
| :--- | :--- |
| `not_found` | The `flow_id` is not in the active flow registry. |
| `internal` | Unexpected daemon-side failure. |

A typical MCP agent workflow using `refresh_flow`:

```
register_flow  → post "Starting code review"  → [work] →
refresh_flow "Analysing 12 files..."           → [work] →
ask "Found 3 issues, proceed?"                 → [wait] →
refresh_flow "Applying fixes..."               → [work] →
terminate_flow completed
```

**Future consideration:** A free-form `status` field (distinct
from `label`) may be added to `RefreshFlowRequest` and
`FlowLifecycleEvent` to carry a transient progress message
(e.g., "Compiling module 3 of 7") separately from the
human-readable flow name. This would allow the mobile dashboard
to display both a stable title and a changing status line. This
is deferred to avoid adding fields before their UI rendering is
designed.

#### `post` MCP Tool

Called by an AI agent to send a fire-and-forget notification
within an active flow. The daemon generates the notification ID,
fills in system fields (`daemon_id`, `workspace_id`, `timestamp`)
from the flow's registration context, publishes the
`NotificationRequest` to NATS, inserts it into the SQLite ledger,
and returns the generated ID.

**Input parameters:**

```go
type PostNotificationRequest struct {
	FlowID   string   `json:"flow_id"`
	Title    string   `json:"title"`
	Body     string   `json:"body,omitempty"`
	Priority Priority `json:"priority,omitempty"`
	Source   string   `json:"source,omitempty"`
}
```

- `flow_id` (required) — the active flow this notification
  belongs to. Must have been returned by a prior `register_flow`
  call.
- `title` (required) — notification title displayed on the
  mobile app.
- `body` (optional) — notification body text.
- `priority` (optional) — `"low"`, `"normal"` (default), or
  `"high"`.
- `source` (optional) — identifies the originating pipeline or
  agent (e.g., `"ci/build-pipeline"`, `"claude-code"`).

The daemon sets `response_types` to `["none"]` automatically.
The `actions` and `timeout_sec` fields do not apply to
fire-and-forget notifications.

**Output:**

```go
type PostNotificationResult struct {
	NotificationID string    `json:"notification_id"`
	Timestamp      time.Time `json:"timestamp"`
}
```

**Exemplar — request:**

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "title": "Build complete",
  "body": "All 42 tests passed in 3m12s.",
  "source": "ci/build-pipeline"
}
```

**Exemplar — result:**

```json
{
  "notification_id": "ntf_0R3FABM6NQKJ71XW",
  "timestamp": "2026-03-27T10:15:00Z"
}
```

**Error conditions:**

| Error Code | Condition |
| :--- | :--- |
| `not_found` | The `flow_id` is not in the active flow registry. |
| `rate_limited` | The flow has exceeded the per-flow notification rate limit (R-CLI-16). |
| `internal` | Unexpected daemon-side failure. |

#### `ask` MCP Tool

Called by an AI agent to send a notification that requires a
human response. Unlike the CLI `ask` command (which blocks until
a response arrives), the MCP `ask` tool **returns immediately**
with a notification ID and a `DecisionResource` URI. The agent
receives the human's decision asynchronously via the MCP
`notifications/resources/updated` event pattern (R-CLI-10).

**Input parameters:**

```go
type AskNotificationRequest struct {
	FlowID        string         `json:"flow_id"`
	Title         string         `json:"title"`
	Body          string         `json:"body,omitempty"`
	ResponseTypes []ResponseType `json:"response_types"`
	Priority      Priority       `json:"priority,omitempty"`
	Source        string         `json:"source,omitempty"`
	Actions       []string       `json:"actions,omitempty"`
	TimeoutSec    int            `json:"timeout_sec,omitempty"`
}
```

- `flow_id` (required) — the active flow this notification
  belongs to.
- `title` (required) — notification title.
- `body` (optional) — notification body text.
- `response_types` (required) — array of accepted response
  types (e.g., `["boolean", "text"]`). Must not include
  `"none"` (use `post` for fire-and-forget).
- `priority` (optional) — default `"normal"`.
- `source` (optional) — originating pipeline/agent identifier.
- `actions` (required when `"choice"` is in `response_types`) —
  list of choice labels (e.g., `["Approve", "Reject", "Defer"]`).
- `timeout_sec` (optional) — server-side timeout in seconds.
  Default from `timeout.default_ask_timeout` config (5 minutes).

**Output:**

```go
type AskNotificationResult struct {
	NotificationID string    `json:"notification_id"`
	ResourceURI    string    `json:"resource_uri"`
	Timestamp      time.Time `json:"timestamp"`
}
```

- `notification_id` — the generated notification ID. Correlates
  to `DecisionResource.request_id`.
- `resource_uri` — the MCP resource URI the agent reads to
  obtain the decision (e.g.,
  `renotify://decisions/ntf_4H7DCRW2VNPK9FMJ`).
- `timestamp` — when the notification was published.

**Exemplar — request:**

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "title": "Deploy to production?",
  "body": "Image sha256:ab12cd34 is ready. 3 migrations pending.",
  "response_types": ["choice"],
  "priority": "high",
  "source": "cd/deploy-pipeline",
  "actions": ["Approve", "Reject", "Defer"],
  "timeout_sec": 300
}
```

**Exemplar — result:**

```json
{
  "notification_id": "ntf_4H7DCRW2VNPK9FMJ",
  "resource_uri": "renotify://decisions/ntf_4H7DCRW2VNPK9FMJ",
  "timestamp": "2026-03-27T14:30:00Z"
}
```

**Error conditions:**

| Error Code | Condition |
| :--- | :--- |
| `not_found` | The `flow_id` is not in the active flow registry. |
| `rate_limited` | The flow has exceeded the per-flow notification rate limit (R-CLI-16). |
| `unroutable` | No mobile client is connected to receive the notification. |
| `internal` | Unexpected daemon-side failure. |

#### MCP `ask` Decision Flow

The MCP `ask` tool is non-blocking. The agent receives the
human's decision asynchronously through the MCP resource
subscription pattern (R-CLI-10). The complete sequence:

1. **Agent calls `ask` tool.** The daemon generates a
   notification ID, publishes the `NotificationRequest` to
   NATS, creates a pending `DecisionResource`, and returns the
   `AskNotificationResult` with the resource URI.

2. **Mobile app renders the notification.** The user sees the
   prompt with the requested response controls (boolean buttons,
   choice list, text field, or a combination).

3. **User responds.** The mobile app publishes a
   `NotificationResponse` to NATS. The daemon receives it,
   inserts it into the SQLite ledger, and updates the
   `DecisionResource` with the decision fields (`accepted`,
   `action`, `text`) and sets `decided: true`.

4. **Daemon sends MCP notification.** The daemon emits a
   `notifications/resources/updated` event to all connected MCP
   clients, referencing the updated resource URI.

5. **Agent reads the decision.** The agent receives the SSE
   event and reads the `DecisionResource` at the resource URI.
   The resource now contains the human's decision:

   ```json
   {
     "request_id": "ntf_4H7DCRW2VNPK9FMJ",
     "decided": true,
     "action": "Approve",
     "timestamp": "2026-03-27T14:32:15Z"
   }
   ```

6. **Timeout handling.** If the timeout expires before the human
   responds, the daemon updates the `DecisionResource` with
   `decided: true` and no response fields (indicating timeout),
   publishes a `FlowLifecycleEvent` with `status: failed`, and
   emits the `notifications/resources/updated` event. The agent
   detects timeout by reading a `DecisionResource` where
   `decided: true` but `accepted`, `action`, and `text` are all
   absent.

**Timeout detection exemplar:**

```json
{
  "request_id": "ntf_4H7DCRW2VNPK9FMJ",
  "decided": true,
  "timestamp": "2026-03-27T14:35:00Z"
}
```

When `decided` is `true` but all response fields are absent, the
agent knows the request timed out.

#### Stale Flow Reaping

The daemon resets the stale reaping timer whenever any MCP tool
call references a flow (`post`, `ask`, `refresh_flow`,
`terminate_flow`). This means an agent that is actively sending
notifications does not need to call `refresh_flow` explicitly —
any interaction with the flow keeps it alive.

If no tool call references the flow for the configurable grace
period (default: 5 minutes, R-CLI-18), the daemon marks the
flow as `failed` and publishes a `FlowLifecycleEvent` with
`status: failed`. The same mechanism applies to CLI flows whose
originating process has terminated. The mobile app and history
ledger observe this event via their normal subscriptions.

### ProvisioningPayload

The secure handshake payload encoded as minified JSON inside a QR code during
`renotify pair`. Field names are single characters to minimise QR density
(R-API-08):
- The `h` field carries the connection target as an IP address or hostname
(e.g., `192.168.1.42` for an embedded broker, or a DNS name for a shared
broker).
- The `p` field carries the WSS port (default 4223 for the embedded broker).
- The `t` field carries the NATS authentication token (`rn_tk_` prefix + 52
Crockford Base32 characters, 256-bit entropy). See [NATS Transport
Design](analysis-nats-transport-design.md) Section 6.
- The `c` field carries the hex-encoded SHA-256 fingerprint of the TLS
certificate used by the connection target (whether that is an embedded daemon or
a shared broker), which the Android app pins for all subsequent connections via
a custom `X509TrustManager` (see [NATS Transport
Design](analysis-nats-transport-design.md) Section 5.5).

```go
type ProvisioningPayload struct {
	Host    string `json:"h"`
	Port    int    `json:"p"`
	Token   string `json:"t"`
	CertSHA string `json:"c"`
}
```

```json
{"h":"192.168.1.42","p":4223,"t":"rn_tk_0A1B2C3D4E5F6G7H8J9K0M1N2P3Q4R5S6T7V8W9X0Y1Z2A3B4C5D","c":"b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"}
```

#### QR Encoding Parameters

The minified JSON payload above is approximately 170 bytes. The
following parameters govern how it is encoded and rendered as a QR code
during `renotify pair`:

| Parameter | Value | Rationale |
| :--- | :--- | :--- |
| Library | [`mdp/qrterminal`][qrterminal] | Terminal-native API, actively maintained (v3.2.1, March 2025), half-block rendering is first-class. Underlying encoder is `rsc.io/qr` (Russ Cox). |
| Error correction | Level L (7% recovery) | The QR code is displayed on a screen and scanned at close range in a controlled environment. Level L minimises module count for a given payload, producing a smaller and faster-to-scan code. |
| QR version | ~7 (45x45 modules) | 170 bytes of alphanumeric/byte data at EC level L fits comfortably in version 7 (capacity: 224 bytes binary). The library selects the minimum version automatically. |
| Terminal rendering | Unicode half-block characters | Uses `U+2580` / `U+2584` / `U+2588` to pack two module rows per terminal line, halving the vertical footprint to ~23 terminal rows for a version 7 code. |
| Quiet zone | 1 module (library default) | The ISO 18004 standard specifies a 4-module quiet zone, but 1 module is sufficient for screen-to-camera scanning where the terminal background provides contrast. |

Terminal output (single function call in the `pair` command):

```go
qrterminal.GenerateHalfBlock(
	provisioningJSON, qrterminal.L, os.Stdout,
)
```

On the Android side, the QR code is scanned using the device camera
(R-MOB-06). The recommended library is [Google ML Kit Barcode
Scanning][mlkit-barcode], which supports all QR versions and error
correction levels, runs on-device without network access, and is
actively maintained as part of the ML Kit SDK.

### InterjectionCommand

An asynchronous, unprompted control signal emitted by the developer from the
Android app targeting a specific flow by its globally unique `flow_id`
(R-API-09). A `stop` action requests graceful termination, `pause` requests the
pipeline to suspend, and `note` delivers free-form context via the `context`
field without altering execution. The flow_id is sufficient for routing; no
workspace or daemon fields are needed.

```go
type InterjectionCommand struct {
	FlowID    string             `json:"flow_id"`
	Action    InterjectionAction `json:"action"`
	Context   string             `json:"context,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
}
```

Stop command:

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "action": "stop",
  "timestamp": "2026-03-27T14:35:00Z"
}
```

Free-form note:

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "action": "note",
  "context": "Check the connection pool config before proceeding.",
  "timestamp": "2026-03-27T14:36:00Z"
}
```

### DaemonHeartbeat

Periodic structural context published by each daemon instance to inform the
mobile app's dashboard. The heartbeat carries the daemon's identity, hostname,
and a snapshot of its workspaces with their active flow IDs. Published every 30
seconds as a staleness backstop, and immediately on significant state changes
(flow started/ended, workspace added/removed). Transported over plain NATS
Pub/Sub (not JetStream) because heartbeats are ephemeral — a missed heartbeat is
superseded by the next one.

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

### ActiveFlowsQuery

A Core NATS Request-Reply query sent by the Android app to the daemon to list
currently active flows (R-CLI-14, R-MOB-09). Optional filters narrow results to
a specific daemon or workspace; when omitted, all active flows are returned.

```go
type ActiveFlowsQuery struct {
	DaemonID    string `json:"daemon_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}
```

Unfiltered (all flows):

```json
{}
```

Filtered to one workspace:

```json
{
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW"
}
```

### ActiveFlowsResult

The daemon's reply to an `ActiveFlowsQuery`, containing an array of
`FlowLifecycleEvent` records representing currently active flows. An empty
`flows` array indicates no active work.

```go
type ActiveFlowsResult struct {
	Flows []FlowLifecycleEvent `json:"flows"`
}
```

```json
{
  "flows": [
    {
      "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
      "daemon_id": "dn_3G2K7V9WNFQ4J",
      "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
      "status": "active",
      "label": "Production Deploy",
      "timestamp": "2026-03-27T14:00:00Z"
    },
    {
      "flow_id": "fl_0R3FABM6NQKJ71XWCD4PG9V2HE",
      "daemon_id": "dn_3G2K7V9WNFQ4J",
      "workspace_id": "ws_R7CV4WFQE2NM1KGX",
      "status": "active",
      "label": "Lint & Vet",
      "metadata": {
        "branch": "feature/auth"
      },
      "timestamp": "2026-03-27T14:10:00Z"
    }
  ]
}
```

### HistoryQueryRequest

A Core NATS Request-Reply query sent by the Android app to retrieve historical
notification records from the daemon's SQLite ledger (R-CLI-13, R-MOB-07). All
fields are optional filters; when all are omitted the daemon returns the most
recent records up to its configured default limit.

```go
type HistoryQueryRequest struct {
	WorkspaceID string     `json:"workspace_id,omitempty"`
	FlowID      string     `json:"flow_id,omitempty"`
	Since       *time.Time `json:"since,omitempty"`
	Until       *time.Time `json:"until,omitempty"`
	Limit       int        `json:"limit,omitempty"`
}
```

```json
{
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "limit": 25
}
```

### HistoryQueryResult

The daemon's reply to a `HistoryQueryRequest`, wrapping an array of
`HistoryRecord` entries. Each record pairs the original
`NotificationRequest` with its `NotificationResponse` (if one was
received). The underlying storage model, query patterns, and
indices are defined in the [SQLite Ledger
Schema](analysis-sqlite-ledger.md). The `total` field reports the
full count of matching records, allowing the client to detect when results
have been truncated by the `limit`.

```go
type HistoryRecord struct {
	Request  NotificationRequest   `json:"request"`
	Response *NotificationResponse `json:"response,omitempty"`
}

type HistoryQueryResult struct {
	Records []HistoryRecord `json:"records"`
	Total   int             `json:"total"`
}
```

```json
{
  "records": [
    {
      "request": {
        "id": "ntf_4H7DCRW2VNPK9FMJ",
        "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
        "daemon_id": "dn_3G2K7V9WNFQ4J",
        "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
        "title": "Deploy to production?",
        "response_types": ["choice"],
        "priority": "high",
        "source": "cd/deploy-pipeline",
        "actions": ["Approve", "Reject", "Defer"],
        "timeout_sec": 300,
        "timestamp": "2026-03-27T14:30:00Z"
      },
      "response": {
        "request_id": "ntf_4H7DCRW2VNPK9FMJ",
        "action": "Approve",
        "timestamp": "2026-03-27T14:32:15Z"
      }
    },
    {
      "request": {
        "id": "ntf_0R3FABM6NQKJ71XW",
        "flow_id": "fl_0R3FABM6NQKJ71XWCD4PG9V2HE",
        "daemon_id": "dn_3G2K7V9WNFQ4J",
        "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
        "title": "Build complete",
        "response_types": ["none"],
        "priority": "normal",
        "source": "ci/build-pipeline",
        "timestamp": "2026-03-27T10:15:00Z"
      }
    }
  ],
  "total": 2
}
```

### ErrorResponse

A generic error envelope returned when any request fails at the daemon level
(R-API-11). The `correlation_id` matches the `id` of the originating request (or
is empty for unsolicited errors). The `code` field uses a fixed set of string
codes to enable programmatic error handling:

* `timeout` — blocking request expired without a human response.
* `rate_limited` — per-flow notification rate limit exceeded (R-CLI-16).
* `not_found` — referenced flow or notification does not exist.
* `unroutable` — no mobile client connected to receive the notification.
* `internal` — unexpected daemon-side failure.

```go
type ErrorResponse struct {
	CorrelationID string    `json:"correlation_id,omitempty"`
	Code          string    `json:"code"`
	Message       string    `json:"message"`
	Timestamp     time.Time `json:"timestamp"`
}
```

Timeout error:

```json
{
  "correlation_id": "ntf_4H7DCRW2VNPK9FMJ",
  "code": "timeout",
  "message": "No response received within 300s.",
  "timestamp": "2026-03-27T14:35:00Z"
}
```

Rate-limit rejection:

```json
{
  "correlation_id": "ntf_2PQVFN8YKBM4XWDH",
  "code": "rate_limited",
  "message": "Flow fl_0R3FABM7TP2XE89YWCGKN4QJ5V exceeded 60 notifications/min.",
  "timestamp": "2026-03-27T15:00:01Z"
}
```

### DecisionResource

The MCP dynamic resource that agents read after receiving a
`notifications/resources/updated` notification (R-CLI-10). This is not a NATS
message; it is served by the daemon's MCP server as a resource. The `decided`
flag indicates whether a human response has been received. While `decided` is
`false`, the `accepted`, `action`, and `text` fields are absent. Once decided,
the resource is immutable. The response fields mirror `NotificationResponse`:
`accepted` for boolean decisions, `action` for choice selections, and `text` for
free-form input.

```go
type DecisionResource struct {
	RequestID string    `json:"request_id"`
	Decided   bool      `json:"decided"`
	Accepted  *bool     `json:"accepted,omitempty"`
	Action    string    `json:"action,omitempty"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
```

Pending (not yet decided):

```json
{
  "request_id": "ntf_4H7DCRW2VNPK9FMJ",
  "decided": false,
  "timestamp": "2026-03-27T14:30:00Z"
}
```

Decided:

```json
{
  "request_id": "ntf_4H7DCRW2VNPK9FMJ",
  "decided": true,
  "action": "Approve",
  "timestamp": "2026-03-27T14:32:15Z"
}
```

### InterjectionResource

The MCP dynamic resource that agents read after receiving a
`notifications/resources/updated` notification referencing an
interjection. Served at
`renotify://interjections/{flow_id}`. Contains the most recent
interjection for the flow. The daemon updates this resource each
time a new `InterjectionCommand` arrives for the flow.

This mirrors the `DecisionResource` pattern: the daemon emits
the MCP notification, and the agent reads the resource to obtain
the details. See [NATS Transport
Design](analysis-nats-transport-design.md) Section 8.8 for the
full interjection delivery path.

```go
type InterjectionResource struct {
	FlowID    string             `json:"flow_id"`
	Action    InterjectionAction `json:"action"`
	Context   string             `json:"context,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
}
```

Stop interjection:

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "action": "stop",
  "timestamp": "2026-03-27T14:35:00Z"
}
```

Note with context:

```json
{
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "action": "note",
  "context": "Check the connection pool config before proceeding.",
  "timestamp": "2026-03-27T14:36:00Z"
}
```

[qrterminal]: https://github.com/mdp/qrterminal
[mlkit-barcode]: https://developers.google.com/ml-kit/vision/barcode-scanning
