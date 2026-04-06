package mcpserver

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/payload"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

// stateClient wraps NATS request-reply calls to the daemon's
// state management service endpoints (R-CLI-20, C-17). It
// replaces the direct ledger.DB accessor that previously
// coupled the MCP server to the SQLite database.
type stateClient struct {
	nc       *nats.Conn
	username string
	timeout  time.Duration
}

func newStateClient(
	nc *nats.Conn,
	username string,
) *stateClient {
	return &stateClient{
		nc:       nc,
		username: username,
		timeout:  2 * time.Second,
	}
}

// LookupFlow queries svc.flows for a specific flow and returns
// its context. Returns an error if the flow is not found.
func (c *stateClient) LookupFlow(
	flowID string,
) (*statesvc.FlowEntry, error) {
	query := statesvc.FlowsQuery{FlowID: flowID}
	data, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	subject := broker.ServiceFlowsSubject(c.username)
	resp, err := c.nc.Request(subject, data, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("svc.flows request: %w", err)
	}

	var result statesvc.FlowsResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal flows result: %w", err)
	}

	for i := range result.Flows {
		if result.Flows[i].FlowID == flowID {
			return &result.Flows[i], nil
		}
	}
	return nil, fmt.Errorf(
		"flow %q not found in active registry", flowID)
}

// InsertRequest sends a notification request to the state
// subsystem for ledger insertion.
func (c *stateClient) InsertRequest(
	cmd statesvc.InsertRequestCmd,
) error {
	return c.writeRequest(
		broker.ServiceInsertRequestSubject(c.username), cmd)
}

// InsertResponse sends a notification response to the state
// subsystem for ledger insertion.
func (c *stateClient) InsertResponse(
	resp *payload.NotificationResponse,
) error {
	cmd := statesvc.InsertResponseCmd{Response: *resp}
	return c.writeRequest(
		broker.ServiceInsertResponseSubject(c.username), cmd)
}

// InsertInterjection sends an interjection audit record to the
// state subsystem for ledger insertion.
func (c *stateClient) InsertInterjection(
	cmd statesvc.InsertInterjectionCmd,
) error {
	return c.writeRequest(
		broker.ServiceInsertInterjectionSubject(c.username), cmd)
}

// UpdateActivity updates a flow's last activity timestamp via
// the state subsystem.
func (c *stateClient) UpdateActivity(
	flowID string, ts time.Time,
) error {
	cmd := statesvc.UpdateActivityCmd{
		FlowID:    flowID,
		Timestamp: ts,
	}
	return c.writeRequest(
		broker.ServiceUpdateActivitySubject(c.username), cmd)
}

// writeRequest marshals cmd, sends a NATS request to subject,
// and checks the WriteResult for errors.
func (c *stateClient) writeRequest(
	subject string, cmd any,
) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	resp, err := c.nc.Request(subject, data, c.timeout)
	if err != nil {
		return fmt.Errorf("request %s: %w", subject, err)
	}

	var result statesvc.WriteResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("state service: %s", result.Error)
	}
	return nil
}
