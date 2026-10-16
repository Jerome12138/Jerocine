package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.R
import com.jerocine.tv.data.FilterTag
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.ui.CategoryFilterGroup

class CategoryFilterAdapter(
    private val onSelect: (String, String) -> Unit,
) : ListAdapter<CategoryFilterGroup, CategoryFilterAdapter.ViewHolder>(GroupDiff) {
    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_category_filter_group, parent, false)
        return ViewHolder(view, onSelect)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) = holder.bind(getItem(position))

    class ViewHolder(itemView: View, onSelect: (String, String) -> Unit) :
        RecyclerView.ViewHolder(itemView) {
        private val title = itemView.findViewById<TextView>(R.id.category_filter_group_title)
        private val options = itemView.findViewById<RecyclerView>(R.id.category_filter_options)
        private val adapter = FilterOptionAdapter(onSelect)

        init {
            options.layoutManager = GridLayoutManager(itemView.context, 3)
            options.adapter = adapter
            options.itemAnimator = null
        }

        fun bind(group: CategoryFilterGroup) {
            title.text = group.title
            adapter.bindGroup(group.key, group.current, group.options)
            options.layoutParams = options.layoutParams.apply {
                height = ((group.options.size + 2) / 3) * itemView.dp(48)
            }
        }
    }

    private object GroupDiff : DiffUtil.ItemCallback<CategoryFilterGroup>() {
        override fun areItemsTheSame(oldItem: CategoryFilterGroup, newItem: CategoryFilterGroup) =
            oldItem.key == newItem.key
        override fun areContentsTheSame(oldItem: CategoryFilterGroup, newItem: CategoryFilterGroup) =
            oldItem == newItem
    }
}

private class FilterOptionAdapter(
    private val onSelect: (String, String) -> Unit,
) : ListAdapter<FilterTag, FilterOptionAdapter.ViewHolder>(OptionDiff) {
    private var key = ""
    private var current = ""

    fun bindGroup(key: String, current: String, options: List<FilterTag>) {
        this.key = key
        this.current = current
        submitList(options)
        notifyItemRangeChanged(0, itemCount)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_category_filter_chip, parent, false) as TextView
        return ViewHolder(view) { value -> onSelect(key, value) }
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) {
        holder.bind(getItem(position), current)
    }

    class ViewHolder(
        private val label: TextView,
        onClick: (String) -> Unit,
    ) : RecyclerView.ViewHolder(label) {
        private var value = ""

        init {
            label.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            label.setOnClickListener { onClick(value) }
        }

        fun bind(option: FilterTag, current: String) {
            value = option.value
            label.text = option.name
            label.isSelected = option.value == current
        }
    }

    private object OptionDiff : DiffUtil.ItemCallback<FilterTag>() {
        override fun areItemsTheSame(oldItem: FilterTag, newItem: FilterTag) = oldItem.value == newItem.value
        override fun areContentsTheSame(oldItem: FilterTag, newItem: FilterTag) = oldItem == newItem
    }
}

private fun View.dp(value: Int): Int = (value * resources.displayMetrics.density).toInt()
