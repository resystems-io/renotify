# SQLite Ledger Schema

This document defines the persistent storage schema for the Renotify
daemon's SQLite database. It covers the notification history ledger,
the active flow registry, and the flow lifecycle audit log.

For the payload definitions stored in these tables, see the
[Payload Schema Analysis](analysis-payload-schemas.md). For the NATS
transport that feeds data into the ledger, see the
[NATS Transport Design](analysis-nats-transport-design.md).

This document addresses refinement item **A-05** (SQLite Ledger
Schema).

---

## 1. Storage Location and Conventions

### 1.1 Database Path

The daemon stores its SQLite database under the XDG state directory
(R-CLI-09):

```
$XDG_STATE_HOME/renotify/renotify.db
```

`$XDG_STATE_HOME` defaults to `~/.local/state`. Each running daemon
process operates its own database file. The database is created on
first daemon startup if it does not exist.

### 1.2 Data Conventions

* **Timestamps:** RFC 3339 strings stored as `TEXT` columns (e.g.,
  `"2026-03-27T14:30:00Z"`). All timestamps are UTC. This is
  consistent with the payload schema conventions and allows direct
  serialisation to JSON without conversion. SQLite's `datetime()`
  functions operate correctly on ISO 8601 strings.
* **JSON fields:** Arrays (`response_types`, `actions`) and maps
  (`metadata`) are stored as JSON-encoded `TEXT` columns. The
  application layer serialises and deserialises these using
  `encoding/json`. No dependency on SQLite's JSON1 extension.
* **Identifiers:** All identifier columns use `TEXT` with no fixed
  length constraint. Format validation (prefixes, Base32 encoding)
  is enforced at the application layer, not via SQL constraints.
  This allows identifier formats to evolve without schema migration.
* **Enums:** Status and priority values use `TEXT` columns with
  `CHECK` constraints for defence in depth. The application layer
  is the primary enforcement point.
* **Nullability:** Optional payload fields (`body`, `label`,
  `metadata`, `actions`, `timeout_sec`, `accepted`, `action`,
  `text`) map to nullable columns. Required fields are `NOT NULL`.

### 1.3 Schema Versioning

The database uses SQLite's built-in `PRAGMA user_version` to track
the schema version. On startup the daemon reads the current version
and applies any outstanding migrations:

```go
var version int
err := db.QueryRow("PRAGMA user_version").Scan(&version)

// Apply migrations sequentially
if version < 1 {
    // Initial schema (this document)
    tx.Exec(schemaV1)
    tx.Exec("PRAGMA user_version = 1")
}
// Future migrations follow the same pattern:
// if version < 2 { tx.Exec(schemaV2); tx.Exec("PRAGMA user_version = 2") }
```

All DDL statements use `CREATE TABLE IF NOT EXISTS` and
`CREATE INDEX IF NOT EXISTS` so that the migration is idempotent.
Re-running the same migration against an already-migrated database
is a safe no-op.

---

## 2. Table Definitions

The schema consists of five tables serving two distinct purposes:

* **Hot working set:** `active_flows` holds the currently running
  flows. Rows are inserted on flow registration and deleted on
  termination or stale reaping. This table is small (bounded by
  R-SYS-01: 20 concurrent flows) and queried frequently.
* **Cold audit log:** `notification_requests`,
  `notification_responses`, `flow_lifecycle_events`, and
  `interjections` are append-only history tables. They grow over
  time and serve the history API (C-09) and mobile history
  viewer (M-07).

### 2.1 `notification_requests`

Stores every `NotificationRequest` published by a CLI command or
MCP agent. One row per notification. This is the primary history
table — the `HistoryQueryRequest` filters operate against it.

```sql
CREATE TABLE IF NOT EXISTS notification_requests (
    id             TEXT PRIMARY KEY,
    username       TEXT NOT NULL,
    flow_id        TEXT NOT NULL,
    daemon_id      TEXT NOT NULL,
    workspace_id   TEXT NOT NULL,
    title          TEXT NOT NULL,
    body           TEXT,
    response_types TEXT NOT NULL,  -- JSON array, e.g. '["boolean","text"]'
    priority       TEXT NOT NULL
                   CHECK (priority IN ('low', 'normal', 'high')),
    source         TEXT NOT NULL,
    actions        TEXT,           -- JSON array, e.g. '["Approve","Reject"]'
    timeout_sec    INTEGER,
    timestamp      TEXT NOT NULL   -- RFC 3339 UTC
);
```

