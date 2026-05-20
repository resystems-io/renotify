// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.telemetry

import org.json.JSONArray
import org.json.JSONObject

data class IncidentReport(
    val reportId: String,
    val deviceId: String,
    val timestamp: String,
    val incidentType: String,
    val incidentDetails: IncidentDetails,
    val deviceContext: DeviceContext,
    val breadcrumbs: List<Breadcrumb> = emptyList(),
    val logcatTail: String = ""
) {
    fun toJson(): JSONObject {
        val root = JSONObject()
        root.put("report_id", reportId)
        root.put("device_id", deviceId)
        root.put("timestamp", timestamp)
        root.put("incident_type", incidentType)
        root.put("incident_details", incidentDetails.toJson())
        root.put("device_context", deviceContext.toJson())
        
        val breadcrumbsArray = JSONArray()
        breadcrumbs.forEach { breadcrumbsArray.put(it.toJson()) }
        root.put("breadcrumbs", breadcrumbsArray)
        
        root.put("logcat_tail", logcatTail)
        return root
    }
}

data class IncidentDetails(
    val exceptionType: String,
    val message: String,
    val stackTrace: String,
    val exitReason: Int? = null
) {
    fun toJson(): JSONObject {
        val obj = JSONObject()
        obj.put("exception_type", exceptionType)
        obj.put("message", message)
        obj.put("stack_trace", stackTrace)
        if (exitReason != null) {
            obj.put("exit_reason", exitReason)
        }
        return obj
    }
}

data class DeviceContext(
    val osVersion: String,
    val appVersion: String,
    val batteryLevel: Int,
    val memoryState: String
) {
    fun toJson(): JSONObject {
        val obj = JSONObject()
        obj.put("os_version", osVersion)
        obj.put("app_version", appVersion)
        obj.put("battery_level", batteryLevel)
        obj.put("memory_state", memoryState)
        return obj
    }
}

data class Breadcrumb(
    val timestamp: String,
    val message: String
) {
    fun toJson(): JSONObject {
        val obj = JSONObject()
        obj.put("timestamp", timestamp)
        obj.put("message", message)
        return obj
    }
}
