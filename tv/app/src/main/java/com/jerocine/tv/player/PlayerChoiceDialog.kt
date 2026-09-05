package com.jerocine.tv.player

import android.app.Activity
import android.app.Dialog
import android.graphics.Color
import android.graphics.drawable.ColorDrawable
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.Window
import android.view.WindowManager
import android.widget.ArrayAdapter
import android.widget.ListView
import android.widget.TextView
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator
import kotlin.math.min

fun interface OnPlayerChoiceListener {
    fun onSelected(index: Int)
}

object PlayerChoiceDialog {
    @JvmStatic
    fun show(
        activity: Activity,
        title: String,
        items: Array<String>,
        selectedIndex: Int,
        anchor: View?,
        listener: OnPlayerChoiceListener,
    ) {
        if (items.isEmpty()) return

        val dialog = Dialog(activity)
        dialog.requestWindowFeature(Window.FEATURE_NO_TITLE)
        dialog.setContentView(R.layout.dialog_player_choice)
        dialog.window?.setBackgroundDrawable(ColorDrawable(Color.TRANSPARENT))

        dialog.findViewById<TextView>(R.id.player_choice_title).text = title
        val listView = dialog.findViewById<ListView>(R.id.player_choice_list)
        val safeSelected = selectedIndex.coerceIn(0, items.lastIndex)
        listView.adapter = ChoiceAdapter(activity, items, safeSelected)
        listView.setSelection(safeSelected)
        listView.setItemChecked(safeSelected, true)
        listView.setOnItemClickListener { _, _, position, _ ->
            dialog.dismiss()
            listener.onSelected(position)
        }
        dialog.setOnDismissListener { anchor?.requestFocus() }

        dialog.setOnShowListener {
            val density = activity.resources.displayMetrics.density
            val width = (520 * density).toInt()
            val desiredHeight = ((84 + min(items.size, 8) * 60) * density).toInt()
            val maxHeight = (activity.resources.displayMetrics.heightPixels * 0.8f).toInt()
            dialog.window?.setLayout(width, min(desiredHeight, maxHeight))
            dialog.window?.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN)
            dialog.window?.decorView?.apply {
                if (ServiceLocator.reduceMotionMode() == "on") {
                    alpha = 1f
                } else {
                    alpha = 0f
                    animate().alpha(1f).setDuration(140L).start()
                }
            }
            listView.requestFocus()
            listView.post {
                listView.setSelection(safeSelected)
                listView.setItemChecked(safeSelected, true)
            }
        }
        dialog.show()
    }

    private class ChoiceAdapter(
        activity: Activity,
        private val items: Array<String>,
        private val selectedIndex: Int,
    ) : ArrayAdapter<String>(activity, R.layout.item_player_choice, items) {
        private val inflater = LayoutInflater.from(activity)

        override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
            val view = convertView ?: inflater.inflate(R.layout.item_player_choice, parent, false)
            view.findViewById<TextView>(R.id.player_choice_text).apply {
                text = items[position]
                isActivated = position == selectedIndex
            }
            return view
        }
    }
}
