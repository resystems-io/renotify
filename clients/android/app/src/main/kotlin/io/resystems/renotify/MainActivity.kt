package io.resystems.renotify

import android.content.Intent
import android.os.Bundle
import android.view.Gravity
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import io.resystems.renotify.pairing.EncryptedProvisioningStore
import io.resystems.renotify.pairing.ScannerActivity

/**
 * Minimal launcher activity. This stub is replaced with the full
 * UI in later phases (M-05 branding, M-09 dashboard).
 *
 * Current responsibilities:
 * - Display pairing status (paired/not paired)
 * - Launch [ScannerActivity] to scan a pairing QR code
 * - Refresh status when returning from the scanner
 */
class MainActivity : ComponentActivity() {

    private lateinit var statusText: TextView
    private lateinit var store: EncryptedProvisioningStore

    /**
     * Activity result callback for [ScannerActivity]. When the
     * scanner returns [RESULT_OK], the provisioning credentials
     * have been stored and we refresh the status display.
     */
    private val scanLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == RESULT_OK) {
            updateStatus()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Encrypted store for pairing credentials. Initialised
        // early so updateStatus() can read on first layout.
        store = EncryptedProvisioningStore(this)

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

        // Pairing status line — updated by updateStatus().
        statusText = TextView(this).apply {
            textSize = 14f
            gravity = Gravity.CENTER
            setPadding(0, 24, 0, 24)
        }
        layout.addView(statusText)

        // Launches the camera-based QR scanner. On success the
        // scanner stores credentials and finishes with RESULT_OK,
        // which triggers the scanLauncher callback above.
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

        setContentView(layout)
        updateStatus()
    }

    /**
     * Refresh status on resume so that external state changes
     * (e.g. re-pairing from a different entry point) are
     * reflected immediately.
     */
    override fun onResume() {
        super.onResume()
        updateStatus()
    }

    /**
     * Read the stored provisioning payload and update the status
     * text to show the paired WSS endpoint or "Not paired".
     */
    private fun updateStatus() {
        val payload = store.load()
        statusText.text = if (payload != null) {
            "Paired: wss://${payload.host}:${payload.port}"
        } else {
            "Not paired"
        }
    }
}
