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
     * Individual flow row within a workspace group.
     */
    data class FlowItem(
        val flowId: String
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
                for (flowId in ws.activeFlows) {
                    items.add(FlowItem(flowId))
                }
            }
            return items
        }
    }
}
