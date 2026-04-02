package io.resystems.renotify.dashboard

import android.graphics.Typeface
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

/**
 * RecyclerView adapter for the dashboard. Renders a flat list
 * of [DashboardItem] entries: workspace headers, flow rows, and
 * an empty-state placeholder.
 *
 * Uses programmatic views (no XML layouts) to match the existing
 * codebase pattern established in M-01.
 */
class DashboardAdapter :
    RecyclerView.Adapter<DashboardAdapter.ViewHolder>() {

    private var items: List<DashboardItem> =
        DashboardItem.fromHeartbeat(null)

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
            orientation = LinearLayout.HORIZONTAL
            setPadding(dp(32), dp(6), dp(16), dp(6))
            layoutParams = RecyclerView.LayoutParams(
                RecyclerView.LayoutParams.MATCH_PARENT,
                RecyclerView.LayoutParams.WRAP_CONTENT
            )

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
                    0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f
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

        // Show label if present, otherwise flow ID.
        labelText.text = item.label.ifEmpty { item.flowId }

        // Show metadata as "key: value" lines, flow ID, and
        // last activity timestamp.
        val parts = mutableListOf<String>()
        if (item.label.isNotEmpty()) {
            parts.add(item.flowId)
        }
        if (item.lastActivity.isNotEmpty()) {
            parts.add("updated: ${formatTimestamp(item.lastActivity)}")
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
        private const val TAG_EMPTY = "empty"
    }
}
