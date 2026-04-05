package io.resystems.renotify.dashboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DashboardItemTest {

    @Test
    fun nullHeartbeat_showsWaiting() {
        val items = DashboardItem.fromHeartbeat(null)

        assertEquals(1, items.size)
        assertTrue(items[0] is DashboardItem.EmptyState)
        assertTrue(
            (items[0] as DashboardItem.EmptyState)
                .message.contains("Waiting"))
    }

    @Test
    fun emptyWorkspaces_showsNoActive() {
        val hb = DaemonHeartbeat(
            daemonId = "dn_TEST",
            username = "test",
            hostname = "host",
            gracePeriodMs = 900_000,
            workspaces = emptyList(),
            timestamp = "2026-04-01T10:00:00Z"
        )

        val items = DashboardItem.fromHeartbeat(hb)

        assertEquals(1, items.size)
        assertTrue(items[0] is DashboardItem.EmptyState)
        assertTrue(
            (items[0] as DashboardItem.EmptyState)
                .message.contains("No active"))
    }

    @Test
    fun singleWorkspaceWithFlows() {
        val hb = DaemonHeartbeat(
            daemonId = "dn_TEST",
            username = "test",
            hostname = "host",
            gracePeriodMs = 900_000,
            workspaces = listOf(
                WorkspaceInfo(
                    workspaceId = "ws_01",
                    displayName = "myproject",
                    absPath = "/home/test/myproject",
                    activeFlows = listOf(
                        FlowInfo("fl_A", "Build",
                            mapOf("branch" to "main"), "2026-04-01T10:00:00Z"),
                        FlowInfo("fl_B", "Test", emptyMap(), "")
                    )
                )
            ),
            timestamp = "2026-04-01T10:00:00Z"
        )

        val items = DashboardItem.fromHeartbeat(hb)

        assertEquals(3, items.size)

        val header = items[0] as DashboardItem.WorkspaceHeader
        assertEquals("myproject", header.displayName)
        assertEquals(2, header.flowCount)

        val flow1 = items[1] as DashboardItem.FlowItem
        assertEquals("fl_A", flow1.flowId)
        assertEquals("Build", flow1.label)
        assertEquals("main", flow1.metadata["branch"])

        val flow2 = items[2] as DashboardItem.FlowItem
        assertEquals("fl_B", flow2.flowId)
        assertEquals("Test", flow2.label)
    }

    @Test
    fun multipleWorkspaces_preserved() {
        val hb = DaemonHeartbeat(
            daemonId = "dn_TEST",
            username = "test",
            hostname = "host",
            gracePeriodMs = 900_000,
            workspaces = listOf(
                WorkspaceInfo(
                    workspaceId = "ws_01",
                    displayName = "project-a",
                    absPath = "/a",
                    activeFlows = listOf(
                        FlowInfo("fl_1", "Deploy", emptyMap(), ""))
                ),
                WorkspaceInfo(
                    workspaceId = "ws_02",
                    displayName = "project-b",
                    absPath = "/b",
                    activeFlows = listOf(
                        FlowInfo("fl_2", "", emptyMap(), ""),
                        FlowInfo("fl_3", "CI", emptyMap(), ""))
                )
            ),
            timestamp = "2026-04-01T10:00:00Z"
        )

        val items = DashboardItem.fromHeartbeat(hb)

        // ws_01 header + 1 flow + ws_02 header + 2 flows = 5
        assertEquals(5, items.size)

        val h1 = items[0] as DashboardItem.WorkspaceHeader
        assertEquals("project-a", h1.displayName)

        val h2 = items[2] as DashboardItem.WorkspaceHeader
        assertEquals("project-b", h2.displayName)
    }

    @Test
    fun workspaceWithNoFlows_headerOnly() {
        val hb = DaemonHeartbeat(
            daemonId = "dn_TEST",
            username = "test",
            hostname = "host",
            gracePeriodMs = 900_000,
            workspaces = listOf(
                WorkspaceInfo(
                    workspaceId = "ws_01",
                    displayName = "empty-project",
                    absPath = "/empty",
                    activeFlows = emptyList()
                )
            ),
            timestamp = "2026-04-01T10:00:00Z"
        )

        val items = DashboardItem.fromHeartbeat(hb)

        assertEquals(1, items.size)
        val header = items[0] as DashboardItem.WorkspaceHeader
        assertEquals(0, header.flowCount)
    }
}
