package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.annotation.LayoutRes
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator

data class DetailChoiceItem(
    val key: String,
    val label: String,
    val selected: Boolean,
)

class DetailChoiceAdapter(
    @LayoutRes private val layoutRes: Int,
    private val onClick: (String) -> Unit,
) : ListAdapter<DetailChoiceItem, DetailChoiceAdapter.ViewHolder>(DiffCallback) {
    init {
        setHasStableIds(true)
    }

    override fun getItemId(position: Int): Long = getItem(position).key.hashCode().toLong()

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(layoutRes, parent, false)
        return ViewHolder(view, onClick)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) = holder.bind(getItem(position))

    override fun onViewRecycled(holder: ViewHolder) {
        holder.itemView.resetTvFocusAnimation()
    }

    class ViewHolder(itemView: View, private val onClick: (String) -> Unit) :
        RecyclerView.ViewHolder(itemView) {
        private val label = itemView.findViewById<TextView>(R.id.detail_choice_label)
        private var key = ""

        init {
            itemView.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            itemView.setOnClickListener { onClick(key) }
        }

        fun bind(item: DetailChoiceItem) {
            key = item.key
            label.text = item.label
            itemView.isSelected = item.selected
            itemView.contentDescription = item.label
        }
    }

    private object DiffCallback : DiffUtil.ItemCallback<DetailChoiceItem>() {
        override fun areItemsTheSame(oldItem: DetailChoiceItem, newItem: DetailChoiceItem) =
            oldItem.key == newItem.key

        override fun areContentsTheSame(oldItem: DetailChoiceItem, newItem: DetailChoiceItem) =
            oldItem == newItem
    }
}
