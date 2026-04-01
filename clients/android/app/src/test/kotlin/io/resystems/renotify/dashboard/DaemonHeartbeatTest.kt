package io.resystems.renotify.dashboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DaemonHeartbeatTest {

    @Test
    fun parsesFullHeartbeat() {
        val json = """
            {
                "daemon_id": "dn_3G2K7V9WNFQ4J",
                "username": "stewart",
                "hostname": "dev-laptop",
                "workspaces": [
                    {
                        "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
                        "display_name": "renotify",
                        "abs_path": "/home/stewart/projects/renotify",
                        "active_flows": ["fl_FLOW01", "fl_FLOW02"]
                    },
                    {
                        "workspace_id": "ws_R7CV4WFQE2NM1KGX",
                        "display_name": "gethos-api",
                        "abs_path": "/home/stewart/projects/gethos-api",
                        "active_flows": []
                    }
                ],
                "timestamp": "2026-03-27T14:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)

        assertEquals("dn_3G2K7V9WNFQ4J", hb.daemonId)
        assertEquals("stewart", hb.username)
        assertEquals("dev-laptop", hb.hostname)
        assertEquals(2, hb.workspaces.size)

        val ws0 = hb.workspaces[0]
        assertEquals("ws_5MBJR1HXNP3KQ8DW", ws0.workspaceId)
        assertEquals("renotify", ws0.displayName)
        assertEquals(
            "/home/stewart/projects/renotify", ws0.absPath)
        assertEquals(
            listOf("fl_FLOW01", "fl_FLOW02"), ws0.activeFlows)

        val ws1 = hb.workspaces[1]
        assertEquals("gethos-api", ws1.displayName)
        assertTrue(ws1.activeFlows.isEmpty())
    }

    @Test
    fun parsesEmptyWorkspaces() {
        val json = """
            {
                "daemon_id": "dn_EMPTY01",
                "username": "testuser",
                "hostname": "test-host",
                "workspaces": [],
                "timestamp": "2026-03-27T14:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)

        assertTrue(hb.workspaces.isEmpty())
    }

    @Test
    fun parsesWithMissingOptionalFields() {
        val json = """
            {
                "daemon_id": "dn_MINIMAL",
                "workspaces": [
                    {
                        "workspace_id": "ws_MIN01"
                    }
                ],
                "timestamp": "2026-03-27T14:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)

        assertEquals("", hb.username)
        assertEquals("", hb.hostname)
        assertEquals(1, hb.workspaces.size)
        assertEquals("", hb.workspaces[0].displayName)
        assertEquals("", hb.workspaces[0].absPath)
        assertTrue(hb.workspaces[0].activeFlows.isEmpty())
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsMissingDaemonId() {
        DaemonHeartbeat.fromJson("""
            {
                "username": "test",
                "workspaces": [],
                "timestamp": "2026-03-27T14:00:00Z"
            }
        """.trimIndent())
    }

    @Test
    fun parsesNullWorkspacesAsEmpty() {
        val json = """
            {
                "daemon_id": "dn_NULL01",
                "timestamp": "2026-03-27T14:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)
        assertTrue(hb.workspaces.isEmpty())
    }
}
