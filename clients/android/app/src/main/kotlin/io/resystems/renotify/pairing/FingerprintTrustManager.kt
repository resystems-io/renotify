package io.resystems.renotify.pairing

import android.annotation.SuppressLint
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.X509TrustManager

/**
 * A custom [X509TrustManager] that pins the server's TLS
 * certificate by SHA-256 fingerprint. The fingerprint is
 * provisioned via the QR pairing code.
 *
 * This is more restrictive than the platform default: it trusts
 * exactly one leaf certificate (vs ~150 system CAs). See
 * docs/analysis-nats-transport-design.md Section 5.5.
 */
@SuppressLint("CustomX509TrustManager")
class FingerprintTrustManager(
    private val expectedFingerprint: String
) : X509TrustManager {

    override fun checkClientTrusted(
        chain: Array<out X509Certificate>,
        authType: String
    ) {
        throw CertificateException("Client auth not supported")
    }

    override fun checkServerTrusted(
        chain: Array<out X509Certificate>,
        authType: String
    ) {
        if (chain.isEmpty()) {
            throw CertificateException(
                "Empty certificate chain"
            )
        }

        val cert = chain[0]
        val digest = MessageDigest.getInstance("SHA-256")
        val hash = digest.digest(cert.encoded)
        val actual = hash.joinToString("") { "%02x".format(it) }

        if (actual != expectedFingerprint) {
            throw CertificateException(
                "Fingerprint mismatch: expected " +
                    "$expectedFingerprint, got $actual"
            )
        }
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> =
        emptyArray()
}
