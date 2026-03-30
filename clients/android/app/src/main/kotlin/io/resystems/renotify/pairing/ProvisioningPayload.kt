package io.resystems.renotify.pairing

import org.json.JSONException
import org.json.JSONObject

/**
 * The provisioning payload decoded from the pairing QR code.
 * Field names use single characters in JSON to minimise QR density
 * (R-API-08). See docs/analysis-payload-schemas.md.
 */
data class ProvisioningPayload(
    val version: Int,
    val host: String,
    val port: Int,
    val token: String,
    val certFingerprint: String,
    val username: String
) {
    companion object {
        // Crockford Base32 body: excludes I, L, O, U in both cases.
        // Ranges: A-H, J-K, M-N, P-T, V-Z (and lowercase).
        private val CROCKFORD_BODY =
            Regex("^[0-9A-HJKMNP-TV-Za-hjkmnp-tv-z]{52}$")
        private val HEX_64 = Regex("^[0-9a-f]{64}$")

        /**
         * Parse and validate a provisioning payload from minified
         * JSON. Throws [IllegalArgumentException] on any
         * validation failure.
         */
        fun fromJson(json: String): ProvisioningPayload {
            val obj = try {
                JSONObject(json)
            } catch (e: JSONException) {
                throw IllegalArgumentException(
                    "invalid JSON: ${e.message}", e
                )
            }

            val version = requireInt(obj, "v")
            if (version != 1) {
                throw IllegalArgumentException(
                    "unsupported version: $version (expected 1)"
                )
            }

            val host = requireString(obj, "h")
            if (host.isEmpty()) {
                throw IllegalArgumentException("host is empty")
            }

            val port = requireInt(obj, "p")
            if (port < 1 || port > 65535) {
                throw IllegalArgumentException(
                    "port out of range: $port"
                )
            }

            val token = requireString(obj, "t")
            validateToken(token)

            val cert = requireString(obj, "c")
            if (!HEX_64.matches(cert)) {
                throw IllegalArgumentException(
                    "cert fingerprint must be 64 lowercase hex " +
                        "chars, got ${cert.length} chars"
                )
            }

            val username = requireString(obj, "u")
            if (username.isEmpty()) {
                throw IllegalArgumentException("username is empty")
            }

            return ProvisioningPayload(
                version = version,
                host = host,
                port = port,
                token = token,
                certFingerprint = cert,
                username = username
            )
        }

        private fun validateToken(token: String) {
            if (token.length != 58) {
                throw IllegalArgumentException(
                    "token must be 58 chars, got ${token.length}"
                )
            }
            if (!token.startsWith("rn_tk_")) {
                throw IllegalArgumentException(
                    "token must start with rn_tk_"
                )
            }
            val body = token.substring(6)
            if (!CROCKFORD_BODY.matches(body)) {
                throw IllegalArgumentException(
                    "token body contains invalid characters"
                )
            }
        }

        private fun requireInt(obj: JSONObject, key: String): Int {
            if (!obj.has(key)) {
                throw IllegalArgumentException(
                    "missing field: $key"
                )
            }
            val raw = obj.get(key)
            if (raw !is Int && raw !is Long) {
                throw IllegalArgumentException(
                    "field $key must be an integer, got ${raw::class.simpleName}"
                )
            }
            return (raw as Number).toInt()
        }

        private fun requireString(
            obj: JSONObject,
            key: String
        ): String {
            if (!obj.has(key)) {
                throw IllegalArgumentException(
                    "missing field: $key"
                )
            }
            val raw = obj.get(key)
            if (raw !is String) {
                throw IllegalArgumentException(
                    "field $key must be a string, got ${raw::class.simpleName}"
                )
            }
            return raw
        }
    }
}
