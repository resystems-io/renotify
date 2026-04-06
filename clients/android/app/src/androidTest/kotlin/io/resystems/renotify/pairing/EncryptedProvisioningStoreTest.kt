// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.pairing

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class EncryptedProvisioningStoreTest {

    private lateinit var store: EncryptedProvisioningStore

    private val testPayload = ProvisioningPayload(
        version = 2,
        host = "192.168.1.42",
        port = 4223,
        token = "rn_tk_0A1B2C3D4E5F6G7H8J9K0M1N2P3Q4R5S6T7V8W9X0Y1Z2A3B4C5D",
        certFingerprint = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
        username = "testuser",
        deviceId = "mb_TESTDEV01",
        natsUsername = "mobile-mb_TESTDEV01"
    )

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation()
            .targetContext
        store = EncryptedProvisioningStore(context)
        store.clear()
    }

    @After
    fun tearDown() {
        store.clear()
    }

    @Test
    fun saveAndLoad_roundTrips() {
        store.save(testPayload)
        val loaded = store.load()
        assertNotNull(loaded)
        assertEquals(testPayload, loaded)
    }

    @Test
    fun load_whenEmpty_returnsNull() {
        assertNull(store.load())
    }

    @Test
    fun clear_removesData() {
        store.save(testPayload)
        store.clear()
        assertNull(store.load())
    }

    @Test
    fun isPaired_afterSave_returnsTrue() {
        store.save(testPayload)
        assertTrue(store.isPaired())
    }

    @Test
    fun isPaired_afterClear_returnsFalse() {
        store.save(testPayload)
        store.clear()
        assertFalse(store.isPaired())
    }

    @Test
    fun overwrite_replacesData() {
        store.save(testPayload)

        val updated = testPayload.copy(
            host = "10.0.0.99",
            token = "rn_tk_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
        )
        store.save(updated)

        val loaded = store.load()
        assertEquals(updated, loaded)
    }

    @Test
    fun newInstance_persistsData() {
        store.save(testPayload)

        // Create a fresh store instance.
        val context = InstrumentationRegistry.getInstrumentation()
            .targetContext
        val store2 = EncryptedProvisioningStore(context)

        val loaded = store2.load()
        assertEquals(testPayload, loaded)
    }
}
