package io.resystems.renotify.nats

import io.resystems.renotify.pairing.ProvisioningPayload
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

class NatsOptionsBuilderTest {

    private val payload = ProvisioningPayload(
        version = 1,
        host = "192.168.1.42",
        port = 4223,
        token = "rn_tk_0A1B2C3D4E5F6G7H8J9K0M1N2P3Q4R5S6T7V8W9X0Y1Z2A3B4C5D",
        certFingerprint = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
        username = "testuser"
    )

    @Test
    fun build_producesWssUrl() {
        val opts = NatsOptionsBuilder.build(payload)
        val servers = opts.servers
        assertNotNull(servers)
        val urls = servers.map { it.toString() }
        assertTrue(
            "Expected wss:// URL, got $urls",
            urls.any { it.startsWith("wss://") }
        )
        assertTrue(
            "Expected host:port in URL, got $urls",
            urls.any {
                it.contains("192.168.1.42") &&
                    it.contains("4223")
            }
        )
    }

    @Test
    fun build_setsSSLContext() {
        val opts = NatsOptionsBuilder.build(payload)
        assertNotNull("SSLContext should be set", opts.sslContext)
    }

    @Test
    fun build_setsUserInfo() {
        val opts = NatsOptionsBuilder.build(payload)
        assertEquals("mobile", opts.username)
    }

    @Test
    fun build_disablesAutoReconnect() {
        val opts = NatsOptionsBuilder.build(payload)
        // noReconnect() sets maxReconnect to 0.
        assertEquals(0, opts.maxReconnect)
    }

    @Test
    fun build_setsConnectionName() {
        val opts = NatsOptionsBuilder.build(payload)
        assertEquals("renotify-mobile", opts.connectionName)
    }

    @Test
    fun build_ipv6Host_bracketed() {
        val ipv6Payload = payload.copy(host = "::1")
        val opts = NatsOptionsBuilder.build(ipv6Payload)
        val urls = opts.servers.map { it.toString() }
        assertTrue(
            "IPv6 should be bracketed, got $urls",
            urls.any { it.contains("[::1]") }
        )
    }
}
