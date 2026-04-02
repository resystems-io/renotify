package broker

import "fmt"

// Flow-scoped NATS subject constructors. These match the subject
// catalogue in docs/analysis-nats-transport-design.md Section 1.1.

// FlowRequestSubject returns the JetStream subject for publishing
// a NotificationRequest.
func FlowRequestSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.request",
		username, flowID)
}

// FlowResponseSubject returns the JetStream subject for
// NotificationResponse messages.
func FlowResponseSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.response",
		username, flowID)
}

// FlowLifecycleSubject returns the JetStream subject for
// FlowLifecycleEvent messages.
func FlowLifecycleSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.lifecycle",
		username, flowID)
}

// FlowInterjectSubject returns the JetStream subject for
// InterjectionCommand messages.
func FlowInterjectSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.interject",
		username, flowID)
}

// ServiceFlowsSubject returns the Core NATS Request-Reply subject
// for the active flows query endpoint (R-CLI-14).
func ServiceFlowsSubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.svc.flows", username)
}

// ServiceHistorySubject returns the Core NATS Request-Reply
// subject for the notification history query endpoint (C-09).
func ServiceHistorySubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.svc.history", username)
}
