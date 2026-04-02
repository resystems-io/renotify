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

/**
 * RecyclerView adapter for the dashboard. Renders a flat list
 * of [DashboardItem] entries: workspace headers, flow rows
 * (expandable with Stop/Note actions), and an empty-state
 * placeholder.
 *
 * Uses programmatic views (no XML layouts) to match the existing
 * codebase pattern established in M-01.
 */
class DashboardAdapter :
    RecyclerView.Adapter<DashboardAdapter.ViewHolder>() {

    private var items: List<DashboardItem> =
        DashboardItem.fromHeartbeat(null)

    private val expandedFlows = mutableSetOf<String>()

    /**
     * Callback for flow actions. Parameters: flowId, action
     * ("stop" or "note"), optional context message.
     */
    var onFlowAction: ((
        flowId: String, action: String, context: String?
    ) -> Unit)? = null

    /** Update the adapter with a new heartbeat snapshot. */
    fun update(heartbeat: DaemonHeartbeat?) {
        items = DashboardItem.fromHeartbeat(heartbeat)
        notifyDataSetChanged()
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
                setTextColor(0xFF888888.toInt())
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
                    setBackgroundColor(0xFF4CAF50.toInt())
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
                        setTextColor(0xFF888888.toInt())
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
                        setColor(0xFFB71C1C.toInt())
                        cornerRadius = dp(6).toFloat()
                    }
                    setTextColor(0xFFFFFFFF.toInt())
                    setPadding(dp(12), dp(6), dp(12), dp(6))
                    minWidth = 0
                    minimumWidth = 0
                })

                addView(Button(context).apply {
                    tag = TAG_BTN_NOTE
                    text = "Note"
                    textSize = 12f
                    background = GradientDrawable().apply {
                        setColor(0xFF444444.toInt())
                        cornerRadius = dp(6).toFloat()
                    }
                    setTextColor(0xFFFFFFFF.toInt())
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
        }
    }

    private fun createEmptyView(
        parent: ViewGroup
    ): TextView {
        return TextView(parent.context).apply {
            tag = TAG_EMPTY
            textSize = 14f
            setTextColor(0xFF888888.toInt())
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

        // Show label if present, otherwise flow ID.
        labelText.text = item.label.ifEmpty { item.flowId }

        // Show metadata as "key: value" lines, flow ID, and
        // last activity timestamp.
        val parts = mutableListOf<String>()
        if (item.label.isNotEmpty()) {
            parts.add(item.flowId)
        }
        if (item.lastActivity.isNotEmpty()) {
            parts.add(
                "updated: ${formatTimestamp(item.lastActivity)}")
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

        // Expand/collapse on tap.
        val expanded = item.flowId in expandedFlows
        actionsRow.visibility =
            if (expanded) View.VISIBLE else View.GONE

        holder.itemView.setOnClickListener {
            if (item.flowId in expandedFlows) {
                expandedFlows.remove(item.flowId)
            } else {
                expandedFlows.add(item.flowId)
            }
            notifyItemChanged(holder.adapterPosition)
        }

        // Action buttons.
        stopBtn.setOnClickListener {
            onFlowAction?.invoke(item.flowId, "stop", null)
            expandedFlows.remove(item.flowId)
            notifyItemChanged(holder.adapterPosition)
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
                    expandedFlows.remove(item.flowId)
                    notifyItemChanged(holder.adapterPosition)
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

    private fun View.dp(value: Int): Int {
        return (value * resources.displayMetrics.density).toInt()
    }

    class ViewHolder(view: View) : RecyclerView.ViewHolder(view)

    companion object {
        const val VIEW_WORKSPACE = 0
        const val VIEW_FLOW = 1
        const val VIEW_EMPTY = 2

        private const val TAG_WS_NAME = "ws_name"
        private const val TAG_WS_DETAIL = "ws_detail"
        private const val TAG_FLOW_DOT = "flow_dot"
        private const val TAG_FLOW_LABEL = "flow_label"
        private const val TAG_FLOW_META = "flow_meta"
        private const val TAG_FLOW_SUMMARY = "flow_summary"
        private const val TAG_FLOW_ACTIONS = "flow_actions"
        private const val TAG_BTN_STOP = "btn_stop"
        private const val TAG_BTN_NOTE = "btn_note"
        private const val TAG_EMPTY = "empty"
    }
}
