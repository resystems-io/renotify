package io.resystems.renotify.pairing

import android.Manifest
import android.content.pm.PackageManager
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.Gravity
import android.widget.FrameLayout
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.core.content.ContextCompat
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.BarcodeScannerOptions
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage

/**
 * Camera-based QR code scanner for pairing with the Renotify
 * daemon. Decodes the [ProvisioningPayload], validates it, and
 * stores credentials via [EncryptedProvisioningStore].
 *
 * Launched from [io.resystems.renotify.MainActivity] via
 * [ActivityResultContracts.StartActivityForResult]. Returns
 * [RESULT_OK] on successful pairing, [RESULT_CANCELED] on back
 * press or permission denial.
 */
class ScannerActivity : ComponentActivity() {

    private lateinit var previewView: PreviewView
    private lateinit var statusText: TextView
    private lateinit var store: EncryptedProvisioningStore

    private var scanComplete = false
    private var lastErrorTime = 0L

    private val requestPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) {
            startCamera()
        } else {
            statusText.text = "Camera permission is required " +
                "to scan the pairing QR code."
            setResult(RESULT_CANCELED)
            Handler(Looper.getMainLooper()).postDelayed(
                { finish() }, 2000
            )
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        store = EncryptedProvisioningStore(this)

        // Build layout programmatically (no XML, matching
        // existing MainActivity pattern).
        val root = FrameLayout(this)

        previewView = PreviewView(this).apply {
            layoutParams = FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        }
        root.addView(previewView)

        statusText = TextView(this).apply {
            layoutParams = FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.BOTTOM
            )
            textSize = 16f
            setPadding(32, 24, 32, 24)
            setBackgroundColor(0xCC000000.toInt())
            setTextColor(0xFFFFFFFF.toInt())
            text = "Point the camera at the pairing QR code"
        }
        root.addView(statusText)

        setContentView(root)

        // Check camera permission.
        if (ContextCompat.checkSelfPermission(
                this, Manifest.permission.CAMERA
            ) == PackageManager.PERMISSION_GRANTED
        ) {
            startCamera()
        } else {
            requestPermission.launch(Manifest.permission.CAMERA)
        }
    }

    @androidx.annotation.OptIn(androidx.camera.core.ExperimentalGetImage::class)
    private fun startCamera() {
        val cameraProviderFuture =
            ProcessCameraProvider.getInstance(this)

        cameraProviderFuture.addListener({
            val cameraProvider = cameraProviderFuture.get()

            val preview = Preview.Builder().build().also {
                it.surfaceProvider = previewView.surfaceProvider
            }

            val analysis = ImageAnalysis.Builder()
                .setBackpressureStrategy(
                    ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST
                )
                .build()
                .also {
                    it.setAnalyzer(
                        ContextCompat.getMainExecutor(this),
                        ::analyzeImage
                    )
                }

            cameraProvider.unbindAll()
            cameraProvider.bindToLifecycle(
                this,
                CameraSelector.DEFAULT_BACK_CAMERA,
                preview,
                analysis
            )
        }, ContextCompat.getMainExecutor(this))
    }

    @androidx.camera.core.ExperimentalGetImage
    private fun analyzeImage(imageProxy: ImageProxy) {
        if (scanComplete) {
            imageProxy.close()
            return
        }

        val mediaImage = imageProxy.image
        if (mediaImage == null) {
            imageProxy.close()
            return
        }

        val inputImage = InputImage.fromMediaImage(
            mediaImage, imageProxy.imageInfo.rotationDegrees
        )

        val options = BarcodeScannerOptions.Builder()
            .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
            .build()
        val scanner = BarcodeScanning.getClient(options)

        scanner.process(inputImage)
            .addOnSuccessListener { barcodes ->
                for (barcode in barcodes) {
                    val raw = barcode.rawValue ?: continue
                    processQrPayload(raw)
                    if (scanComplete) break
                }
            }
            .addOnFailureListener { e ->
                Log.w(TAG, "Barcode scan failed", e)
            }
            .addOnCompleteListener {
                imageProxy.close()
            }
    }

    private fun processQrPayload(raw: String) {
        if (scanComplete) return

        try {
            val payload = ProvisioningPayload.fromJson(raw)
            store.save(payload)
            scanComplete = true

            statusText.text = "Paired with ${payload.host}:${payload.port}"
            Log.i(TAG, "Pairing successful: ${payload.host}:${payload.port}")

            setResult(RESULT_OK)
            Handler(Looper.getMainLooper()).postDelayed(
                { finish() }, 1000
            )
        } catch (e: IllegalArgumentException) {
            // Debounce error messages (one per 2 seconds).
            val now = System.currentTimeMillis()
            if (now - lastErrorTime > 2000) {
                lastErrorTime = now
                statusText.text = "Invalid QR code: ${e.message}"
                Log.w(TAG, "Invalid QR payload: ${e.message}")
            }
        }
    }

    companion object {
        private const val TAG = "ScannerActivity"
    }
}
