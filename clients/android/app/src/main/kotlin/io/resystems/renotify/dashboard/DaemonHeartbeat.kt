package io.resystems.renotify.dashboard

import org.json.JSONObject

/**
 * Kotlin representation of the Go `heartbeat.WorkspaceInfo`
 * type. Describes a single workspace within a daemon heartbeat
 * snapshot.
 */
data class WorkspaceInfo(
    val workspaceId: String,
    val displayName: String,
    val absPath: String,
    val activeFlows: List<String>
)

/**
 * Kotlin representation of the Go `heartbeat.DaemonHeartbeat`
 * type. Published periodically (30s) and on structural changes
 * over Core NATS Pub/Sub. Used by [MainActivity] to render the
 * dashboard.
 *
 * See docs/analysis-payload-schemas.md (DaemonHeartbeat).
 */
data class DaemonHeartbeat(
    val daemonId: String,
    val username: String,
    val hostname: String,
    val workspaces: List<WorkspaceInfo>,
    val timestamp: String
) {
    companion object {
        /**
         * Parse a [DaemonHeartbeat] from a JSON string.
         *
         * @throws IllegalArgumentException if required fields
         *         are missing.
         */
        fun fromJson(json: String): DaemonHeartbeat {
            val obj = JSONObject(json)

            require(obj.has("daemon_id")) {
                "daemon_id is required"
            }

            val workspaces = mutableListOf<WorkspaceInfo>()
            if (obj.has("workspaces") &&
                !obj.isNull("workspaces")
            ) {
                val arr = obj.getJSONArray("workspaces")
                for (i in 0 until arr.length()) {
                    val ws = arr.getJSONObject(i)
                    val flows = mutableListOf<String>()
                    if (ws.has("active_flows") &&
                        !ws.isNull("active_flows")
                    ) {
                        val fa = ws.getJSONArray("active_flows")
                        for (j in 0 until fa.length()) {
                            flows.add(fa.getString(j))
                        }
                    }
                    workspaces.add(
                        WorkspaceInfo(
                            workspaceId = ws.getString(
                                "workspace_id"),
                            displayName = ws.optString(
                                "display_name", ""),
                            absPath = ws.optString(
                                "abs_path", ""),
                            activeFlows = flows
                        )
                    )
                }
            }

            return DaemonHeartbeat(
                daemonId = obj.getString("daemon_id"),
                username = obj.optString("username", ""),
                hostname = obj.optString("hostname", ""),
                workspaces = workspaces,
                timestamp = obj.optString("timestamp", "")
            )
        }
    }
}
