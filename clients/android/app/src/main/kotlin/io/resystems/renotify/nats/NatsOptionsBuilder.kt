package io.resystems.renotify.nats

import io.nats.client.Options
import io.resystems.renotify.pairing.PinnedSSLContext
import io.resystems.renotify.pairing.ProvisioningPayload

/**
 * Builds a jnats [Options] from a [ProvisioningPayload]. This is
 * a pure function with no Android dependencies, making it
 * JVM-testable.
 *
 * The NATS auth uses username `"mobile"` with the pairing token
 * as the password. See docs/analysis-nats-transport-design.md
 * Section 6.4.
 */
object NatsOptionsBuilder {

    /** NATS auth username for the mobile account. */
    private const val NATS_USERNAME = "mobile"

    /**
     * Build jnats connection options from provisioning
     * credentials.
     *
     * - WSS URL with TLS fingerprint pinning
     * - Username/password auth (mobile + pairing token)
     * - Auto-reconnect disabled (managed manually with
     *   exponential backoff in [NatsConnectionManager])
     */
    fun build(payload: ProvisioningPayload): Options {
        val host = formatHost(payload.host, payload.port)
        val sslContext = PinnedSSLContext.create(
            payload.certFingerprint
        )

        return Options.Builder()
            .server("wss://$host")
            .sslContext(sslContext)
            .userInfo(NATS_USERNAME, payload.token)
            .noReconnect()
            .connectionName("renotify-mobile")
            .build()
    }

    /**
     * Format host:port, wrapping IPv6 addresses in brackets.
     */
    private fun formatHost(host: String, port: Int): String {
        val h = if (host.contains(':')) "[$host]" else host
        return "$h:$port"
    }
}
