// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.dashboard

import org.json.JSONObject

/**
 * A single history record pairing a notification request with
 * its optional response. Mirrors the Go wire format from
 * [registry.HistoryQueryResult].
 */
/**
 * A single history record in the unified timeline. Either a
 * notification (type "notification") or a lifecycle event
 * (type "lifecycle"), discriminated by [type].
 */
sealed class HistoryRecord {
    abstract val type: String
    abstract val timestamp: String
    abstract val flowId: String

    companion object {
        fun fromJson(obj: JSONObject): HistoryRecord {
            val type = obj.optString("type", "notification")
            return if (type == "lifecycle") {
                LifecycleRecord.fromJson(obj)
            } else {
                NotificationRecord.fromJson(obj)
            }
        }
    }
}

/**
 * A notification history record pairing a request with its
 * optional response.
 */
data class NotificationRecord(
    val id: String,
    override val flowId: String,
    val workspaceId: String,
    val flowLabel: String?,
    val workspaceName: String?,
    val title: String,
    val body: String?,
    val priority: String,
    val source: String,
    val responseTypes: List<String>,
    val actions: List<String>?,
    override val timestamp: String,
    val responseAccepted: Boolean?,
    val responseAction: String?,
    val responseText: String?,
    val responseTimestamp: String?
) : HistoryRecord() {
    override val type: String get() = "notification"

    /** True when a human response was received. */
    val hasResponse: Boolean
        get() = responseAccepted != null ||
            !responseAction.isNullOrEmpty() ||
            !responseText.isNullOrEmpty()

    /**
     * True when the notification expects a human response
     * (boolean, choice, or text — not fire-and-forget).
     */
    val isInteractive: Boolean
        get() = responseTypes.any { it != "none" }

    /**
     * True when the notification expects a response but none
     * has been received yet.
     */
    val isOpen: Boolean
        get() = isInteractive && !hasResponse

    /**
     * True when the record has content worth showing in an
     * expanded view — a body or a response text that was
     * truncated in the summary.
     */
    val isExpandable: Boolean
        get() = !body.isNullOrEmpty() ||
            (!responseText.isNullOrEmpty() && responseText.length > 30)

    /** Human-readable response summary for display. */
    val responseSummary: String
        get() = when {
            responseAccepted == true -> "accepted"
            responseAccepted == false -> "denied"
            !responseAction.isNullOrEmpty() -> responseAction
            !responseText.isNullOrEmpty() -> {
                if (responseText.length > 30)
                    responseText.take(27) + "..."
                else responseText
            }
            else -> "\u2014"
        }

    companion object {
        fun fromJson(obj: JSONObject): NotificationRecord {
            val req = obj.getJSONObject("request")
            val resp = if (obj.has("response") &&
                !obj.isNull("response")
            ) obj.getJSONObject("response") else null

            return NotificationRecord(
                id = req.getString("id"),
                flowId = req.getString("flow_id"),
                workspaceId = req.getString("workspace_id"),
                flowLabel = obj.optString("flow_label", "")
                    .ifEmpty { null },
                workspaceName = obj.optString("workspace_name", "")
                    .ifEmpty { null },
                title = req.getString("title"),
                body = req.optString("body", "")
                    .ifEmpty { null },
                priority = req.optString("priority", "normal"),
                source = req.optString("source", ""),
                responseTypes = run {
                    val arr = req.optJSONArray("response_types")
                    if (arr != null)
                        (0 until arr.length()).map {
                            arr.getString(it)
                        }
                    else listOf("none")
                },
                actions = run {
                    val arr = req.optJSONArray("actions")
                    if (arr != null)
                        (0 until arr.length()).map {
                            arr.getString(it)
                        }
                    else null
                },
                timestamp = req.getString("timestamp"),
                responseAccepted = resp?.let {
                    if (it.has("accepted") && !it.isNull("accepted"))
                        it.getBoolean("accepted") else null
                },
                responseAction = resp?.optString("action", "")
                    ?.ifEmpty { null },
                responseText = resp?.optString("text", "")
                    ?.ifEmpty { null },
                responseTimestamp = resp?.optString("timestamp", "")
                    ?.ifEmpty { null }
            )
        }
    }
}

/**
 * A flow lifecycle event (started, completed, failed) in the
 * history timeline.
 */
data class LifecycleRecord(
    override val flowId: String,
    val daemonId: String,
    val workspaceId: String,
    val status: String,
    val label: String?,
    val metadata: Map<String, String>?,
    override val timestamp: String
) : HistoryRecord() {
    override val type: String get() = "lifecycle"

    /** Human-readable summary for display. */
    val summary: String
        get() {
            val name = label ?: flowId
            return when (status) {
                "active" -> "Flow started: $name"
                "completed" -> "Flow completed: $name"
                "failed" -> "Flow failed: $name"
                else -> "Flow $status: $name"
            }
        }

    companion object {
        fun fromJson(obj: JSONObject): LifecycleRecord {
            val lc = obj.getJSONObject("lifecycle")
            val meta = if (lc.has("metadata") &&
                !lc.isNull("metadata")
            ) {
                val m = lc.getJSONObject("metadata")
                m.keys().asSequence().associateWith { m.getString(it) }
            } else null

            return LifecycleRecord(
                flowId = lc.getString("flow_id"),
                daemonId = lc.getString("daemon_id"),
                workspaceId = lc.getString("workspace_id"),
                status = lc.getString("status"),
                label = lc.optString("label", "").ifEmpty { null },
                metadata = meta,
                timestamp = lc.getString("timestamp")
            )
        }
    }
}

/**
 * Paginated history query result from the svc.history endpoint.
 */
data class HistoryQueryResult(
    val records: List<HistoryRecord>,
    val total: Int
) {
    companion object {
        fun fromJson(json: String): HistoryQueryResult {
            val obj = JSONObject(json)
            val arr = obj.getJSONArray("records")
            val records = (0 until arr.length()).map {
                HistoryRecord.fromJson(arr.getJSONObject(it))
            }
            return HistoryQueryResult(
                records = records,
                total = obj.optInt("total", records.size)
            )
        }
    }
}
