package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/payload"
	"go.resystems.io/renotify/internal/statesvc"
)

// debounceMap tracks the last-processed timestamp per
// flow_id + action for interjection deduplication.
type debounceMap struct {
	mu    sync.Mutex
	times map[string]time.Time
}

func newDebounceMap() *debounceMap {
	return &debounceMap{times: make(map[string]time.Time)}
}

// check returns true if the action should be processed (not
// within the debounce window). Updates the timestamp if allowed.
func (dm *debounceMap) check(
	flowID string,
	action payload.InterjectionAction,
	window time.Duration,
) bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	key := flowID + ":" + string(action)
	last, ok := dm.times[key]
	now := time.Now()
	if ok && now.Sub(last) < window {
		return false
	}
	dm.times[key] = now
	return true
}

// startInterjectConsumer binds to the daemon-interject JetStream
// consumer and starts a goroutine that processes interjections.
func (s *Server) startInterjectConsumer(ctx context.Context) error {
	js, err := natsjs.New(s.nc)
	if err != nil {
		return fmt.Errorf("mcpserver: create jetstream: %w", err)
	}

	consumerName := broker.InterjectConsumerName(s.username)
	consumer, err := js.Consumer(ctx, broker.StreamName, consumerName)
	if err != nil {
		return fmt.Errorf("mcpserver: bind consumer %s: %w",
			consumerName, err)
	}

	iter, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("mcpserver: start messages: %w", err)
	}

	s.logger.Info("interjection consumer started",
		"consumer", consumerName)

	go s.consumeInterjections(ctx, iter)
	return nil
}

// consumeInterjections reads messages from the JetStream
// iterator until the context is cancelled.
func (s *Server) consumeInterjections(
	ctx context.Context,
	iter natsjs.MessagesContext,
) {
	defer iter.Stop()
	debounce := newDebounceMap()

	for {
		msg, err := iter.Next()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.logger.Error("interjection next", "err", err)
				return
			}
		}

		s.processInterjection(msg, debounce)
	}
}

// processInterjection handles a single interjection message.
func (s *Server) processInterjection(
	msg natsjs.Msg,
	debounce *debounceMap,
) {
	var cmd payload.InterjectionCommand
	if err := json.Unmarshal(msg.Data(), &cmd); err != nil {
		s.logger.Error("interjection unmarshal",
			"err", err, "subject", msg.Subject())
		msg.Ack()
		return
	}

	// Debounce: skip if same flow+action within window.
	if !debounce.check(cmd.FlowID, cmd.Action,
		s.cfg.Interjection.DebounceWindow.Duration) {
		s.logger.Debug("interjection debounced",
			"flow_id", cmd.FlowID, "action", cmd.Action)
		msg.Ack()
		return
	}

	// Insert into ledger for audit via state service.
	if s.state != nil {
		s.state.InsertInterjection(statesvc.InsertInterjectionCmd{
			Username:     s.username,
			Interjection: cmd,
		})
	}

	switch cmd.Action {
	case payload.InterjectionStop:
		s.handleStop(&cmd)
	case payload.InterjectionNote:
		s.handleNote(&cmd)
	case payload.InterjectionPause:
		// Deferred to post-MVP — treat as note.
		s.logger.Warn("pause treated as note (not yet implemented)",
			"flow_id", cmd.FlowID)
		cmd.Context = "Pause requested"
		s.handleNote(&cmd)
	default:
		s.logger.Warn("unknown interjection action",
			"action", cmd.Action, "flow_id", cmd.FlowID)
	}

	msg.Ack()
}

// handleStop processes a stop interjection: terminates the flow,
// resolves any pending decisions, updates InterjectionStore.
func (s *Server) handleStop(cmd *payload.InterjectionCommand) {
	s.logger.Info("interjection stop",
		"flow_id", cmd.FlowID)

	// Update InterjectionStore.
	s.interjections.Append(cmd.FlowID, payload.InterjectionResource{
		FlowID:    cmd.FlowID,
		Action:    cmd.Action,
		Context:   cmd.Context,
		Timestamp: cmd.Timestamp,
	})

	// Publish failed lifecycle to NATS (registry handles DB).
	var workspaceID string
	if flow, err := s.lookupFlow(cmd.FlowID); err == nil {
		workspaceID = flow.WorkspaceID
	}

	event := &payload.FlowLifecycleEvent{
		FlowID:      cmd.FlowID,
		DaemonID:    s.daemonID,
		WorkspaceID: workspaceID,
		Status:      payload.FlowFailed,
		Timestamp:   cmd.Timestamp,
	}
	broker.PublishJSON(s.js,
		broker.FlowLifecycleSubject(s.username, cmd.FlowID),
		cmd.FlowID+"-stopped", event)

	// Resolve any pending DecisionResources for this flow with
	// timeout-like state (decided=true, no response fields).
	// This wakes any await_decision caller.
	s.resolveDecisionsForFlow(cmd.FlowID, cmd.Timestamp)

	// Emit MCP resource notifications.
	s.mcpServer.ResourceUpdated(context.Background(),
		&mcp.ResourceUpdatedNotificationParams{
			URI: InterjectionResourceURI(cmd.FlowID),
		})
}

// handleNote processes a note interjection: forwards context
// without altering flow state.
func (s *Server) handleNote(cmd *payload.InterjectionCommand) {
	s.logger.Info("interjection note",
		"flow_id", cmd.FlowID,
		"context", cmd.Context)

	s.interjections.Append(cmd.FlowID, payload.InterjectionResource{
		FlowID:    cmd.FlowID,
		Action:    cmd.Action,
		Context:   cmd.Context,
		Timestamp: cmd.Timestamp,
	})

	// Emit MCP resource notification.
	s.mcpServer.ResourceUpdated(context.Background(),
		&mcp.ResourceUpdatedNotificationParams{
			URI: InterjectionResourceURI(cmd.FlowID),
		})
}

// resolveDecisionsForFlow resolves all pending DecisionResources
// associated with a flow. Used when a stop interjection
// terminates the flow — any pending ask decisions become
// "decided with no response" (same as timeout).
func (s *Server) resolveDecisionsForFlow(
	flowID string, ts time.Time,
) {
	// We don't have a direct flow→notification mapping, so we
	// iterate all pending decisions. This is O(n) but the number
	// of pending decisions is typically very small.
	s.decisions.mu.RLock()
	var ids []string
	for id, r := range s.decisions.resources {
		if !r.Decided {
			ids = append(ids, id)
		}
	}
	s.decisions.mu.RUnlock()

	for _, id := range ids {
		s.decisions.ResolveTimeout(id, ts)
		uri := DecisionResourceURI(id)
		s.mcpServer.ResourceUpdated(context.Background(),
			&mcp.ResourceUpdatedNotificationParams{URI: uri})
	}
}
