package io.resystems.renotify.nats

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Binder
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import io.resystems.renotify.MainActivity
import io.resystems.renotify.dashboard.ActiveFlowsResult
import io.resystems.renotify.dashboard.DaemonHeartbeat
import io.resystems.renotify.dashboard.HistoryQueryResult
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
        manager = NatsConnectionManager(
            serviceScope, ::handleMessage, ::handleHeartbeat,
            ::handleInitialDashboard, ::handleDeviceControl)
        store = EncryptedProvisioningStore(this)

        // Load silent mode from preferences.
        val prefs = getSharedPreferences(PREFS_NAME, MODE_PRIVATE)
        _silentMode.value = prefs.getBoolean(KEY_SILENT, false)

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
        if (intent?.action == ACTION_PUBLISH_INTERJECTION) {
            handlePublishInterjection(intent)
            return START_STICKY
        }
        if (intent?.action == ACTION_QUERY_HISTORY) {
            handleQueryHistory(intent)
            return START_STICKY
        }
        if (intent?.action == ACTION_QUERY_FLOW_HISTORY) {
            handleQueryFlowHistory(intent)
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

        if (!_silentMode.value) {
            NotificationRenderer.render(this, payload)
        } else {
            Log.d(TAG, "Silent: suppressed ${payload.id}")
        }
        manager.markRendered(payload.id)
        ack()

        // Signal that a notification arrived for this flow so
        // the dashboard can refresh if the flow is expanded.
        _lastNotificationFlowId.value =
            Pair(payload.flowId, ++notificationSeq)
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

    // --- Heartbeat handling (M-09) ---

    /**
     * Callback invoked by [NatsConnectionManager] for each
     * Core NATS heartbeat message. Runs on jnats' dispatcher
     * thread.
     */
    private fun handleHeartbeat(data: ByteArray) {
        try {
            val json = String(data, Charsets.UTF_8)
            val heartbeat = DaemonHeartbeat.fromJson(json)
            _dashboardState.value = heartbeat
            Log.d(TAG, "Heartbeat: ${heartbeat.hostname} " +
                "${heartbeat.workspaces.size} workspace(s)")
        } catch (e: Exception) {
            Log.w(TAG, "Error parsing heartbeat", e)
        }
    }

    /**
     * Callback for the initial svc.flows query on connect.
     * Converts the flat flow list to a heartbeat for the
     * dashboard.
     */
    private fun handleInitialDashboard(data: ByteArray) {
        try {
            val json = String(data, Charsets.UTF_8)
            val result = ActiveFlowsResult.fromJson(json)
            _dashboardState.value = result.toDaemonHeartbeat()
            Log.i(TAG, "Initial dashboard: " +
                "${result.flows.size} flow(s)")
        } catch (e: Exception) {
            Log.w(TAG, "Error parsing initial dashboard", e)
        }
    }

    // --- History query (M-07) ---

    /**
     * Handle a history query intent from MainActivity. Sends a
     * svc.history request to the daemon and updates the global
     * [historyState] StateFlow.
     */
    private fun handleQueryHistory(intent: Intent) {
        val limitVal = intent.getIntExtra(EXTRA_HISTORY_LIMIT, 25)
        val offsetVal = intent.getIntExtra(EXTRA_HISTORY_OFFSET, 0)
        val append = intent.getBooleanExtra(
            EXTRA_HISTORY_APPEND, false)

        val reqObj = JSONObject()
        reqObj.put("limit", limitVal)
        reqObj.put("offset", offsetVal)

        serviceScope.launch(Dispatchers.IO) {
            val data = manager.queryHistory(
                reqObj.toString().toByteArray())
            if (data != null) {
                try {
                    val json = String(data, Charsets.UTF_8)
                    val result = HistoryQueryResult.fromJson(json)
                    if (append) {
                        // Merge with existing.
                        val existing = _historyState.value
                        if (existing != null) {
                            _historyState.value =
                                HistoryQueryResult(
                                    records = existing.records +
                                        result.records,
                                    total = result.total
                                )
                        } else {
                            _historyState.value = result
                        }
                    } else {
                        _historyState.value = result
                    }
                    Log.i(TAG, "History: ${result.records.size} " +
                        "records (total ${result.total})")
                } catch (e: Exception) {
                    Log.w(TAG, "Error parsing history", e)
                }
            } else {
                Log.w(TAG, "History query returned null")
            }
        }
    }

    // --- Flow-scoped history query ---

    /**
     * Handle a flow-scoped history query from the dashboard.
     * Queries svc.history filtered by flow_id and updates the
     * [flowHistoryState] StateFlow (separate from the global
     * history tab state).
     */
    private fun handleQueryFlowHistory(intent: Intent) {
        val flowId = intent.getStringExtra(
            EXTRA_FLOW_HISTORY_FLOW_ID) ?: return
        val limitVal = intent.getIntExtra(
            EXTRA_FLOW_HISTORY_LIMIT, 10)

        val reqObj = JSONObject()
        reqObj.put("flow_id", flowId)
        reqObj.put("limit", limitVal)
        reqObj.put("offset", 0)

        serviceScope.launch(Dispatchers.IO) {
            val data = manager.queryHistory(
                reqObj.toString().toByteArray())
            if (data != null) {
                try {
                    val json = String(data, Charsets.UTF_8)
                    val result = HistoryQueryResult.fromJson(json)
                    _flowHistoryState.value = Pair(flowId, result)
                    Log.i(TAG, "Flow history ($flowId): " +
                        "${result.records.size} records " +
                        "(total ${result.total})")
                } catch (e: Exception) {
                    Log.w(TAG, "Error parsing flow history", e)
                }
            }
        }
    }

    // --- Device control (C-16) ---

    /**
     * Callback invoked by [NatsConnectionManager] for device
     * control messages. Runs on jnats' dispatcher thread.
     */
    private fun handleDeviceControl(data: ByteArray) {
        try {
            val json = String(data, Charsets.UTF_8)
            val obj = JSONObject(json)
            val command = obj.optString("command", "")

            when (command) {
                "set_silent" -> {
                    val value = obj.getBoolean("value")
                    setSilentMode(this, value)
                    Log.i(TAG, "Remote silent mode: $value")
                }
                else -> Log.w(TAG,
                    "Unknown device control: $command")
            }
        } catch (e: Exception) {
            Log.w(TAG, "Error parsing device control", e)
        }
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

    // --- Interjection publishing (M-08) ---

    /**
     * Publish an InterjectionCommand to a running flow. Called
     * from the dashboard when the user taps Stop or Note.
     */
    private fun handlePublishInterjection(intent: Intent) {
        val flowId = intent.getStringExtra(
            EXTRA_INTERJECT_FLOW_ID) ?: return
        val action = intent.getStringExtra(
            EXTRA_INTERJECT_ACTION) ?: return
        val context = intent.getStringExtra(
            EXTRA_INTERJECT_CONTEXT) ?: ""

        val payload = store.load()
        if (payload == null) {
            Log.w(TAG, "Cannot publish interjection: not paired")
            return
        }

        val nc = manager.connection
        if (nc == null || nc.status !=
            io.nats.client.Connection.Status.CONNECTED
        ) {
            Log.w(TAG,
                "Cannot publish interjection: not connected")
            return
        }

        val now = java.time.Instant.now()
        val json = buildInterjectionJson(
            flowId, action, context, now.toString())
        val subject = "resystems.renotify.${payload.username}" +
            ".flow.$flowId.interject"
        val dedupId = "$flowId-interject-$action-${now.toEpochMilli()}"

        serviceScope.launch(kotlinx.coroutines.Dispatchers.IO) {
            try {
                val js = nc.jetStream()
                val headers = io.nats.client.impl.Headers()
                headers.add("Nats-Msg-Id", dedupId)
                val msg = io.nats.client.impl.NatsMessage
                    .builder()
                    .subject(subject)
                    .headers(headers)
                    .data(json.toByteArray())
                    .build()
                js.publish(msg)
                Log.i(TAG,
                    "Interjection published: $action for $flowId")
            } catch (e: Exception) {
                Log.e(TAG, "Failed to publish interjection", e)
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

        val launchIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or
                Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val contentIntent = PendingIntent.getActivity(
            this, 0, launchIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or
                PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Renotify")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentIntent(contentIntent)
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

        // Intent action and extras for M-07 history query.
        const val ACTION_QUERY_HISTORY =
            "io.resystems.renotify.QUERY_HISTORY"
        const val EXTRA_HISTORY_LIMIT = "history_limit"
        const val EXTRA_HISTORY_OFFSET = "history_offset"
        const val EXTRA_HISTORY_APPEND = "history_append"

        // Intent action and extras for flow-scoped history.
        const val ACTION_QUERY_FLOW_HISTORY =
            "io.resystems.renotify.QUERY_FLOW_HISTORY"
        const val EXTRA_FLOW_HISTORY_FLOW_ID = "flow_history_flow_id"
        const val EXTRA_FLOW_HISTORY_LIMIT = "flow_history_limit"

        // Intent action and extras for M-08 interjections.
        const val ACTION_PUBLISH_INTERJECTION =
            "io.resystems.renotify.PUBLISH_INTERJECTION"
        const val EXTRA_INTERJECT_FLOW_ID = "interject_flow_id"
        const val EXTRA_INTERJECT_ACTION = "interject_action"
        const val EXTRA_INTERJECT_CONTEXT = "interject_context"

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
         * Build an InterjectionCommand JSON string. Extracted
         * as a companion function for testability.
         */
        fun buildInterjectionJson(
            flowId: String,
            action: String,
            context: String,
            timestamp: String
        ): String {
            val obj = JSONObject()
            obj.put("flow_id", flowId)
            obj.put("action", action)
            if (context.isNotEmpty()) {
                obj.put("context", context)
            }
            obj.put("timestamp", timestamp)
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

        /**
         * Latest daemon heartbeat for the dashboard (M-09).
         * Null before the first heartbeat arrives.
         */
        private val _dashboardState = kotlinx.coroutines.flow
            .MutableStateFlow<DaemonHeartbeat?>(null)
        val dashboardState: StateFlow<DaemonHeartbeat?> =
            _dashboardState

        /**
         * Latest history query result for the history viewer
         * (M-07). Null before the first query.
         */
        private val _historyState = kotlinx.coroutines.flow
            .MutableStateFlow<HistoryQueryResult?>(null)
        val historyState: StateFlow<HistoryQueryResult?> =
            _historyState

        /**
         * Flow-scoped history for dashboard drill-down. Carries
         * the flow ID and its query result; null before any
         * flow-scoped query. Separate from [historyState] to
         * avoid clobbering the History tab.
         */
        private val _flowHistoryState = kotlinx.coroutines.flow
            .MutableStateFlow<Pair<String, HistoryQueryResult>?>(
                null)
        val flowHistoryState:
            StateFlow<Pair<String, HistoryQueryResult>?> =
            _flowHistoryState

        /**
         * Flow ID and sequence counter for the most recently
         * received notification. The counter ensures StateFlow
         * re-emits even when consecutive notifications share
         * the same flow ID.
         */
        private var notificationSeq = 0L
        private val _lastNotificationFlowId =
            kotlinx.coroutines.flow
                .MutableStateFlow<Pair<String, Long>?>(null)
        val lastNotificationFlowId:
            StateFlow<Pair<String, Long>?> =
            _lastNotificationFlowId

        /**
         * Silent mode suppresses notification rendering while
         * still receiving and ACKing messages.
         */
        private val _silentMode = kotlinx.coroutines.flow
            .MutableStateFlow(false)
        val silentMode: StateFlow<Boolean> = _silentMode

        private const val PREFS_NAME = "renotify_settings"
        private const val KEY_SILENT = "silent_mode"

        /**
         * Toggle silent mode and persist to SharedPreferences.
         */
        fun setSilentMode(
            context: android.content.Context,
            silent: Boolean
        ) {
            _silentMode.value = silent
            context.getSharedPreferences(PREFS_NAME,
                MODE_PRIVATE)
                .edit().putBoolean(KEY_SILENT, silent).apply()
        }
    }
}
