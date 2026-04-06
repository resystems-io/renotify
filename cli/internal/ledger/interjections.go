package ledger

import (
	"fmt"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

// InsertInterjection inserts an interjection into the audit log.
// Uses INSERT OR IGNORE for idempotency on the composite primary
// key (flow_id, timestamp).
func (d *DB) InsertInterjection(wc WriteContext, i *payload.InterjectionCommand) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO interjections
		    (flow_id, username, action, context, timestamp)
		VALUES (?, ?, ?, ?, ?)`,
		i.FlowID, wc.Username, string(i.Action),
		nullString(i.Context),
		i.Timestamp.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("ledger: insert interjection: %w", err)
	}
	return nil
}
