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

class HomeNavAdapter(
    private val onClick: (Long) -> Unit,
) : ListAdapter<HomeNavItem, HomeNavAdapter.ViewHolder>(DiffCallback) {
    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_home_nav, parent, false)
        return ViewHolder(view, onClick)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) {
        holder.bind(getItem(position))
    }

    override fun onViewRecycled(holder: ViewHolder) {
        holder.itemView.resetTvFocusAnimation()
    }

    class ViewHolder(
        itemView: View,
        private val onClick: (Long) -> Unit,
    ) : RecyclerView.ViewHolder(itemView) {
        private val title = itemView as TextView
        private var id = 0L

        init {
            itemView.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            itemView.setOnClickListener { onClick(id) }
        }

        fun bind(item: HomeNavItem) {
            id = item.id
            title.text = item.title
            itemView.contentDescription = "打开${item.title}分类"
        }
    }

    private object DiffCallback : DiffUtil.ItemCallback<HomeNavItem>() {
        override fun areItemsTheSame(oldItem: HomeNavItem, newItem: HomeNavItem) = oldItem.id == newItem.id
        override fun areContentsTheSame(oldItem: HomeNavItem, newItem: HomeNavItem) = oldItem == newItem
    }
}
