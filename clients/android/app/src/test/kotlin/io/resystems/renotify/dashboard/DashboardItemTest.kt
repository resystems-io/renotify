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
            workspaces = listOf(
                WorkspaceInfo(
                    workspaceId = "ws_01",
                    displayName = "myproject",
                    absPath = "/home/test/myproject",
                    activeFlows = listOf("fl_A", "fl_B")
                )
            ),
            timestamp = "2026-04-01T10:00:00Z"
        )

        val items = DashboardItem.fromHeartbeat(hb)

        assertEquals(3, items.size)

        val header = items[0] as DashboardItem.WorkspaceHeader
        assertEquals("myproject", header.displayName)
        assertEquals(2, header.flowCount)

        assertEquals("fl_A",
            (items[1] as DashboardItem.FlowItem).flowId)
        assertEquals("fl_B",
            (items[2] as DashboardItem.FlowItem).flowId)
    }

    @Test
    fun multipleWorkspaces_preserved() {
        val hb = DaemonHeartbeat(
            daemonId = "dn_TEST",
            username = "test",
            hostname = "host",
            workspaces = listOf(
                WorkspaceInfo(
                    workspaceId = "ws_01",
                    displayName = "project-a",
                    absPath = "/a",
                    activeFlows = listOf("fl_1")
                ),
                WorkspaceInfo(
                    workspaceId = "ws_02",
                    displayName = "project-b",
                    absPath = "/b",
                    activeFlows = listOf("fl_2", "fl_3")
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