### 2.2 `notification_responses`

Stores the human decision returned from the Android app. One row
per response, correlated 1:0..1 with `notification_requests` via
`request_id`. Notifications with `response_types: ["none"]` and
timed-out interactive notifications have no corresponding row.

```sql
CREATE TABLE IF NOT EXISTS notification_responses (
    request_id TEXT PRIMARY KEY,
    accepted   BOOLEAN,       -- NULL when not a boolean response
    action     TEXT,           -- NULL when not a choice response
    text       TEXT,           -- NULL when not a text response
    timestamp  TEXT NOT NULL,  -- RFC 3339 UTC
    FOREIGN KEY (request_id)
        REFERENCES notification_requests (id)
);
```

### 2.3 `flow_lifecycle_events`

Append-only audit log of every flow state change. A single flow
generates multiple rows over its lifetime: one `active` event on
registration, zero or more `active` events from `refresh_flow`
calls, and one terminal `completed` or `failed` event. The
composite primary key `(flow_id, timestamp)` allows multiple events
per flow while preserving insertion order.

```sql
CREATE TABLE IF NOT EXISTS flow_lifecycle_events (
    flow_id      TEXT NOT NULL,
    username     TEXT NOT NULL,
    daemon_id    TEXT NOT NULL,
    workspace_id TEXT NOT NULL,
    status       TEXT NOT NULL
                 CHECK (status IN ('active', 'completed', 'failed')),
    label        TEXT,
    metadata     TEXT,           -- JSON object, e.g. '{"branch":"main"}'
    timestamp    TEXT NOT NULL,  -- RFC 3339 UTC
    PRIMARY KEY (flow_id, timestamp)
);
```

### 2.4 `active_flows`

Working set of currently running flows. Populated when a
`FlowLifecycleEvent` with `status: active` is received; deleted
when the flow terminates (`completed` or `failed`) or is reaped
for staleness. The `last_activity_timestamp` column tracks the
most recent tool interaction with the flow and is the basis for
stale reaping (R-CLI-18).

```sql
CREATE TABLE IF NOT EXISTS active_flows (
    flow_id                 TEXT PRIMARY KEY,
    username                TEXT NOT NULL,
    daemon_id               TEXT NOT NULL,
    workspace_id            TEXT NOT NULL,
    label                   TEXT,
    metadata                TEXT,           -- JSON object
    registered_at           TEXT NOT NULL,  -- RFC 3339 UTC (creation)
    last_activity_timestamp TEXT NOT NULL   -- RFC 3339 UTC (reaping basis)
);
```

Unlike `flow_lifecycle_events`, this table does not store a `status`
column — all rows are implicitly `active`. When a flow terminates,
its row is deleted.

### 2.5 `interjections`

Append-only audit log of out-of-band interjection commands sent by
the mobile user. Linked to flows by `flow_id`. A single flow may
receive multiple interjections (e.g., a `note` followed by a
`stop`). Uses the same composite PK pattern as
`flow_lifecycle_events`.

```sql
CREATE TABLE IF NOT EXISTS interjections (
    flow_id   TEXT NOT NULL,
    username  TEXT NOT NULL,
    action    TEXT NOT NULL
              CHECK (action IN ('stop', 'pause', 'note')),
    context   TEXT,            -- free-form text (for note/pause)
    timestamp TEXT NOT NULL,   -- RFC 3339 UTC
    PRIMARY KEY (flow_id, timestamp)
);
```

---

## 3. Indices

Indices are designed around the five documented query patterns
(Section 4). All use `IF NOT EXISTS` for idempotent migration.

```sql
-- HistoryQueryRequest: filter by workspace, ordered by time
CREATE INDEX IF NOT EXISTS idx_nr_workspace_ts
    ON notification_requests (workspace_id, timestamp DESC);

-- HistoryQueryRequest: filter by flow, ordered by time
CREATE INDEX IF NOT EXISTS idx_nr_flow_ts
    ON notification_requests (flow_id, timestamp DESC);

-- HistoryQueryRequest: global time ordering (no filter)
CREATE INDEX IF NOT EXISTS idx_nr_ts
    ON notification_requests (timestamp DESC);

-- Rate limiting (R-CLI-16): count recent notifications per flow
-- Covered by idx_nr_flow_ts above (flow_id, timestamp DESC)

-- Stale reaping (R-CLI-18): find inactive active flows
CREATE INDEX IF NOT EXISTS idx_af_last_activity
    ON active_flows (last_activity_timestamp);

-- ActiveFlowsQuery: filter by workspace or daemon
CREATE INDEX IF NOT EXISTS idx_af_workspace
    ON active_flows (workspace_id);
CREATE INDEX IF NOT EXISTS idx_af_daemon
    ON active_flows (daemon_id);

-- Flow lifecycle audit: lookup events by flow
CREATE INDEX IF NOT EXISTS idx_fle_flow
    ON flow_lifecycle_events (flow_id, timestamp DESC);

-- Flow lifecycle audit: filter by workspace, ordered by time
CREATE INDEX IF NOT EXISTS idx_fle_workspace_ts
    ON flow_lifecycle_events (workspace_id, timestamp DESC);

-- Interjection audit: lookup by flow
CREATE INDEX IF NOT EXISTS idx_inj_flow
    ON interjections (flow_id, timestamp DESC);
```

