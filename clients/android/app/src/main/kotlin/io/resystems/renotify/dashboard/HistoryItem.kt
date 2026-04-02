package io.resystems.renotify.dashboard

import org.json.JSONObject

/**
 * A single history record pairing a notification request with
 * its optional response. Mirrors the Go wire format from
 * [registry.HistoryQueryResult].
 */
data class HistoryRecord(
    val id: String,
    val flowId: String,
    val workspaceId: String,
    val title: String,
    val body: String?,
    val priority: String,
    val source: String,
    val timestamp: String,
    val responseAccepted: Boolean?,
    val responseAction: String?,
    val responseText: String?,
    val responseTimestamp: String?
) {
    /** True when a human response was received. */
    val hasResponse: Boolean
        get() = responseAccepted != null ||
            !responseAction.isNullOrEmpty() ||
            !responseText.isNullOrEmpty()

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
        fun fromJson(obj: JSONObject): HistoryRecord {
            val req = obj.getJSONObject("Request")
            val resp = if (obj.has("Response") &&
                !obj.isNull("Response")
            ) obj.getJSONObject("Response") else null

            return HistoryRecord(
                id = req.getString("id"),
                flowId = req.getString("flow_id"),
                workspaceId = req.getString("workspace_id"),
                title = req.getString("title"),
                body = req.optString("body", "")
                    .ifEmpty { null },
                priority = req.optString("priority", "normal"),
                source = req.optString("source", ""),
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
