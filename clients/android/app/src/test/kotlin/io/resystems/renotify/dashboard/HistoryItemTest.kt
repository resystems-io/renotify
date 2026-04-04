package io.resystems.renotify.dashboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HistoryItemTest {

    @Test
    fun parsesResultWithResponse() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_001",
                            "flow_id": "fl_001",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Deploy?",
                            "body": "Deploy to prod",
                            "response_types": ["boolean"],
                            "priority": "high",
                            "source": "ci/deploy",
                            "timestamp": "2026-04-01T10:00:00Z"
                        },
                        "Response": {
                            "request_id": "ntf_001",
                            "accepted": true,
                            "timestamp": "2026-04-01T10:01:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val result = HistoryQueryResult.fromJson(json)
        assertEquals(1, result.total)
        assertEquals(1, result.records.size)

        val rec = result.records[0]
        assertEquals("ntf_001", rec.id)
        assertEquals("fl_001", rec.flowId)
        assertEquals("Deploy?", rec.title)
        assertEquals("Deploy to prod", rec.body)
        assertEquals("high", rec.priority)
        assertEquals("ci/deploy", rec.source)
        assertTrue(rec.hasResponse)
        assertEquals(true, rec.responseAccepted)
        assertEquals("accepted", rec.responseSummary)
    }

    @Test
    fun parsesResultWithoutResponse() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_002",
                            "flow_id": "fl_002",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Build complete",
                            "response_types": ["none"],
                            "priority": "normal",
                            "source": "ci",
                            "timestamp": "2026-04-01T11:00:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val result = HistoryQueryResult.fromJson(json)
        val rec = result.records[0]
        assertEquals("ntf_002", rec.id)
        assertNull(rec.body)
        assertNull(rec.responseAccepted)
        assertEquals("\u2014", rec.responseSummary)
    }

    @Test
    fun parsesChoiceResponse() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_003",
                            "flow_id": "fl_003",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Pick env",
                            "response_types": ["choice"],
                            "priority": "normal",
                            "actions": ["staging", "prod"],
                            "timestamp": "2026-04-01T12:00:00Z"
                        },
                        "Response": {
                            "request_id": "ntf_003",
                            "action": "staging",
                            "timestamp": "2026-04-01T12:01:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val result = HistoryQueryResult.fromJson(json)
        val rec = result.records[0]
        assertTrue(rec.hasResponse)
        assertEquals("staging", rec.responseSummary)
    }

    @Test
    fun parsesTextResponse() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_004",
                            "flow_id": "fl_004",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Feedback?",
                            "response_types": ["text"],
                            "priority": "normal",
                            "timestamp": "2026-04-01T13:00:00Z"
                        },
                        "Response": {
                            "request_id": "ntf_004",
                            "text": "Looks good to me",
                            "timestamp": "2026-04-01T13:01:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val result = HistoryQueryResult.fromJson(json)
        val rec = result.records[0]
        assertTrue(rec.hasResponse)
        assertEquals("Looks good to me", rec.responseSummary)
    }

    @Test
    fun parsesDeniedResponse() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_005",
                            "flow_id": "fl_005",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Permission: Bash",
                            "response_types": ["boolean"],
                            "priority": "high",
                            "timestamp": "2026-04-01T14:00:00Z"
                        },
                        "Response": {
                            "request_id": "ntf_005",
                            "accepted": false,
                            "timestamp": "2026-04-01T14:00:10Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val result = HistoryQueryResult.fromJson(json)
        val rec = result.records[0]
        assertEquals(false, rec.responseAccepted)
        assertEquals("denied", rec.responseSummary)
    }

    @Test
    fun parsesEmptyResult() {
        val json = """{"records":[],"total":0}"""
        val result = HistoryQueryResult.fromJson(json)
        assertEquals(0, result.total)
        assertTrue(result.records.isEmpty())
    }

    @Test
    fun parsesMultipleRecords() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_A",
                            "flow_id": "fl_A",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "First",
                            "response_types": ["none"],
                            "priority": "normal",
                            "timestamp": "2026-04-01T10:00:00Z"
                        }
                    },
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_B",
                            "flow_id": "fl_B",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Second",
                            "response_types": ["none"],
                            "priority": "low",
                            "timestamp": "2026-04-01T09:00:00Z"
                        }
                    }
                ],
                "total": 5
            }
        """.trimIndent()

        val result = HistoryQueryResult.fromJson(json)
        assertEquals(5, result.total)
        assertEquals(2, result.records.size)
        assertEquals("ntf_A", result.records[0].id)
        assertEquals("ntf_B", result.records[1].id)
    }

    // --- hasResponse ---

    @Test
    fun hasResponse_allNull_isFalse() {
        val rec = record()
        assertFalse(rec.hasResponse)
    }

    @Test
    fun hasResponse_acceptedTrue_isTrue() {
        val rec = record(responseAccepted = true)
        assertTrue(rec.hasResponse)
    }

    @Test
    fun hasResponse_actionOnly_isTrue() {
        val rec = record(responseAction = "staging")
        assertTrue(rec.hasResponse)
    }

    @Test
    fun hasResponse_textOnly_isTrue() {
        val rec = record(responseText = "ok")
        assertTrue(rec.hasResponse)
    }

    // --- responseSummary truncation boundary ---

    @Test
    fun responseSummary_exactly30Chars_noTruncation() {
        val text = "a".repeat(30)
        val rec = record(responseText = text)
        assertEquals(text, rec.responseSummary)
    }

    @Test
    fun responseSummary_31Chars_truncatedWithEllipsis() {
        val text = "a".repeat(31)
        val rec = record(responseText = text)
        assertEquals("a".repeat(27) + "...", rec.responseSummary)
    }

    // --- isExpandable ---

    @Test
    fun isExpandable_withBody_isTrue() {
        val rec = record(body = "Deploy details")
        assertTrue(rec.isExpandable)
    }

    @Test
    fun isExpandable_nullBody_noLongText_isFalse() {
        val rec = record()
        assertFalse(rec.isExpandable)
    }

    @Test
    fun isExpandable_nullBody_shortText_isFalse() {
        val rec = record(responseText = "short")
        assertFalse(rec.isExpandable)
    }

    @Test
    fun isExpandable_nullBody_exactly30CharText_isFalse() {
        val rec = record(responseText = "a".repeat(30))
        assertFalse(rec.isExpandable)
    }

    @Test
    fun isExpandable_nullBody_31CharText_isTrue() {
        val rec = record(responseText = "a".repeat(31))
        assertTrue(rec.isExpandable)
    }

    @Test
    fun isExpandable_bodyAndLongText_isTrue() {
        val rec = record(
            body = "details",
            responseText = "a".repeat(40)
        )
        assertTrue(rec.isExpandable)
    }

    // --- fromJson: responseTimestamp ---

    @Test
    fun parsesResponseTimestamp() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_006",
                            "flow_id": "fl_006",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Approve?",
                            "response_types": ["boolean"],
                            "priority": "normal",
                            "timestamp": "2026-04-01T15:00:00Z"
                        },
                        "Response": {
                            "request_id": "ntf_006",
                            "accepted": true,
                            "timestamp": "2026-04-01T15:00:30Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val rec = HistoryQueryResult.fromJson(json).records[0]
        assertEquals("2026-04-01T15:00:30Z", rec.responseTimestamp)
    }

    @Test
    fun missingResponseTimestamp_isNull() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_007",
                            "flow_id": "fl_007",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Approve?",
                            "response_types": ["boolean"],
                            "priority": "normal",
                            "timestamp": "2026-04-01T16:00:00Z"
                        },
                        "Response": {
                            "request_id": "ntf_007",
                            "accepted": false
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val rec = HistoryQueryResult.fromJson(json).records[0]
        assertNull(rec.responseTimestamp)
    }

    // --- isInteractive ---

    @Test
    fun isInteractive_none_isFalse() {
        val rec = record(responseTypes = listOf("none"))
        assertFalse(rec.isInteractive)
    }

    @Test
    fun isInteractive_boolean_isTrue() {
        val rec = record(responseTypes = listOf("boolean"))
        assertTrue(rec.isInteractive)
    }

    @Test
    fun isInteractive_choice_isTrue() {
        val rec = record(responseTypes = listOf("choice"))
        assertTrue(rec.isInteractive)
    }

    @Test
    fun isInteractive_text_isTrue() {
        val rec = record(responseTypes = listOf("text"))
        assertTrue(rec.isInteractive)
    }

    @Test
    fun isInteractive_mixed_isTrue() {
        val rec = record(responseTypes = listOf("boolean", "text"))
        assertTrue(rec.isInteractive)
    }

    // --- isOpen ---

    @Test
    fun isOpen_interactiveNoResponse_isTrue() {
        val rec = record(responseTypes = listOf("boolean"))
        assertTrue(rec.isOpen)
    }

    @Test
    fun isOpen_interactiveWithResponse_isFalse() {
        val rec = record(
            responseTypes = listOf("boolean"),
            responseAccepted = true
        )
        assertFalse(rec.isOpen)
    }

    @Test
    fun isOpen_fireAndForget_isFalse() {
        val rec = record(responseTypes = listOf("none"))
        assertFalse(rec.isOpen)
    }

    // --- fromJson: responseTypes and actions ---

    @Test
    fun parsesResponseTypes() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_008",
                            "flow_id": "fl_008",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Pick env",
                            "response_types": ["choice"],
                            "priority": "normal",
                            "actions": ["staging", "prod"],
                            "timestamp": "2026-04-01T17:00:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val rec = HistoryQueryResult.fromJson(json).records[0]
        assertEquals(listOf("choice"), rec.responseTypes)
        assertEquals(listOf("staging", "prod"), rec.actions)
        assertTrue(rec.isInteractive)
        assertTrue(rec.isOpen)
    }

    @Test
    fun missingResponseTypes_defaultsToNone() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_009",
                            "flow_id": "fl_009",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Alert",
                            "priority": "normal",
                            "timestamp": "2026-04-01T18:00:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val rec = HistoryQueryResult.fromJson(json).records[0]
        assertEquals(listOf("none"), rec.responseTypes)
        assertNull(rec.actions)
        assertFalse(rec.isInteractive)
    }

    @Test
    fun missingActions_isNull() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_010",
                            "flow_id": "fl_010",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Approve?",
                            "response_types": ["boolean"],
                            "priority": "normal",
                            "timestamp": "2026-04-01T19:00:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val rec = HistoryQueryResult.fromJson(json).records[0]
        assertNull(rec.actions)
    }

    // --- fromJson: flow context ---

    @Test
    fun parsesFlowContext() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "FlowLabel": "CI Pipeline",
                        "WorkspaceName": "renotify",
                        "WorkspacePath": "/home/user/renotify",
                        "Request": {
                            "id": "ntf_011",
                            "flow_id": "fl_011",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Build done",
                            "response_types": ["none"],
                            "priority": "normal",
                            "timestamp": "2026-04-01T20:00:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val rec = HistoryQueryResult.fromJson(json).records[0]
        assertEquals("CI Pipeline", rec.flowLabel)
        assertEquals("renotify", rec.workspaceName)
    }

    @Test
    fun missingFlowContext_isNull() {
        val json = """
            {
                "records": [
                    {
                        "Username": "testuser",
                        "Request": {
                            "id": "ntf_012",
                            "flow_id": "fl_012",
                            "daemon_id": "dn_001",
                            "workspace_id": "ws_001",
                            "title": "Old record",
                            "response_types": ["none"],
                            "priority": "normal",
                            "timestamp": "2026-04-01T21:00:00Z"
                        }
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val rec = HistoryQueryResult.fromJson(json).records[0]
        assertNull(rec.flowLabel)
        assertNull(rec.workspaceName)
    }

    // --- Helper ---

    private fun record(
        body: String? = null,
        responseTypes: List<String> = listOf("none"),
        actions: List<String>? = null,
        responseAccepted: Boolean? = null,
        responseAction: String? = null,
        responseText: String? = null,
        responseTimestamp: String? = null
    ) = HistoryRecord(
        id = "ntf_test",
        flowId = "fl_test",
        workspaceId = "ws_test",
        flowLabel = null,
        workspaceName = null,
        title = "Test",
        body = body,
        priority = "normal",
        source = "",
        responseTypes = responseTypes,
        actions = actions,
        timestamp = "2026-04-01T10:00:00Z",
        responseAccepted = responseAccepted,
        responseAction = responseAction,
        responseText = responseText,
        responseTimestamp = responseTimestamp
    )
}
