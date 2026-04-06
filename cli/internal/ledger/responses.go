package ledger

import (
	"database/sql"
	"fmt"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

// InsertResponse inserts a notification response into the ledger.
// Uses INSERT OR IGNORE for idempotent handling of JetStream
// redelivery (Section 4.5). No WriteContext needed — the
// responses table has no enrichment columns.
func (d *DB) InsertResponse(r *payload.NotificationResponse) error {
	var accepted sql.NullBool
	if r.Accepted != nil {
		accepted = sql.NullBool{Bool: *r.Accepted, Valid: true}
	}

	var action sql.NullString
	if r.Action != "" {
		action = sql.NullString{String: r.Action, Valid: true}
	}

	var text sql.NullString
	if r.Text != "" {
		text = sql.NullString{String: r.Text, Valid: true}
	}

	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO notification_responses
		    (request_id, accepted, action, text, timestamp)
		VALUES (?, ?, ?, ?, ?)`,
		r.RequestID, accepted, action, text,
		r.Timestamp.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("ledger: insert response: %w", err)
	}
	return nil
}
