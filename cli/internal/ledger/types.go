package ledger

import (
	"time"

	"go.resystems.io/renotify/internal/payload"
)

// WriteContext carries daemon-side enrichment data that is stored
// alongside the payload but not part of the NATS wire format.
// Passed to ledger write methods that create new records.
type WriteContext struct {
	Username string
}

// ActiveFlow maps to the active_flows table. This is a storage
// type (not a wire-format payload) representing the denormalised
// hot working set of currently running flows.
type ActiveFlow struct {
	FlowID                string
	Username              string
	DaemonID              string
	WorkspaceID           string
	DisplayName           string
	AbsPath               string
	Label                 string
	Metadata              map[string]string
	RegisteredAt          time.Time
	LastActivityTimestamp time.Time
}

// HistoryQuery holds the filter parameters for QueryHistory.
type HistoryQuery struct {
	WorkspaceID string
	FlowID      string
	Since       *time.Time
	Until       *time.Time
	Limit       int
	Offset      int
}

// HistoryRecord pairs a request with its optional response.
// Username is included so records are self-describing when
// histories from multiple daemons are aggregated.
type HistoryRecord struct {
	Username string
	Request  payload.NotificationRequest
	Response *payload.NotificationResponse
}

// HistoryResult holds the paginated query result.
type HistoryResult struct {
	Records []HistoryRecord
	Total   int
}

// ActiveFlowsQuery holds the filter parameters for
// ListActiveFlows.
type ActiveFlowsQuery struct {
	DaemonID    string
	WorkspaceID string
}
