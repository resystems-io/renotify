package io.resystems.renotify.notification

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.RemoteInput

/**
 * Builds and posts Android system notifications from
 * [NotificationPayload] data. Differentiates between
 * informational (fire-and-forget) and interactive (blocking)
 * notifications per R-MOB-03.
 *
 * Action buttons are rendered here but the [BroadcastReceiver]
 * that handles taps is implemented in M-04. Until then, tapping
 * a button does nothing.
 */
object NotificationRenderer {

    private const val TAG = "NotificationRenderer"

    /** Notification group key for stacking. */
    private const val GROUP_KEY = "io.resystems.renotify.FLOW"

    // Intent extras carried by action PendingIntents.
    const val EXTRA_NOTIFICATION_ID = "notification_id"
    const val EXTRA_FLOW_ID = "flow_id"
    const val EXTRA_ACTION_TYPE = "action_type"
    const val EXTRA_ACTION_VALUE = "action_value"
    const val EXTRA_REMOTE_INPUT_KEY = "remote_input_text"

    // Extras for carrying notification data to the app.
    const val EXTRA_ACTIONS_ARRAY = "actions_array"
    const val EXTRA_RESPONSE_TYPES = "response_types"

    // Action types for PendingIntent discrimination.
    const val ACTION_TYPE_ACCEPTED = "accepted"
    const val ACTION_TYPE_REJECTED = "rejected"
    const val ACTION_TYPE_CHOICE = "choice"
    const val ACTION_TYPE_TEXT = "text"

    // Intent action for the BroadcastReceiver (M-04).
    const val INTENT_ACTION =
        "io.resystems.renotify.ACTION_RESPOND"

    /**
     * Render and post a notification. Returns the Android
     * notification ID (for later dismissal).
     */
    fun render(
        context: Context,
        payload: NotificationPayload
    ): Int {
        val androidId = androidNotificationId(payload.id)
        val channelId = selectChannel(payload)

        val builder = NotificationCompat.Builder(context, channelId)
            .setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentTitle(payload.title)
            .setAutoCancel(payload.isFireAndForget)
            .setOngoing(payload.isInteractive)
            .setGroup(GROUP_KEY)
            .setWhen(System.currentTimeMillis())
            .setShowWhen(true)

        // Body text.
        if (!payload.body.isNullOrEmpty()) {
            builder.setContentText(payload.body)
            builder.setStyle(
                NotificationCompat.BigTextStyle()
                    .bigText(payload.body)
            )
        }

        // Sub-text: workspace name and/or source identifier.
        val subText = composeSubText(
            payload.workspaceName, payload.source)
        if (subText.isNotEmpty()) {
            builder.setSubText(subText)
        }

        // Priority mapping.
        when (payload.priority) {
            NotificationPayload.PRIORITY_HIGH -> {
                builder.priority = NotificationCompat.PRIORITY_HIGH
            }
            NotificationPayload.PRIORITY_LOW -> {
                builder.priority = NotificationCompat.PRIORITY_LOW
                builder.setSilent(true)
            }
            else -> {
                builder.priority = NotificationCompat.PRIORITY_DEFAULT
            }
        }

        // Add action buttons based on response types.
        addActions(context, builder, payload, androidId)

        val nm = context.getSystemService(
            NotificationManager::class.java)
        nm.notify(androidId, builder.build())

        Log.i(TAG, "Posted notification ${payload.id} " +
            "(android_id=$androidId, interactive=${payload.isInteractive})")

        return androidId
    }

    /**
     * Dismiss a notification by its payload ID. Called when a
     * flow lifecycle event with status completed/failed arrives.
     */
    fun dismiss(context: Context, notificationId: String) {
        val androidId = androidNotificationId(notificationId)
        val nm = context.getSystemService(
            NotificationManager::class.java)
        nm.cancel(androidId)
        Log.i(TAG, "Dismissed notification $notificationId")
    }

    /**
     * Create notification channels for incoming notifications.
     * Call from [NatsService.onCreate].
     */
    fun createChannels(context: Context) {
        val nm = context.getSystemService(
            NotificationManager::class.java)

        nm.createNotificationChannel(
            NotificationChannel(
                CHANNEL_URGENT,
                "Urgent Notifications",
                NotificationManager.IMPORTANCE_HIGH
            ).apply {
                description =
                    "High-priority and interactive notifications"
            }
        )

        nm.createNotificationChannel(
            NotificationChannel(
                CHANNEL_NOTIFICATIONS,
                "Notifications",
                NotificationManager.IMPORTANCE_DEFAULT
            ).apply {
                description =
                    "Normal and low-priority notifications"
            }
        )
    }

