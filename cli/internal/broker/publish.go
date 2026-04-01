package broker

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

// PublishJSON marshals v as JSON and publishes it to the given
// JetStream subject with a Nats-Msg-Id header for deduplication.
// Used by both CLI commands and the MCP server.
func PublishJSON(js nats.JetStreamContext, subject, msgID string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}
	msg.Header.Set("Nats-Msg-Id", msgID)

	_, err = js.PublishMsg(msg)
	return err
}
