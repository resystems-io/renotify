// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.dashboard

/**
 * View items for the flat dashboard RecyclerView. The adapter
 * flattens the workspace→flow hierarchy into a single list
 * using these types.
 */
sealed class DashboardItem {

    /**
     * Workspace header row showing the display name, path, and
     * active flow count.
     */
    data class WorkspaceHeader(
        val workspaceId: String,
        val displayName: String,
        val absPath: String,
        val flowCount: Int
    ) : DashboardItem()

    /**
     * Individual flow row within a workspace group showing
     * label and metadata.
     */
    data class FlowItem(
        val flowId: String,
        val label: String,
        val metadata: Map<String, String>,
        val lastActivity: String,
        val gracePeriodMs: Long
    ) : DashboardItem()

    /**
     * Placeholder shown when there are no active workspaces or
     * before the first heartbeat arrives.
     */
    data class EmptyState(
        val message: String
    ) : DashboardItem()

    companion object {
        /**
         * Flatten a [DaemonHeartbeat] into a list of dashboard
         * items suitable for a RecyclerView adapter.
         */
        fun fromHeartbeat(
            heartbeat: DaemonHeartbeat?
        ): List<DashboardItem> {
            if (heartbeat == null) {
                return listOf(
                    EmptyState("Waiting for heartbeat\u2026"))
            }
            if (heartbeat.workspaces.isEmpty()) {
                return listOf(
                    EmptyState("No active workspaces"))
            }

            val items = mutableListOf<DashboardItem>()
            for (ws in heartbeat.workspaces) {
                items.add(
                    WorkspaceHeader(
                        workspaceId = ws.workspaceId,
                        displayName = ws.displayName,
                        absPath = ws.absPath,
                        flowCount = ws.activeFlows.size
                    )
                )
                for (flow in ws.activeFlows) {
                    items.add(FlowItem(
                        flowId = flow.flowId,
                        label = flow.label,
                        metadata = flow.metadata,
                        lastActivity = flow.lastActivity,
                        gracePeriodMs = heartbeat.gracePeriodMs
                    ))
                }
            }
            return items
        }
    }
}
