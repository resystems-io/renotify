package io.resystems.renotify.nats

import android.util.Log
import io.nats.client.Connection
import io.nats.client.JetStream
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
class NatsConnectionManager(private val scope: CoroutineScope) {

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
            // scope (which may be cancelled in parallel).
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

            // Bind to the pre-existing durable consumer created
            // by C-08 during daemon startup.
            val consumerName = "mobile-${payload.username}"
            val js: JetStream = nc.jetStream()
            val subOpts = PushSubscribeOptions.bind(
                STREAM_NAME, consumerName
            )
            val subject =
                "resystems.renotify.${payload.username}.flow.>"
            js.subscribe(subject, subOpts)

            _state.value = ConnectionState.Connected
            Log.i(TAG, "Connected to " +
                "${payload.host}:${payload.port}")

            // Monitor for disconnection. When jnats detects a
            // disconnect, nc.status changes. Poll periodically
            // since we disabled auto-reconnect.
            while (nc.status == Connection.Status.CONNECTED) {
                delay(1000)
            }

            // Connection lost — enter reconnection loop.
            Log.w(TAG, "Connection lost, status: ${nc.status}")
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

                val consumerName = "mobile-${payload.username}"
                val js: JetStream = nc.jetStream()
                val subOpts = PushSubscribeOptions.bind(
                    STREAM_NAME, consumerName
                )
                val subject =
                    "resystems.renotify.${payload.username}.flow.>"
                js.subscribe(subject, subOpts)

                _state.value = ConnectionState.Connected
                Log.i(TAG, "Reconnected to " +
                    "${payload.host}:${payload.port}")

                // Resume monitoring.
                while (nc.status == Connection.Status.CONNECTED) {
                    delay(1000)
                }
                Log.w(TAG, "Connection lost again")
                connection = null
                attempt = 0 // reset backoff after a period of
                            // successful connection

            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Log.w(TAG, "Reconnect attempt $attempt failed: " +
                    "${e.message}")
                connection = null
            }
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
