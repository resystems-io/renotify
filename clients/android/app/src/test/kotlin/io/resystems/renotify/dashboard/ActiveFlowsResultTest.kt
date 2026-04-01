package io.resystems.renotify.dashboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ActiveFlowsResultTest {

    @Test
    fun parsesFlowsInTwoWorkspaces() {
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
                        "registered_at": "2026-04-01T10:00:00Z",
                        "last_activity_timestamp": "2026-04-01T10:01:00Z"
                    },
                    {
                        "flow_id": "fl_B",
                        "daemon_id": "dn_D1",
                        "workspace_id": "ws_W1",
                        "display_name": "project-a",
                        "abs_path": "/home/user/a",
                        "label": "Test",
                        "registered_at": "2026-04-01T10:00:00Z",
                        "last_activity_timestamp": "2026-04-01T10:02:00Z"
                    },
                    {
                        "flow_id": "fl_C",
                        "daemon_id": "dn_D1",
                        "workspace_id": "ws_W2",
                        "display_name": "project-b",
                        "abs_path": "/home/user/b",
                        "label": "Deploy",
                        "registered_at": "2026-04-01T10:00:00Z",
                        "last_activity_timestamp": "2026-04-01T10:03:00Z"
                    }
                ]
            }
        """.trimIndent()

        val result = ActiveFlowsResult.fromJson(json)
        assertEquals(3, result.flows.size)
        assertEquals("fl_A", result.flows[0].flowId)
        assertEquals("project-a", result.flows[0].displayName)
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
                    flowId = "fl_A",
                    daemonId = "dn_D1",
                    workspaceId = "ws_W1",
                    displayName = "project-a",
                    absPath = "/a",
                    label = "Build",
                    registeredAt = "2026-04-01T10:00:00Z",
                    lastActivityTimestamp = "2026-04-01T10:01:00Z"
                ),
                ActiveFlowEntry(
                    flowId = "fl_B",
                    daemonId = "dn_D1",
                    workspaceId = "ws_W1",
                    displayName = "project-a",
                    absPath = "/a",
                    label = "Test",
                    registeredAt = "2026-04-01T10:00:00Z",
                    lastActivityTimestamp = "2026-04-01T10:02:00Z"
                ),
                ActiveFlowEntry(
                    flowId = "fl_C",
                    daemonId = "dn_D1",
                    workspaceId = "ws_W2",
                    displayName = "project-b",
                    absPath = "/b",
                    label = "Deploy",
                    registeredAt = "2026-04-01T10:00:00Z",
                    lastActivityTimestamp = "2026-04-01T10:03:00Z"
                )
            )
        )

        val hb = result.toDaemonHeartbeat()
        assertEquals("dn_D1", hb.daemonId)
        assertEquals(2, hb.workspaces.size)

        val ws1 = hb.workspaces.find { it.workspaceId == "ws_W1" }!!
        assertEquals("project-a", ws1.displayName)
        assertEquals("/a", ws1.absPath)
        assertEquals(listOf("fl_A", "fl_B"), ws1.activeFlows)

        val ws2 = hb.workspaces.find { it.workspaceId == "ws_W2" }!!
        assertEquals("project-b", ws2.displayName)
        assertEquals(listOf("fl_C"), ws2.activeFlows)
    }

    @Test
    fun toDaemonHeartbeat_emptyFlows() {
        val result = ActiveFlowsResult(flows = emptyList())
        val hb = result.toDaemonHeartbeat()
        assertEquals("", hb.daemonId)
        assertTrue(hb.workspaces.isEmpty())
    }
}
