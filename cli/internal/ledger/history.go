package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"go.resystems.io/renotify/internal/payload"
)

const defaultHistoryLimit = 50

// QueryHistory executes the paginated history query with optional
// filters (Section 4.1). Returns requests LEFT JOINed with their
// responses.
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

	// Total count (without LIMIT/OFFSET).
	var total int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM notification_requests req
		WHERE (? IS NULL OR req.workspace_id = ?)
		  AND (? IS NULL OR req.flow_id = ?)
		  AND (? IS NULL OR req.timestamp >= ?)
		  AND (? IS NULL OR req.timestamp <= ?)`,
		wsID, wsID, fID, fID, since, since, until, until,
	).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("ledger: count history: %w", err)
	}

	// Records query with LEFT JOIN.
	rows, err := d.db.Query(`
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
		ORDER BY req.timestamp DESC
		LIMIT ? OFFSET ?`,
		wsID, wsID, fID, fID, since, since, until, until,
		limit, q.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: query history: %w", err)
	}
	defer rows.Close()

	var records []HistoryRecord
	for rows.Next() {
		rec, err := scanHistoryRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: iterate history: %w", err)
	}

	if records == nil {
		records = []HistoryRecord{}
	}

	return &HistoryResult{Records: records, Total: total}, nil
}

// scanHistoryRow scans a single row from the history LEFT JOIN
// query into a HistoryRecord.
func scanHistoryRow(rows *sql.Rows) (HistoryRecord, error) {
	var rec HistoryRecord

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
