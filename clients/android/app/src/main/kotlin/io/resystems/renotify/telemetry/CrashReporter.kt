// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.telemetry

import android.app.ActivityManager
import android.app.ApplicationExitInfo
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.BatteryManager
import android.os.Build
import android.util.Log
import io.resystems.renotify.pairing.EncryptedProvisioningStore
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.UUID

object CrashReporter {
    private const val TAG = "CrashReporter"
    private const val PREFS_NAME = "crash_reporter_prefs"
    private const val PREF_LAST_EXIT_TIMESTAMP = "last_exit_timestamp"

    fun getCrashDir(context: Context): File {
        val dir = File(context.cacheDir, "telemetry/crashes")
        if (!dir.exists()) {
            dir.mkdirs()
        }
        return dir
    }

    fun reportManagedException(context: Context, thread: Thread, throwable: Throwable) {
        try {
            val store = EncryptedProvisioningStore(context)
            val provisioning = store.load()
            val deviceId = provisioning?.deviceId ?: "unknown"

            val details = IncidentDetails(
                exceptionType = throwable.javaClass.name,
                message = throwable.message ?: "",
                stackTrace = Log.getStackTraceString(throwable)
            )

            val report = IncidentReport(
                reportId = generateReportId(),
                deviceId = deviceId,
                timestamp = getCurrentTimestamp(),
                incidentType = "managed_crash",
                incidentDetails = details,
                deviceContext = buildDeviceContext(context),
                breadcrumbs = emptyList(), // Breadcrumbs not implemented in MVP
                logcatTail = fetchLogcatTail()
            )

            writeReport(context, report)
        } catch (e: Exception) {
            Log.e(TAG, "Failed to capture managed exception", e)
        }
    }

    fun ingestUnmanagedExits(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) {
            return
        }

        try {
            val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
            val exits = am.getHistoricalProcessExitReasons(context.packageName, 0, 10)
            
            val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            val lastProcessedTimestamp = prefs.getLong(PREF_LAST_EXIT_TIMESTAMP, 0L)
            
            var maxTimestamp = lastProcessedTimestamp

            val store = EncryptedProvisioningStore(context)
            val provisioning = store.load()
            val deviceId = provisioning?.deviceId ?: "unknown"

            for (exit in exits) {
                if (exit.timestamp <= lastProcessedTimestamp) {
                    continue
                }
                
                maxTimestamp = maxOf(maxTimestamp, exit.timestamp)

                // Only capture significant abnormal exits
                when (exit.reason) {
                    ApplicationExitInfo.REASON_ANR,
                    ApplicationExitInfo.REASON_CRASH,
                    ApplicationExitInfo.REASON_CRASH_NATIVE,
                    ApplicationExitInfo.REASON_INITIALIZATION_FAILURE -> {
                        
                        val traceText = try {
                            exit.traceInputStream?.bufferedReader()?.use { it.readText() } ?: ""
                        } catch (e: Exception) {
                            ""
                        }

                        val details = IncidentDetails(
                            exceptionType = "UnmanagedKill",
                            message = exit.description ?: "Process terminated by system",
                            stackTrace = traceText,
                            exitReason = exit.reason
                        )

                        val report = IncidentReport(
                            reportId = generateReportId(),
                            deviceId = deviceId,
                            timestamp = formatTimestamp(exit.timestamp),
                            incidentType = "unmanaged_kill",
                            incidentDetails = details,
                            deviceContext = buildDeviceContext(context),
                            breadcrumbs = emptyList(),
                            logcatTail = ""
                        )

                        writeReport(context, report)
                    }
                }
            }

            if (maxTimestamp > lastProcessedTimestamp) {
                prefs.edit().putLong(PREF_LAST_EXIT_TIMESTAMP, maxTimestamp).apply()
            }
        } catch (e: Exception) {
            Log.e(TAG, "Failed to ingest unmanaged exits", e)
        }
    }

    private fun writeReport(context: Context, report: IncidentReport) {
        val file = File(getCrashDir(context), "${report.reportId}.json")
        file.writeText(report.toJson().toString())
        Log.i(TAG, "Saved crash report to ${file.absolutePath}")
    }

    private fun buildDeviceContext(context: Context): DeviceContext {
        val batteryIntent = context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
        val level = batteryIntent?.getIntExtra(BatteryManager.EXTRA_LEVEL, -1) ?: -1
        val scale = batteryIntent?.getIntExtra(BatteryManager.EXTRA_SCALE, -1) ?: -1
        val batteryPct = if (level != -1 && scale != -1) (level * 100 / scale.toFloat()).toInt() else -1

        val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val memoryInfo = ActivityManager.MemoryInfo()
        am.getMemoryInfo(memoryInfo)
        
        val memoryState = if (memoryInfo.lowMemory) "low" else "normal"

        val appVersion = try {
            val pInfo = context.packageManager.getPackageInfo(context.packageName, 0)
            pInfo.versionName ?: "unknown"
        } catch (e: Exception) {
            "unknown"
        }

        return DeviceContext(
            osVersion = "Android ${Build.VERSION.RELEASE} (API ${Build.VERSION.SDK_INT})",
            appVersion = appVersion,
            batteryLevel = batteryPct,
            memoryState = memoryState
        )
    }

    private fun fetchLogcatTail(): String {
        return try {
            val process = Runtime.getRuntime().exec("logcat -d -t 100")
            process.inputStream.bufferedReader().use { it.readText() }
        } catch (e: Exception) {
            "Failed to read logcat: ${e.message}"
        }
    }

    private fun generateReportId(): String {
        return "ntf_${UUID.randomUUID().toString().replace("-", "").take(16).lowercase()}"
    }

    private fun getCurrentTimestamp(): String {
        return formatTimestamp(System.currentTimeMillis())
    }

    private fun formatTimestamp(timeMs: Long): String {
        val sdf = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US)
        sdf.timeZone = java.util.TimeZone.getTimeZone("UTC")
        return sdf.format(Date(timeMs))
    }
}
