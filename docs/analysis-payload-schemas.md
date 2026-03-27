# Payload Schema Analysis

This document defines the JSON payload schemas for all message types in the
Renotify system. It serves as the authoritative reference for Go struct
definitions and JSON wire formats.

For the system element hierarchy, identifier design, and NATS namespace that
these payloads operate within, see the
[Naming & Addressing Analysis](analysis-naming-and-addressing.md).

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
* **NATS Pub/Sub** — plain Core NATS pub/sub without JetStream persistence.
* **NATS Request-Reply** — synchronous Core NATS query/response.
* **MCP Resource** — Model Context Protocol dynamic resource read.
* **Offline (QR)** — out-of-band provisioning via terminal QR code.
* **Any (contextual)** — transport depends on the originating request.

| Payload Name | Transport | Direction | Requirement Cross-Ref | ConOps Workflow | Description |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **NotificationRequest** | NATS JetStream | CLI/Agent -> App | R-API-01, N-01 | W2, W3 | The core domain model representing an interrupt or alert. Contains the title, body, priority, source, and the type of response required. |
| **NotificationResponse** | NATS JetStream | App -> CLI/Agent | R-API-02, N-03 | W3 | The human decision. Correlates to a `NotificationRequest` ID, capturing the selected action or free-form text input alongside the decision timestamp. |
| **FlowLifecycleEvent** | NATS JetStream | CLI/Agent -> Daemon | R-API-10, N-04 | W3, W5 | A structured event indicating the birth or death of a distinct pipeline flow. Used by the daemon to maintain the active flow registry. |
| **ProvisioningPayload** | Offline (QR) | CLI -> App | R-API-08, N-01 | W1 | The secure handshake payload containing the target IP, port, auth token, and required TLS certificate fingerprints in a minified JSON map. |
| **InterjectionCommand** | NATS JetStream | App -> Daemon/Agent | R-API-09, N-05 | W5 | An asynchronous, unprompted control signal emitted by the user targeting a specific flow by its globally unique flow_id. |
| **DaemonHeartbeat** | NATS Pub/Sub | Daemon -> App | R-CLI-14 | W5 | Periodic structural context (daemon identity, hostname, workspaces, active flows) enabling the mobile dashboard to group and display the system hierarchy. |
| **ActiveFlowsQuery** | NATS Request-Reply | App -> Daemon | R-CLI-14, R-MOB-09 | W5 | Core NATS query sent by the Android app to list all currently running flows across the host. |
| **ActiveFlowsResult** | NATS Request-Reply | Daemon -> App | R-CLI-14, R-MOB-09 | W5 | The daemon's reply containing the array of currently active `FlowLifecycleEvent` contexts. |
| **HistoryQueryRequest** | NATS Request-Reply | App -> Daemon | R-CLI-13, R-MOB-07 | W4 | Core NATS query sent by the Android app requesting the historical ledger of past notifications and decisions. |
| **HistoryQueryResult** | NATS Request-Reply | Daemon -> App | R-CLI-13, R-MOB-07 | W4 | The daemon's structured payload wrapping the requested SQLite history records to be rendered native on the device. |
| **ErrorResponse** | Any (contextual) | Daemon -> Caller | R-API-11, N-04 | W2, W3, W4, W5 | A generic error envelope returned when any request fails at the daemon or broker level. Contains a correlation ID, error code, human-readable message, and timestamp. |
| **DecisionResource** | MCP Resource | Daemon -> Agent | R-CLI-10 | W3 | The MCP dynamic resource exposing a decision result that agents read after receiving the `notifications/resources/updated` notification. |

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
command or AI agent to the Android app. For fire-and-forget notifications
(`response_type: "none"`), the `actions` and `timeout_sec` fields are omitted.
For blocking prompts, `actions` lists the available choices and `timeout_sec`
sets the server-side deadline. The `daemon_id` and `workspace_id` fields are
denormalised from the heartbeat so that each notification record is
self-contained.

```go
type NotificationRequest struct {
	ID           string       `json:"id"`
	FlowID       string       `json:"flow_id"`
	DaemonID     string       `json:"daemon_id"`
	WorkspaceID  string       `json:"workspace_id"`
	Title        string       `json:"title"`
	Body         string       `json:"body,omitempty"`
	ResponseType ResponseType `json:"response_type"`
	Priority     Priority     `json:"priority"`
	Source       string       `json:"source"`
	Actions      []string     `json:"actions,omitempty"`
	TimeoutSec   int          `json:"timeout_sec,omitempty"`
	Timestamp    time.Time    `json:"timestamp"`
}
```

Fire-and-forget notification (via `renotify post`):

