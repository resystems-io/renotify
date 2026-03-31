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
