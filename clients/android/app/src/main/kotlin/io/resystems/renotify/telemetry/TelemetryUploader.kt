// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.telemetry

import android.content.Context
import android.util.Log
import io.nats.client.Connection
import io.resystems.renotify.pairing.ProvisioningPayload
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

object TelemetryUploader {
    private const val TAG = "TelemetryUploader"

    /**
     * Scan the secure crash telemetry cache directory and upload any deferred
     * incident reports to JetStream, purging them locally upon NATS persistence confirmation.
     */
    fun transmitDeferredCrashes(
        context: Context,
        scope: CoroutineScope,
        nc: Connection,
        payload: ProvisioningPayload,
        dispatcher: kotlinx.coroutines.CoroutineDispatcher = Dispatchers.IO
    ) {
        val crashDir = CrashReporter.getCrashDir(context)
        val files = crashDir.listFiles { _, name -> name.endsWith(".json") } ?: return

        if (files.isEmpty()) return

        Log.i(TAG, "Found ${files.size} deferred crash reports to upload")

        scope.launch(dispatcher) {
            try {
                val js = nc.jetStream()
                val subject = "resystems.renotify.${payload.username}.device.${payload.deviceId}.telemetry.crash"

                for (file in files) {
                    try {
                        val data = file.readBytes()
                        val reportJson = String(data, Charsets.UTF_8)
                        val reportObj = org.json.JSONObject(reportJson)
                        val reportId = reportObj.optString("report_id", file.nameWithoutExtension)

                        val headers = io.nats.client.impl.Headers()
                        headers.add("Nats-Msg-Id", reportId)

                        val msg = io.nats.client.impl.NatsMessage.builder()
                            .subject(subject)
                            .headers(headers)
                            .data(data)
                            .build()

                        js.publish(msg)
                        Log.i(TAG, "Successfully uploaded crash report: $reportId")

                        // Delete file only after successful JetStream persistence confirmation
                        file.delete()
                    } catch (e: Exception) {
                        Log.e(TAG, "Failed to upload crash report file: ${file.name}", e)
                    }
                }
            } catch (e: Exception) {
                Log.e(TAG, "Failed to initialize JetStream for crash upload", e)
            }
        }
    }
}
