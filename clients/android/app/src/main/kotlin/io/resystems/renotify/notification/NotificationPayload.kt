package io.resystems.renotify.notification

import org.json.JSONArray
import org.json.JSONObject

/**
 * Kotlin representation of the Go `payload.NotificationRequest`
 * wire-format type. Parsed from JSON delivered via the JetStream
 * durable consumer.
 *
 * See docs/analysis-payload-schemas.md (NotificationRequest).
 */
data class NotificationPayload(
    val id: String,
    val flowId: String,
    val daemonId: String,
    val workspaceId: String,
    val title: String,
    val body: String?,
    val responseTypes: List<String>,
    val priority: String,
    val source: String,
    val workspaceName: String,
    val actions: List<String>?,
    val timeoutSec: Int?,
    val timestamp: String
) {
    /** True when no human response is expected. */
    val isFireAndForget: Boolean
        get() = responseTypes == listOf(RESPONSE_NONE)

    /** True when the notification requires a human decision. */
    val isInteractive: Boolean
        get() = !isFireAndForget

    companion object {
        const val RESPONSE_NONE = "none"
        const val RESPONSE_BOOLEAN = "boolean"
        const val RESPONSE_CHOICE = "choice"
        const val RESPONSE_TEXT = "text"

        const val PRIORITY_LOW = "low"
        const val PRIORITY_NORMAL = "normal"
        const val PRIORITY_HIGH = "high"

        /**
         * Parse a [NotificationPayload] from a JSON string.
         *
         * @throws IllegalArgumentException if required fields are
         *         missing or invalid.
         */
        fun fromJson(json: String): NotificationPayload {
            val obj = JSONObject(json)

            require(obj.has("id")) { "id is required" }
            val id = obj.getString("id")
            require(id.isNotEmpty()) { "id must not be empty" }

            require(obj.has("title")) { "title is required" }
            val title = obj.getString("title")
            require(title.isNotEmpty()) { "title must not be empty" }

            val rtArray = obj.getJSONArray("response_types")
            require(rtArray.length() > 0) {
                "response_types must not be empty"
            }
            val responseTypes = (0 until rtArray.length()).map {
                rtArray.getString(it)
            }

            val actions = if (obj.has("actions") &&
                !obj.isNull("actions")
            ) {
                val arr = obj.getJSONArray("actions")
                (0 until arr.length()).map { arr.getString(it) }
            } else {
                null
            }

            return NotificationPayload(
                id = id,
                flowId = obj.getString("flow_id"),
                daemonId = obj.getString("daemon_id"),
                workspaceId = obj.getString("workspace_id"),
                title = title,
                body = if (obj.has("body")) obj.getString("body")
                    else null,
                responseTypes = responseTypes,
                priority = obj.optString("priority", PRIORITY_NORMAL),
                source = obj.optString("source", ""),
                workspaceName = obj.optString("workspace_name", ""),
                actions = actions,
                timeoutSec = if (obj.has("timeout_sec"))
                    obj.getInt("timeout_sec") else null,
                timestamp = obj.getString("timestamp")
            )
        }
    }
}
