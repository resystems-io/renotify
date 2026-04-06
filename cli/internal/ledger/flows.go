package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

// RegisterFlow inserts a new active flow and its initial lifecycle
// event. Both operations run in a single transaction (Section 5.2).
func (d *DB) RegisterFlow(f *ActiveFlow) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("ledger: begin register flow: %w", err)
	}

	metadata := marshalMetadata(f.Metadata)
	ts := f.RegisteredAt.UTC().Format(time.RFC3339)

	if _, err := tx.Exec(`
		INSERT INTO active_flows
		    (flow_id, username, daemon_id, workspace_id,
		     display_name, abs_path, label,
		     metadata, registered_at, last_activity_timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.FlowID, f.Username, f.DaemonID, f.WorkspaceID,
		nullString(f.DisplayName), nullString(f.AbsPath),
		nullString(f.Label), metadata, ts, ts,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("ledger: insert active flow: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO flow_lifecycle_events
		    (flow_id, username, daemon_id, workspace_id, status,
		     label, metadata, timestamp)
		VALUES (?, ?, ?, ?, 'active', ?, ?, ?)`,
		f.FlowID, f.Username, f.DaemonID, f.WorkspaceID,
		nullString(f.Label), metadata, ts,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("ledger: insert lifecycle event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ledger: commit register flow: %w", err)
	}
	return nil
}

// UpdateFlowActivity updates the last_activity_timestamp for an
// active flow. Called on any tool interaction referencing the flow
// (Section 5.2).
func (d *DB) UpdateFlowActivity(flowID string, ts time.Time) error {
	_, err := d.db.Exec(`
		UPDATE active_flows
		SET last_activity_timestamp = ?
		WHERE flow_id = ?`,
		ts.UTC().Format(time.RFC3339), flowID,
	)
	if err != nil {
		return fmt.Errorf("ledger: update flow activity: %w", err)
	}
	return nil
}

