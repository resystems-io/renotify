package io.resystems.renotify.dashboard

import org.json.JSONObject

/**
 * Kotlin representation of the Go `registry.ActiveFlowEntry`
 * wire-format type. Returned by the daemon's `svc.flows`
 * Core NATS Request-Reply endpoint.
 */
data class ActiveFlowEntry(
    val flowId: String,
    val daemonId: String,
    val workspaceId: String,
    val displayName: String,
    val absPath: String,
    val label: String,
    val metadata: Map<String, String>,
    val registeredAt: String,
    val lastActivityTimestamp: String
)

/**
 * Response payload from the daemon's `svc.flows` endpoint.
 * Contains a flat list of active flows that can be grouped by
 * workspace to produce a [DaemonHeartbeat] for the dashboard.
 */
data class ActiveFlowsResult(
    val flows: List<ActiveFlowEntry>
) {
    /**
     * Convert the flat flow list into a [DaemonHeartbeat] by
     * grouping flows by workspace. Used to populate the
     * dashboard immediately on connect without waiting for the
     * periodic heartbeat.
     */
    fun toDaemonHeartbeat(): DaemonHeartbeat {
        val firstFlow = flows.firstOrNull()

        val byWorkspace = flows.groupBy { it.workspaceId }
        val workspaces = byWorkspace.map { (wsId, entries) ->
            val first = entries.first()
            WorkspaceInfo(
                workspaceId = wsId,
                displayName = first.displayName,
                absPath = first.absPath,
                activeFlows = entries.map { e ->
                    FlowInfo(
                        flowId = e.flowId,
                        label = e.label,
                        metadata = e.metadata,
                        lastActivity = e.lastActivityTimestamp
                    )
                }
            )
        }

        return DaemonHeartbeat(
            daemonId = firstFlow?.daemonId ?: "",
            username = "",
            hostname = "",
            workspaces = workspaces,
            timestamp = firstFlow?.registeredAt ?: ""
        )
    }

    companion object {
        fun fromJson(json: String): ActiveFlowsResult {
            val obj = JSONObject(json)
            val flows = mutableListOf<ActiveFlowEntry>()

            if (obj.has("flows") && !obj.isNull("flows")) {
                val arr = obj.getJSONArray("flows")
                for (i in 0 until arr.length()) {
                    val f = arr.getJSONObject(i)
                    val meta = mutableMapOf<String, String>()
                    if (f.has("metadata") &&
                        !f.isNull("metadata")
                    ) {
                        val mo = f.getJSONObject("metadata")
                        for (key in mo.keys()) {
                            meta[key] = mo.optString(key, "")
                        }
                    }
                    flows.add(
                        ActiveFlowEntry(
                            flowId = f.getString("flow_id"),
                            daemonId = f.optString(
                                "daemon_id", ""),
                            workspaceId = f.optString(
                                "workspace_id", ""),
                            displayName = f.optString(
                                "display_name", ""),
                            absPath = f.optString(
                                "abs_path", ""),
                            label = f.optString("label", ""),
                            metadata = meta,
                            registeredAt = f.optString(
                                "registered_at", ""),
                            lastActivityTimestamp = f.optString(
                                "last_activity_timestamp", "")
                        )
                    )
                }
            }

            return ActiveFlowsResult(flows)
        }
    }
}
