package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.TextView
import androidx.annotation.LayoutRes
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import coil.dispose
import com.jerocine.tv.R
import com.jerocine.tv.data.Card
import com.jerocine.tv.data.ServiceLocator

class PosterAdapter(
    private val onClick: (Long) -> Unit,
    @LayoutRes private val layoutRes: Int = R.layout.item_poster,
) : ListAdapter<Card, PosterAdapter.ViewHolder>(DiffCallback) {
    init {
        setHasStableIds(true)
    }

    override fun getItemId(position: Int): Long = getItem(position).mid

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(layoutRes, parent, false)
        return ViewHolder(view, onClick)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) {
        holder.bind(getItem(position))
    }

    override fun onViewRecycled(holder: ViewHolder) {
        holder.recycle()
    }

    class ViewHolder(
        itemView: View,
        private val onClick: (Long) -> Unit,
    ) : RecyclerView.ViewHolder(itemView) {
        private val cover = itemView.findViewById<ImageView>(R.id.poster_cover)
        private val title = itemView.findViewById<TextView>(R.id.poster_title)
        private val meta = itemView.findViewById<TextView>(R.id.poster_meta)
        private var mid: Long = 0

        init {
            itemView.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            itemView.setOnClickListener { onClick(mid) }
        }

        fun bind(card: Card) {
            itemView.resetTvFocusAnimation()
            if (itemView.isFocused) {
                val scale = focusMotion(ServiceLocator.tokenStore.reduceMotion).scale
                itemView.scaleX = scale
                itemView.scaleY = scale
                itemView.translationZ = 8f
            }
            mid = card.mid
            itemView.tag = card.mid
            itemView.contentDescription = card.name
            title.text = card.name
            meta.text = listOfNotNull(
                card.year.takeIf { it > 0 }?.toString(),
                card.remarks.ifBlank { null },
            ).joinToString(" · ")
            cover.loadTvPoster(card.cover, card.name)
        }

        fun recycle() {
            itemView.resetTvFocusAnimation()
            cover.dispose()
            cover.setImageDrawable(null)
        }
    }

    private object DiffCallback : DiffUtil.ItemCallback<Card>() {
        override fun areItemsTheSame(oldItem: Card, newItem: Card) = oldItem.mid == newItem.mid
        override fun areContentsTheSame(oldItem: Card, newItem: Card) = oldItem == newItem
    }
}
