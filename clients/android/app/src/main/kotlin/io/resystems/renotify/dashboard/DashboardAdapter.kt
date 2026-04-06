package io.resystems.renotify.dashboard

import android.app.AlertDialog
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import io.resystems.renotify.Brand
import io.resystems.renotify.notification.NotificationRenderer

/**
 * RecyclerView adapter for the dashboard. Renders a flat list
 * of [DashboardItem] entries: workspace headers, flow rows
 * (expandable with Stop/Note actions and notification sub-list),
 * and an empty-state placeholder.
 *
 * Only one flow may be expanded at a time (accordion). Expanding
 * a flow triggers [onFlowExpanded] to fetch its notifications.
 *
 * Uses programmatic views (no XML layouts) to match the existing
 * codebase pattern established in M-01.
 */
class DashboardAdapter :
    RecyclerView.Adapter<DashboardAdapter.ViewHolder>() {

    private var items: List<DashboardItem> =
        DashboardItem.fromHeartbeat(null)

    /** Flow ID of the single expanded flow, or null. */
    private var expandedFlowId: String? = null

    /** Cached flow-scoped notification results. */
    private val flowNotifications =
        mutableMapOf<String, HistoryQueryResult>()

    /** Cached per-flow counts: total and open questions. */
    private val flowCounts =
        mutableMapOf<String, Pair<Int, Int>>()

    /** ID of the notification sub-item currently expanded. */
    private var expandedNotificationId: String? = null

    /**
     * Callback for flow actions. Parameters: flowId, action
     * ("stop" or "note"), optional context message.
     */
    var onFlowAction: ((
        flowId: String, action: String, context: String?
    ) -> Unit)? = null

    /**
     * Callback when a flow is expanded. The receiver should
     * query flow-scoped history and call
     * [updateFlowNotifications].
     */
    var onFlowExpanded: ((flowId: String) -> Unit)? = null

    /**
     * Callback to submit a response to an open notification.
     * Parameters match [NatsService.ACTION_PUBLISH_RESPONSE].
     */
    var onNotificationResponse: ((
        notificationId: String,
        flowId: String,
        actionType: String,
        actionValue: String,
        text: String?
    ) -> Unit)? = null

    /** Update the adapter with a new heartbeat snapshot. */
    fun update(heartbeat: DaemonHeartbeat?) {
        items = DashboardItem.fromHeartbeat(heartbeat)
        // Prune cached data for flows no longer present.
        val activeIds = items.filterIsInstance<DashboardItem.FlowItem>()
            .map { it.flowId }.toSet()
        flowNotifications.keys.retainAll(activeIds)
        flowCounts.keys.retainAll(activeIds)
        if (expandedFlowId != null &&
            expandedFlowId !in activeIds
        ) {
            expandedFlowId = null
            expandedNotificationId = null
        }
        notifyDataSetChanged()
        // Re-query the expanded flow so new notifications appear
        // without requiring collapse/re-expand.
        if (expandedFlowId != null) {
            onFlowExpanded?.invoke(expandedFlowId!!)
        }
    }

    /**
     * Programmatically expand a specific flow (accordion).
     * Triggers [onFlowExpanded] to fetch notifications.
     * No-op if the flow is not in the current item list.
     */
    fun expandFlow(flowId: String) {
        val pos = items.indexOfFirst {
            it is DashboardItem.FlowItem && it.flowId == flowId
        }
        if (pos < 0) return

        val prev = expandedFlowId
        expandedFlowId = flowId
        expandedNotificationId = null
        onFlowExpanded?.invoke(flowId)

        if (prev != null && prev != flowId) {
            val prevPos = items.indexOfFirst {
                it is DashboardItem.FlowItem && it.flowId == prev
            }
            if (prevPos >= 0) notifyItemChanged(prevPos)
        }
        notifyItemChanged(pos)
    }

    /**
     * Receive flow-scoped notification results from a
     * svc.history query. Updates badges and, if the flow is
     * currently expanded, renders the notification sub-list.
     */
    fun updateFlowNotifications(
        flowId: String,
        result: HistoryQueryResult
    ) {
        flowNotifications[flowId] = result
        flowCounts[flowId] = Pair(
            result.total,
            result.records.count { it.isOpen }
        )
        val pos = items.indexOfFirst {
            it is DashboardItem.FlowItem && it.flowId == flowId
        }
        if (pos >= 0) notifyItemChanged(pos)
    }

    override fun getItemCount(): Int = items.size

    override fun getItemViewType(position: Int): Int {
        return when (items[position]) {
            is DashboardItem.WorkspaceHeader -> VIEW_WORKSPACE
            is DashboardItem.FlowItem -> VIEW_FLOW
            is DashboardItem.EmptyState -> VIEW_EMPTY
        }
    }

    override fun onCreateViewHolder(
        parent: ViewGroup,
        viewType: Int
    ): ViewHolder {
        return when (viewType) {
            VIEW_WORKSPACE -> ViewHolder(
                createWorkspaceView(parent))
            VIEW_FLOW -> ViewHolder(
                createFlowView(parent))
            else -> ViewHolder(
                createEmptyView(parent))
        }
    }

    override fun onBindViewHolder(
        holder: ViewHolder,
        position: Int
    ) {
        when (val item = items[position]) {
            is DashboardItem.WorkspaceHeader ->
                bindWorkspace(holder, item)
            is DashboardItem.FlowItem ->
                bindFlow(holder, item)
            is DashboardItem.EmptyState ->
                bindEmpty(holder, item)
        }
    }

    // --- View creation (programmatic, no XML) ---

    private fun createWorkspaceView(
        parent: ViewGroup
    ): LinearLayout {
        return LinearLayout(parent.context).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(16), dp(12), dp(16), dp(4))
            layoutParams = RecyclerView.LayoutParams(
                RecyclerView.LayoutParams.MATCH_PARENT,
                RecyclerView.LayoutParams.WRAP_CONTENT
            )

            addView(TextView(context).apply {
                tag = TAG_WS_NAME
                textSize = 16f
                setTypeface(null, Typeface.BOLD)
            })

            addView(TextView(context).apply {
                tag = TAG_WS_DETAIL
                textSize = 12f
                setTextColor(Brand.TEXT_SECONDARY)
            })
        }
    }

    private fun createFlowView(
        parent: ViewGroup
    ): LinearLayout {
        return LinearLayout(parent.context).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(32), dp(6), dp(16), dp(6))
            layoutParams = RecyclerView.LayoutParams(
                RecyclerView.LayoutParams.MATCH_PARENT,
                RecyclerView.LayoutParams.WRAP_CONTENT
            )

            // Summary row (always visible).
            addView(LinearLayout(context).apply {
                tag = TAG_FLOW_SUMMARY
                orientation = LinearLayout.HORIZONTAL

                // Status dot.
                addView(View(context).apply {
                    tag = TAG_FLOW_DOT
                    setBackgroundColor(Brand.STATUS_ACTIVE)
                    layoutParams = LinearLayout.LayoutParams(
                        dp(8), dp(8)
                    ).apply {
                        marginEnd = dp(8)
                        topMargin = dp(6)
                    }
                })

                // Label + metadata column.
                addView(LinearLayout(context).apply {
                    orientation = LinearLayout.VERTICAL
                    layoutParams = LinearLayout.LayoutParams(
                        0,
                        LinearLayout.LayoutParams.WRAP_CONTENT,
                        1f
                    )

                    addView(TextView(context).apply {
                        tag = TAG_FLOW_LABEL
                        textSize = 13f
                    })

                    addView(TextView(context).apply {
                        tag = TAG_FLOW_META
                        textSize = 11f
                        setTextColor(Brand.TEXT_SECONDARY)
                    })
                })

                // Badges column (right-aligned).
                addView(LinearLayout(context).apply {
                    tag = TAG_FLOW_BADGES
                    orientation = LinearLayout.VERTICAL
                    gravity = Gravity.END or Gravity.CENTER_VERTICAL

                    addView(TextView(context).apply {
                        tag = TAG_BADGE_TOTAL
                        textSize = 11f
                        setTextColor(Brand.TEXT_SECONDARY)
                        visibility = View.GONE
                    })

                    addView(TextView(context).apply {
                        tag = TAG_BADGE_OPEN
                        textSize = 11f
                        setTextColor(Brand.PRIMARY)
                        setTypeface(null, Typeface.BOLD)
                        visibility = View.GONE
                    })
                })
            })

            // Expandable action row (initially hidden).
            addView(LinearLayout(context).apply {
                tag = TAG_FLOW_ACTIONS
                orientation = LinearLayout.HORIZONTAL
                gravity = Gravity.END
                setPadding(0, dp(6), 0, dp(4))
                visibility = View.GONE

                addView(Button(context).apply {
                    tag = TAG_BTN_STOP
                    text = "Stop"
                    textSize = 12f
                    background = GradientDrawable().apply {
                        setColor(Brand.ACTION_STOP)
                        cornerRadius = dp(6).toFloat()
                    }
                    setTextColor(Brand.BUTTON_TEXT)
                    setPadding(dp(12), dp(6), dp(12), dp(6))
                    minWidth = 0
                    minimumWidth = 0
                })

                addView(Button(context).apply {
                    tag = TAG_BTN_NOTE
                    text = "Note"
                    textSize = 12f
                    background = GradientDrawable().apply {
                        setColor(Brand.BUTTON_BG)
                        cornerRadius = dp(6).toFloat()
                    }
                    setTextColor(Brand.BUTTON_TEXT)
                    setPadding(dp(12), dp(6), dp(12), dp(6))
                    minWidth = 0
                    minimumWidth = 0
                    val lp = LinearLayout.LayoutParams(
                        LinearLayout.LayoutParams.WRAP_CONTENT,
                        LinearLayout.LayoutParams.WRAP_CONTENT
                    )
                    lp.marginStart = dp(8)
                    layoutParams = lp
                })
            })

            // Notification sub-list container (initially hidden).
            addView(LinearLayout(context).apply {
                tag = TAG_FLOW_NOTIFICATIONS
                orientation = LinearLayout.VERTICAL
                visibility = View.GONE
            })
        }
    }

    private fun createEmptyView(
        parent: ViewGroup
    ): TextView {
        return TextView(parent.context).apply {
            tag = TAG_EMPTY
            textSize = 14f
            setTextColor(Brand.TEXT_SECONDARY)
            gravity = Gravity.CENTER
            setPadding(dp(16), dp(48), dp(16), dp(48))
            layoutParams = RecyclerView.LayoutParams(
                RecyclerView.LayoutParams.MATCH_PARENT,
                RecyclerView.LayoutParams.WRAP_CONTENT
            )
        }
    }

    // --- View binding ---

    private fun bindWorkspace(
        holder: ViewHolder,
        item: DashboardItem.WorkspaceHeader
    ) {
        val name = holder.itemView.findViewWithTag<TextView>(
            TAG_WS_NAME)
        val detail = holder.itemView.findViewWithTag<TextView>(
            TAG_WS_DETAIL)

        val label = item.displayName.ifEmpty { item.workspaceId }
        val count = item.flowCount
        val suffix = if (count == 1) "flow" else "flows"
        name.text = "$label  \u2014  $count $suffix"

        detail.text = item.absPath.ifEmpty { item.workspaceId }
    }

    private fun bindFlow(
        holder: ViewHolder,
        item: DashboardItem.FlowItem
    ) {
        val labelText = holder.itemView.findViewWithTag<TextView>(
            TAG_FLOW_LABEL)
        val metaText = holder.itemView.findViewWithTag<TextView>(
            TAG_FLOW_META)
        val actionsRow = holder.itemView.findViewWithTag<View>(
            TAG_FLOW_ACTIONS)
        val stopBtn = holder.itemView.findViewWithTag<Button>(
            TAG_BTN_STOP)
        val noteBtn = holder.itemView.findViewWithTag<Button>(
            TAG_BTN_NOTE)
        val badgeTotal =
            holder.itemView.findViewWithTag<TextView>(
                TAG_BADGE_TOTAL)
        val badgeOpen =
            holder.itemView.findViewWithTag<TextView>(
                TAG_BADGE_OPEN)
        val notifContainer =
            holder.itemView.findViewWithTag<LinearLayout>(
                TAG_FLOW_NOTIFICATIONS)

        // Show label if present, otherwise flow ID.
        labelText.text = item.label.ifEmpty { item.flowId }

        // Show metadata as "key: value" lines, flow ID, and
        // last activity timestamp.
        val parts = mutableListOf<String>()
        if (item.label.isNotEmpty()) {
            parts.add(item.flowId)
        }
        if (item.lastActivity.isNotEmpty()) {
            val ttlStr = formatTTL(
                item.lastActivity, item.gracePeriodMs)
            if (ttlStr != null) {
                parts.add(
                    "updated: ${formatTimestamp(item.lastActivity)}" +
                    "  TTL: $ttlStr")
            } else {
                parts.add(
                    "updated: ${formatTimestamp(item.lastActivity)}")
            }
        }
        for ((k, v) in item.metadata.toSortedMap()) {
            parts.add("$k: $v")
        }
        if (parts.isNotEmpty()) {
            metaText.text = parts.joinToString("\n")
            metaText.visibility = View.VISIBLE
        } else {
            metaText.visibility = View.GONE
        }

        // Badges.
        val counts = flowCounts[item.flowId]
        if (counts != null && counts.first > 0) {
            badgeTotal.text = "${counts.first} notif"
            badgeTotal.visibility = View.VISIBLE
        } else {
            badgeTotal.visibility = View.GONE
        }
        if (counts != null && counts.second > 0) {
            badgeOpen.text = "${counts.second} open"
            badgeOpen.visibility = View.VISIBLE
        } else {
            badgeOpen.visibility = View.GONE
        }

        // Expand/collapse on tap (accordion — one at a time).
        val expanded = item.flowId == expandedFlowId
        actionsRow.visibility =
            if (expanded) View.VISIBLE else View.GONE

        // Notification sub-list.
        if (expanded) {
            val result = flowNotifications[item.flowId]
            if (result != null && result.records.isNotEmpty()) {
                populateNotifications(
                    notifContainer, item.flowId, result)
                notifContainer.visibility = View.VISIBLE
            } else {
                notifContainer.removeAllViews()
                notifContainer.visibility = View.GONE
            }
        } else {
            notifContainer.removeAllViews()
            notifContainer.visibility = View.GONE
        }

        holder.itemView.setOnClickListener {
            val prev = expandedFlowId
            val tapped = item.flowId

            if (prev == tapped) {
                // Collapse current.
                expandedFlowId = null
                expandedNotificationId = null
            } else {
                // Expand new, collapse previous.
                expandedFlowId = tapped
                expandedNotificationId = null
                onFlowExpanded?.invoke(tapped)
            }

            // Refresh old position.
            if (prev != null && prev != tapped) {
                val prevPos = items.indexOfFirst {
                    it is DashboardItem.FlowItem &&
                        it.flowId == prev
                }
                if (prevPos >= 0) notifyItemChanged(prevPos)
            }
            notifyItemChanged(
                holder.bindingAdapterPosition)
        }

        // Action buttons.
        stopBtn.setOnClickListener {
            onFlowAction?.invoke(item.flowId, "stop", null)
            expandedFlowId = null
            expandedNotificationId = null
            notifyItemChanged(holder.bindingAdapterPosition)
        }

        noteBtn.setOnClickListener {
            val context = holder.itemView.context
            val editText = EditText(context).apply {
                hint = "Message (optional)"
                setPadding(dp(16), dp(12), dp(16), dp(12))
            }
            AlertDialog.Builder(context)
                .setTitle("Send Note to Flow")
                .setView(editText)
                .setPositiveButton("Send") { _, _ ->
                    val msg = editText.text.toString()
                    onFlowAction?.invoke(
                        item.flowId, "note",
                        msg.ifEmpty { null })
                    expandedFlowId = null
                    expandedNotificationId = null
                    notifyItemChanged(
                        holder.bindingAdapterPosition)
                }
                .setNegativeButton("Cancel", null)
                .show()
        }
    }

    private fun bindEmpty(
        holder: ViewHolder,
        item: DashboardItem.EmptyState
    ) {
        val text = holder.itemView as TextView
        text.text = item.message
    }

    // --- Notification sub-list ---

    /**
     * Populate the notification container with sub-items for the
     * given flow's history results. Each sub-item is tappable to
     * reveal body text and response controls for open questions.
     */
    private fun populateNotifications(
        container: LinearLayout,
        flowId: String,
        result: HistoryQueryResult
    ) {
        container.removeAllViews()
        val ctx = container.context

        // Summary header.
        val openCount = result.records.count { it.isOpen }
        val headerText = buildString {
            append("${result.total} notification")
            if (result.total != 1) append("s")
            if (openCount > 0) append(" \u00b7 $openCount open")
        }
        container.addView(TextView(ctx).apply {
            text = headerText
            textSize = 11f
            setTextColor(Brand.TEXT_SECONDARY)
            setPadding(0, dp(4), 0, dp(4))
        })

        // Divider.
        container.addView(View(ctx).apply {
            setBackgroundColor(Brand.DIVIDER)
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, 1)
        })

        // Sub-items.
        for (rec in result.records) {
            container.addView(
                createNotificationSubItem(ctx, flowId, rec))
        }

        // "View all in history" link if truncated.
        if (result.records.size < result.total) {
            container.addView(TextView(ctx).apply {
                text = "View all in history\u2026"
                textSize = 11f
                setTextColor(Brand.LINK)
                setPadding(0, dp(6), 0, dp(2))
            })
        }
    }

    /**
     * Build a single notification sub-item view. Contains the
     * title, detail line, and (when expanded) body text and
     * response controls for open questions.
     */
    private fun createNotificationSubItem(
        ctx: android.content.Context,
        flowId: String,
        rec: HistoryRecord
    ): LinearLayout {
        val root = LinearLayout(ctx).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(0, dp(6), 0, dp(6))
        }

        // Title + status.
        val titleRow = LinearLayout(ctx).apply {
            orientation = LinearLayout.HORIZONTAL
        }
        titleRow.addView(TextView(ctx).apply {
            text = rec.title
            textSize = 12f
            maxLines = 1
            layoutParams = LinearLayout.LayoutParams(
                0,
                LinearLayout.LayoutParams.WRAP_CONTENT,
                1f
            )
        })
        if (rec.isOpen) {
            titleRow.addView(TextView(ctx).apply {
                text = "open"
                textSize = 10f
                setTextColor(Brand.PRIMARY)
                setTypeface(null, Typeface.BOLD)
                setPadding(dp(6), 0, 0, 0)
            })
        } else if (rec.hasResponse) {
            titleRow.addView(TextView(ctx).apply {
                text = rec.responseSummary
                textSize = 10f
                setTextColor(Brand.TEXT_SECONDARY)
                setPadding(dp(6), 0, 0, 0)
            })
        }
        root.addView(titleRow)

        // Detail line: timestamp + priority + source.
        val ts = formatTimestamp(rec.timestamp)
        val detailParts = mutableListOf(ts)
        if (rec.priority != "normal") detailParts.add(rec.priority)
        if (rec.source.isNotEmpty()) detailParts.add(rec.source)
        root.addView(TextView(ctx).apply {
            text = detailParts.joinToString(" \u00b7 ")
            textSize = 10f
            setTextColor(Brand.TEXT_SECONDARY)
        })

        // Expandable body (hidden by default).
        val bodyView = TextView(ctx).apply {
            tag = "notif_body_${rec.id}"
            textSize = 11f
            setTextColor(Brand.TEXT_DARK)
            setPadding(0, dp(4), 0, 0)
            visibility = View.GONE
        }
        if (!rec.body.isNullOrEmpty()) {
            bodyView.text = rec.body
        }
        root.addView(bodyView)

        // Expandable response controls container (hidden).
        // Uses vertical orientation; buttons are laid out in
        // horizontal rows of 3.
        val responseContainer = LinearLayout(ctx).apply {
            tag = "notif_resp_${rec.id}"
            orientation = LinearLayout.VERTICAL
            setPadding(0, dp(4), 0, 0)
            visibility = View.GONE
        }
        root.addView(responseContainer)

        // Bind expansion state.
        val isExpanded = rec.id == expandedNotificationId
        if (isExpanded) {
            if (!rec.body.isNullOrEmpty()) {
                bodyView.visibility = View.VISIBLE
            }
            if (rec.isOpen) {
                populateResponseControls(
                    responseContainer, flowId, rec)
                responseContainer.visibility = View.VISIBLE
            }
        }

        // Tap to expand/collapse.
        root.setOnClickListener {
            val wasExpanded = rec.id == expandedNotificationId
            expandedNotificationId =
                if (wasExpanded) null else rec.id

            // Refresh: re-populate the parent container.
            val parent =
                root.parent as? LinearLayout ?: return@setOnClickListener
            val result = flowNotifications[flowId]
                ?: return@setOnClickListener
            populateNotifications(parent, flowId, result)
        }

        return root
    }

    /**
     * Populate response controls for an open interactive
     * notification: accept/deny for boolean, action buttons
     * for choice, text input for text. Buttons are laid out
     * in horizontal rows of [BUTTONS_PER_ROW].
     */
    private fun populateResponseControls(
        container: LinearLayout,
        flowId: String,
        rec: HistoryRecord
    ) {
        container.removeAllViews()
        val ctx = container.context

        // Collect all buttons first, then lay out in rows.
        val buttons = mutableListOf<View>()

        for (type in rec.responseTypes) {
            when (type) {
                "boolean" -> {
                    buttons.add(
                        actionButton(ctx, "Accept") {
                            onNotificationResponse?.invoke(
                                rec.id, flowId,
                                NotificationRenderer
                                    .ACTION_TYPE_ACCEPTED,
                                "true", null)
                            markResponded(flowId, rec.id)
                        })
                    buttons.add(
                        actionButton(ctx, "Deny") {
                            onNotificationResponse?.invoke(
                                rec.id, flowId,
                                NotificationRenderer
                                    .ACTION_TYPE_REJECTED,
                                "false", null)
                            markResponded(flowId, rec.id)
                        })
                }
                "choice" -> {
                    val actions = rec.actions ?: continue
                    for (action in actions) {
                        buttons.add(
                            actionButton(ctx, action) {
                                onNotificationResponse?.invoke(
                                    rec.id, flowId,
                                    NotificationRenderer
                                        .ACTION_TYPE_CHOICE,
                                    action, null)
                                markResponded(flowId, rec.id)
                            })
                    }
                }
                "text" -> {
                    // Text input gets its own full-width row.
                    addButtonRows(container, buttons)
                    buttons.clear()
                    val row = LinearLayout(ctx).apply {
                        orientation = LinearLayout.HORIZONTAL
                        setPadding(0, dp(2), 0, dp(2))
                    }
                    val editText = EditText(ctx).apply {
                        hint = "Reply\u2026"
                        textSize = 11f
                        setPadding(dp(8), dp(4), dp(8), dp(4))
                        layoutParams = LinearLayout.LayoutParams(
                            0,
                            LinearLayout.LayoutParams.WRAP_CONTENT,
                            1f
                        )
                    }
                    row.addView(editText)
                    row.addView(
                        actionButton(ctx, "Send") {
                            val text = editText.text.toString()
                            onNotificationResponse?.invoke(
                                rec.id, flowId,
                                NotificationRenderer
                                    .ACTION_TYPE_TEXT,
                                "", text.ifEmpty { null })
                            markResponded(flowId, rec.id)
                        })
                    container.addView(row)
                }
            }
        }

        addButtonRows(container, buttons)
    }

    /**
     * Lay out buttons in horizontal rows of [BUTTONS_PER_ROW].
     */
    private fun addButtonRows(
        container: LinearLayout,
        buttons: List<View>
    ) {
        if (buttons.isEmpty()) return
        val ctx = container.context
        for (chunk in buttons.chunked(BUTTONS_PER_ROW)) {
            val row = LinearLayout(ctx).apply {
                orientation = LinearLayout.HORIZONTAL
                setPadding(0, dp(2), 0, dp(2))
            }
            for (btn in chunk) {
                row.addView(btn)
            }
            container.addView(row)
        }
    }

    /**
     * After a response is submitted, refresh the flow's
     * notification sub-list. Triggers a re-query via
     * [onFlowExpanded].
     */
    /**
     * After a response is submitted, collapse the notification
     * sub-item. We do NOT re-query immediately because the
     * response may not have been written to the ledger yet —
     * the next heartbeat or notification event will refresh
     * the data showing the updated status.
     */
    private fun markResponded(flowId: String, notificationId: String) {
        expandedNotificationId = null
        val result = flowNotifications[flowId] ?: return
        val container = expandedFlowId?.let { id ->
            val pos = items.indexOfFirst {
                it is DashboardItem.FlowItem && it.flowId == id
            }
            if (pos >= 0) pos else null
        }
        if (container != null) notifyItemChanged(container)
    }

    private fun actionButton(
        ctx: android.content.Context,
        label: String,
        onClick: () -> Unit
    ): Button {
        return Button(ctx).apply {
            text = label
            textSize = 10f
            background = GradientDrawable().apply {
                setColor(Brand.BUTTON_BG)
                cornerRadius = dp(4).toFloat()
            }
            setTextColor(Brand.BUTTON_TEXT)
            setPadding(dp(10), dp(4), dp(10), dp(4))
            minWidth = 0
            minimumWidth = 0
            minHeight = 0
            minimumHeight = 0
            val lp = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
            )
            lp.marginEnd = dp(6)
            layoutParams = lp
            setOnClickListener { onClick() }
        }
    }

    // --- Helpers ---

    /** Format an RFC 3339 timestamp to local date/time. */
    private fun formatTimestamp(ts: String): String {
        return try {
            val instant = java.time.Instant.parse(ts)
            val local = java.time.LocalDateTime.ofInstant(
                instant, java.time.ZoneId.systemDefault())
            local.format(java.time.format.DateTimeFormatter
                .ofPattern("yyyy-MM-dd HH:mm:ss"))
        } catch (_: Exception) {
            ts
        }
    }

    /**
     * Compute the remaining TTL for a flow given its last
     * activity timestamp and the grace period. Returns a
     * human-readable string (e.g. "12m30s") or null if the
     * grace period is unknown.
     */
    private fun formatTTL(
        lastActivity: String,
        gracePeriodMs: Long
    ): String? {
        if (gracePeriodMs <= 0) return null
        return try {
            val instant = java.time.Instant.parse(lastActivity)
            val expiresAt = instant.plusMillis(gracePeriodMs)
            val remainingMs = expiresAt.toEpochMilli() -
                System.currentTimeMillis()
            if (remainingMs <= 0) return "expired"
            val totalSec = remainingMs / 1000
            val min = totalSec / 60
            val sec = totalSec % 60
            val base = if (min > 0) "${min}m${sec}s" else "${sec}s"
            // Visual warning when TTL is running low.
            when {
                remainingMs < 60_000  -> "$base \u26a0"
                remainingMs < 180_000 -> "$base \u23f2"
                else                  -> base
            }
        } catch (_: Exception) {
            null
        }
    }

    private fun View.dp(value: Int): Int {
        return (value * resources.displayMetrics.density).toInt()
    }

    class ViewHolder(view: View) : RecyclerView.ViewHolder(view)

    companion object {
        const val VIEW_WORKSPACE = 0
        const val VIEW_FLOW = 1
        const val VIEW_EMPTY = 2
        private const val BUTTONS_PER_ROW = 3

        private const val TAG_WS_NAME = "ws_name"
        private const val TAG_WS_DETAIL = "ws_detail"
        private const val TAG_FLOW_DOT = "flow_dot"
        private const val TAG_FLOW_LABEL = "flow_label"
        private const val TAG_FLOW_META = "flow_meta"
        private const val TAG_FLOW_SUMMARY = "flow_summary"
        private const val TAG_FLOW_ACTIONS = "flow_actions"
        private const val TAG_FLOW_BADGES = "flow_badges"
        private const val TAG_FLOW_NOTIFICATIONS =
            "flow_notifications"
        private const val TAG_BADGE_TOTAL = "badge_total"
        private const val TAG_BADGE_OPEN = "badge_open"
        private const val TAG_BTN_STOP = "btn_stop"
        private const val TAG_BTN_NOTE = "btn_note"
        private const val TAG_EMPTY = "empty"
    }
}
