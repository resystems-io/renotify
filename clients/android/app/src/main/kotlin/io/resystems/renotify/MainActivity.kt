package io.resystems.renotify

import android.os.Bundle
import android.widget.TextView
import androidx.activity.ComponentActivity

/**
 * Minimal launcher activity. This stub is replaced with the full
 * UI in later phases (M-05 branding, M-09 dashboard).
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val view = TextView(this).apply {
            text = "Renotify"
            textSize = 24f
        }
        setContentView(view)
    }
}
