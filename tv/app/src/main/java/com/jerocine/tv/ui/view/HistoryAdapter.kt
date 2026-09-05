package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.ProgressBar
import android.widget.TextView
import androidx.core.view.isVisible
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import coil.dispose
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.ui.HistoryRow
import com.jerocine.tv.ui.historyUpdatedLabel

class HistoryAdapter(
    private val onItemClick: (com.jerocine.tv.data.HistoryItem) -> Unit,
) : ListAdapter<HistoryRow, RecyclerView.ViewHolder>(DiffCallback) {
    var manageMode: Boolean = false
        set(value) {
            if (field == value) return
            field = value
            notifyItemRangeChanged(0, itemCount, PAYLOAD_MODE)
        }

    init {
        setHasStableIds(true)
    }

    override fun getItemViewType(position: Int): Int = when (getItem(position)) {
        is HistoryRow.Header -> TYPE_HEADER
        is HistoryRow.Item -> TYPE_ITEM
    }

    override fun getItemId(position: Int): Long = when (val row = getItem(position)) {
        is HistoryRow.Header -> Long.MIN_VALUE + row.bucket.hashCode().toLong()
        is HistoryRow.Item -> 31L * row.bucket.hashCode() + 17L * row.history.mid + row.history.episodeNumber()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RecyclerView.ViewHolder {
        val inflater = LayoutInflater.from(parent.context)
        return if (viewType == TYPE_HEADER) {
            HeaderHolder(inflater.inflate(R.layout.item_category_header, parent, false))
        } else {
            ItemHolder(inflater.inflate(R.layout.item_history, parent, false), onItemClick)
        }
    }

    override fun onBindViewHolder(holder: RecyclerView.ViewHolder, position: Int) = bind(holder, position)

    override fun onBindViewHolder(holder: RecyclerView.ViewHolder, position: Int, payloads: MutableList<Any>) {
        if (payloads.contains(PAYLOAD_MODE) && holder is ItemHolder) {
            holder.bindMode(manageMode)
        } else {
            bind(holder, position)
        }
    }

    private fun bind(holder: RecyclerView.ViewHolder, position: Int) {
        when (val row = getItem(position)) {
            is HistoryRow.Header -> (holder as HeaderHolder).bind(row)
            is HistoryRow.Item -> (holder as ItemHolder).bind(row, manageMode)
        }
    }

    override fun onViewRecycled(holder: RecyclerView.ViewHolder) {
        if (holder is ItemHolder) holder.recycle()
    }

    class HeaderHolder(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val title = itemView.findViewById<TextView>(R.id.category_section_title)
        private val subtitle = itemView.findViewById<TextView>(R.id.category_section_subtitle)

        fun bind(row: HistoryRow.Header) {
            title.text = row.label
            subtitle.text = ""
        }
    }

    class ItemHolder(
        itemView: View,
        private val onItemClick: (com.jerocine.tv.data.HistoryItem) -> Unit,
    ) : RecyclerView.ViewHolder(itemView) {
        private val cover = itemView.findViewById<ImageView>(R.id.history_cover)
        private val episode = itemView.findViewById<TextView>(R.id.history_episode)
        private val title = itemView.findViewById<TextView>(R.id.history_title)
        private val meta = itemView.findViewById<TextView>(R.id.history_meta)
        private val progress = itemView.findViewById<ProgressBar>(R.id.history_item_progress)
        private val delete = itemView.findViewById<TextView>(R.id.history_delete)
        private lateinit var history: com.jerocine.tv.data.HistoryItem

        init {
            itemView.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            itemView.setOnClickListener { if (::history.isInitialized) onItemClick(history) }
        }

        fun bind(row: HistoryRow.Item, manageMode: Boolean) {
            itemView.resetTvFocusAnimation()
            if (itemView.isFocused) {
                val scale = focusMotion(ServiceLocator.tokenStore.reduceMotion).scale
                itemView.scaleX = scale
                itemView.scaleY = scale
                itemView.translationZ = 8f
            }
            history = row.history
            val card = history.previewCard()
            itemView.tag = history.mid
            itemView.contentDescription = card.name
            cover.loadTvBackdrop(card.cover)
            episode.text = history.episodeLabel()
            title.text = card.name
            meta.text = listOf(
                history.source().ifBlank { "默认线路" },
                historyUpdatedLabel(history),
            )
                .joinToString(" · ")
            progress.progress = (history.progressFraction() * 1000).toInt()
            progress.isVisible = history.progressFraction() > 0f
            bindMode(manageMode)
        }

        fun bindMode(manageMode: Boolean) {
            delete.isVisible = manageMode
        }

        fun recycle() {
            itemView.resetTvFocusAnimation()
            cover.dispose()
            cover.setImageDrawable(null)
        }
    }

    private object DiffCallback : DiffUtil.ItemCallback<HistoryRow>() {
        override fun areItemsTheSame(oldItem: HistoryRow, newItem: HistoryRow): Boolean = when {
            oldItem is HistoryRow.Header && newItem is HistoryRow.Header -> oldItem.bucket == newItem.bucket
            oldItem is HistoryRow.Item && newItem is HistoryRow.Item ->
                oldItem.bucket == newItem.bucket && oldItem.history.mid == newItem.history.mid &&
                    oldItem.history.episodeNumber() == newItem.history.episodeNumber()
            else -> false
        }

        override fun areContentsTheSame(oldItem: HistoryRow, newItem: HistoryRow) = oldItem == newItem
    }

    companion object {
        const val TYPE_HEADER = 0
        const val TYPE_ITEM = 1
        private const val PAYLOAD_MODE = "manage-mode"
    }
}
