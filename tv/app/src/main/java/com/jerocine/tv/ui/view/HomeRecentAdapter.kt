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
import com.jerocine.tv.data.HistoryItem
import com.jerocine.tv.data.ServiceLocator

class HomeRecentAdapter(
    private val onClick: (Long) -> Unit,
) : ListAdapter<HistoryItem, HomeRecentAdapter.ViewHolder>(DiffCallback) {
    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_home_recent, parent, false)
        return ViewHolder(view, onClick)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) = holder.bind(getItem(position))

    override fun onViewRecycled(holder: ViewHolder) = holder.recycle()

    class ViewHolder(itemView: View, private val onClick: (Long) -> Unit) :
        RecyclerView.ViewHolder(itemView) {
        private val cover = itemView.findViewById<ImageView>(R.id.home_recent_cover)
        private val title = itemView.findViewById<TextView>(R.id.home_recent_title)
        private val episode = itemView.findViewById<TextView>(R.id.home_recent_episode)
        private val progress = itemView.findViewById<ProgressBar>(R.id.home_recent_progress)
        private var mid = 0L

        init {
            itemView.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            itemView.setOnClickListener { onClick(mid) }
        }

        fun bind(item: HistoryItem) {
            mid = item.mid
            itemView.tag = mid
            val card = item.previewCard()
            cover.loadTvPoster(card.cover, card.name)
            title.text = card.name
            episode.text = "看到 ${item.episodeLabel()}"
            progress.progress = (item.progressFraction() * 1000).toInt()
            progress.isVisible = item.progressFraction() > 0f
        }

        fun recycle() {
            itemView.resetTvFocusAnimation()
            cover.dispose()
            cover.setImageDrawable(null)
        }
    }

    private object DiffCallback : DiffUtil.ItemCallback<HistoryItem>() {
        override fun areItemsTheSame(oldItem: HistoryItem, newItem: HistoryItem) = oldItem.mid == newItem.mid
        override fun areContentsTheSame(oldItem: HistoryItem, newItem: HistoryItem) = oldItem == newItem
    }
}
