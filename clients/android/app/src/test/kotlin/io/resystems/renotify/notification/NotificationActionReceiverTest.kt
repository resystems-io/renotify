package io.resystems.renotify.notification

import io.resystems.renotify.nats.NatsService
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests for [NatsService.buildResponseJson] — the pure function
 * that constructs NotificationResponse JSON from action
 * parameters.
 */
class NotificationActionReceiverTest {

    @Test
    fun booleanAccepted_setsAcceptedTrue() {
        val json = NatsService.buildResponseJson(
            requestId = "ntf_TEST01",
            actionType = NotificationRenderer.ACTION_TYPE_ACCEPTED,
            actionValue = "true",
            text = null
        )
        val obj = JSONObject(json)

        assertEquals("ntf_TEST01", obj.getString("request_id"))
        assertTrue(obj.getBoolean("accepted"))
        assertFalse(obj.has("action"))
        assertFalse(obj.has("text"))
        assertTrue(obj.has("timestamp"))
    }

    @Test
    fun booleanRejected_setsAcceptedFalse() {
        val json = NatsService.buildResponseJson(
            requestId = "ntf_TEST02",
            actionType = NotificationRenderer.ACTION_TYPE_REJECTED,
            actionValue = "false",
            text = null
        )
        val obj = JSONObject(json)

        assertEquals("ntf_TEST02", obj.getString("request_id"))
        assertFalse(obj.getBoolean("accepted"))
    }

    @Test
    fun choiceAction_setsActionField() {
        val json = NatsService.buildResponseJson(
            requestId = "ntf_TEST03",
            actionType = NotificationRenderer.ACTION_TYPE_CHOICE,
            actionValue = "Approve",
            text = null
        )
        val obj = JSONObject(json)

        assertEquals("ntf_TEST03", obj.getString("request_id"))
        assertEquals("Approve", obj.getString("action"))
        assertFalse(obj.has("accepted"))
    }

    @Test
    fun textResponse_setsTextField() {
        val json = NatsService.buildResponseJson(
            requestId = "ntf_TEST04",
            actionType = NotificationRenderer.ACTION_TYPE_TEXT,
            actionValue = "",
            text = "Looks good to me"
        )
        val obj = JSONObject(json)

        assertEquals("ntf_TEST04", obj.getString("request_id"))
        assertEquals("Looks good to me", obj.getString("text"))
        assertFalse(obj.has("accepted"))
        assertFalse(obj.has("action"))
    }

    @Test
    fun textResponse_nullText_omitsField() {
        val json = NatsService.buildResponseJson(
            requestId = "ntf_TEST05",
            actionType = NotificationRenderer.ACTION_TYPE_TEXT,
            actionValue = "",
            text = null
        )
        val obj = JSONObject(json)

        assertEquals("ntf_TEST05", obj.getString("request_id"))
        assertFalse(obj.has("text"))
    }

    @Test
    fun timestamp_alwaysPresent() {
        val json = NatsService.buildResponseJson(
            requestId = "ntf_TEST06",
            actionType = NotificationRenderer.ACTION_TYPE_ACCEPTED,
            actionValue = "true",
            text = null
        )
        val obj = JSONObject(json)

        assertTrue(obj.has("timestamp"))
        val ts = obj.getString("timestamp")
        assertTrue(
            "timestamp should be ISO-8601, got $ts",
            ts.contains("T") || ts.contains("Z")
        )
    }
}
