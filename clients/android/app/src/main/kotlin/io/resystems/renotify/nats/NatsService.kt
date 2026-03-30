package io.resystems.renotify.nats

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Binder
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import io.resystems.renotify.pairing.EncryptedProvisioningStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/**
 * Android Foreground Service that maintains the NATS WebSocket
 * connection. The persistent notification satisfies R-MOB-10
 * (visible connectivity status indicator at all times).
 *
 * Start with:
 * ```
 * startForegroundService(Intent(context, NatsService::class.java))
 * ```
 *
 * Observe connection state via [connectionState] after binding,
 * or read [NatsService.state] from the companion object.
 */
class NatsService : Service() {

    private val serviceScope = CoroutineScope(
        SupervisorJob() + Dispatchers.Main
    )
    private lateinit var manager: NatsConnectionManager
    private lateinit var store: EncryptedProvisioningStore

    /** Binder for activities that bind to this service. */
    inner class LocalBinder : Binder() {
        val service: NatsService get() = this@NatsService
    }

    private val binder = LocalBinder()

    /** Observable connection state. */
    val connectionState: StateFlow<ConnectionState>
        get() = manager.connectionState

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        manager = NatsConnectionManager(serviceScope)
        store = EncryptedProvisioningStore(this)

        // Update the persistent notification when state changes.
        serviceScope.launch {
            manager.connectionState.collect { state ->
                updateNotification(state)
                // Also update companion for non-bound observers.
                _state.value = state
            }
        }
    }

    override fun onStartCommand(
        intent: Intent?,
        flags: Int,
        startId: Int
    ): Int {
        val payload = store.load()
        if (payload == null) {
            Log.w(TAG, "No provisioning data, stopping")
            _state.value = ConnectionState.Unpaired
            stopSelf()
            return START_NOT_STICKY
        }

        // Start as foreground immediately to avoid ANR.
        startForeground(
            NOTIFICATION_ID,
            buildNotification(ConnectionState.Connecting),
            ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC
        )

        manager.connect(payload)
        return START_STICKY
    }

    override fun onDestroy() {
        manager.disconnect()
        serviceScope.cancel()
        _state.value = ConnectionState.Idle
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder = binder

    // --- Notification management ---

    private fun createNotificationChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID,
            "Connection Status",
            NotificationManager.IMPORTANCE_LOW
        ).apply {
            description = "Shows the NATS connection status"
            setShowBadge(false)
        }
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(channel)
    }

    private fun buildNotification(
        state: ConnectionState
    ): Notification {
        val text = when (state) {
            is ConnectionState.Idle -> "Idle"
            is ConnectionState.Unpaired -> "Not paired"
            is ConnectionState.Connecting -> "Connecting..."
            is ConnectionState.Connected -> {
                val p = store.load()
                if (p != null) "Connected to ${p.host}:${p.port}"
                else "Connected"
            }
            is ConnectionState.Disconnected ->
                "Disconnected \u2014 reconnecting..."
            is ConnectionState.Error ->
                "Error: ${state.message}"
        }

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Renotify")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setOngoing(true)
            .setSilent(true)
            .build()
    }

    private fun updateNotification(state: ConnectionState) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIFICATION_ID, buildNotification(state))
    }

    companion object {
        private const val TAG = "NatsService"
        private const val CHANNEL_ID = "renotify_connection"
        private const val NOTIFICATION_ID = 1

        /**
         * Global state for non-bound observers (e.g.,
         * [io.resystems.renotify.MainActivity]). Updated by
         * the service's state collector.
         */
        private val _state = kotlinx.coroutines.flow
            .MutableStateFlow<ConnectionState>(
                ConnectionState.Idle
            )
        val state: StateFlow<ConnectionState> = _state
    }
}