    // --- Channel IDs ---

    const val CHANNEL_URGENT = "renotify_urgent"
    const val CHANNEL_NOTIFICATIONS = "renotify_notifications"

    /**
     * Select the notification channel based on priority and
     * interactivity.
     */
    internal fun selectChannel(
        payload: NotificationPayload
    ): String {
        // Interactive notifications or high priority → urgent
        // channel (heads-up).
        if (payload.isInteractive ||
            payload.priority == NotificationPayload.PRIORITY_HIGH
        ) {
            return CHANNEL_URGENT
        }
        return CHANNEL_NOTIFICATIONS
    }

    /**
     * Deterministic Android notification ID from the payload's
     * string ID. Same notification always maps to the same
     * integer, enabling updates and dedup on reconnect.
     */
    internal fun androidNotificationId(id: String): Int {
        // Avoid collision with the foreground service
        // notification (ID = 1). Use absolute value to ensure
        // positive, and add an offset.
        return (id.hashCode() and Int.MAX_VALUE) or 0x1000
    }

    /**
     * Compose notification sub-text from workspace name and
     * source. Returns "{workspace} · {source}" when both are
     * present, or whichever is non-empty.
     */
    internal fun composeSubText(
        workspace: String,
        source: String
    ): String {
        return when {
            workspace.isNotEmpty() && source.isNotEmpty() ->
                "$workspace · $source"
            workspace.isNotEmpty() -> workspace
            else -> source
        }
    }

    // --- Action button builders ---

    /** Android enforces a maximum of 3 action buttons. */
    private const val MAX_NOTIFICATION_ACTIONS = 3

    /** Action type for the "More..." overflow button. */
    const val ACTION_TYPE_MORE = "more"

    private fun addActions(
        context: Context,
        builder: NotificationCompat.Builder,
        payload: NotificationPayload,
        androidId: Int
    ) {
        val types = payload.responseTypes
        if (types.contains(NotificationPayload.RESPONSE_NONE) &&
            types.size == 1
        ) {
            return // fire-and-forget — no buttons
        }

        // Collect all candidate actions, then enforce the
        // Android notification button limit. RemoteInput (text
        // reply) doesn't expand reliably when sharing a
        // notification with other action buttons, so when text
        // is combined with other types we overflow the text
        // action to the in-app "More..." dialog.
        val isMultiModal = types.size > 1
        val actions = mutableListOf<NotificationCompat.Action>()

        if (types.contains(NotificationPayload.RESPONSE_BOOLEAN)) {
            actions += buildBooleanActions(
                context, payload, androidId)
        }

        if (types.contains(NotificationPayload.RESPONSE_CHOICE)) {
            actions += buildChoiceActions(
                context, payload, androidId)
        }

        // Only add inline text reply when it's the sole
        // response type. Multi-modal text is handled via the
        // "More..." in-app dialog where it works reliably.
        if (types.contains(NotificationPayload.RESPONSE_TEXT) &&
            !isMultiModal
        ) {
            actions += buildTextAction(
                context, payload, androidId)
        }

        // For multi-modal with text, always show "More..." so
        // the user can access the text input in-app.
        val forceMore = isMultiModal &&
            types.contains(NotificationPayload.RESPONSE_TEXT)

        if (!forceMore &&
            actions.size <= MAX_NOTIFICATION_ACTIONS
        ) {
            actions.forEach { builder.addAction(it) }
        } else {
            // Show first 2, then "More...".
            val take = (MAX_NOTIFICATION_ACTIONS - 1)
                .coerceAtMost(actions.size)
            actions.take(take)
                .forEach { builder.addAction(it) }
            builder.addAction(buildMoreAction(
                context, payload, androidId))
        }
    }

    private fun buildBooleanActions(
        context: Context,
        payload: NotificationPayload,
        androidId: Int
    ): List<NotificationCompat.Action> {
        // Use custom labels from actions if exactly 2 provided,
        // otherwise default to Yes/No.
        val yesLabel = payload.actions?.getOrNull(0) ?: "Yes"
        val noLabel = payload.actions?.getOrNull(1) ?: "No"

        return listOf(
            NotificationCompat.Action.Builder(
                0, yesLabel,
                actionIntent(context, androidId, payload,
                    ACTION_TYPE_ACCEPTED, "true")
            ).build(),
            NotificationCompat.Action.Builder(
                0, noLabel,
                actionIntent(context, androidId, payload,
                    ACTION_TYPE_REJECTED, "false")
            ).build()
        )
    }

