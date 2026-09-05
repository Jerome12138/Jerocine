package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator

class SearchKeyboardAdapter(
    private val keys: List<String>,
    private val onKey: (String) -> Unit,
) : RecyclerView.Adapter<SearchKeyboardAdapter.ViewHolder>() {
    init {
        setHasStableIds(true)
    }

    override fun getItemId(position: Int): Long = keys[position].hashCode().toLong()

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_tv_key, parent, false)
        return ViewHolder(view, onKey)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) = holder.bind(keys[position])

    override fun onViewRecycled(holder: ViewHolder) {
        holder.itemView.resetTvFocusAnimation()
    }

    override fun getItemCount(): Int = keys.size

    class ViewHolder(itemView: View, private val onKey: (String) -> Unit) : RecyclerView.ViewHolder(itemView) {
        private val label = itemView.findViewById<TextView>(R.id.tv_key_label)
        private var key = ""

        init {
            itemView.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            itemView.setOnClickListener { onKey(key) }
        }

        fun bind(value: String) {
            key = value
            label.text = value
            itemView.contentDescription = value
        }
    }
}
