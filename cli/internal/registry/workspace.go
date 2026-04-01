package registry

import (
	"go.resystems.io/renotify/internal/heartbeat"
	"go.resystems.io/renotify/internal/ledger"
)

// rebuildWorkspaceSnapshot queries all active flows, groups them
// by workspace, and pushes the snapshot to the heartbeat
// publisher. Triggers an immediate heartbeat publish.
func (s *Service) rebuildWorkspaceSnapshot() {
	flows, err := s.db.ListActiveFlows(ledger.ActiveFlowsQuery{})
	if err != nil {
		s.logger.Error("rebuild workspace snapshot", "err", err)
		return
	}

	// Group flows by workspace_id.
	type wsInfo struct {
		displayName string
		absPath     string
		flowIDs     []string
	}
	byWorkspace := make(map[string]*wsInfo)

	for _, f := range flows {
		ws, ok := byWorkspace[f.WorkspaceID]
		if !ok {
			ws = &wsInfo{
				displayName: f.DisplayName,
				absPath:     f.AbsPath,
			}
			byWorkspace[f.WorkspaceID] = ws
		}
		ws.flowIDs = append(ws.flowIDs, f.FlowID)
	}

	snapshot := make([]heartbeat.WorkspaceInfo, 0, len(byWorkspace))
	for wsID, ws := range byWorkspace {
		snapshot = append(snapshot, heartbeat.WorkspaceInfo{
			WorkspaceID: wsID,
			DisplayName: ws.displayName,
			AbsPath:     ws.absPath,
			ActiveFlows: ws.flowIDs,
		})
	}

	s.hb.SetWorkspaces(snapshot)
	s.hb.Publish()
}
