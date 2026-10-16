package com.jerocine.tv.ui

import com.jerocine.tv.data.Card

data class SearchTvModel(
    val query: String,
    val hotKeywords: List<String>,
    val histories: List<String>,
    val results: List<Card>,
    val emptyTitle: String,
    val emptyDesc: String,
)

fun searchKeyboardKeys(): List<String> =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789".map(Char::toString) +
        listOf("退格", "空格", "清空", "搜索")

fun deriveSearchTvModel(
    keyword: String,
    hotKeywords: List<String>,
    histories: List<String>,
    results: List<Card>,
): SearchTvModel {
    val query = keyword.trim()
    return SearchTvModel(
        query = query,
        hotKeywords = hotKeywords.take(8),
        histories = histories.take(6),
        results = results.take(12),
        emptyTitle = if (query.isBlank()) "开始你的搜索" else "未查询到对应影片",
        emptyDesc = if (query.isBlank()) {
            "输入片名或拼音首字母"
        } else {
            "换一个关键词试试，或从首页分类发现内容"
        },
    )
}
