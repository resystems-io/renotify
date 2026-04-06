// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.pairing

import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager

/**
 * Factory for creating an [SSLContext] that pins TLS connections
 * to a specific certificate fingerprint. Used by M-02 (NATS
 * Client Service) to establish the WSS connection.
 */
object PinnedSSLContext {

    /**
     * Create an [SSLContext] that trusts only the server
     * certificate matching [fingerprint] (hex-encoded SHA-256).
     */
    fun create(fingerprint: String): SSLContext {
        val tm = FingerprintTrustManager(fingerprint)
        val ctx = SSLContext.getInstance("TLS")
        ctx.init(null, arrayOf<TrustManager>(tm), null)
        return ctx
    }
}
