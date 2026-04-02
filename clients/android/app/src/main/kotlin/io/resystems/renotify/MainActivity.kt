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
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import io.resystems.renotify.dashboard.DashboardAdapter
import io.resystems.renotify.nats.ConnectionState
import io.resystems.renotify.nats.NatsService
import io.resystems.renotify.pairing.EncryptedProvisioningStore
import io.resystems.renotify.pairing.ScannerActivity
import kotlinx.coroutines.launch

/**
 * Launcher activity showing the Renotify dashboard. Displays
 * connection status (R-MOB-10) and active workspaces with their
 * flows (R-MOB-09) using daemon heartbeat data.
 */
class MainActivity : ComponentActivity() {

    private lateinit var statusText: TextView
    private lateinit var connectButton: Button
    private lateinit var silentButton: Button
    private lateinit var store: EncryptedProvisioningStore
    private lateinit var dashboardAdapter: DashboardAdapter

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

        // --- Programmatic layout ---

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            fitsSystemWindows = true
        }

        // App title.
        if (false) {
            val title = TextView(this).apply {
                text = "Renotify"
                textSize = 24f
                gravity = Gravity.CENTER
                setPadding(dp(16), dp(8), dp(16), dp(4))
            }
            root.addView(title)
        }

        // Connection status line (R-MOB-10).
        statusText = TextView(this).apply {
            textSize = 14f
            gravity = Gravity.CENTER
            setTextColor(0xFF333333.toInt())
            setPadding(dp(16), dp(8), dp(16), dp(12))
            // Show provisioning details immediately if paired,
            // before the first state update arrives.
            val p = store.load()
            text = if (p != null) "Paired: ${p.host}:${p.port}"
                else "Not paired"
        }
        root.addView(statusText)

        // Dashboard RecyclerView (M-09).
        dashboardAdapter = DashboardAdapter()
        val recycler = RecyclerView(this).apply {
            layoutManager = LinearLayoutManager(this@MainActivity)
            adapter = dashboardAdapter
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f
            )
        }
        root.addView(recycler)

        // Bottom bar with pair and connect buttons.
        val bottomBar = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER
            setPadding(dp(16), dp(12), dp(16), dp(32))
        }

        // Shared button style for the bottom bar.
        fun styledButton(marginStart: Int = 0) = Button(this).apply {
            textSize = 12f
            background = android.graphics.drawable.GradientDrawable().apply {
                setColor(0xFF444444.toInt())
                cornerRadius = dp(6).toFloat()
            }
            setTextColor(0xFFFFFFFF.toInt())
            setPadding(dp(10), dp(8), dp(10), dp(8))
            minWidth = 0
            minimumWidth = 0
            if (marginStart > 0) {
                val lp = LinearLayout.LayoutParams(
                    LinearLayout.LayoutParams.WRAP_CONTENT,
                    LinearLayout.LayoutParams.WRAP_CONTENT
                )
                lp.marginStart = dp(marginStart)
                layoutParams = lp
            }
        }

        // Pair button (R-MOB-01).
        val pairButton = styledButton().apply {
            text = "Pair"
            setOnClickListener {
                scanLauncher.launch(
                    Intent(
                        this@MainActivity,
                        ScannerActivity::class.java
                    )
                )
            }
        }
        bottomBar.addView(pairButton)

        // Connect/disconnect button (R-MOB-02).
        connectButton = styledButton(marginStart = 6).apply {
            setOnClickListener { toggleConnection() }
        }
        bottomBar.addView(connectButton)

        // Silent mode toggle.
        silentButton = styledButton(marginStart = 6).apply {
            setOnClickListener {
                val newState = !NatsService.silentMode.value
                NatsService.setSilentMode(this@MainActivity, newState)
            }
        }
        bottomBar.addView(silentButton)

        root.addView(bottomBar)

        setContentView(root)

        // Observe connection state.
        lifecycleScope.launch {
            NatsService.state.collect { state ->
                statusText.text = formatState(state)
                updateConnectButton(state)
            }
        }

        // Observe dashboard heartbeat (M-09).
        lifecycleScope.launch {
            NatsService.dashboardState.collect { heartbeat ->
                dashboardAdapter.update(heartbeat)
            }
        }

        // Observe silent mode.
        lifecycleScope.launch {
            NatsService.silentMode.collect { silent ->
                silentButton.text = if (silent) "Unmute" else "Silent"
                // Update status text to reflect silent state.
                statusText.text = formatState(NatsService.state.value)
            }
        }

        // Auto-start service if already paired.
        if (store.isPaired() && !isServiceActive()) {
            startNatsService()
        }
    }

    // --- Service management ---

    private fun startNatsService() {
        if (store.isPaired()) {
            startForegroundService(
                Intent(this, NatsService::class.java)
            )
        }
    }

    private fun stopNatsService() {
        stopService(Intent(this, NatsService::class.java))
    }

    private fun toggleConnection() {
        val state = NatsService.state.value
        if (isServiceActive(state)) {
            stopNatsService()
        } else {
            startNatsService()
        }
    }

    private fun updateConnectButton(state: ConnectionState) {
        if (!store.isPaired()) {
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

    private fun isServiceActive(
        state: ConnectionState = NatsService.state.value
    ): Boolean = when (state) {
        is ConnectionState.Connecting,
        is ConnectionState.Connected,
        is ConnectionState.Disconnected -> true
        else -> false
    }

    private fun formatState(state: ConnectionState): String {
        val base = when (state) {
            is ConnectionState.Idle -> {
                if (store.isPaired()) "Paired (disconnected)"
                else "Not paired"
            }
            is ConnectionState.Unpaired -> "Not paired"
            is ConnectionState.Connecting -> {
                val p = store.load()
                if (p != null) "Connecting to ${p.host}:${p.port}\u2026"
                else "Connecting\u2026"
            }
            is ConnectionState.Connected -> {
                val p = store.load()
                if (p != null) "Connected to ${p.host}:${p.port}"
                else "Connected"
            }
            is ConnectionState.Disconnected ->
                "Disconnected \u2014 reconnecting\u2026"
            is ConnectionState.Error ->
                "Error: ${state.message}"
        }
        return if (NatsService.silentMode.value)
            "$base (silent)" else base
    }

    private fun dp(value: Int): Int {
        return (value * resources.displayMetrics.density).toInt()
    }
}
