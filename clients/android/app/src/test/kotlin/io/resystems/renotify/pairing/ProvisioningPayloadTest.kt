// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.pairing

import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test

class ProvisioningPayloadTest {

    // A valid 52-char Crockford Base32 body (uppercase).
    private val validBody = "0A1B2C3D4E5F6G7H8J9K0M1N2P3Q4R5S6T7V8W9X0Y1Z2A3B4C5D"
    private val validToken = "rn_tk_$validBody"
    private val validFingerprint =
        "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

    private fun validJson(
        v: String = "1",
        h: String = "\"192.168.1.42\"",
        p: String = "4223",
        t: String = "\"$validToken\"",
        c: String = "\"$validFingerprint\"",
        u: String = "\"testuser\""
    ): String = """{"v":$v,"h":$h,"p":$p,"t":$t,"c":$c,"u":$u}"""

    // --- Valid parsing ---

    @Test
    fun validPayload_parsesAllFields() {
        val pp = ProvisioningPayload.fromJson(validJson())
        assertEquals(1, pp.version)
        assertEquals("192.168.1.42", pp.host)
        assertEquals(4223, pp.port)
        assertEquals(validToken, pp.token)
        assertEquals(validFingerprint, pp.certFingerprint)
        assertEquals("testuser", pp.username)
    }

    @Test
    fun validPayload_extraFieldsIgnored() {
        val json = """{"v":1,"h":"10.0.0.1","p":4223,"t":"$validToken","c":"$validFingerprint","u":"testuser","extra":"ignored"}"""
        val pp = ProvisioningPayload.fromJson(json)
        assertEquals("10.0.0.1", pp.host)
    }

    @Test
    fun validPayload_hostIsIPv6() {
        val pp = ProvisioningPayload.fromJson(validJson(h = "\"::1\""))
        assertEquals("::1", pp.host)
    }

    @Test
    fun validPayload_portBoundary_1() {
        val pp = ProvisioningPayload.fromJson(validJson(p = "1"))
        assertEquals(1, pp.port)
    }

    @Test
    fun validPayload_portBoundary_65535() {
        val pp = ProvisioningPayload.fromJson(validJson(p = "65535"))
        assertEquals(65535, pp.port)
    }

    @Test
    fun validPayload_tokenUppercase() {
        // validBody is already all uppercase; just confirm it parses.
        ProvisioningPayload.fromJson(validJson())
    }

    @Test
    fun validPayload_tokenMixedCase() {
        val mixed = "0a1b2c3d4e5f6g7h8j9k0m1n2p3q4r5s6t7v8w9x0y1z2a3b4c5d"
        val json = validJson(t = "\"rn_tk_$mixed\"")
        val pp = ProvisioningPayload.fromJson(json)
        assertEquals("rn_tk_$mixed", pp.token)
    }

    // --- Version validation ---

