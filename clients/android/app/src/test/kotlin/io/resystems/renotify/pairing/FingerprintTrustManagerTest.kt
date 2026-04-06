// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.pairing

import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test
import java.io.ByteArrayInputStream
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.util.Base64

class FingerprintTrustManagerTest {

    // Pre-computed ECDSA P-256 self-signed certificate generated
    // by the Go tlsgen package (CN=renotify-dn_TESTCERT001).
    private val certDerB64 =
        "MIIBlDCCATmgAwIBAgIQHfArAaPV4S+5A9+WR/xWuzAKBggqhkjOPQQDAjAi" +
        "MSAwHgYDVQQDDBdyZW5vdGlmeS1kbl9URVNUQ0VSVDAwMTAeFw0yNjAzMjkx" +
        "OTA3NTBaFw0yOTAzMjgxOTA3NTBaMCIxIDAeBgNVBAMMF3Jlbm90aWZ5LWRu" +
        "X1RFU1RDRVJUMDAxMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEjA5jR/Hy" +
        "AlVEVuqNZJJrAYSkt1hOGECbVgaG8yYeFYL1FfUyoD6dvEKx+SvfIwUjSYiF" +
        "b8c+FUY4igwUNkjXPKNRME8wDgYDVR0PAQH/BAQDAgeAMBMGA1UdJQQMMAoG" +
        "CCsGAQUFBwMBMAwGA1UdEwEB/wQCMAAwGgYDVR0RBBMwEYIJbG9jYWxob3N0" +
        "hwR/AAABMAoGCCqGSM49BAMCA0kAMEYCIQCSQGrM05DeeUyEu8IR6bZ4qJ/a" +
        "ARC9WpNJWZhnzplOcAIhAIC6tAnNWhqn/xbHRiWAvU0FhUr+RKPzHKDAfmzb" +
        "7mGv"

    private val certFingerprint =
        "395870a9ec3a9128a46d35f8959451aeb9acdaa6302f6c24c69397d93076071c"

    private fun parseCert(): X509Certificate {
        val der = Base64.getDecoder().decode(certDerB64)
        val cf = CertificateFactory.getInstance("X.509")
        return cf.generateCertificate(
            ByteArrayInputStream(der)
        ) as X509Certificate
    }

    @Test
    fun matchingFingerprint_noException() {
        val tm = FingerprintTrustManager(certFingerprint)
        val cert = parseCert()
        // Should not throw.
        tm.checkServerTrusted(arrayOf(cert), "ECDHE_ECDSA")
    }

    @Test
    fun mismatchedFingerprint_throwsCertificateException() {
        val wrongFp = "a".repeat(64)
        val tm = FingerprintTrustManager(wrongFp)
        val cert = parseCert()
        try {
            tm.checkServerTrusted(arrayOf(cert), "ECDHE_ECDSA")
            fail("Expected CertificateException")
        } catch (e: CertificateException) {
            assert(e.message!!.contains("mismatch"))
            assert(e.message!!.contains(certFingerprint))
        }
    }

    @Test
    fun emptyChain_throwsCertificateException() {
        val tm = FingerprintTrustManager(certFingerprint)
        try {
            tm.checkServerTrusted(emptyArray(), "ECDHE_ECDSA")
            fail("Expected CertificateException")
        } catch (e: CertificateException) {
            assert(e.message!!.contains("Empty"))
        }
    }

    @Test
    fun checkClientTrusted_alwaysThrows() {
        val tm = FingerprintTrustManager(certFingerprint)
        try {
            tm.checkClientTrusted(emptyArray(), "RSA")
            fail("Expected CertificateException")
        } catch (_: CertificateException) {
            // expected
        }
    }

    @Test
    fun getAcceptedIssuers_returnsEmpty() {
        val tm = FingerprintTrustManager(certFingerprint)
        assertEquals(0, tm.acceptedIssuers.size)
    }

    @Test
    fun fingerprintComputation_matchesDaemonAlgorithm() {
        // Independently compute SHA-256 of the cert's DER bytes
        // and verify it matches the Go daemon's fingerprint.
        val der = Base64.getDecoder().decode(certDerB64)
        val cert = CertificateFactory.getInstance("X.509")
            .generateCertificate(ByteArrayInputStream(der))
            as X509Certificate

        val digest = MessageDigest.getInstance("SHA-256")
        val hash = digest.digest(cert.encoded)
        val computed = hash.joinToString("") { "%02x".format(it) }

        assertEquals(certFingerprint, computed)
    }
}
