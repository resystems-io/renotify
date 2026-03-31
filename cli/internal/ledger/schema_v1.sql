-- Schema V1: Initial ledger schema.
-- See docs/analysis-sqlite-ledger.md Section 6.4.
-- All statements use IF NOT EXISTS for idempotent migration.

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

CREATE TABLE IF NOT EXISTS interjections (
    flow_id   TEXT NOT NULL,
    username  TEXT NOT NULL,
    action    TEXT NOT NULL
              CHECK (action IN ('stop', 'pause', 'note')),
    context   TEXT,
    timestamp TEXT NOT NULL,
    PRIMARY KEY (flow_id, timestamp)
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

-- Index for interjection audit
CREATE INDEX IF NOT EXISTS idx_inj_flow
    ON interjections (flow_id, timestamp DESC);
