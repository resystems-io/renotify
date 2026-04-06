package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

// InsertRequest inserts a notification request into the ledger.
// Uses INSERT OR IGNORE for idempotent handling of JetStream
// redelivery (Section 4.5).
func (d *DB) InsertRequest(wc WriteContext, r *payload.NotificationRequest) error {
	responseTypes, err := json.Marshal(r.ResponseTypes)
	if err != nil {
		return fmt.Errorf("ledger: marshal response_types: %w", err)
	}

	var actions sql.NullString
	if r.Actions != nil {
		b, err := json.Marshal(r.Actions)
		if err != nil {
			return fmt.Errorf("ledger: marshal actions: %w", err)
		}
		actions = sql.NullString{String: string(b), Valid: true}
	}

	var body sql.NullString
	if r.Body != "" {
		body = sql.NullString{String: r.Body, Valid: true}
	}

	var timeoutSec sql.NullInt64
	if r.TimeoutSec != 0 {
		timeoutSec = sql.NullInt64{Int64: int64(r.TimeoutSec), Valid: true}
	}

	_, err = d.db.Exec(`
		INSERT OR IGNORE INTO notification_requests
		    (id, username, flow_id, daemon_id, workspace_id,
		     title, body, response_types, priority, source,
		     actions, timeout_sec, timestamp,
		     flow_label, workspace_name, workspace_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, wc.Username, r.FlowID, r.DaemonID, r.WorkspaceID,
		r.Title, body,
		string(responseTypes), string(r.Priority), r.Source, actions,
		timeoutSec, r.Timestamp.UTC().Format(time.RFC3339),
		nullString(wc.FlowLabel),
		nullString(wc.WorkspaceName),
		nullString(wc.WorkspacePath),
	)
	if err != nil {
		return fmt.Errorf("ledger: insert request: %w", err)
	}
	return nil
}

// CountRecentRequests returns the number of notification requests
// for the given flow within the time window. Used for rate
// limiting (Section 4.4).
func (d *DB) CountRecentRequests(flowID string, window time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-window).Format(time.RFC3339)
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM notification_requests
		WHERE flow_id = ? AND timestamp > ?`,
		flowID, cutoff,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ledger: count recent requests: %w", err)
	}
	return count, nil
}
