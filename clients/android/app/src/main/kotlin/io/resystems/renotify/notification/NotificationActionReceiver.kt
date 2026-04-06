// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.notification

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import androidx.core.app.RemoteInput
import io.resystems.renotify.MainActivity
import io.resystems.renotify.nats.NatsService

/**
 * BroadcastReceiver that handles notification action button
 * taps. Extracts the user's response from the intent extras
 * and delegates publishing to [NatsService] via startService.
 *
 * See M-03 [NotificationRenderer] for how the PendingIntents
 * are constructed and M-04 plan for the design rationale.
 */
class NotificationActionReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != NotificationRenderer.INTENT_ACTION) {
            return
        }

        val notificationId = intent.getStringExtra(
            NotificationRenderer.EXTRA_NOTIFICATION_ID) ?: return
        val flowId = intent.getStringExtra(
            NotificationRenderer.EXTRA_FLOW_ID) ?: return
        val actionType = intent.getStringExtra(
            NotificationRenderer.EXTRA_ACTION_TYPE) ?: return
        val actionValue = intent.getStringExtra(
            NotificationRenderer.EXTRA_ACTION_VALUE) ?: ""

        Log.i(TAG, "Action received: type=$actionType " +
            "value=$actionValue notification=$notificationId")

        // "More..." overflow: open the app instead of publishing.
        if (actionType == NotificationRenderer.ACTION_TYPE_MORE) {
            // Dismiss the notification so the user sees the app
            // when it comes to the foreground.
            NotificationRenderer.dismiss(context, notificationId)

            val appIntent = Intent(context, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or
                    Intent.FLAG_ACTIVITY_SINGLE_TOP)
                .putExtra(NotificationRenderer.EXTRA_NOTIFICATION_ID,
                    notificationId)
                .putExtra(NotificationRenderer.EXTRA_FLOW_ID, flowId)
            // Forward the full actions list and response types
            // for in-app rendering.
            val actions = intent.getStringArrayExtra(
                NotificationRenderer.EXTRA_ACTIONS_ARRAY)
            if (actions != null) {
                appIntent.putExtra(
                    NotificationRenderer.EXTRA_ACTIONS_ARRAY,
                    actions)
            }
            val responseTypes = intent.getStringArrayExtra(
                NotificationRenderer.EXTRA_RESPONSE_TYPES)
            if (responseTypes != null) {
                appIntent.putExtra(
                    NotificationRenderer.EXTRA_RESPONSE_TYPES,
                    responseTypes)
            }
            context.startActivity(appIntent)
            return
        }

        // Extract RemoteInput text for text responses.
        var text: String? = null
        if (actionType == NotificationRenderer.ACTION_TYPE_TEXT) {
            val results = RemoteInput.getResultsFromIntent(intent)
            text = results?.getCharSequence(
                NotificationRenderer.EXTRA_REMOTE_INPUT_KEY
            )?.toString()
        }

        // Delegate publishing to NatsService.
        val serviceIntent = Intent(context, NatsService::class.java)
            .setAction(NatsService.ACTION_PUBLISH_RESPONSE)
            .putExtra(NatsService.EXTRA_NOTIFICATION_ID,
                notificationId)
            .putExtra(NatsService.EXTRA_FLOW_ID, flowId)
            .putExtra(NatsService.EXTRA_ACTION_TYPE, actionType)
            .putExtra(NatsService.EXTRA_ACTION_VALUE, actionValue)

        if (text != null) {
            serviceIntent.putExtra(NatsService.EXTRA_TEXT, text)
        }

        context.startService(serviceIntent)
    }

    companion object {
        private const val TAG = "ActionReceiver"
    }
}
