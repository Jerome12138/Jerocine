package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.ui.CategoryRow

class CategoryAdapter(
    private val onPosterClick: (Long) -> Unit,
    private val onMoreClick: (String) -> Unit,
) : ListAdapter<CategoryRow, RecyclerView.ViewHolder>(DiffCallback) {
    init {
        setHasStableIds(true)
    }

    override fun getItemViewType(position: Int): Int = when (getItem(position)) {
        is CategoryRow.Header -> TYPE_HEADER
        is CategoryRow.Poster -> TYPE_POSTER
    }

    override fun getItemId(position: Int): Long = when (val row = getItem(position)) {
        is CategoryRow.Header -> Long.MIN_VALUE + row.key.hashCode().toLong()
        is CategoryRow.Poster -> 31L * row.sectionKey.hashCode() + row.card.mid
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RecyclerView.ViewHolder {
        val inflater = LayoutInflater.from(parent.context)
        return if (viewType == TYPE_HEADER) {
            HeaderHolder(inflater.inflate(R.layout.item_category_header, parent, false), onMoreClick)
        } else {
            PosterAdapter.ViewHolder(
                inflater.inflate(R.layout.item_category_poster, parent, false),
                onPosterClick,
            )
        }
    }

    override fun onBindViewHolder(holder: RecyclerView.ViewHolder, position: Int) {
        when (val row = getItem(position)) {
            is CategoryRow.Header -> (holder as HeaderHolder).bind(row)
            is CategoryRow.Poster -> (holder as PosterAdapter.ViewHolder).bind(row.card)
        }
    }

    override fun onViewRecycled(holder: RecyclerView.ViewHolder) {
        if (holder is PosterAdapter.ViewHolder) holder.recycle()
    }

    class HeaderHolder(itemView: View, private val onMoreClick: (String) -> Unit) :
        RecyclerView.ViewHolder(itemView) {
        private val title = itemView.findViewById<TextView>(R.id.category_section_title)
        private val subtitle = itemView.findViewById<TextView>(R.id.category_section_subtitle)
        private val more = itemView.findViewById<View>(R.id.category_section_more)
        private var sort = ""

        init {
            more.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            more.setOnClickListener { onMoreClick(sort) }
        }

        fun bind(row: CategoryRow.Header) {
            title.text = row.title
            subtitle.text = row.subtitle
            sort = row.sort
            more.visibility = View.VISIBLE
        }
    }

    private object DiffCallback : DiffUtil.ItemCallback<CategoryRow>() {
        override fun areItemsTheSame(oldItem: CategoryRow, newItem: CategoryRow): Boolean = when {
            oldItem is CategoryRow.Header && newItem is CategoryRow.Header -> oldItem.key == newItem.key
            oldItem is CategoryRow.Poster && newItem is CategoryRow.Poster ->
                oldItem.sectionKey == newItem.sectionKey && oldItem.card.mid == newItem.card.mid
            else -> false
        }

        override fun areContentsTheSame(oldItem: CategoryRow, newItem: CategoryRow) = oldItem == newItem
    }

    companion object {
        const val TYPE_HEADER = 0
        const val TYPE_POSTER = 1
    }
}
