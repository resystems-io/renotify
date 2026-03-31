package io.resystems.renotify.notification

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationPayloadTest {

    @Test
    fun fireAndForget_parsesCorrectly() {
        val json = """
            {
                "id": "ntf_TEST0001",
                "flow_id": "fl_FLOW0001",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "title": "Build complete",
                "body": "All 42 tests passed.",
                "response_types": ["none"],
                "priority": "normal",
                "source": "ci/pipeline",
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        val payload = NotificationPayload.fromJson(json)

        assertEquals("ntf_TEST0001", payload.id)
        assertEquals("fl_FLOW0001", payload.flowId)
        assertEquals("dn_DAEMON01", payload.daemonId)
        assertEquals("ws_WORK0001", payload.workspaceId)
        assertEquals("Build complete", payload.title)
        assertEquals("All 42 tests passed.", payload.body)
        assertEquals(listOf("none"), payload.responseTypes)
        assertEquals("normal", payload.priority)
        assertEquals("ci/pipeline", payload.source)
        assertNull(payload.actions)
        assertNull(payload.timeoutSec)
        assertTrue(payload.isFireAndForget)
        assertFalse(payload.isInteractive)
    }

    @Test
    fun booleanInteractive_parsesCorrectly() {
        val json = """
            {
                "id": "ntf_TEST0002",
                "flow_id": "fl_FLOW0002",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "title": "Deploy?",
                "response_types": ["boolean"],
                "priority": "high",
                "source": "cd/deploy",
                "timeout_sec": 300,
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        val payload = NotificationPayload.fromJson(json)

        assertEquals(listOf("boolean"), payload.responseTypes)
        assertEquals("high", payload.priority)
        assertEquals(300, payload.timeoutSec)
        assertFalse(payload.isFireAndForget)
        assertTrue(payload.isInteractive)
    }

    @Test
    fun choiceInteractive_parsesActions() {
        val json = """
            {
                "id": "ntf_TEST0003",
                "flow_id": "fl_FLOW0003",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "title": "Select environment",
                "response_types": ["choice"],
                "priority": "high",
                "source": "cd/deploy",
                "actions": ["Staging", "Production", "Skip"],
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        val payload = NotificationPayload.fromJson(json)

        assertNotNull(payload.actions)
        assertEquals(
            listOf("Staging", "Production", "Skip"),
            payload.actions
        )
    }

    @Test
    fun multiModal_parsesBooleanAndText() {
        val json = """
            {
                "id": "ntf_TEST0004",
                "flow_id": "fl_FLOW0004",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "title": "Proceed with migration?",
                "body": "3 pending migrations on prod.",
                "response_types": ["boolean", "text"],
                "priority": "high",
                "source": "cd/migrate",
                "timeout_sec": 600,
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        val payload = NotificationPayload.fromJson(json)

        assertEquals(
            listOf("boolean", "text"),
            payload.responseTypes
        )
        assertTrue(payload.isInteractive)
    }

    @Test(expected = IllegalArgumentException::class)
    fun missingId_throws() {
        val json = """
            {
                "flow_id": "fl_FLOW0001",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "title": "Test",
                "response_types": ["none"],
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        NotificationPayload.fromJson(json)
    }

    @Test(expected = IllegalArgumentException::class)
    fun missingTitle_throws() {
        val json = """
            {
                "id": "ntf_TEST0001",
                "flow_id": "fl_FLOW0001",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "response_types": ["none"],
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        NotificationPayload.fromJson(json)
    }

    @Test(expected = IllegalArgumentException::class)
    fun emptyResponseTypes_throws() {
        val json = """
            {
                "id": "ntf_TEST0001",
                "flow_id": "fl_FLOW0001",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "title": "Test",
                "response_types": [],
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        NotificationPayload.fromJson(json)
    }

    @Test
    fun unknownResponseType_preservedForForwardCompat() {
        val json = """
            {
                "id": "ntf_TEST0005",
                "flow_id": "fl_FLOW0005",
                "daemon_id": "dn_DAEMON01",
                "workspace_id": "ws_WORK0001",
                "title": "Test",
                "response_types": ["future_type"],
                "timestamp": "2026-03-31T10:00:00Z"
            }
        """.trimIndent()

        val payload = NotificationPayload.fromJson(json)

        assertEquals(listOf("future_type"), payload.responseTypes)
        // Unknown types are interactive (not fire-and-forget).
        assertTrue(payload.isInteractive)
    }

    @Test
    fun priorityMapping() {
        for (priority in listOf("low", "normal", "high")) {
            val json = """
                {
                    "id": "ntf_PRI_$priority",
                    "flow_id": "fl_FLOW0001",
                    "daemon_id": "dn_DAEMON01",
                    "workspace_id": "ws_WORK0001",
                    "title": "Test",
                    "response_types": ["none"],
                    "priority": "$priority",
                    "timestamp": "2026-03-31T10:00:00Z"
                }
            """.trimIndent()

            val payload = NotificationPayload.fromJson(json)
            assertEquals(priority, payload.priority)
        }
    }

    @Test
    fun channelSelection_highPriority_urgent() {
        assertEquals(
            NotificationRenderer.CHANNEL_URGENT,
            NotificationRenderer.selectChannel(
                makePayload(priority = "high", responseTypes = listOf("none"))
            )
        )
    }

    @Test
    fun channelSelection_interactive_urgent() {
        assertEquals(
            NotificationRenderer.CHANNEL_URGENT,
            NotificationRenderer.selectChannel(
                makePayload(priority = "normal", responseTypes = listOf("boolean"))
            )
        )
    }

    @Test
    fun channelSelection_normalFireAndForget_default() {
        assertEquals(
            NotificationRenderer.CHANNEL_NOTIFICATIONS,
            NotificationRenderer.selectChannel(
                makePayload(priority = "normal", responseTypes = listOf("none"))
            )
        )
    }

    @Test
    fun androidNotificationId_deterministic() {
        val id1 = NotificationRenderer.androidNotificationId("ntf_TEST0001")
        val id2 = NotificationRenderer.androidNotificationId("ntf_TEST0001")
        assertEquals(id1, id2)
    }

    @Test
    fun androidNotificationId_distinct() {
        val id1 = NotificationRenderer.androidNotificationId("ntf_TEST0001")
        val id2 = NotificationRenderer.androidNotificationId("ntf_TEST0002")
        assertTrue(
            "Different notification IDs should produce different Android IDs",
            id1 != id2
        )
    }

    private fun makePayload(
        priority: String = "normal",
        responseTypes: List<String> = listOf("none")
    ) = NotificationPayload(
        id = "ntf_TEST",
        flowId = "fl_TEST",
        daemonId = "dn_TEST",
        workspaceId = "ws_TEST",
        title = "Test",
        body = null,
        responseTypes = responseTypes,
        priority = priority,
        source = "test",
        actions = null,
        timeoutSec = null,
        timestamp = "2026-03-31T10:00:00Z"
    )
}
