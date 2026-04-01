package io.resystems.renotify

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.Gravity
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import io.resystems.renotify.nats.ConnectionState
import io.resystems.renotify.nats.NatsService
import io.resystems.renotify.pairing.EncryptedProvisioningStore
import io.resystems.renotify.pairing.ScannerActivity
import kotlinx.coroutines.launch

/**
 * Minimal launcher activity. This stub is replaced with the full
 * UI in later phases (M-05 branding, M-09 dashboard).
 *
 * Current responsibilities:
 * - Display connection status (R-MOB-10)
 * - Launch [ScannerActivity] to scan a pairing QR code
 * - Start/stop [NatsService] via connect/disconnect toggle
 */
class MainActivity : ComponentActivity() {

    private lateinit var statusText: TextView
    private lateinit var connectButton: Button
    private lateinit var store: EncryptedProvisioningStore

    /**
     * Request notification permission on Android 13+. The result
     * is informational — the app works without it but
     * notifications will be silently dropped.
     */
    private val notificationPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { _ ->
        // No action needed — if denied, notifications won't
        // display but the service still runs.
    }

    /**
     * Activity result callback for [ScannerActivity]. When the
     * scanner returns [RESULT_OK], credentials are stored and we
     * start the NATS service to connect immediately.
     */
    private val scanLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == RESULT_OK) {
            // Stop the existing service (which holds old
            // credentials) and start a fresh one with the
            // newly scanned credentials.
            stopNatsService()
            startNatsService()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Encrypted store for pairing credentials.
        store = EncryptedProvisioningStore(this)

        // Request notification permission on Android 13+ (API 33).
        // Without this, notifications are silently suppressed.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(
                    this, Manifest.permission.POST_NOTIFICATIONS
                ) != PackageManager.PERMISSION_GRANTED
            ) {
                notificationPermission.launch(
                    Manifest.permission.POST_NOTIFICATIONS
                )
            }
        }

        // --- Programmatic layout (no XML) ---
        // Matches the M-01 scaffold pattern. Later phases (M-05,
        // M-09) will replace this with a full Compose or
        // fragment-based UI.

        val layout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            setPadding(48, 48, 48, 48)
        }

        // App title.
        val title = TextView(this).apply {
            text = "Renotify"
            textSize = 24f
            gravity = Gravity.CENTER
        }
        layout.addView(title)

        // Connection status line — updated by observing
        // NatsService.state (R-MOB-10).
        statusText = TextView(this).apply {
            textSize = 14f
            gravity = Gravity.CENTER
            setPadding(0, 24, 0, 24)
        }
        layout.addView(statusText)

        // Launches the camera-based QR scanner. On success the
        // scanner stores credentials and finishes with RESULT_OK,
        // which starts the NATS service automatically.
        val pairButton = Button(this).apply {
            text = "Scan Pairing QR Code"
            setOnClickListener {
                scanLauncher.launch(
                    Intent(
                        this@MainActivity,
                        ScannerActivity::class.java
                    )
                )
            }
        }
        layout.addView(pairButton)

        // Connect/disconnect toggle. Visible only when paired.
        // Allows the user to stop connection attempts (e.g.,
        // when the daemon is known to be unreachable) and
        // reconnect later.
        connectButton = Button(this).apply {
            setOnClickListener { toggleConnection() }
        }
        layout.addView(connectButton)

        setContentView(layout)

        // Observe the NATS connection state and update both the
        // status text and the connect/disconnect button label.
        lifecycleScope.launch {
            NatsService.state.collect { state ->
                statusText.text = formatState(state)
                updateConnectButton(state)
            }
        }

        // On app relaunch, auto-start the service if already
        // paired and the service is not running.
        if (store.isPaired() && !isServiceActive()) {
            startNatsService()
        }
    }

    /**
     * Start [NatsService]. The service reads credentials from
     * [EncryptedProvisioningStore] and connects to the daemon.
     */
    private fun startNatsService() {
        if (store.isPaired()) {
            startForegroundService(
                Intent(this, NatsService::class.java)
            )
        }
    }

    /**
     * Stop [NatsService], cancelling any connection or
     * reconnection attempts.
     */
    private fun stopNatsService() {
        stopService(Intent(this, NatsService::class.java))
    }

    /**
     * Toggle the NATS service based on current state. If
     * connected or connecting, disconnect. If idle or
     * disconnected, reconnect.
     */
    private fun toggleConnection() {
        val state = NatsService.state.value
        if (isServiceActive(state)) {
            stopNatsService()
        } else {
            startNatsService()
        }
    }

    /**
     * Update the connect/disconnect button text and visibility
     * based on the current connection state.
     */
    private fun updateConnectButton(state: ConnectionState) {
        if (!store.isPaired()) {
            // Not paired — hide the button.
            connectButton.visibility = Button.GONE
            return
        }

        connectButton.visibility = Button.VISIBLE
        connectButton.text = if (isServiceActive(state)) {
            "Disconnect"
        } else {
            "Connect"
        }
    }

    /**
     * Check whether the service is in an active state
     * (connecting, connected, or reconnecting).
     */
    private fun isServiceActive(
        state: ConnectionState = NatsService.state.value
    ): Boolean = when (state) {
        is ConnectionState.Connecting,
        is ConnectionState.Connected,
        is ConnectionState.Disconnected -> true
        else -> false
    }

    /**
     * Format the [ConnectionState] for display in the status
     * text.
     */
    private fun formatState(state: ConnectionState): String {
        return when (state) {
            is ConnectionState.Idle -> {
                if (store.isPaired()) "Paired (disconnected)"
                else "Not paired"
            }
            is ConnectionState.Unpaired -> "Not paired"
            is ConnectionState.Connecting -> {
                val p = store.load()
                if (p != null) "Connecting to ${p.host}:${p.port}..."
                else "Connecting..."
            }
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
    }
}