---

## 4. Query Patterns

### 4.1 History Query (`HistoryQueryRequest` → `HistoryQueryResult`)

The daemon serves this query via the Core NATS Request-Reply
endpoint `resystems.renotify.{username}.svc.history` (C-09). All
filter fields are optional; when omitted they are treated as
unconstrained.

**Records query:**

```sql
SELECT
    req.id, req.flow_id, req.daemon_id, req.workspace_id,
    req.title, req.body, req.response_types, req.priority,
    req.source, req.actions, req.timeout_sec, req.timestamp,
    resp.request_id, resp.accepted, resp.action, resp.text,
    resp.timestamp AS response_timestamp
FROM notification_requests req
LEFT JOIN notification_responses resp
    ON req.id = resp.request_id
WHERE (:workspace_id IS NULL OR req.workspace_id = :workspace_id)
  AND (:flow_id      IS NULL OR req.flow_id      = :flow_id)
  AND (:since         IS NULL OR req.timestamp    >= :since)
  AND (:until         IS NULL OR req.timestamp    <= :until)
ORDER BY req.timestamp DESC
LIMIT :limit
OFFSET :offset;
```

**Total count query** (for `HistoryQueryResult.total`):

```sql
SELECT COUNT(*) AS total
FROM notification_requests req
WHERE (:workspace_id IS NULL OR req.workspace_id = :workspace_id)
  AND (:flow_id      IS NULL OR req.flow_id      = :flow_id)
  AND (:since         IS NULL OR req.timestamp    >= :since)
  AND (:until         IS NULL OR req.timestamp    <= :until);
```

The application assembles each row into a `HistoryRecord` struct
pairing the `NotificationRequest` with an optional
`NotificationResponse` (NULL columns when no response exists).

### 4.2 Active Flows Query (`ActiveFlowsQuery` → `ActiveFlowsResult`)

Served via `resystems.renotify.{username}.svc.flows` (C-10).

```sql
SELECT
    flow_id, daemon_id, workspace_id, label, metadata,
    registered_at AS timestamp
FROM active_flows
WHERE (:daemon_id    IS NULL OR daemon_id    = :daemon_id)
  AND (:workspace_id IS NULL OR workspace_id = :workspace_id)
ORDER BY registered_at DESC;
```

The application maps each row to a `FlowLifecycleEvent` with
`status: "active"` for the `ActiveFlowsResult.Flows` array.

### 4.3 Stale Flow Reaping (R-CLI-18)

The daemon runs this query periodically (e.g., every 30 seconds,
aligned with the heartbeat interval) to detect flows with no
recent activity.

```sql
SELECT flow_id, daemon_id, workspace_id, label, metadata
FROM active_flows
WHERE last_activity_timestamp < datetime('now', :grace_period);
```

Where `:grace_period` is `'-5 minutes'` by default (configurable).

For each stale flow returned, the daemon:

1. Inserts a `FlowLifecycleEvent` with `status: failed` into the
   audit log.
2. Publishes the same event to the NATS lifecycle subject.
3. Deletes the row from `active_flows`.

These three steps are wrapped in a single transaction per flow.

### 4.4 Rate Limiting (R-CLI-16)

Checked before inserting a new `NotificationRequest`. The daemon
counts recent notifications for the flow and rejects the request
with error code `rate_limited` if the limit is exceeded.

```sql
SELECT COUNT(*) AS recent_count
FROM notification_requests
WHERE flow_id = :flow_id
  AND timestamp > datetime('now', '-1 minute');
```

If `recent_count >= 60` (default, configurable), the daemon
returns an `ErrorResponse` and does not insert the notification.

### 4.5 Deduplication

