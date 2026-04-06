package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
)

// sessionIDPayload is the JSON envelope for stdio MCP session
// open and close messages.
type sessionIDPayload struct {
	SessionID string `json:"session_id"`
}

// stdioSession tracks a single stdio relay session on the
// daemon side.
type stdioSession struct {
	session *mcp.ServerSession
	cancel  context.CancelFunc
	conn    *natsConn
}

// startStdioRelay subscribes to MCP session open/close
// subjects and bridges stdio relay sessions into the
// mcp.Server. Each open creates a new ServerSession backed
// by NATS; each close terminates it.
func (s *Server) startStdioRelay(ctx context.Context) error {
	s.stdioSessions = make(map[string]*stdioSession)

	openSubj := broker.MCPSessionOpenSubject(s.username)
	closSubj := broker.MCPSessionCloseSubject(s.username)

	var err error
	s.stdioOpenSub, err = s.nc.Subscribe(openSubj, func(msg *nats.Msg) {
		s.handleStdioOpen(ctx, msg)
	})
	if err != nil {
		return err
	}

	s.stdioCloseSub, err = s.nc.Subscribe(closSubj, func(msg *nats.Msg) {
		s.handleStdioClose(msg)
	})
	if err != nil {
		s.stdioOpenSub.Unsubscribe()
		return err
	}

	s.logger.Info("stdio relay listener started",
		"open_subject", openSubj, "close_subject", closSubj)
	return nil
}

// handleStdioOpen processes a session open message. The
// payload is a JSON object with a "session_id" field.
func (s *Server) handleStdioOpen(ctx context.Context, msg *nats.Msg) {
	var payload sessionIDPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		s.logger.Error("stdio open: invalid payload", "err", err)
		return
	}
	if payload.SessionID == "" {
		s.logger.Error("stdio open: empty session_id")
		return
	}

	s.stdioMu.Lock()
	if _, exists := s.stdioSessions[payload.SessionID]; exists {
		s.stdioMu.Unlock()
		s.logger.Warn("stdio open: duplicate session",
			"session_id", payload.SessionID)
		return
	}
	s.stdioMu.Unlock()

	c2sSubj := broker.MCPClientToServerSubject(
		s.username, payload.SessionID)
	s2cSubj := broker.MCPServerToClientSubject(
		s.username, payload.SessionID)

	conn, err := newNATSConn(payload.SessionID, s.nc, c2sSubj, s2cSubj)
	if err != nil {
		s.logger.Error("stdio open: subscribe failed",
			"session_id", payload.SessionID, "err", err)
		return
	}

	sessCtx, cancel := context.WithCancel(ctx)
	transport := &natsTransport{conn: conn}

	ss, err := s.mcpServer.Connect(sessCtx, transport, &mcp.ServerSessionOptions{})
	if err != nil {
		cancel()
		conn.Close()
		s.logger.Error("stdio open: connect failed",
			"session_id", payload.SessionID, "err", err)
		return
	}

	s.stdioMu.Lock()
	s.stdioSessions[payload.SessionID] = &stdioSession{
		session: ss,
		cancel:  cancel,
		conn:    conn,
	}
	s.stdioMu.Unlock()

	s.logger.Info("stdio session opened",
		"session_id", payload.SessionID)

	// Wait for session to end (in background) and clean up.
	go func() {
		ss.Wait()
		s.removeStdioSession(payload.SessionID)
	}()
}

// handleStdioClose processes a session close message.
func (s *Server) handleStdioClose(msg *nats.Msg) {
	var payload sessionIDPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		s.logger.Error("stdio close: invalid payload", "err", err)
		return
	}
	s.removeStdioSession(payload.SessionID)
}

// removeStdioSession cancels and removes a session.
func (s *Server) removeStdioSession(sessionID string) {
	s.stdioMu.Lock()
	sess, ok := s.stdioSessions[sessionID]
	if ok {
		delete(s.stdioSessions, sessionID)
	}
	s.stdioMu.Unlock()

	if ok {
		sess.cancel()
		sess.conn.Close()
		s.logger.Info("stdio session closed",
			"session_id", sessionID)
	}
}

// stopStdioRelay unsubscribes the session listeners and
// closes all active stdio sessions.
func (s *Server) stopStdioRelay() {
	if s.stdioOpenSub != nil {
		s.stdioOpenSub.Unsubscribe()
	}
	if s.stdioCloseSub != nil {
		s.stdioCloseSub.Unsubscribe()
	}

	s.stdioMu.Lock()
	sessions := make(map[string]*stdioSession, len(s.stdioSessions))
	for k, v := range s.stdioSessions {
		sessions[k] = v
	}
	s.stdioSessions = nil
	s.stdioMu.Unlock()

	for id, sess := range sessions {
		sess.cancel()
		sess.conn.Close()
		s.logger.Info("stdio session closed (shutdown)",
			"session_id", id)
	}
}
