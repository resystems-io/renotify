package io.resystems.renotify.dashboard

import org.junit.Assert.assertEquals
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
}
