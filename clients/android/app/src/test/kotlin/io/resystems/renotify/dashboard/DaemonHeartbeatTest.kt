// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

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
                "username": "alice",
                "hostname": "dev-laptop",
                "workspaces": [
                    {
                        "workspace_id": "ws_5MBJR1HXNP3KQ8DW",
                        "display_name": "renotify",
                        "abs_path": "/home/alice/projects/renotify",
                        "active_flows": [
                            {"flow_id": "fl_FLOW01", "label": "Build"},
                            {"flow_id": "fl_FLOW02", "label": "Test", "metadata": {"branch": "main"}}
                        ]
                    },
                    {
                        "workspace_id": "ws_R7CV4WFQE2NM1KGX",
                        "display_name": "example-api",
                        "abs_path": "/home/alice/projects/example-api",
                        "active_flows": []
                    }
                ],
                "timestamp": "2026-03-27T14:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)

        assertEquals("dn_3G2K7V9WNFQ4J", hb.daemonId)
        assertEquals("alice", hb.username)
        assertEquals("dev-laptop", hb.hostname)
        assertEquals(2, hb.workspaces.size)

        val ws0 = hb.workspaces[0]
        assertEquals("ws_5MBJR1HXNP3KQ8DW", ws0.workspaceId)
        assertEquals("renotify", ws0.displayName)
        assertEquals(2, ws0.activeFlows.size)
        assertEquals("fl_FLOW01", ws0.activeFlows[0].flowId)
        assertEquals("Build", ws0.activeFlows[0].label)
        assertEquals("fl_FLOW02", ws0.activeFlows[1].flowId)
        assertEquals("main",
            ws0.activeFlows[1].metadata["branch"])

        val ws1 = hb.workspaces[1]
        assertEquals("example-api", ws1.displayName)
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

    @Test
    fun flowMetadataOmittedDefaultsToEmpty() {
        val json = """
            {
                "daemon_id": "dn_META01",
                "workspaces": [
                    {
                        "workspace_id": "ws_01",
                        "active_flows": [
                            {"flow_id": "fl_NOMETA"}
                        ]
                    }
                ],
                "timestamp": "2026-04-02T10:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)
        val flow = hb.workspaces[0].activeFlows[0]
        assertEquals("fl_NOMETA", flow.flowId)
        assertEquals("", flow.label)
        assertTrue(flow.metadata.isEmpty())
    }

    // --- parseGoDuration ---

    @Test
    fun parseGoDuration_minutesAndSeconds() {
        assertEquals(900_000,
            DaemonHeartbeat.parseGoDuration("15m0s"))
    }

    @Test
    fun parseGoDuration_minutesOnly() {
        assertEquals(300_000,
            DaemonHeartbeat.parseGoDuration("5m"))
    }

    @Test
    fun parseGoDuration_hoursMinutes() {
        assertEquals(5_400_000,
            DaemonHeartbeat.parseGoDuration("1h30m"))
    }

    @Test
    fun parseGoDuration_secondsOnly() {
        assertEquals(45_000,
            DaemonHeartbeat.parseGoDuration("45s"))
    }

    @Test
    fun parseGoDuration_empty() {
        assertEquals(0, DaemonHeartbeat.parseGoDuration(""))
    }

    @Test
    fun parseGoDuration_hoursMinutesSeconds() {
        assertEquals(3_661_000,
            DaemonHeartbeat.parseGoDuration("1h1m1s"))
    }

    // --- gracePeriodMs in heartbeat ---

    @Test
    fun parsesGracePeriod() {
        val json = """
            {
                "daemon_id": "dn_GP01",
                "grace_period": "15m0s",
                "workspaces": [],
                "timestamp": "2026-04-05T10:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)
        assertEquals(900_000, hb.gracePeriodMs)
    }

    @Test
    fun missingGracePeriod_defaultsToZero() {
        val json = """
            {
                "daemon_id": "dn_GP02",
                "workspaces": [],
                "timestamp": "2026-04-05T10:00:00Z"
            }
        """.trimIndent()

        val hb = DaemonHeartbeat.fromJson(json)
        assertEquals(0, hb.gracePeriodMs)
    }
}
