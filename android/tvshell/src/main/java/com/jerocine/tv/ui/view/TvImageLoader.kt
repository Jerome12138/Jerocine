package com.jerocine.tv.ui.view

import android.graphics.drawable.ColorDrawable
import android.widget.ImageView
import androidx.core.content.ContextCompat
import coil.load
import com.jerocine.player.R as PlayerR

/**
 * 占位 / 失败底色取自 player-core 的公共设计令牌.
 *
 * 本次资源收敛后, jc_glass_soft / jc_bg 只在 player-core 保留一份(壳里不再重复定义),
 * 而 AGP 的非传递 R 类下壳的 R 不含库资源, 因此这里改用库的 R 引用.
 */
fun ImageView.loadTvPoster(url: String, description: String?) {
    contentDescription = description
    load(url) {
        size(360, 480)
        crossfade(false)
        placeholder(ColorDrawable(ContextCompat.getColor(context, PlayerR.color.jc_glass_soft)))
        error(ColorDrawable(ContextCompat.getColor(context, PlayerR.color.jc_glass_soft)))
    }
}

fun ImageView.loadTvBackdrop(url: String) {
    load(url) {
        size(960, 540)
        crossfade(false)
        placeholder(ColorDrawable(ContextCompat.getColor(context, PlayerR.color.jc_bg)))
        error(ColorDrawable(ContextCompat.getColor(context, PlayerR.color.jc_bg)))
    }
}
