// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.dashboard

import org.json.JSONObject

/**
 * Describes an active flow within a workspace heartbeat
 * snapshot. Mirrors Go `heartbeat.FlowInfo`.
 */
data class FlowInfo(
    val flowId: String,
    val label: String,
    val metadata: Map<String, String>,
    val lastActivity: String
)

/**
 * Kotlin representation of the Go `heartbeat.WorkspaceInfo`
 * type. Describes a single workspace within a daemon heartbeat
 * snapshot.
 */
data class WorkspaceInfo(
    val workspaceId: String,
    val displayName: String,
    val absPath: String,
    val activeFlows: List<FlowInfo>
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
    val gracePeriodMs: Long,
    val deviceHeartbeatIntervalMs: Long,
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
                    val flows = mutableListOf<FlowInfo>()
                    if (ws.has("active_flows") &&
                        !ws.isNull("active_flows")
                    ) {
                        val fa = ws.getJSONArray("active_flows")
                        for (j in 0 until fa.length()) {
                            val fo = fa.getJSONObject(j)
                            val meta = mutableMapOf<String, String>()
                            if (fo.has("metadata") &&
                                !fo.isNull("metadata")
                            ) {
                                val mo = fo.getJSONObject("metadata")
                                for (key in mo.keys()) {
                                    meta[key] = mo.optString(key, "")
                                }
                            }
                            flows.add(FlowInfo(
                                flowId = fo.getString("flow_id"),
                                label = fo.optString("label", ""),
                                metadata = meta,
                                lastActivity = fo.optString(
                                    "last_activity", "")
                            ))
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

            val gracePeriodStr = obj.optString(
                "grace_period", "")
            val deviceHbIntervalStr = obj.optString(
                "device_heartbeat_interval", "")

            return DaemonHeartbeat(
                daemonId = obj.getString("daemon_id"),
                username = obj.optString("username", ""),
                hostname = obj.optString("hostname", ""),
                gracePeriodMs = parseGoDuration(gracePeriodStr),
                deviceHeartbeatIntervalMs = parseGoDuration(
                    deviceHbIntervalStr),
                workspaces = workspaces,
                timestamp = obj.optString("timestamp", "")
            )
        }

        /**
         * Parse a Go duration string (e.g. "15m0s", "5m",
         * "1h30m") into milliseconds. Returns 0 on parse
         * failure.
         */
        fun parseGoDuration(s: String): Long {
            if (s.isEmpty()) return 0
            var remaining = s
            var totalMs = 0L
            val pattern = Regex("(\\d+)(h|m|s|ms)")
            for (match in pattern.findAll(remaining)) {
                val value = match.groupValues[1].toLongOrNull()
                    ?: continue
                when (match.groupValues[2]) {
                    "ms" -> totalMs += value
                    "s"  -> totalMs += value * 1000
                    "m"  -> totalMs += value * 60_000
                    "h"  -> totalMs += value * 3_600_000
                }
            }
            return totalMs
        }
    }
}
