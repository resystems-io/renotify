// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.nats

import org.junit.Assert.assertEquals
import org.junit.Test

class BackoffTest {

    @Test
    fun attempt0_1s() {
        assertEquals(1000L, NatsConnectionManager.backoffDelay(0))
    }

    @Test
    fun attempt1_2s() {
        assertEquals(2000L, NatsConnectionManager.backoffDelay(1))
    }

    @Test
    fun attempt2_4s() {
        assertEquals(4000L, NatsConnectionManager.backoffDelay(2))
    }

    @Test
    fun attempt3_8s() {
        assertEquals(8000L, NatsConnectionManager.backoffDelay(3))
    }

    @Test
    fun attempt4_16s() {
        assertEquals(16000L, NatsConnectionManager.backoffDelay(4))
    }

    @Test
    fun attempt5_30s_capped() {
        assertEquals(30000L, NatsConnectionManager.backoffDelay(5))
    }

    @Test
    fun attempt99_capped() {
        assertEquals(30000L, NatsConnectionManager.backoffDelay(99))
    }
}
