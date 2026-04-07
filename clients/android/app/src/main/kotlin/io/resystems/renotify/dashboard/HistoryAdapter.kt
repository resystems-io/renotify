// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify.dashboard

import android.graphics.Typeface
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import io.resystems.renotify.Brand

/**
 * RecyclerView adapter for the notification history viewer
 * (M-07). Renders a flat list of [HistoryRecord] entries with
 * a "Load more" footer for pagination.
 *
 * Uses programmatic views (no XML) matching the codebase
 * pattern.
 */
class HistoryAdapter :
    RecyclerView.Adapter<HistoryAdapter.ViewHolder>() {

    private var records: List<HistoryRecord> = emptyList()
    private var total: Int = 0

    /** Filtered view of records for display. */
    private var displayRecords: List<HistoryRecord> = emptyList()

    /** Adapter position of the currently expanded item, or -1. */
    private var expandedPosition: Int = -1

    /** Callback when the user taps "Load more". */
    var onLoadMore: (() -> Unit)? = null

    /** Callback when lifecycle visibility changes. */
    var onLifecycleToggled: ((Boolean) -> Unit)? = null

    /** Whether lifecycle events (flow started/completed) are
     *  shown. Hidden by default to reduce noise. */
    var showLifecycle: Boolean = false
        set(value) {
            field = value
            rebuildDisplay()
            expandedPosition = -1
            notifyDataSetChanged()
            onLifecycleToggled?.invoke(value)
        }

    /** Replace the full result set. */
    fun update(result: HistoryQueryResult) {
        records = result.records
        total = result.total
        expandedPosition = -1
        rebuildDisplay()
        notifyDataSetChanged()
    }

    /** Append a page of records to the existing list. */
    fun append(result: HistoryQueryResult) {
        records = records + result.records
        total = result.total
        rebuildDisplay()
        notifyDataSetChanged()
    }

    /** Current record count (for pagination offset). */
    val recordCount: Int get() = records.size

    private val hasMore: Boolean get() = records.size < total

    private fun rebuildDisplay() {
        displayRecords = if (showLifecycle) {
            records
        } else {
            records.filter { it !is LifecycleRecord }
        }
    }

    override fun getItemCount(): Int {
        if (displayRecords.isEmpty()) return 1 // empty state
        return displayRecords.size + if (hasMore) 1 else 0
    }

    override fun getItemViewType(position: Int): Int {
        if (displayRecords.isEmpty()) return VIEW_EMPTY
        if (position < displayRecords.size) {
            return when (displayRecords[position]) {
                is LifecycleRecord -> VIEW_LIFECYCLE
                is NotificationRecord -> VIEW_RECORD
            }
        }
        return VIEW_LOAD_MORE
    }

    override fun onCreateViewHolder(
        parent: ViewGroup,
        viewType: Int
    ): ViewHolder {
        val ctx = parent.context
        val dp = { v: Int ->
            (v * ctx.resources.displayMetrics.density).toInt()
        }

        return when (viewType) {
            VIEW_RECORD -> {
                val root = LinearLayout(ctx).apply {
                    orientation = LinearLayout.VERTICAL
                    setPadding(dp(16), dp(10), dp(16), dp(10))
                    layoutParams = RecyclerView.LayoutParams(
                        RecyclerView.LayoutParams.MATCH_PARENT,
                        RecyclerView.LayoutParams.WRAP_CONTENT
                    )
                }

                val title = TextView(ctx).apply {
                    textSize = 14f
                    setTypeface(null, Typeface.BOLD)
                    maxLines = 2
                }
                root.addView(title)

                // Flow context: label + workspace (child 1).
                val flowContext = TextView(ctx).apply {
                    textSize = 11f
                    setTextColor(Brand.TEXT_SECONDARY)
                    visibility = View.GONE
                }
                root.addView(flowContext)

                val detail = TextView(ctx).apply {
                    textSize = 12f
                    setTextColor(Brand.TEXT_PRIMARY)
                }
                root.addView(detail)

                val response = TextView(ctx).apply {
                    textSize = 12f
                    setTextColor(Brand.TEXT_SECONDARY)
                }
                root.addView(response)

                // Expandable body (child 3) — hidden by default.
                val body = TextView(ctx).apply {
                    textSize = 12f
                    setTextColor(Brand.TEXT_DARK)
                    setPadding(0, dp(6), 0, 0)
                    visibility = View.GONE
                }
                root.addView(body)

                // Expandable full response (child 4) — hidden by
                // default, shown when response text was truncated.
                val fullResponse = TextView(ctx).apply {
                    textSize = 12f
                    setTextColor(Brand.TEXT_SECONDARY)
                    setPadding(0, dp(4), 0, 0)
                    visibility = View.GONE
                }
                root.addView(fullResponse)

                ViewHolder(root)
            }

            VIEW_LIFECYCLE -> {
                val tv = TextView(ctx).apply {
                    textSize = 12f
                    setTextColor(Brand.TEXT_SECONDARY)
                    setPadding(dp(16), dp(6), dp(16), dp(6))
                    layoutParams = RecyclerView.LayoutParams(
                        RecyclerView.LayoutParams.MATCH_PARENT,
                        RecyclerView.LayoutParams.WRAP_CONTENT
                    )
                }
                ViewHolder(tv)
            }

            VIEW_LOAD_MORE -> {
                val tv = TextView(ctx).apply {
                    text = "Load more\u2026"
                    textSize = 14f
                    gravity = Gravity.CENTER
                    setTextColor(Brand.LINK)
                    setPadding(dp(16), dp(16), dp(16), dp(16))
                    layoutParams = RecyclerView.LayoutParams(
                        RecyclerView.LayoutParams.MATCH_PARENT,
                        RecyclerView.LayoutParams.WRAP_CONTENT
                    )
                    setOnClickListener { onLoadMore?.invoke() }
                }
                ViewHolder(tv)
            }

            else -> {
                // Empty state.
                val tv = TextView(ctx).apply {
                    text = "No history records."
                    textSize = 14f
                    gravity = Gravity.CENTER
                    setTextColor(Brand.TEXT_SECONDARY)
                    setPadding(dp(16), dp(32), dp(16), dp(32))
                    layoutParams = RecyclerView.LayoutParams(
                        RecyclerView.LayoutParams.MATCH_PARENT,
                        RecyclerView.LayoutParams.WRAP_CONTENT
                    )
                }
                ViewHolder(tv)
            }
        }
    }

    override fun onBindViewHolder(holder: ViewHolder, pos: Int) {
        if (displayRecords.isEmpty() || pos >= displayRecords.size) return

        when (val rec = displayRecords[pos]) {
            is LifecycleRecord -> bindLifecycle(holder, rec)
            is NotificationRecord -> bindNotification(holder, pos, rec)
        }
    }

    private fun bindLifecycle(holder: ViewHolder, rec: LifecycleRecord) {
        val tv = holder.itemView as? TextView ?: return
        val ts = formatTimestamp(rec.timestamp)
        tv.text = "$ts \u00b7 ${rec.summary}"
    }

    private fun bindNotification(
        holder: ViewHolder,
        pos: Int,
        rec: NotificationRecord
    ) {
        val root = holder.itemView as? LinearLayout ?: return

        // Title (child 0).
        (root.getChildAt(0) as? TextView)?.text = rec.title

        // Flow context (child 1): label + workspace name.
        val ctxView = root.getChildAt(1) as? TextView
        val ctxParts = mutableListOf<String>()
        if (!rec.flowLabel.isNullOrEmpty()) ctxParts.add(rec.flowLabel)
        if (!rec.workspaceName.isNullOrEmpty()) ctxParts.add(rec.workspaceName)
        if (ctxParts.isNotEmpty()) {
            ctxView?.text = ctxParts.joinToString(" \u00b7 ")
            ctxView?.visibility = View.VISIBLE
        } else {
            ctxView?.visibility = View.GONE
        }

        // Detail (child 2): timestamp + priority + source.
        val ts = formatTimestamp(rec.timestamp)
        val parts = mutableListOf(ts)
        if (rec.priority != "normal") parts.add(rec.priority)
        if (rec.source.isNotEmpty()) parts.add(rec.source)
        (root.getChildAt(2) as? TextView)?.text =
            parts.joinToString(" \u00b7 ")

        // Response summary (child 3).
        (root.getChildAt(3) as? TextView)?.text =
            if (rec.hasResponse) rec.responseSummary
            else "\u2014"

        // Expansion state.
        val expanded = pos == expandedPosition && rec.isExpandable
        bindExpansion(root, rec, expanded)

        // Accordion tap handler.
        root.setOnClickListener {
            val adapterPos = holder.bindingAdapterPosition
            if (adapterPos == RecyclerView.NO_POSITION) return@setOnClickListener
            val clicked = displayRecords.getOrNull(adapterPos)
            if (clicked !is NotificationRecord || !clicked.isExpandable) return@setOnClickListener

            val prev = expandedPosition
            expandedPosition = if (prev == adapterPos) -1 else adapterPos
            if (prev >= 0 && prev < itemCount) notifyItemChanged(prev)
            if (expandedPosition >= 0) notifyItemChanged(expandedPosition)
        }
    }

    /**
     * Show or hide the expandable body (child 4) and full
     * response (child 5) for the given record.
     */
    private fun bindExpansion(
        root: LinearLayout,
        rec: NotificationRecord,
        expanded: Boolean
    ) {
        val bodyView = root.getChildAt(4) as? TextView
        val fullRespView = root.getChildAt(5) as? TextView

        if (!expanded) {
            bodyView?.visibility = View.GONE
            fullRespView?.visibility = View.GONE
            return
        }

        // Body text.
        if (!rec.body.isNullOrEmpty()) {
            bodyView?.text = rec.body
            bodyView?.visibility = View.VISIBLE
        } else {
            bodyView?.visibility = View.GONE
        }

        // Full response text (only when truncated in summary).
        val hasFullText = !rec.responseText.isNullOrEmpty() &&
            rec.responseText.length > 30
        if (hasFullText) {
            val respTs = rec.responseTimestamp?.let {
                " · ${formatTimestamp(it)}"
            } ?: ""
            fullRespView?.text = "Response: ${rec.responseText}$respTs"
            fullRespView?.visibility = View.VISIBLE
        } else {
            fullRespView?.visibility = View.GONE
        }
    }

    class ViewHolder(view: android.view.View) :
        RecyclerView.ViewHolder(view)

    companion object {
        private const val VIEW_RECORD = 0
        private const val VIEW_LOAD_MORE = 1
        private const val VIEW_EMPTY = 2
        private const val VIEW_LIFECYCLE = 3

        /** Format an RFC 3339 timestamp for display. */
        private fun formatTimestamp(rfc3339: String): String {
            return try {
                val instant = java.time.Instant.parse(rfc3339)
                val local = java.time.LocalDateTime.ofInstant(
                    instant, java.time.ZoneId.systemDefault())
                local.format(java.time.format.DateTimeFormatter
                    .ofPattern("yyyy-MM-dd HH:mm:ss"))
            } catch (_: Exception) {
                rfc3339
            }
        }
    }
}
