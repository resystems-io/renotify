// nats_transport.go implements mcp.Transport and mcp.Connection
// backed by NATS Pub/Sub. This enables the daemon's mcp.Server
// to serve stdio MCP sessions relayed via the `renotify mcp` CLI.
//
// Each stdio session uses two Core NATS subjects:
//   - c2s (client-to-server): CLI publishes stdin lines here
//   - s2c (server-to-client): daemon publishes responses here
//
// The CLI is a raw byte forwarder; all JSON-RPC framing is
// handled by the SDK via this Connection implementation.
//
// Daemon restart behaviour: exit immediately.
// The CLI process exits on NATS disconnect. The MCP client
// detects subprocess death and relaunches. See the plan in
// .claude/plans/ for future session state persistence notes.
package mcpserver

import (
	"context"
	"io"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nats-io/nats.go"
)

// Compile-time interface checks.
var (
	_ mcp.Transport  = (*natsTransport)(nil)
	_ mcp.Connection = (*natsConn)(nil)
)

// natsTransport implements mcp.Transport for a single stdio
// relay session. Connect returns the pre-built natsConn.
type natsTransport struct {
	conn *natsConn
}

func (t *natsTransport) Connect(context.Context) (mcp.Connection, error) {
	return t.conn, nil
}

// natsConn implements mcp.Connection. It reads JSON-RPC
// messages from a NATS subscription (c2s) and writes
// responses to a NATS subject (s2c).
type natsConn struct {
	sessionID string
	nc        *nats.Conn
	sub       *nats.Subscription
	s2cSubj   string
	incoming  chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

// newNATSConn creates a Connection backed by NATS subjects.
// The caller must subscribe c2sSub before the MCP session
// starts reading.
func newNATSConn(
	sessionID string,
	nc *nats.Conn,
	c2sSubject, s2cSubject string,
) (*natsConn, error) {
	c := &natsConn{
		sessionID: sessionID,
		nc:        nc,
		s2cSubj:   s2cSubject,
		incoming:  make(chan []byte, 64),
		closed:    make(chan struct{}),
	}

	sub, err := nc.Subscribe(c2sSubject, func(msg *nats.Msg) {
		select {
		case c.incoming <- msg.Data:
		case <-c.closed:
		}
	})
	if err != nil {
		return nil, err
	}
	c.sub = sub
	return c, nil
}

// SessionID returns the stdio relay session identifier.
func (c *natsConn) SessionID() string { return c.sessionID }

// Read blocks until a JSON-RPC message arrives from the CLI
// (via the c2s NATS subscription) or the context is cancelled.
func (c *natsConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, io.EOF
	case data := <-c.incoming:
		if data == nil {
			return nil, io.EOF
		}
		return jsonrpc.DecodeMessage(data)
	}
}

// Write encodes a JSON-RPC message and publishes it to the
// s2c NATS subject for the CLI to write to stdout.
func (c *natsConn) Write(ctx context.Context, msg jsonrpc.Message) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	data, err := jsonrpc.EncodeMessage(msg)
	if err != nil {
		return err
	}
	return c.nc.Publish(c.s2cSubj, data)
}

// Close terminates the connection: unsubscribes from NATS and
// signals the read loop to stop.
func (c *natsConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.sub != nil {
			c.sub.Unsubscribe()
		}
	})
	return nil
}
