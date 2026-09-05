package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.TextView
import androidx.core.view.isVisible
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator

class HomeSectionAdapter(
    private val onPosterClick: (Long) -> Unit,
    private val onCategoryClick: (Long) -> Unit,
) : ListAdapter<HomeSectionPair, HomeSectionAdapter.ViewHolder>(DiffCallback) {
    private val recycledViewPool = RecyclerView.RecycledViewPool()

    init {
        setHasStableIds(true)
    }

    override fun getItemId(position: Int): Long = getItem(position).let { pair ->
        31L * pair.left.stableKey.hashCode() + (pair.right?.stableKey?.hashCode() ?: 0)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_home_section, parent, false)
        return ViewHolder(view as ViewGroup, recycledViewPool, onPosterClick, onCategoryClick)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) = holder.bind(getItem(position))

    fun requestPosterFocus(outer: RecyclerView, mid: Long, adapterOffset: Int): Boolean {
        val pairIndex = currentList.indexOfFirst { pair ->
            pair.left.items.any { it.mid == mid } || pair.right?.items?.any { it.mid == mid } == true
        }
        if (pairIndex < 0) return false
        val outerPosition = pairIndex + adapterOffset
        outer.scrollToPosition(outerPosition)
        outer.post {
            (outer.findViewHolderForAdapterPosition(outerPosition) as? ViewHolder)
                ?.requestPosterFocus(mid)
        }
        return true
    }

    class ViewHolder(
        root: ViewGroup,
        pool: RecyclerView.RecycledViewPool,
        onPosterClick: (Long) -> Unit,
        onCategoryClick: (Long) -> Unit,
    ) : RecyclerView.ViewHolder(root) {
        private val left = PanelBinding(root, "left", pool, onPosterClick, onCategoryClick)
        private val right = PanelBinding(root, "right", pool, onPosterClick, onCategoryClick)

        fun bind(pair: HomeSectionPair) {
            left.bind(pair.left)
            right.root.isVisible = pair.right != null
            pair.right?.let(right::bind)
        }

        fun requestPosterFocus(mid: Long) {
            if (left.requestPosterFocus(mid)) return
            right.requestPosterFocus(mid)
        }
    }

    private class PanelBinding(
        parent: ViewGroup,
        prefix: String,
        pool: RecyclerView.RecycledViewPool,
        onPosterClick: (Long) -> Unit,
        private val onCategoryClick: (Long) -> Unit,
    ) {
        val root: View = parent.findViewById(
            if (prefix == "left") R.id.home_panel_left else R.id.home_panel_right,
        )
        private val title: TextView = root.findViewById(
            if (prefix == "left") R.id.home_panel_left_title else R.id.home_panel_right_title,
        )
        private val subtitle: TextView = root.findViewById(
            if (prefix == "left") R.id.home_panel_left_subtitle else R.id.home_panel_right_subtitle,
        )
        private val more: Button = root.findViewById(
            if (prefix == "left") R.id.home_panel_left_more else R.id.home_panel_right_more,
        )
        private val posters: RecyclerView = root.findViewById(
            if (prefix == "left") R.id.home_panel_left_posters else R.id.home_panel_right_posters,
        )
        private val adapter = PosterAdapter(onPosterClick)
        private var categoryId: Long? = null

        init {
            posters.layoutManager = LinearLayoutManager(root.context, RecyclerView.HORIZONTAL, false).apply {
                initialPrefetchItemCount = 3
            }
            posters.adapter = adapter
            posters.setRecycledViewPool(pool)
            posters.itemAnimator = null
            more.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            more.setOnClickListener { categoryId?.let(onCategoryClick) }
        }

        fun bind(panel: HomeSectionItem) {
            root.setBackgroundResource(
                when (panel.tone) {
                    HomePanelTone.CYAN -> R.drawable.gf_tv_panel_cyan
                    HomePanelTone.AMBER -> R.drawable.gf_tv_panel_amber
                    HomePanelTone.PURPLE -> R.drawable.gf_tv_panel_purple
                },
            )
            title.text = panel.title
            subtitle.text = panel.subtitle
            categoryId = panel.categoryId
            more.isVisible = panel.categoryId != null
            adapter.submitList(panel.items)
        }

        fun requestPosterFocus(mid: Long): Boolean {
            val position = adapter.currentList.indexOfFirst { it.mid == mid }
            if (position < 0) return false
            posters.scrollToPosition(position)
            posters.post {
                posters.findViewHolderForAdapterPosition(position)?.itemView?.requestFocus()
            }
            return true
        }
    }

    private object DiffCallback : DiffUtil.ItemCallback<HomeSectionPair>() {
        override fun areItemsTheSame(oldItem: HomeSectionPair, newItem: HomeSectionPair) =
            oldItem.left.stableKey == newItem.left.stableKey &&
                oldItem.right?.stableKey == newItem.right?.stableKey

        override fun areContentsTheSame(oldItem: HomeSectionPair, newItem: HomeSectionPair) =
            oldItem == newItem
    }
}
