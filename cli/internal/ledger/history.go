// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

const defaultHistoryLimit = 50

// QueryHistory returns a unified timeline of notification records
// and flow lifecycle events, sorted by timestamp (newest first).
// Notification records are LEFT JOINed with their responses.
// Lifecycle events (flow started, completed, failed) are
// interleaved by timestamp. Both record types respect the same
// filter parameters.
func (d *DB) QueryHistory(q HistoryQuery) (*HistoryResult, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultHistoryLimit
	}

	// Resolve optional time filters to nullable parameters.
	var since, until sql.NullString
	if q.Since != nil {
		since = sql.NullString{
			String: q.Since.UTC().Format(time.RFC3339),
			Valid:  true,
		}
	}
	if q.Until != nil {
		until = sql.NullString{
			String: q.Until.UTC().Format(time.RFC3339),
			Valid:  true,
		}
	}

	wsID := sql.NullString{String: q.WorkspaceID, Valid: q.WorkspaceID != ""}
	fID := sql.NullString{String: q.FlowID, Valid: q.FlowID != ""}

	// Total count across both tables.
	var total int
	err := d.db.QueryRow(`
		SELECT (
			SELECT COUNT(*) FROM notification_requests req
			WHERE (? IS NULL OR req.workspace_id = ?)
			  AND (? IS NULL OR req.flow_id = ?)
			  AND (? IS NULL OR req.timestamp >= ?)
			  AND (? IS NULL OR req.timestamp <= ?)
		) + (
			SELECT COUNT(*) FROM flow_lifecycle_events lc
			WHERE (? IS NULL OR lc.workspace_id = ?)
			  AND (? IS NULL OR lc.flow_id = ?)
			  AND (? IS NULL OR lc.timestamp >= ?)
			  AND (? IS NULL OR lc.timestamp <= ?)
		)`,
		wsID, wsID, fID, fID, since, since, until, until,
		wsID, wsID, fID, fID, since, since, until, until,
	).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("ledger: count history: %w", err)
	}

	// Notification records with LEFT JOIN.
	notifRows, err := d.db.Query(`
		SELECT
		    req.username,
		    req.flow_label, req.workspace_name, req.workspace_path,
		    req.id, req.flow_id, req.daemon_id, req.workspace_id,
		    req.title, req.body, req.response_types, req.priority,
		    req.source, req.actions, req.timeout_sec, req.timestamp,
		    resp.request_id, resp.accepted, resp.action, resp.text,
		    resp.timestamp
		FROM notification_requests req
		LEFT JOIN notification_responses resp
		    ON req.id = resp.request_id
		WHERE (? IS NULL OR req.workspace_id = ?)
		  AND (? IS NULL OR req.flow_id = ?)
		  AND (? IS NULL OR req.timestamp >= ?)
		  AND (? IS NULL OR req.timestamp <= ?)
		ORDER BY req.timestamp DESC`,
		wsID, wsID, fID, fID, since, since, until, until,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: query notifications: %w", err)
	}
	defer notifRows.Close()

	var all []HistoryRecord
	for notifRows.Next() {
		rec, err := scanHistoryRow(notifRows)
		if err != nil {
			return nil, err
		}
		all = append(all, rec)
	}
	if err := notifRows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: iterate notifications: %w", err)
	}

	// Lifecycle events.
	lcRows, err := d.db.Query(`
		SELECT username, flow_id, daemon_id, workspace_id,
		       status, label, metadata, timestamp
		FROM flow_lifecycle_events
		WHERE (? IS NULL OR workspace_id = ?)
		  AND (? IS NULL OR flow_id = ?)
		  AND (? IS NULL OR timestamp >= ?)
		  AND (? IS NULL OR timestamp <= ?)
		ORDER BY timestamp DESC`,
		wsID, wsID, fID, fID, since, since, until, until,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: query lifecycle: %w", err)
	}
	defer lcRows.Close()

	for lcRows.Next() {
		rec, err := scanLifecycleRow(lcRows)
		if err != nil {
			return nil, err
		}
		all = append(all, rec)
	}
	if err := lcRows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: iterate lifecycle: %w", err)
	}

	// Sort combined records by timestamp descending.
	sort.Slice(all, func(i, j int) bool {
		return recordTimestamp(all[i]).After(recordTimestamp(all[j]))
	})

	// Apply pagination.
	if q.Offset > 0 && q.Offset < len(all) {
		all = all[q.Offset:]
	} else if q.Offset >= len(all) {
		all = nil
	}
	if limit < len(all) {
		all = all[:limit]
	}

	if all == nil {
		all = []HistoryRecord{}
	}

	return &HistoryResult{Records: all, Total: total}, nil
}

