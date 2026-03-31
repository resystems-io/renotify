package io.resystems.renotify.nats

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Binder
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import io.resystems.renotify.notification.NotificationPayload
import io.resystems.renotify.notification.NotificationRenderer
import io.resystems.renotify.pairing.EncryptedProvisioningStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import org.json.JSONObject

/**
 * Android Foreground Service that maintains the NATS WebSocket
 * connection. The persistent notification satisfies R-MOB-10
 * (visible connectivity status indicator at all times).
 *
 * Incoming JetStream messages are routed to
 * [NotificationRenderer] for display (M-03). Subject
 * discrimination:
 * - `.request` -> parse and render notification
 * - `.lifecycle` (completed/failed) -> dismiss notification
 * - Other suffixes -> ignored
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
        createNotificationChannels()
        manager = NatsConnectionManager(serviceScope, ::handleMessage)
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
        // Handle response publishing from NotificationActionReceiver
        // (M-04). This is dispatched while the service is already
        // running as foreground.
        if (intent?.action == ACTION_PUBLISH_RESPONSE) {
            handlePublishResponse(intent)
            return START_STICKY
        }

        val payload = store.load()
        if (payload == null) {
            Log.w(TAG, "No provisioning data, stopping")
            _state.value = ConnectionState.Unpaired
            stopSelf()
            return START_NOT_STICKY
        }

        // Start as foreground immediately to avoid ANR.
        // The 3-arg overload with foregroundServiceType requires
        // API 29; use the 2-arg version on older devices.
        val notification = buildNotification(ConnectionState.Connecting)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(
                NOTIFICATION_ID, notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }

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

    // --- Message handling (M-03) ---

    /**
     * Callback invoked by [NatsConnectionManager] for each
     * JetStream message. Runs on jnats' internal thread.
     */
    private fun handleMessage(
        subject: String,
        data: ByteArray,
        ack: () -> Unit
    ) {
        try {
            when {
                subject.endsWith(".request") ->
                    handleRequest(data, ack)
                subject.endsWith(".lifecycle") ->
                    handleLifecycle(data, ack)
                else -> ack() // unrecognised suffix — ACK and skip
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error handling message on $subject", e)
            ack() // ACK to prevent infinite redelivery
        }
    }

    /**
     * Parse a NotificationRequest and render it as an Android
     * notification. Deduplicates on notification ID.
     */
    private fun handleRequest(data: ByteArray, ack: () -> Unit) {
        val json = String(data, Charsets.UTF_8)
        val payload = NotificationPayload.fromJson(json)

        if (manager.isRendered(payload.id)) {
            Log.d(TAG, "Dedup: ${payload.id} already rendered")
            ack()
            return
        }

        NotificationRenderer.render(this, payload)
        manager.markRendered(payload.id)
        ack()
    }

    /**
     * Parse a FlowLifecycleEvent and dismiss the notification
     * when the flow is completed or failed.
     */
    private fun handleLifecycle(data: ByteArray, ack: () -> Unit) {
        val json = String(data, Charsets.UTF_8)
        val obj = JSONObject(json)
        val status = obj.optString("status", "")

        if (status == "completed" || status == "failed") {
            // Dismiss any notification for this flow. The flow_id
            // is in the lifecycle event but we need the
            // notification_id. Since we can't look it up without
            // maintaining a flow→notification mapping, we skip
            // dismissal for now. The notification remains until
            // the user swipes it or responds (M-04).
            //
            // TODO: maintain flow_id → notification_id mapping for
            // automatic dismissal on lifecycle events.
        }

        ack()
    }

    // --- Response publishing (M-04) ---

    /**
     * Handle a publish-response intent from
     * [NotificationActionReceiver]. Builds a NotificationResponse
     * JSON and publishes it to the NATS .response subject.
     */
    private fun handlePublishResponse(intent: Intent) {
        val notificationId = intent.getStringExtra(
            EXTRA_NOTIFICATION_ID) ?: return
        val flowId = intent.getStringExtra(
            EXTRA_FLOW_ID) ?: return
        val actionType = intent.getStringExtra(
            EXTRA_ACTION_TYPE) ?: return
        val actionValue = intent.getStringExtra(
            EXTRA_ACTION_VALUE) ?: ""
        val text = intent.getStringExtra(EXTRA_TEXT)

        val payload = store.load()
        if (payload == null) {
            Log.w(TAG, "Cannot publish response: not paired")
            return
        }

        val nc = manager.connection
        if (nc == null || nc.status != io.nats.client.Connection.Status.CONNECTED) {
            Log.w(TAG, "Cannot publish response: not connected")
            return
        }

        val responseJson = buildResponseJson(
            notificationId, actionType, actionValue, text)
        val subject = "resystems.renotify.${payload.username}" +
            ".flow.$flowId.response"

        serviceScope.launch(kotlinx.coroutines.Dispatchers.IO) {
            try {
                val js = nc.jetStream()
                val headers = io.nats.client.impl.Headers()
                headers.add("Nats-Msg-Id",
                    "$notificationId-response")
                val msg = io.nats.client.impl.NatsMessage
                    .builder()
                    .subject(subject)
                    .headers(headers)
                    .data(responseJson.toByteArray())
                    .build()
                js.publish(msg)

                Log.i(TAG, "Response published for $notificationId")

                // Dismiss the notification.
                NotificationRenderer.dismiss(
                    this@NatsService, notificationId)
            } catch (e: Exception) {
                Log.e(TAG, "Failed to publish response", e)
            }
        }
    }

    // --- Notification management ---

    private fun createNotificationChannels() {
        val nm = getSystemService(NotificationManager::class.java)

        // Existing connection status channel.
        nm.createNotificationChannel(
            NotificationChannel(
                CHANNEL_ID,
                "Connection Status",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Shows the NATS connection status"
                setShowBadge(false)
            }
        )

        // Notification channels for incoming messages (M-03).
        NotificationRenderer.createChannels(this)
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

        // Intent action and extras for M-04 response publishing.
        const val ACTION_PUBLISH_RESPONSE =
            "io.resystems.renotify.PUBLISH_RESPONSE"
        const val EXTRA_NOTIFICATION_ID = "notification_id"
        const val EXTRA_FLOW_ID = "flow_id"
        const val EXTRA_ACTION_TYPE = "action_type"
        const val EXTRA_ACTION_VALUE = "action_value"
        const val EXTRA_TEXT = "text"

        /**
         * Build a NotificationResponse JSON string from the
         * action type and value. Extracted as a companion
         * function for testability.
         */
        fun buildResponseJson(
            requestId: String,
            actionType: String,
            actionValue: String,
            text: String?
        ): String {
            val obj = JSONObject()
            obj.put("request_id", requestId)

            when (actionType) {
                NotificationRenderer.ACTION_TYPE_ACCEPTED ->
                    obj.put("accepted", true)
                NotificationRenderer.ACTION_TYPE_REJECTED ->
                    obj.put("accepted", false)
                NotificationRenderer.ACTION_TYPE_CHOICE ->
                    obj.put("action", actionValue)
                NotificationRenderer.ACTION_TYPE_TEXT ->
                    if (text != null) obj.put("text", text)
            }

            obj.put("timestamp",
                java.time.Instant.now().toString())
            return obj.toString()
        }

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