    private fun buildChoiceActions(
        context: Context,
        payload: NotificationPayload,
        androidId: Int
    ): List<NotificationCompat.Action> {
        return payload.actions?.map { label ->
            NotificationCompat.Action.Builder(
                0, label,
                actionIntent(context, androidId, payload,
                    ACTION_TYPE_CHOICE, label)
            ).build()
        } ?: emptyList()
    }

    private fun buildTextAction(
        context: Context,
        payload: NotificationPayload,
        androidId: Int
    ): List<NotificationCompat.Action> {
        val remoteInput = RemoteInput.Builder(
            EXTRA_REMOTE_INPUT_KEY)
            .setLabel("Enter response")
            .build()

        return listOf(
            NotificationCompat.Action.Builder(
                0, "Reply",
                actionIntent(context, androidId, payload,
                    ACTION_TYPE_TEXT, "")
            )
                .addRemoteInput(remoteInput)
                .build()
        )
    }

    /**
     * Build a "More..." action that opens the app to show the
     * full set of choices. The intent carries the notification
     * metadata so the app can render the complete choice list.
     */
    private fun buildMoreAction(
        context: Context,
        payload: NotificationPayload,
        androidId: Int
    ): NotificationCompat.Action {
        return NotificationCompat.Action.Builder(
            0, "More\u2026",
            moreActionIntent(context, androidId, payload)
        ).build()
    }

    /**
     * Build a PendingIntent for the "More..." overflow that
     * includes the full actions list for in-app rendering.
     */
    private fun moreActionIntent(
        context: Context,
        androidId: Int,
        payload: NotificationPayload
    ): PendingIntent {
        val intent = Intent(
            context,
            NotificationActionReceiver::class.java
        ).apply {
            action = INTENT_ACTION
            putExtra(EXTRA_NOTIFICATION_ID, payload.id)
            putExtra(EXTRA_FLOW_ID, payload.flowId)
            putExtra(EXTRA_ACTION_TYPE, ACTION_TYPE_MORE)
            putExtra(EXTRA_ACTION_VALUE, "")
            payload.actions?.let {
                putExtra(EXTRA_ACTIONS_ARRAY,
                    it.toTypedArray())
            }
            putExtra(EXTRA_RESPONSE_TYPES,
                payload.responseTypes.toTypedArray())
        }

        val requestCode =
            (payload.id + ACTION_TYPE_MORE).hashCode()

        return PendingIntent.getBroadcast(
            context, requestCode, intent,
            PendingIntent.FLAG_UPDATE_CURRENT or
                PendingIntent.FLAG_MUTABLE
        )
    }

    /**
     * Build a PendingIntent for an action button. The intent
     * carries all metadata needed by the M-04 BroadcastReceiver.
     *
     * Text actions (RemoteInput) require FLAG_MUTABLE so the
     * system can attach the user's typed text to the intent.
     * All other actions use FLAG_IMMUTABLE per best practice.
     */
    private fun actionIntent(
        context: Context,
        androidId: Int,
        payload: NotificationPayload,
        actionType: String,
        actionValue: String
    ): PendingIntent {
        val intent = Intent(context, NotificationActionReceiver::class.java).apply {
            action = INTENT_ACTION
            putExtra(EXTRA_NOTIFICATION_ID, payload.id)
            putExtra(EXTRA_FLOW_ID, payload.flowId)
            putExtra(EXTRA_ACTION_TYPE, actionType)
            putExtra(EXTRA_ACTION_VALUE, actionValue)
        }

        // Use a unique request code per action to prevent
        // PendingIntent reuse across buttons.
        val requestCode =
            (payload.id + actionType + actionValue).hashCode()

        // RemoteInput (text reply) requires FLAG_MUTABLE on
        // Android 12+ (API 31+) so the system can write the
        // user's input into the intent extras.
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or
            if (actionType == ACTION_TYPE_TEXT)
                PendingIntent.FLAG_MUTABLE
            else
                PendingIntent.FLAG_IMMUTABLE

        return PendingIntent.getBroadcast(
            context, requestCode, intent, flags
        )
    }
}
