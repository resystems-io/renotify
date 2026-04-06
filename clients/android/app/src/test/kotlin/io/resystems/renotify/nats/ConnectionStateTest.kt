// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.nats

import org.junit.Assert.assertEquals
import org.junit.Test

class ConnectionStateTest {

    @Test
    fun disconnected_capturesTimestamp() {
        val ts = System.currentTimeMillis()
        val state = ConnectionState.Disconnected(since = ts)
        assertEquals(ts, state.since)
    }

    @Test
    fun error_capturesMessage() {
        val state = ConnectionState.Error("fingerprint mismatch")
        assertEquals("fingerprint mismatch", state.message)
    }

    @Test
    fun allStates_exhaustiveWhen() {
        // Compile-time exhaustiveness check via when expression.
        val states: List<ConnectionState> = listOf(
            ConnectionState.Idle,
            ConnectionState.Unpaired,
            ConnectionState.Connecting,
            ConnectionState.Connected,
            ConnectionState.Disconnected(since = 0),
            ConnectionState.Error(message = "test"),
        )
        for (s in states) {
            val label = when (s) {
                is ConnectionState.Idle -> "idle"
                is ConnectionState.Unpaired -> "unpaired"
                is ConnectionState.Connecting -> "connecting"
                is ConnectionState.Connected -> "connected"
                is ConnectionState.Disconnected -> "disconnected"
                is ConnectionState.Error -> "error"
            }
            assert(label.isNotEmpty())
        }
    }
}