```json
{
  "id": "ntf_a1b2c3d4",
  "flow_id": "fl_0R3FABM6NQKJ71XWCD4PG9V2HE",
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "title": "Build complete",
  "body": "All 42 tests passed in 3m12s.",
  "response_type": "none",
  "priority": "normal",
  "source": "ci/build-pipeline",
  "timestamp": "2026-03-27T10:15:00Z"
}
```

Blocking interactive prompt (via `renotify ask`):

```json
{
  "id": "ntf_e5f6g7h8",
  "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
  "daemon_id": "dn_3G2K7V9WNFQ4J",
  "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
  "title": "Deploy to production?",
  "body": "Image sha256:ab12cd34 is ready. 3 migrations pending.",
  "response_type": "choice",
  "priority": "high",
  "source": "cd/deploy-pipeline",
  "actions": ["Approve", "Reject", "Defer"],
  "timeout_sec": 300,
  "timestamp": "2026-03-27T14:30:00Z"
}
```

### NotificationResponse

The human decision returned from the Android app, correlated to a
`NotificationRequest` by `request_id`. For choice and boolean responses,
`action` carries the selected option. For free-form text responses, `text`
carries the input. Both may be present if the UI allows a comment alongside a
choice.

```go
type NotificationResponse struct {
	RequestID string    `json:"request_id"`
	Action    string    `json:"action,omitempty"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
```

Choice selection:

```json
{
  "request_id": "ntf_e5f6g7h8",
  "action": "Approve",
  "timestamp": "2026-03-27T14:32:15Z"
}
```

Free-form text response:

```json
{
  "request_id": "ntf_e5f6g7h8",
  "text": "Hold off until the hotfix lands on main.",
  "timestamp": "2026-03-27T14:33:42Z"
}
```

### FlowLifecycleEvent

A structured event marking the start or end of a pipeline flow. Published by
the CLI or MCP agent when a flow begins (`active`) and when it terminates
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

### ProvisioningPayload

The secure handshake payload encoded as minified JSON inside a QR code during
`renotify pair`. Field names are single characters to minimise QR density
(R-API-08). The `c` field carries the hex-encoded SHA-256 fingerprint of the
TLS certificate used by the connection target (whether that is an embedded
daemon or a shared broker), which the Android app pins for all subsequent
connections.

```go
type ProvisioningPayload struct {
	Host    string `json:"h"`
	Port    int    `json:"p"`
	Token   string `json:"t"`
	CertSHA string `json:"c"`
}
```

```json
{"h":"192.168.1.42","p":4222,"t":"rn_tk_8a3b5c7d9e","c":"b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"}
```

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
Pub/Sub (not JetStream) because heartbeats are ephemeral — a missed heartbeat
is superseded by the next one.

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
`HistoryRecord` entries. Each record pairs the original `NotificationRequest`
with its `NotificationResponse` (if one was received). The `total` field reports
the full count of matching records, allowing the client to detect when results
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
        "id": "ntf_e5f6g7h8",
        "flow_id": "fl_0R3FABM7TP2XE89YWCGKN4QJ5V",
        "daemon_id": "dn_3G2K7V9WNFQ4J",
        "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
        "title": "Deploy to production?",
        "response_type": "choice",
        "priority": "high",
        "source": "cd/deploy-pipeline",
        "actions": ["Approve", "Reject", "Defer"],
        "timeout_sec": 300,
        "timestamp": "2026-03-27T14:30:00Z"
      },
      "response": {
        "request_id": "ntf_e5f6g7h8",
        "action": "Approve",
        "timestamp": "2026-03-27T14:32:15Z"
      }
    },
    {
      "request": {
        "id": "ntf_a1b2c3d4",
        "flow_id": "fl_0R3FABM6NQKJ71XWCD4PG9V2HE",
        "daemon_id": "dn_3G2K7V9WNFQ4J",
        "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
        "title": "Build complete",
        "response_type": "none",
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
(R-API-11). The `correlation_id` matches the `id` of the originating request
(or is empty for unsolicited errors). The `code` field uses a fixed set of
string codes to enable programmatic error handling:

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
  "correlation_id": "ntf_e5f6g7h8",
  "code": "timeout",
  "message": "No response received within 300s.",
  "timestamp": "2026-03-27T14:35:00Z"
}
```

Rate-limit rejection:

```json
{
  "correlation_id": "ntf_r4s5t6u7",
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
`false`, the `action` and `text` fields are absent. Once decided, the resource
is immutable.

```go
type DecisionResource struct {
	RequestID string    `json:"request_id"`
	Decided   bool      `json:"decided"`
	Action    string    `json:"action,omitempty"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
```

Pending (not yet decided):

```json
{
  "request_id": "ntf_e5f6g7h8",
  "decided": false,
  "timestamp": "2026-03-27T14:30:00Z"
}
```

Decided:

```json
{
  "request_id": "ntf_e5f6g7h8",
  "decided": true,
  "action": "Approve",
  "timestamp": "2026-03-27T14:32:15Z"
}
```
