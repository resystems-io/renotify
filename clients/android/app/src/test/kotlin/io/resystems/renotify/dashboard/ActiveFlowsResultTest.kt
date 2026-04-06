// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.dashboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ActiveFlowsResultTest {

    @Test
    fun parsesFlowsWithMetadata() {
        val json = """
            {
                "flows": [
                    {
                        "flow_id": "fl_A",
                        "daemon_id": "dn_D1",
                        "workspace_id": "ws_W1",
                        "display_name": "project-a",
                        "abs_path": "/home/user/a",
                        "label": "Build",
                        "metadata": {"branch": "main", "step": "3/5"},
                        "registered_at": "2026-04-01T10:00:00Z",
                        "last_activity_timestamp": "2026-04-01T10:01:00Z"
                    }
                ]
            }
        """.trimIndent()

        val result = ActiveFlowsResult.fromJson(json)
        assertEquals(1, result.flows.size)
        assertEquals("Build", result.flows[0].label)
        assertEquals("main", result.flows[0].metadata["branch"])
        assertEquals("3/5", result.flows[0].metadata["step"])
    }

    @Test
    fun parsesEmptyFlows() {
        val json = """{"flows": []}"""
        val result = ActiveFlowsResult.fromJson(json)
        assertTrue(result.flows.isEmpty())
    }

    @Test
    fun parsesNullFlows() {
        val json = """{}"""
        val result = ActiveFlowsResult.fromJson(json)
        assertTrue(result.flows.isEmpty())
    }

    @Test
    fun toDaemonHeartbeat_groupsByWorkspace() {
        val result = ActiveFlowsResult(
            flows = listOf(
                ActiveFlowEntry(
                    flowId = "fl_A", daemonId = "dn_D1",
                    workspaceId = "ws_W1",
                    displayName = "project-a", absPath = "/a",
                    label = "Build",
                    metadata = mapOf("branch" to "main"),
                    registeredAt = "2026-04-01T10:00:00Z",
                    lastActivityTimestamp = "2026-04-01T10:01:00Z"
                ),
                ActiveFlowEntry(
                    flowId = "fl_B", daemonId = "dn_D1",
                    workspaceId = "ws_W1",
                    displayName = "project-a", absPath = "/a",
                    label = "Test",
                    metadata = emptyMap(),
                    registeredAt = "2026-04-01T10:00:00Z",
                    lastActivityTimestamp = "2026-04-01T10:02:00Z"
                ),
                ActiveFlowEntry(
                    flowId = "fl_C", daemonId = "dn_D1",
                    workspaceId = "ws_W2",
                    displayName = "project-b", absPath = "/b",
                    label = "Deploy",
                    metadata = mapOf("env" to "staging"),
                    registeredAt = "2026-04-01T10:00:00Z",
                    lastActivityTimestamp = "2026-04-01T10:03:00Z"
                )
            )
        )

        val hb = result.toDaemonHeartbeat()
        assertEquals("dn_D1", hb.daemonId)
        assertEquals(2, hb.workspaces.size)

        val ws1 = hb.workspaces.find {
            it.workspaceId == "ws_W1" }!!
        assertEquals("project-a", ws1.displayName)
        assertEquals(2, ws1.activeFlows.size)
        assertEquals("Build", ws1.activeFlows[0].label)
        assertEquals("main",
            ws1.activeFlows[0].metadata["branch"])

        val ws2 = hb.workspaces.find {
            it.workspaceId == "ws_W2" }!!
        assertEquals("staging",
            ws2.activeFlows[0].metadata["env"])
    }

    @Test
    fun toDaemonHeartbeat_emptyFlows() {
        val result = ActiveFlowsResult(flows = emptyList())
        val hb = result.toDaemonHeartbeat()
        assertEquals("", hb.daemonId)
        assertTrue(hb.workspaces.isEmpty())
    }

    @Test
    fun metadataDefaultsToEmpty() {
        val json = """
            {
                "flows": [
                    {
                        "flow_id": "fl_NOMETA",
                        "daemon_id": "dn_D1",
                        "workspace_id": "ws_W1",
                        "registered_at": "2026-04-01T10:00:00Z",
                        "last_activity_timestamp": "2026-04-01T10:00:00Z"
                    }
                ]
            }
        """.trimIndent()

        val result = ActiveFlowsResult.fromJson(json)
        assertTrue(result.flows[0].metadata.isEmpty())
    }
}
