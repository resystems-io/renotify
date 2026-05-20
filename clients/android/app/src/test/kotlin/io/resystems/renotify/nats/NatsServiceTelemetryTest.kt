// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.nats

import android.content.Context
import io.nats.client.Connection
import io.nats.client.JetStream
import io.nats.client.impl.NatsMessage
import io.resystems.renotify.pairing.ProvisioningPayload
import io.resystems.renotify.telemetry.CrashReporter
import io.resystems.renotify.telemetry.TelemetryUploader
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.mockito.Mockito.mock
import org.mockito.Mockito.never
import org.mockito.Mockito.verify
import org.mockito.kotlin.any
import org.mockito.kotlin.argumentCaptor
import org.mockito.kotlin.doThrow
import org.mockito.kotlin.whenever
import java.io.File

@OptIn(ExperimentalCoroutinesApi::class)
class NatsServiceTelemetryTest {

    @get:Rule
    val tempFolder = TemporaryFolder()

    private val payload = ProvisioningPayload(
        version = 2,
        host = "localhost",
        port = 4222,
        token = "rn_tk_1234567890123456789012345678901234567890123456789012",
        certFingerprint = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
        username = "stewart",
        deviceId = "mb_test",
        natsUsername = "mobile"
    )

    @Test
    fun testTransmitDeferredCrashes_EmptyDirectory() = runTest {
        val context = mock<Context>()
        val cacheDir = tempFolder.newFolder("cache")
        whenever(context.cacheDir).thenReturn(cacheDir)

        val nc = mock<Connection>()
        val js = mock<JetStream>()
        whenever(nc.jetStream()).thenReturn(js)

        // Invoke uploader
        TelemetryUploader.transmitDeferredCrashes(
            context,
            this,
            nc,
            payload,
            kotlinx.coroutines.Dispatchers.Unconfined
        )

        // Verify jetStream was never requested because the directory was empty
        verify(nc, never()).jetStream()
    }

    @Test
    fun testTransmitDeferredCrashes_SuccessfulPublish() = runTest {
        val context = mock<Context>()
        val cacheDir = tempFolder.newFolder("cache")
        whenever(context.cacheDir).thenReturn(cacheDir)

        // Create a mock crash report file
        val crashDir = CrashReporter.getCrashDir(context)
        val crashFile = File(crashDir, "ntf_test123.json")
        val crashJson = """
            {
                "report_id": "ntf_test123",
                "device_id": "mb_test",
                "timestamp": "2026-05-17T12:00:00Z",
                "incident_type": "managed_crash"
            }
        """.trimIndent()
        crashFile.writeText(crashJson)

        assertTrue(crashFile.exists())

        // Mock NATS
        val nc = mock<Connection>()
        val js = mock<JetStream>()
        whenever(nc.jetStream()).thenReturn(js)

        // Invoke uploader
        TelemetryUploader.transmitDeferredCrashes(
            context,
            this,
            nc,
            payload,
            kotlinx.coroutines.Dispatchers.Unconfined
        )

        // Wait until coroutine executes
        testScheduler.advanceUntilIdle()

        // Verify JS publish was invoked with correct NatsMessage
        val msgCaptor = argumentCaptor<NatsMessage>()
        verify(js).publish(msgCaptor.capture())

        val publishedMsg = msgCaptor.firstValue
        assertEquals("resystems.renotify.stewart.device.mb_test.telemetry.crash", publishedMsg.subject)
        assertEquals("ntf_test123", publishedMsg.headers.getFirst("Nats-Msg-Id"))
        assertEquals(crashJson, String(publishedMsg.data, Charsets.UTF_8))

        // Verify local crash report file was successfully deleted
        assertFalse(crashFile.exists())
    }

    @Test
    fun testTransmitDeferredCrashes_FailedPublish_RetainsFile() = runTest {
        val context = mock<Context>()
        val cacheDir = tempFolder.newFolder("cache")
        whenever(context.cacheDir).thenReturn(cacheDir)

        // Create a mock crash report file
        val crashDir = CrashReporter.getCrashDir(context)
        val crashFile = File(crashDir, "ntf_test999.json")
        val crashJson = """
            {
                "report_id": "ntf_test999",
                "device_id": "mb_test",
                "timestamp": "2026-05-17T12:00:00Z",
                "incident_type": "managed_crash"
            }
        """.trimIndent()
        crashFile.writeText(crashJson)

        assertTrue(crashFile.exists())

        // Mock NATS connection to throw error on publish
        val nc = mock<Connection>()
        val js = mock<JetStream>()
        whenever(nc.jetStream()).thenReturn(js)
        whenever(js.publish(any())).doThrow(RuntimeException("NATS broker offline"))

        // Invoke uploader
        TelemetryUploader.transmitDeferredCrashes(
            context,
            this,
            nc,
            payload,
            kotlinx.coroutines.Dispatchers.Unconfined
        )

        testScheduler.advanceUntilIdle()

        // Verify JS publish was invoked
        verify(js).publish(any())

        // Verify local crash report file was RETAINED (not deleted)
        assertTrue(crashFile.exists())
    }
}
