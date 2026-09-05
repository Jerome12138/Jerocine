package com.jerocine.tv.ui

import com.jerocine.tv.data.Card
import com.jerocine.tv.data.FavoriteItem

data class FavoritesTvModel(
    val sourceChip: String,
    val subtitle: String,
)

fun deriveFavoritesTvModel(isLoggedIn: Boolean, count: Int): FavoritesTvModel {
    val sourceChip = if (isLoggedIn) "云端" else "本地"
    val sourceLabel = if (isLoggedIn) "云端收藏 · 跨设备同步" else "本地收藏 · 仅当前设备"
    return FavoritesTvModel(
        sourceChip = sourceChip,
        subtitle = "$sourceLabel · 共 $count 部",
    )
}

fun favoriteCards(items: List<FavoriteItem>): List<Card> = items.map(FavoriteItem::previewCard)
