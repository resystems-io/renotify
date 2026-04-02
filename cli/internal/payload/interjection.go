package payload

import "time"

// InterjectionCommand is an asynchronous, unprompted control
// signal emitted by the developer from the Android app targeting
// a specific flow by its flow_id.
type InterjectionCommand struct {
	FlowID    string             `json:"flow_id"`
	Action    InterjectionAction `json:"action"`
	Context   string             `json:"context,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
}

// InterjectionResource is the MCP dynamic resource that agents
// read to obtain interjection details. Served at
// renotify://interjections/{flow_id}. Contains the most recent
// interjection; accumulated interjections are available via the
// check_interjections tool.
type InterjectionResource struct {
	FlowID    string             `json:"flow_id"`
	Action    InterjectionAction `json:"action"`
	Context   string             `json:"context,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
}