// recordTimestamp extracts the timestamp from a HistoryRecord
// regardless of type.
func recordTimestamp(r HistoryRecord) time.Time {
	if r.Request != nil {
		return r.Request.Timestamp
	}
	if r.Lifecycle != nil {
		return r.Lifecycle.Timestamp
	}
	return time.Time{}
}

// scanLifecycleRow scans a single row from the lifecycle events
// query into a HistoryRecord with Type "lifecycle".
func scanLifecycleRow(rows *sql.Rows) (HistoryRecord, error) {
	var (
		username, flowID, daemonID, wsID string
		status, timestamp                string
		label, metadata                  sql.NullString
	)

	err := rows.Scan(
		&username, &flowID, &daemonID, &wsID,
		&status, &label, &metadata, &timestamp,
	)
	if err != nil {
		return HistoryRecord{},
			fmt.Errorf("ledger: scan lifecycle row: %w", err)
	}

	ts, _ := time.Parse(time.RFC3339, timestamp)

	var meta map[string]string
	if metadata.Valid {
		json.Unmarshal([]byte(metadata.String), &meta)
	}

	return HistoryRecord{
		Type:     HistoryTypeLifecycle,
		Username: username,
		Lifecycle: &payload.FlowLifecycleEvent{
			FlowID:      flowID,
			DaemonID:    daemonID,
			WorkspaceID: wsID,
			Status:      payload.FlowStatus(status),
			Label:       label.String,
			Metadata:    meta,
			Timestamp:   ts,
		},
	}, nil
}

// scanHistoryRow scans a single row from the history LEFT JOIN
// query into a HistoryRecord.
func scanHistoryRow(rows *sql.Rows) (HistoryRecord, error) {
	rec := HistoryRecord{
		Type:    HistoryTypeNotification,
		Request: &payload.NotificationRequest{},
	}

	// Flow context fields (nullable — NULL for pre-V3 rows).
	var flowLabel, wsName, wsPath sql.NullString

	// Request fields.
	var body, actions, timeoutSec sql.NullString
	var respTypes, priority, reqTimestamp string

	// Response fields (all nullable due to LEFT JOIN).
	var respID, respAction, respText, respTimestamp sql.NullString
	var respAccepted sql.NullBool

	err := rows.Scan(
		&rec.Username,
		&flowLabel, &wsName, &wsPath,
		&rec.Request.ID, &rec.Request.FlowID,
		&rec.Request.DaemonID, &rec.Request.WorkspaceID,
		&rec.Request.Title, &body, &respTypes,
		&priority, &rec.Request.Source,
		&actions, &timeoutSec, &reqTimestamp,
		&respID, &respAccepted, &respAction, &respText,
		&respTimestamp,
	)
	if err != nil {
		return rec, fmt.Errorf("ledger: scan history row: %w", err)
	}

	// Flow context fields.
	rec.FlowLabel = flowLabel.String
	rec.WorkspaceName = wsName.String
	rec.WorkspacePath = wsPath.String

	// Unmarshal request fields.
	rec.Request.Body = body.String
	rec.Request.Priority = payload.Priority(priority)
	json.Unmarshal([]byte(respTypes), &rec.Request.ResponseTypes)
	if actions.Valid {
		json.Unmarshal([]byte(actions.String), &rec.Request.Actions)
	}
	if timeoutSec.Valid {
		var ts int
		fmt.Sscanf(timeoutSec.String, "%d", &ts)
		rec.Request.TimeoutSec = ts
	}
	rec.Request.Timestamp, _ = time.Parse(time.RFC3339, reqTimestamp)

	// Unmarshal response fields (nil when LEFT JOIN produces NULLs).
	if respID.Valid {
		resp := &payload.NotificationResponse{
			RequestID: respID.String,
			Action:    respAction.String,
			Text:      respText.String,
		}
		if respAccepted.Valid {
			b := respAccepted.Bool
			resp.Accepted = &b
		}
		if respTimestamp.Valid {
			resp.Timestamp, _ = time.Parse(time.RFC3339, respTimestamp.String)
		}
		rec.Response = resp
	}

	return rec, nil
}
