package io.resystems.renotify.nats

import android.util.Log
import io.nats.client.Connection
import io.nats.client.JetStream
import io.nats.client.JetStreamSubscription
import io.nats.client.Nats
import io.nats.client.PushSubscribeOptions
import io.resystems.renotify.pairing.ProvisioningPayload
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlin.math.min

/**
 * Manages the NATS WebSocket connection lifecycle: connect,
 * subscribe to the durable JetStream consumer, and reconnect
 * with exponential backoff on disconnection.
 *
 * This class is not an Android Service — it is owned by
 * [NatsService] and uses a coroutine scope tied to the service
 * lifecycle.
 *
 * See docs/analysis-nats-transport-design.md Sections 8.4 and
 * 8.5.
 */
class NatsConnectionManager(
    private val scope: CoroutineScope,
    private val onMessage: ((subject: String, data: ByteArray, ack: () -> Unit) -> Unit)? = null,
    private val onHeartbeat: ((data: ByteArray) -> Unit)? = null,
    private val onInitialDashboard: ((data: ByteArray) -> Unit)? = null,
    private val onDeviceControl: ((data: ByteArray) -> Unit)? = null
) {

    private val _state = MutableStateFlow<ConnectionState>(
        ConnectionState.Idle
    )

    /** Observable connection state for UI and notification. */
    val connectionState: StateFlow<ConnectionState> =
        _state.asStateFlow()

    /** The current NATS connection, or null if not connected. */
    var connection: Connection? = null
        private set

    private var connectJob: Job? = null
    private var payload: ProvisioningPayload? = null

    /** Tracks rendered notification IDs for deduplication. */
    private val renderedIds = mutableSetOf<String>()

    /**
     * Start a connection attempt. If already connecting or
     * connected, this is a no-op.
     */
    fun connect(payload: ProvisioningPayload) {
        if (connectJob?.isActive == true) return
        this.payload = payload
        connectJob = scope.launch(Dispatchers.IO) {
            connectOnce(payload)
        }
    }

    /**
     * Disconnect and cancel any reconnection attempts. The NATS
     * close runs on [Dispatchers.IO] to avoid
     * NetworkOnMainThreadException when called from the main
     * thread (e.g., Service.onDestroy).
     */
    fun disconnect() {
        connectJob?.cancel()
        connectJob = null
        val nc = connection
        connection = null
        _state.value = ConnectionState.Idle
        if (nc != null) {
            Log.i(TAG, "Disconnecting")
            // Use a standalone coroutine not tied to the service
            // scope, which may be cancelled in parallel (called
            // from onDestroy). GlobalScope is deliberate:
            // connection.close() does blocking network I/O and a
            // lifecycle-bound scope would cancel the close before
            // it completes.
            @OptIn(kotlinx.coroutines.DelicateCoroutinesApi::class)
            kotlinx.coroutines.GlobalScope.launch(Dispatchers.IO) {
                try {
                    nc.close()
                    Log.i(TAG, "Disconnected")
                } catch (e: Exception) {
                    Log.w(TAG, "Error closing connection", e)
                }
            }
        }
    }

    /**
     * Check if a notification ID has already been rendered.
     * Used for deduplication on reconnect when the durable
     * consumer redelivers unacked messages.
     */
    fun isRendered(id: String): Boolean = id in renderedIds

    /**
     * Mark a notification ID as rendered.
     */
    fun markRendered(id: String) {
        renderedIds.add(id)
    }

    /**
     * Single connection attempt: build options, connect, bind
     * JetStream consumer. On failure, enter reconnection loop.
     */
    private suspend fun connectOnce(
        payload: ProvisioningPayload
    ) {
        _state.value = ConnectionState.Connecting

        try {
            val opts = NatsOptionsBuilder.build(payload)
            val nc = Nats.connect(opts)
            connection = nc

            subscribeToConsumer(nc, payload)
            subscribeToHeartbeat(nc, payload)
            subscribeToDeviceControl(nc, payload)
            queryInitialDashboard(nc, payload)

            _state.value = ConnectionState.Connected
            Log.i(TAG, "Connected to " +
                "${payload.host}:${payload.port}")

            // Monitor for disconnection. When jnats detects a
            // disconnect, nc.status changes. Poll periodically
            // since we disabled auto-reconnect.
            while (nc.status == Connection.Status.CONNECTED) {
                delay(1000)
            }

            // Connection lost — close explicitly to release the
            // push consumer's deliver-subject binding on the
            // server, then enter reconnection loop.
            Log.w(TAG, "Connection lost, status: ${nc.status}")
            closeQuietly(nc)
            connection = null
            reconnect(payload)

        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            Log.e(TAG, "Connection failed: ${e.message}", e)
            connection = null
            reconnect(payload)
        }
    }

    /**
     * Bind to the pre-existing durable consumer created by C-08
     * and start a coroutine to pump messages to the callback.
     */
    private fun subscribeToConsumer(
        nc: Connection,
        payload: ProvisioningPayload
    ) {
        val consumerName = if (payload.deviceId.isNotEmpty())
            "mobile-${payload.username}-${payload.deviceId}"
        else
            "mobile-${payload.username}"
        val js: JetStream = nc.jetStream()
        val subOpts = PushSubscribeOptions.bind(
            STREAM_NAME, consumerName
        )
        val subject =
            "resystems.renotify.${payload.username}.flow.>"
        val sub: JetStreamSubscription =
            js.subscribe(subject, subOpts)

        val handler = onMessage
        if (handler != null) {
            // Pump messages from the subscription to the callback
            // in a coroutine on the IO dispatcher.
            scope.launch(Dispatchers.IO) {
                Log.i(TAG, "Message pump started for $consumerName")
                try {
                    while (nc.status == Connection.Status.CONNECTED) {
                        val msg = sub.nextMessage(1000) ?: continue
                        handler(
                            msg.subject,
                            msg.data,
                            { msg.ack() }
                        )
                    }
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    Log.w(TAG, "Message pump error", e)
                }
                Log.i(TAG, "Message pump stopped for $consumerName")
            }
            Log.i(TAG, "Subscribed to $consumerName " +
                "with message handler")
        } else {
            Log.i(TAG, "Subscribed to $consumerName " +
                "(no message handler)")
        }
    }

    /**
     * Subscribe to daemon heartbeat messages over Core NATS
     * Pub/Sub. Heartbeats are ephemeral snapshots — missed ones
     * are superseded by the next. No ACK needed.
     */
    private fun subscribeToHeartbeat(
        nc: Connection,
        payload: ProvisioningPayload
    ) {
        val handler = onHeartbeat ?: return
        val subject = "resystems.renotify.${payload.username}" +
            ".daemon.*.heartbeat"

        val dispatcher = nc.createDispatcher { msg ->
            handler(msg.data)
        }
        dispatcher.subscribe(subject)

        Log.i(TAG, "Subscribed to heartbeat: $subject")
    }

    /**
     * Subscribe to device-specific control commands over Core
     * NATS Pub/Sub (C-16). Enables remote silent mode toggle
     * from the daemon CLI.
     */
    private fun subscribeToDeviceControl(
        nc: Connection,
        payload: ProvisioningPayload
    ) {
        val handler = onDeviceControl ?: return
        if (payload.deviceId.isEmpty()) return

        val subject = "resystems.renotify.${payload.username}" +
            ".device.${payload.deviceId}.control"

        val dispatcher = nc.createDispatcher { msg ->
            handler(msg.data)
        }
        dispatcher.subscribe(subject)

        Log.i(TAG, "Subscribed to device control: $subject")
    }

    /**
     * Query the daemon's svc.flows endpoint for an immediate
     * dashboard snapshot. Non-fatal on failure — the periodic
     * heartbeat will populate the dashboard later.
     */
    private fun queryInitialDashboard(
        nc: Connection,
        payload: ProvisioningPayload
    ) {
        val handler = onInitialDashboard ?: return
        val subject = "resystems.renotify.${payload.username}" +
            ".svc.flows"
        try {
            val resp = nc.request(
                subject, "{}".toByteArray(),
                java.time.Duration.ofSeconds(2))
            handler(resp.data)
            Log.i(TAG, "Initial dashboard loaded from svc.flows")
        } catch (e: Exception) {
            Log.w(TAG, "Initial dashboard query failed: " +
                "${e.message}")
        }
    }

    /**
     * Reconnection loop with exponential backoff: 1s, 2s, 4s,
     * 8s, 16s, 30s (capped). See Section 8.5.
     */
    private suspend fun reconnect(payload: ProvisioningPayload) {
        var attempt = 0
        while (true) {
            val delayMs = backoffDelay(attempt)
            _state.value = ConnectionState.Disconnected(
                since = System.currentTimeMillis()
            )
            Log.i(TAG, "Reconnecting in ${delayMs}ms " +
                "(attempt $attempt)")

            delay(delayMs)
            attempt++

            _state.value = ConnectionState.Connecting
            try {
                val opts = NatsOptionsBuilder.build(payload)
                val nc = Nats.connect(opts)
                connection = nc

                subscribeToConsumer(nc, payload)
                subscribeToHeartbeat(nc, payload)
                subscribeToDeviceControl(nc, payload)
                queryInitialDashboard(nc, payload)

                _state.value = ConnectionState.Connected
                Log.i(TAG, "Reconnected to " +
                    "${payload.host}:${payload.port}")

                // Resume monitoring.
                while (nc.status == Connection.Status.CONNECTED) {
                    delay(1000)
                }
                Log.w(TAG, "Connection lost again")
                closeQuietly(nc)
                connection = null
                attempt = 0 // reset backoff after a period of
                            // successful connection

            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Log.w(TAG, "Reconnect attempt $attempt failed: " +
                    "${e.message}")
                closeQuietly(connection)
                connection = null
            }
        }
    }

    /**
     * Query the daemon's svc.history endpoint. Returns the raw
     * response bytes, or null on failure. Called from
     * [NatsService] on the IO dispatcher.
     */
    fun queryHistory(requestJson: ByteArray): ByteArray? {
        val nc = connection ?: return null
        val p = payload ?: return null
        val subject = "resystems.renotify.${p.username}.svc.history"
        return try {
            val resp = nc.request(
                subject, requestJson,
                java.time.Duration.ofSeconds(2))
            resp.data
        } catch (e: Exception) {
            Log.w(TAG, "History query failed: ${e.message}")
            null
        }
    }

    /**
     * Close a NATS connection without propagating exceptions.
     * Sends a proper disconnect frame to the server so it
     * immediately releases the push consumer's deliver-subject
     * binding instead of waiting for the ping timeout.
     */
    private fun closeQuietly(nc: Connection?) {
        try {
            nc?.close()
        } catch (e: Exception) {
            Log.w(TAG, "Error closing stale connection", e)
        }
    }

    companion object {
        private const val TAG = "NatsConnectionManager"
        private const val STREAM_NAME = "RENOTIFY"

        /** Max backoff delay in milliseconds. */
        private const val MAX_BACKOFF_MS = 30_000L

        /**
         * Compute exponential backoff delay for a given attempt.
         * Sequence: 1s, 2s, 4s, 8s, 16s, 30s (capped).
         */
        fun backoffDelay(attempt: Int): Long {
            val base = 1000L * (1L shl min(attempt, 30))
            return min(base, MAX_BACKOFF_MS)
        }
    }
}
