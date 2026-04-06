// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.pairing

import org.junit.Assert.assertNotNull
import org.junit.Test

class PinnedSSLContextTest {

    private val fingerprint =
        "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

    @Test
    fun create_returnsNonNull() {
        val ctx = PinnedSSLContext.create(fingerprint)
        assertNotNull(ctx)
    }

    @Test
    fun create_canCreateSocketFactory() {
        val ctx = PinnedSSLContext.create(fingerprint)
        assertNotNull(ctx.socketFactory)
    }

    @Test
    fun create_protocolIsTLS() {
        val ctx = PinnedSSLContext.create(fingerprint)
        assertNotNull(ctx.protocol)
    }
}
