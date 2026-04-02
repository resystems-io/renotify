package io.resystems.renotify.dashboard

import android.graphics.Typeface
import android.view.Gravity
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

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

    /** Callback when the user taps "Load more". */
    var onLoadMore: (() -> Unit)? = null

    /** Replace the full result set. */
    fun update(result: HistoryQueryResult) {
        records = result.records
        total = result.total
        notifyDataSetChanged()
    }

    /** Append a page of records to the existing list. */
    fun append(result: HistoryQueryResult) {
        val start = records.size
        records = records + result.records
        total = result.total
        notifyItemRangeInserted(start, result.records.size)
        // Refresh the load-more footer.
        notifyItemChanged(records.size)
    }

    /** Current record count (for pagination offset). */
    val recordCount: Int get() = records.size

    private val hasMore: Boolean get() = records.size < total

    override fun getItemCount(): Int {
        if (records.isEmpty()) return 1 // empty state
        return records.size + if (hasMore) 1 else 0
    }

    override fun getItemViewType(position: Int): Int {
        if (records.isEmpty()) return VIEW_EMPTY
        if (position < records.size) return VIEW_RECORD
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

                val detail = TextView(ctx).apply {
                    textSize = 12f
                    setTextColor(0xFF666666.toInt())
                }
                root.addView(detail)

                val response = TextView(ctx).apply {
                    textSize = 12f
                    setTextColor(0xFF888888.toInt())
                }
                root.addView(response)

                ViewHolder(root)
            }

            VIEW_LOAD_MORE -> {
                val tv = TextView(ctx).apply {
                    text = "Load more\u2026"
                    textSize = 14f
                    gravity = Gravity.CENTER
                    setTextColor(0xFF1A73E8.toInt())
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
                    setTextColor(0xFF999999.toInt())
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
        if (records.isEmpty() || pos >= records.size) return

        val rec = records[pos]
        val root = holder.itemView as? LinearLayout ?: return

        // Title.
        (root.getChildAt(0) as? TextView)?.text = rec.title

        // Detail: timestamp + priority + source.
        val ts = formatTimestamp(rec.timestamp)
        val parts = mutableListOf(ts)
        if (rec.priority != "normal") parts.add(rec.priority)
        if (rec.source.isNotEmpty()) parts.add(rec.source)
        (root.getChildAt(1) as? TextView)?.text =
            parts.joinToString(" · ")

        // Response summary.
        (root.getChildAt(2) as? TextView)?.text =
            if (rec.hasResponse) rec.responseSummary
            else "\u2014"
    }

    class ViewHolder(view: android.view.View) :
        RecyclerView.ViewHolder(view)

    companion object {
        private const val VIEW_RECORD = 0
        private const val VIEW_LOAD_MORE = 1
        private const val VIEW_EMPTY = 2

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
