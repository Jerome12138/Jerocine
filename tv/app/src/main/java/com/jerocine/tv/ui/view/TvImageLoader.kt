package com.jerocine.tv.ui.view

import android.graphics.drawable.ColorDrawable
import android.widget.ImageView
import androidx.core.content.ContextCompat
import coil.load
import com.jerocine.tv.R

fun ImageView.loadTvPoster(url: String, description: String?) {
    contentDescription = description
    load(url) {
        size(360, 480)
        crossfade(false)
        placeholder(ColorDrawable(ContextCompat.getColor(context, R.color.gf_glass_soft)))
        error(ColorDrawable(ContextCompat.getColor(context, R.color.gf_glass_soft)))
    }
}

fun ImageView.loadTvBackdrop(url: String) {
    load(url) {
        size(960, 540)
        crossfade(false)
        placeholder(ColorDrawable(ContextCompat.getColor(context, R.color.gf_bg)))
        error(ColorDrawable(ContextCompat.getColor(context, R.color.gf_bg)))
    }
}