**Notification request dedup** (idempotent insert):

```sql
INSERT OR IGNORE INTO notification_requests (...) VALUES (...);
```

`INSERT OR IGNORE` silently skips the insert if the `id` primary
key already exists. This handles JetStream at-least-once
redelivery without an explicit existence check.

**Notification response dedup** (idempotent insert):

```sql
INSERT OR IGNORE INTO notification_responses (...) VALUES (...);
```

Same pattern — the `request_id` primary key prevents duplicate
responses.

**Flow lifecycle event dedup** (idempotent insert):

```sql
INSERT OR IGNORE INTO flow_lifecycle_events (...) VALUES (...);
```

The composite primary key `(flow_id, timestamp)` prevents exact
duplicate events. Events with the same `flow_id` but different
timestamps (e.g., successive refreshes) are distinct rows.

---

## 5. Record Lifecycle

### 5.1 Write Path

The daemon writes to SQLite at the same time it publishes to (or
receives from) NATS. The NATS message provides ephemeral delivery
(30-minute TTL); the SQLite row provides permanent persistence.

| Event                        | NATS Action                                         | SQLite Action                                                                                                             |
|:-----------------------------|:----------------------------------------------------|:--------------------------------------------------------------------------------------------------------------------------|
| CLI/agent sends notification | Publish `NotificationRequest` to JetStream          | `INSERT OR IGNORE INTO notification_requests`                                                                             |
| Mobile app responds          | Receive `NotificationResponse` from JetStream       | `INSERT OR IGNORE INTO notification_responses`                                                                            |
| Flow registered              | Publish `FlowLifecycleEvent` (`active`)             | Insert into `flow_lifecycle_events` + `active_flows`                                                                      |
| Flow refreshed               | Publish `FlowLifecycleEvent` (`active`)             | Insert into `flow_lifecycle_events`; update `active_flows.last_activity_timestamp`                                        |
| Flow terminated              | Publish `FlowLifecycleEvent` (`completed`/`failed`) | Insert into `flow_lifecycle_events`; delete from `active_flows`                                                           |
| Stale reaping                | Publish `FlowLifecycleEvent` (`failed`)             | Insert into `flow_lifecycle_events`; delete from `active_flows`                                                           |
| Any tool call on a flow      | (varies)                                            | Update `active_flows.last_activity_timestamp`                                                                             |
| Interjection received        | Receive `InterjectionCommand` from JetStream        | `INSERT OR IGNORE INTO interjections`; for `stop`: also insert `flow_lifecycle_events` (`failed`) + delete `active_flows` |

### 5.2 Active Flow Lifecycle

```
register_flow / CLI start
    → INSERT active_flows (registered_at = now, last_activity_timestamp = now)
    → INSERT flow_lifecycle_events (status = 'active')

post / ask / refresh_flow (any activity)
    → UPDATE active_flows SET last_activity_timestamp = now

refresh_flow (with label/metadata update)
    → UPDATE active_flows SET label = ?, metadata = ?, last_activity_timestamp = now
    → INSERT flow_lifecycle_events (status = 'active', updated label/metadata)

terminate_flow / CLI exit
    → DELETE FROM active_flows
    → INSERT flow_lifecycle_events (status = 'completed' or 'failed')

stale reaping (last_activity_timestamp > grace period)
    → DELETE FROM active_flows
    → INSERT flow_lifecycle_events (status = 'failed')
```

### 5.3 History Retention

For MVP, the history tables grow without automatic pruning.
R-SYS-01 specifies a minimum of 10,000 history records, not a
cap. The indices are designed to perform well at this scale.

Post-MVP retention policies (e.g., time-based pruning, record
count caps) may be added as a future schema migration. The
append-only design of the audit tables ensures that pruning is a
simple `DELETE WHERE timestamp < :cutoff` operation.

### 5.4 Daemon Startup Reconciliation

On startup the daemon reconciles the `active_flows` table against
reality:

1. Load all rows from `active_flows`.
2. For each flow, check whether the originating process is still
   alive (CLI) or connected (MCP). If not, apply the stale
   reaping grace period from the `last_activity_timestamp`.
3. Process any buffered `FlowLifecycleEvent` messages from the
   JetStream `daemon-lifecycle-{username}` consumer that arrived
   while the daemon was down.
4. Publish an immediate `DaemonHeartbeat` reflecting the
   reconciled state.

---

## 6. Implementation Notes

### 6.1 Concurrency

