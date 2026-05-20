// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.nats

import android.app.Notification
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import io.resystems.renotify.pairing.EncryptedProvisioningStore
import io.resystems.renotify.pairing.ProvisioningPayload
import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test
import org.mockito.Mockito.doReturn
import org.mockito.Mockito.doThrow
import org.mockito.Mockito.mock
import org.mockito.Mockito.spy
import org.mockito.Mockito.verify
import org.mockito.kotlin.any
import org.mockito.kotlin.doNothing
import org.mockito.kotlin.whenever

class NatsServiceFgsTest {

    @Test
    fun testOnStartCommand_WhenBackgroundStartForbiddenOnApi31_GracefullyStops() {
        val service = spy(NatsService())

        // Use reflection to set private lateinit properties
        val mockStore = mock(EncryptedProvisioningStore::class.java)
        val mockManager = mock(NatsConnectionManager::class.java)

        val storeField = NatsService::class.java.getDeclaredField("store")
        storeField.isAccessible = true
        storeField.set(service, mockStore)

        val managerField = NatsService::class.java.getDeclaredField("manager")
        managerField.isAccessible = true
        managerField.set(service, mockManager)

        // Mock store.load() to return a mock payload
        val payload = mock(ProvisioningPayload::class.java)
        whenever(mockStore.load()).thenReturn(payload)

        // Mock buildNotification to return a mock notification
        val notification = mock(Notification::class.java)
        doReturn(notification).whenever(service).buildNotification(any())

        // Stub stopSelf()
        doNothing().whenever(service).stopSelf()

        // Set sdkVersionProvider to API 31 (Android S) to test background start restriction path
        service.sdkVersionProvider = { Build.VERSION_CODES.S }

        // Stub startForeground to throw ForegroundServiceStartNotAllowedException
        val exception = mock(android.app.ForegroundServiceStartNotAllowedException::class.java)
        doThrow(exception).whenever(service).startForeground(any(), any())

        try {
            val result = service.onStartCommand(null, 0, 1)
            assertEquals(Service.START_NOT_STICKY, result)
            verify(service).stopSelf()
        } catch (e: Exception) {
            fail("Should have handled ForegroundServiceStartNotAllowedException gracefully: " + e.message)
        }
    }

    @Test
    fun testOnStartCommand_WhenBackgroundStartForbiddenOnApi34_GracefullyStops() {
        val service = spy(NatsService())

        // Use reflection to set private lateinit properties
        val mockStore = mock(EncryptedProvisioningStore::class.java)
        val mockManager = mock(NatsConnectionManager::class.java)

        val storeField = NatsService::class.java.getDeclaredField("store")
        storeField.isAccessible = true
        storeField.set(service, mockStore)

        val managerField = NatsService::class.java.getDeclaredField("manager")
        managerField.isAccessible = true
        managerField.set(service, mockManager)

        // Mock store.load() to return a mock payload
        val payload = mock(ProvisioningPayload::class.java)
        whenever(mockStore.load()).thenReturn(payload)

        // Mock buildNotification to return a mock notification
        val notification = mock(Notification::class.java)
        doReturn(notification).whenever(service).buildNotification(any())

        // Stub stopSelf()
        doNothing().whenever(service).stopSelf()

        // Set sdkVersionProvider to API 34 (Android 14+) to test background start restriction path with type
        service.sdkVersionProvider = { Build.VERSION_CODES.UPSIDE_DOWN_CAKE }

        // Stub startForeground to throw ForegroundServiceStartNotAllowedException
        val exception = mock(android.app.ForegroundServiceStartNotAllowedException::class.java)
        doThrow(exception).whenever(service).startForeground(any(), any(), any())

        try {
            val result = service.onStartCommand(null, 0, 1)
            assertEquals(Service.START_NOT_STICKY, result)
            verify(service).stopSelf()
        } catch (e: Exception) {
            fail("Should have handled ForegroundServiceStartNotAllowedException gracefully: " + e.message)
        }
    }

    @Test
    fun testOnTimeout_GracefullyStopsService() {
        val service = spy(NatsService())
        doNothing().whenever(service).stopSelf(any())

        // Call onTimeout directly
        service.onTimeout(456, 1)

        // Verify stopSelf(456) was invoked
        verify(service).stopSelf(456)
    }
}