    @Test
    fun invalidVersion_zero_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(v = "0")) }
    }

    @Test
    fun version_two_accepted() {
        val pp = ProvisioningPayload.fromJson(validJson(v = "2"))
        assertEquals(2, pp.version)
    }

    @Test
    fun invalidVersion_three_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(v = "3")) }
    }

    @Test
    fun invalidVersion_negative_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(v = "-1")) }
    }

    @Test
    fun invalidVersion_missing_throws() {
        val json = """{"h":"10.0.0.1","p":4223,"t":"$validToken","c":"$validFingerprint","u":"testuser"}"""
        expectIAE { ProvisioningPayload.fromJson(json) }
    }

    @Test
    fun invalidVersion_stringType_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(v = "\"1\"")) }
    }

    // --- Host validation ---

    @Test
    fun invalidHost_empty_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(h = "\"\"")) }
    }

    @Test
    fun invalidHost_missing_throws() {
        val json = """{"v":1,"p":4223,"t":"$validToken","c":"$validFingerprint","u":"testuser"}"""
        expectIAE { ProvisioningPayload.fromJson(json) }
    }

    // --- Port validation ---

    @Test
    fun invalidPort_zero_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(p = "0")) }
    }

    @Test
    fun invalidPort_negative_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(p = "-1")) }
    }

    @Test
    fun invalidPort_tooHigh_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(p = "65536")) }
    }

    @Test
    fun invalidPort_missing_throws() {
        val json = """{"v":1,"h":"10.0.0.1","t":"$validToken","c":"$validFingerprint","u":"testuser"}"""
        expectIAE { ProvisioningPayload.fromJson(json) }
    }

    @Test
    fun invalidPort_stringType_throws() {
        expectIAE { ProvisioningPayload.fromJson(validJson(p = "\"4223\"")) }
    }

    // --- Token validation ---

    @Test
    fun invalidToken_wrongPrefix_throws() {
        val bad = "xx_tk_$validBody"
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$bad\"")) }
    }

    @Test
    fun invalidToken_tooShort_throws() {
        val short = "rn_tk_" + validBody.substring(0, 51) // 57 total
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$short\"")) }
    }

    @Test
    fun invalidToken_tooLong_throws() {
        val long = "rn_tk_" + validBody + "A" // 59 total
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$long\"")) }
    }

    @Test
    fun invalidToken_missing_throws() {
        val json = """{"v":1,"h":"10.0.0.1","p":4223,"c":"$validFingerprint","u":"testuser"}"""
        expectIAE { ProvisioningPayload.fromJson(json) }
    }

    @Test
    fun invalidToken_containsI_throws() {
        // Replace first char of body with I (excluded from Crockford).
        val bad = "rn_tk_I" + validBody.substring(1)
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$bad\"")) }
    }

    @Test
    fun invalidToken_containsL_throws() {
        val bad = "rn_tk_L" + validBody.substring(1)
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$bad\"")) }
    }

    @Test
    fun invalidToken_containsO_throws() {
        val bad = "rn_tk_O" + validBody.substring(1)
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$bad\"")) }
    }

    @Test
    fun invalidToken_containsU_throws() {
        val bad = "rn_tk_U" + validBody.substring(1)
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$bad\"")) }
    }

    @Test
    fun invalidToken_containsSpecial_throws() {
        val bad = "rn_tk_@" + validBody.substring(1)
        expectIAE { ProvisioningPayload.fromJson(validJson(t = "\"$bad\"")) }
    }

    // --- Fingerprint validation ---

    @Test
    fun invalidFingerprint_tooShort_throws() {
        val short = validFingerprint.substring(0, 63)
        expectIAE { ProvisioningPayload.fromJson(validJson(c = "\"$short\"")) }
    }

    @Test
    fun invalidFingerprint_tooLong_throws() {
        val long = validFingerprint + "a"
        expectIAE { ProvisioningPayload.fromJson(validJson(c = "\"$long\"")) }
    }

    @Test
    fun invalidFingerprint_uppercase_throws() {
        val upper = validFingerprint.uppercase()
        expectIAE { ProvisioningPayload.fromJson(validJson(c = "\"$upper\"")) }
    }

    @Test
    fun invalidFingerprint_nonHex_throws() {
        val bad = "g" + validFingerprint.substring(1)
        expectIAE { ProvisioningPayload.fromJson(validJson(c = "\"$bad\"")) }
    }

    @Test
    fun invalidFingerprint_missing_throws() {
        val json = """{"v":1,"h":"10.0.0.1","p":4223,"t":"$validToken","u":"testuser"}"""
        expectIAE { ProvisioningPayload.fromJson(json) }
    }

    // --- Malformed JSON ---

    @Test
    fun malformedJson_notJson_throws() {
        expectIAE { ProvisioningPayload.fromJson("not json at all") }
    }

    @Test
    fun malformedJson_emptyString_throws() {
        expectIAE { ProvisioningPayload.fromJson("") }
    }

    @Test
    fun malformedJson_emptyObject_throws() {
        expectIAE { ProvisioningPayload.fromJson("{}") }
    }

    @Test
    fun malformedJson_array_throws() {
        expectIAE { ProvisioningPayload.fromJson("[]") }
    }

    // --- Username validation ---

    @Test
    fun validUsername_withUnderscore() {
        val pp = ProvisioningPayload.fromJson(
            validJson(u = "\"test_user\"")
        )
        assertEquals("test_user", pp.username)
    }

    @Test
    fun invalidUsername_empty_throws() {
        expectIAE {
            ProvisioningPayload.fromJson(validJson(u = "\"\""))
        }
    }

    @Test
    fun invalidUsername_missing_throws() {
        val json = """{"v":1,"h":"10.0.0.1","p":4223,"t":"$validToken","c":"$validFingerprint"}"""
        expectIAE { ProvisioningPayload.fromJson(json) }
    }

    // --- Helper ---

    private fun expectIAE(block: () -> Unit) {
        try {
            block()
            fail("Expected IllegalArgumentException")
        } catch (_: IllegalArgumentException) {
            // expected
        }
    }
}
