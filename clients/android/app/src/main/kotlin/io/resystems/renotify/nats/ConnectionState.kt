package io.resystems.renotify.nats

/**
 * Connection state machine for the NATS WebSocket connection.
 * Observed by [NatsService] (for the persistent notification)
 * and by [io.resystems.renotify.MainActivity] (for the status
 * display). See R-MOB-10.
 */
sealed class ConnectionState {
    /** Service not started or explicitly stopped. */
    data object Idle : ConnectionState()

    /** No provisioning credentials stored. */
    data object Unpaired : ConnectionState()

    /** WSS connection attempt in progress. */
    data object Connecting : ConnectionState()

    /** Connected to the NATS broker. */
    data object Connected : ConnectionState()

    /** Connection lost; reconnection in progress. */
    data class Disconnected(val since: Long) : ConnectionState()

    /** Unrecoverable error (e.g., fingerprint mismatch). */
    data class Error(val message: String) : ConnectionState()
}
