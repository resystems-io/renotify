// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify

import android.app.Application
import io.resystems.renotify.telemetry.CrashReporter

class RenotifyApplication : Application() {
    override fun onCreate() {
        super.onCreate()

        // 1. Setup managed exception handler
        val defaultHandler = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, throwable ->
            // Capture and save the crash report
            CrashReporter.reportManagedException(this, thread, throwable)

            // Let the default handler finish the process termination
            defaultHandler?.uncaughtException(thread, throwable)
        }

        // 2. Ingest unmanaged exits
        CrashReporter.ingestUnmanagedExits(this)
    }
}
