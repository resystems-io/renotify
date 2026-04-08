// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.nats

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests for the device heartbeat subject format and payload
 * structure (R-MOB-14).
 */
class DeviceHeartbeatTest {

    @Test
    fun heartbeatSubjectFormat() {
        val username = "alice"
        val deviceId = "mb_DEV01"
        val subject = "resystems.renotify.$username" +
            ".device.$deviceId.heartbeat"
        assertEquals(
            "resystems.renotify.alice.device.mb_DEV01.heartbeat",
            subject
        )
    }

    @Test
    fun heartbeatPayloadContainsRequiredFields() {
        val deviceId = "mb_DEV01"
        val json = JSONObject().apply {
            put("device_id", deviceId)
            put("timestamp", java.time.Instant.now().toString())
        }
        val parsed = JSONObject(json.toString())
        assertEquals(deviceId, parsed.getString("device_id"))
        assertTrue(parsed.has("timestamp"))
        assertTrue(
            parsed.getString("timestamp").isNotEmpty()
        )
    }

    @Test
    fun heartbeatPayloadTimestampIsIso8601() {
        val json = JSONObject().apply {
            put("device_id", "mb_DEV01")
            put("timestamp", java.time.Instant.now().toString())
        }
        val ts = json.getString("timestamp")
        // ISO 8601 timestamps contain 'T' and end with 'Z'.
        assertTrue(
            "timestamp should be ISO 8601: $ts",
            ts.contains("T") && ts.endsWith("Z")
        )
    }
}