// RefreshFlow updates an active flow's label, metadata, and
// activity timestamp, and records a lifecycle event (Section 5.2).
func (d *DB) RefreshFlow(
	flowID, label string,
	metadata map[string]string,
	ts time.Time,
) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("ledger: begin refresh flow: %w", err)
	}

	meta := marshalMetadata(metadata)
	tsStr := ts.UTC().Format(time.RFC3339)

	if _, err := tx.Exec(`
		UPDATE active_flows
		SET label = ?, metadata = ?, last_activity_timestamp = ?
		WHERE flow_id = ?`,
		nullString(label), meta, tsStr, flowID,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("ledger: update active flow: %w", err)
	}

	// Read username, daemon_id, and workspace_id from the active
	// flow for the lifecycle event.
	var username, daemonID, workspaceID string
	err = tx.QueryRow(`
		SELECT username, daemon_id, workspace_id FROM active_flows
		WHERE flow_id = ?`, flowID,
	).Scan(&username, &daemonID, &workspaceID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("ledger: read active flow: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO flow_lifecycle_events
		    (flow_id, username, daemon_id, workspace_id, status,
		     label, metadata, timestamp)
		VALUES (?, ?, ?, ?, 'active', ?, ?, ?)`,
		flowID, username, daemonID, workspaceID,
		nullString(label), meta, tsStr,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("ledger: insert lifecycle event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ledger: commit refresh flow: %w", err)
	}
	return nil
}

// TerminateFlow removes a flow from the active set and records a
// terminal lifecycle event. The status should be "completed" or
// "failed" (Section 5.2).
func (d *DB) TerminateFlow(flowID, status string, ts time.Time) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("ledger: begin terminate flow: %w", err)
	}

	tsStr := ts.UTC().Format(time.RFC3339)

	// Read the flow's context before deletion for the lifecycle
	// event. If the flow doesn't exist (already terminated or
	// never registered), use empty strings.
	var username, daemonID, workspaceID string
	var label sql.NullString
	var metadata sql.NullString
	err = tx.QueryRow(`
		SELECT username, daemon_id, workspace_id, label, metadata
		FROM active_flows WHERE flow_id = ?`, flowID,
	).Scan(&username, &daemonID, &workspaceID, &label, &metadata)
	if err != nil && err != sql.ErrNoRows {
		tx.Rollback()
		return fmt.Errorf("ledger: read active flow: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM active_flows WHERE flow_id = ?`, flowID,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("ledger: delete active flow: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO flow_lifecycle_events
		    (flow_id, username, daemon_id, workspace_id, status,
		     label, metadata, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		flowID, username, daemonID, workspaceID, status,
		label, metadata, tsStr,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("ledger: insert lifecycle event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ledger: commit terminate flow: %w", err)
	}
	return nil
}

// InsertLifecycleEvent inserts a standalone lifecycle event into
// the audit log. Uses INSERT OR IGNORE for idempotency.
func (d *DB) InsertLifecycleEvent(wc WriteContext, e *payload.FlowLifecycleEvent) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO flow_lifecycle_events
		    (flow_id, username, daemon_id, workspace_id, status,
		     label, metadata, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.FlowID, wc.Username, e.DaemonID, e.WorkspaceID,
		string(e.Status),
		nullString(e.Label), marshalMetadata(e.Metadata),
		e.Timestamp.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("ledger: insert lifecycle event: %w", err)
	}
	return nil
}

// ListActiveFlows returns all active flows matching the query
// filters (Section 4.2).
func (d *DB) ListActiveFlows(q ActiveFlowsQuery) ([]ActiveFlow, error) {
	rows, err := d.db.Query(`
		SELECT flow_id, daemon_id, workspace_id,
		       display_name, abs_path, label, metadata,
		       registered_at, last_activity_timestamp
		FROM active_flows
		WHERE (? = '' OR flow_id = ?)
		  AND (? = '' OR daemon_id = ?)
		  AND (? = '' OR workspace_id = ?)
		ORDER BY registered_at DESC`,
		q.FlowID, q.FlowID,
		q.DaemonID, q.DaemonID,
		q.WorkspaceID, q.WorkspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: list active flows: %w", err)
	}
	defer rows.Close()

	var flows []ActiveFlow
	for rows.Next() {
		var f ActiveFlow
		var displayName, absPath, label, metadata sql.NullString
		var regAt, lastAct string
		if err := rows.Scan(
			&f.FlowID, &f.DaemonID, &f.WorkspaceID,
			&displayName, &absPath, &label, &metadata,
			&regAt, &lastAct,
		); err != nil {
			return nil, fmt.Errorf("ledger: scan active flow: %w", err)
		}
		f.DisplayName = displayName.String
		f.AbsPath = absPath.String
		f.Label = label.String
		f.Metadata = unmarshalMetadata(metadata)
		f.RegisteredAt, _ = time.Parse(time.RFC3339, regAt)
		f.LastActivityTimestamp, _ = time.Parse(time.RFC3339, lastAct)
		flows = append(flows, f)
	}
	return flows, rows.Err()
}

// ReapStaleFlows returns active flows whose last activity is
// older than the grace period (Section 4.3). The caller is
// responsible for terminating each flow (publishing the NATS
// event and calling TerminateFlow).
func (d *DB) ReapStaleFlows(gracePeriod time.Duration) ([]ActiveFlow, error) {
	cutoff := time.Now().UTC().Add(-gracePeriod).Format(time.RFC3339)
	rows, err := d.db.Query(`
		SELECT flow_id, daemon_id, workspace_id,
		       display_name, abs_path, label, metadata,
		       registered_at, last_activity_timestamp
		FROM active_flows
		WHERE last_activity_timestamp < ?`,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: reap stale flows: %w", err)
	}
	defer rows.Close()

	var flows []ActiveFlow
	for rows.Next() {
		var f ActiveFlow
		var displayName, absPath, label, metadata sql.NullString
		var regAt, lastAct string
		if err := rows.Scan(
			&f.FlowID, &f.DaemonID, &f.WorkspaceID,
			&displayName, &absPath, &label, &metadata,
			&regAt, &lastAct,
		); err != nil {
			return nil, fmt.Errorf("ledger: scan stale flow: %w", err)
		}
		f.DisplayName = displayName.String
		f.AbsPath = absPath.String
		f.Label = label.String
		f.Metadata = unmarshalMetadata(metadata)
		f.RegisteredAt, _ = time.Parse(time.RFC3339, regAt)
		f.LastActivityTimestamp, _ = time.Parse(time.RFC3339, lastAct)
		flows = append(flows, f)
	}
	return flows, rows.Err()
}

// marshalMetadata encodes a metadata map as a JSON string, or
// returns a NullString for nil maps.
func marshalMetadata(m map[string]string) sql.NullString {
	if m == nil {
		return sql.NullString{}
	}
	b, _ := json.Marshal(m)
	return sql.NullString{String: string(b), Valid: true}
}

// unmarshalMetadata decodes a JSON string back into a metadata
// map, or returns nil for NULL columns.
func unmarshalMetadata(s sql.NullString) map[string]string {
	if !s.Valid {
		return nil
	}
	var m map[string]string
	json.Unmarshal([]byte(s.String), &m)
	return m
}

// nullString returns a NullString that is NULL when s is empty.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
