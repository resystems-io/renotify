package payload

import "time"

// FlowLifecycleEvent marks the start or end of a pipeline flow.
// Published by the CLI or MCP agent when a flow begins (active)
// and when it terminates (completed or failed). The daemon
// consumes these to maintain the active flow registry.
type FlowLifecycleEvent struct {
	FlowID      string            `json:"flow_id"`
	DaemonID    string            `json:"daemon_id"`
	WorkspaceID string            `json:"workspace_id"`
	Status      FlowStatus        `json:"status"`
	Label       string            `json:"label,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
}