The daemon is a single-process application. SQLite write
operations occur on the NATS subscription goroutines and the
periodic reaping goroutine. Use a single `*sql.DB` connection
pool with `PRAGMA journal_mode = WAL` for concurrent read access
during writes. Wrap multi-step operations (e.g., rate limit check
+ insert) in explicit transactions.

### 6.2 Workspace Discovery

The daemon does not maintain a separate workspaces table.
Workspace information is derived from two sources:

* **`active_flows`:** The set of `workspace_id` values with at
  least one active flow.
* **`flow_lifecycle_events`:** The historical set of all
  workspace IDs ever seen.

The daemon's heartbeat includes workspace display names and
absolute paths, which are resolved at flow registration time (the
CLI or MCP agent provides the workspace context). The daemon
caches this mapping in memory, populated from the most recent
`FlowLifecycleEvent` per workspace.

### 6.3 Go Library

Use `modernc.org/sqlite` (pure Go, CGo-free) or
`github.com/mattn/go-sqlite3` (CGo, widely used). The schema is
standard SQL and works with either driver. The choice is deferred
to implementation (C-03).

### 6.4 Complete Schema V1 (Migration Script)

The following is the complete idempotent migration for schema
version 1:

```sql
-- Schema V1: Initial ledger schema

CREATE TABLE IF NOT EXISTS notification_requests (
    id             TEXT PRIMARY KEY,
    username       TEXT NOT NULL,
    flow_id        TEXT NOT NULL,
    daemon_id      TEXT NOT NULL,
    workspace_id   TEXT NOT NULL,
    title          TEXT NOT NULL,
    body           TEXT,
    response_types TEXT NOT NULL,
    priority       TEXT NOT NULL
                   CHECK (priority IN ('low', 'normal', 'high')),
    source         TEXT NOT NULL,
    actions        TEXT,
    timeout_sec    INTEGER,
    timestamp      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notification_responses (
    request_id TEXT PRIMARY KEY,
    accepted   BOOLEAN,
    action     TEXT,
    text       TEXT,
    timestamp  TEXT NOT NULL,
    FOREIGN KEY (request_id)
        REFERENCES notification_requests (id)
);

CREATE TABLE IF NOT EXISTS flow_lifecycle_events (
    flow_id      TEXT NOT NULL,
    username     TEXT NOT NULL,
    daemon_id    TEXT NOT NULL,
    workspace_id TEXT NOT NULL,
    status       TEXT NOT NULL
                 CHECK (status IN ('active', 'completed', 'failed')),
    label        TEXT,
    metadata     TEXT,
    timestamp    TEXT NOT NULL,
    PRIMARY KEY (flow_id, timestamp)
);

CREATE TABLE IF NOT EXISTS active_flows (
    flow_id                 TEXT PRIMARY KEY,
    username                TEXT NOT NULL,
    daemon_id               TEXT NOT NULL,
    workspace_id            TEXT NOT NULL,
    label                   TEXT,
    metadata                TEXT,
    registered_at           TEXT NOT NULL,
    last_activity_timestamp TEXT NOT NULL
);

-- Indices for history queries
CREATE INDEX IF NOT EXISTS idx_nr_workspace_ts
    ON notification_requests (workspace_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_nr_flow_ts
    ON notification_requests (flow_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_nr_ts
    ON notification_requests (timestamp DESC);

-- Index for stale reaping
CREATE INDEX IF NOT EXISTS idx_af_last_activity
    ON active_flows (last_activity_timestamp);

-- Indices for active flow queries
CREATE INDEX IF NOT EXISTS idx_af_workspace
    ON active_flows (workspace_id);
CREATE INDEX IF NOT EXISTS idx_af_daemon
    ON active_flows (daemon_id);

-- Indices for flow lifecycle audit
CREATE INDEX IF NOT EXISTS idx_fle_flow
    ON flow_lifecycle_events (flow_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_fle_workspace_ts
    ON flow_lifecycle_events (workspace_id, timestamp DESC);

CREATE TABLE IF NOT EXISTS interjections (
    flow_id   TEXT NOT NULL,
    username  TEXT NOT NULL,
    action    TEXT NOT NULL
              CHECK (action IN ('stop', 'pause', 'note')),
    context   TEXT,
    timestamp TEXT NOT NULL,
    PRIMARY KEY (flow_id, timestamp)
);

-- Index for interjection audit
CREATE INDEX IF NOT EXISTS idx_inj_flow
    ON interjections (flow_id, timestamp DESC);

PRAGMA user_version = 1;
```
