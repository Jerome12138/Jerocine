package com.jerocine.tv.data

fun addSearchHistory(
    history: List<String>,
    keyword: String,
    limit: Int = 10,
): List<String> {
    val trimmed = keyword.trim()
    if (trimmed.isBlank()) return history
    val lower = trimmed.lowercase()
    val filtered = history.filterNot { it.lowercase() == lower }
    return (listOf(trimmed) + filtered).take(limit.coerceAtLeast(0))
}

fun removeSearchHistory(history: List<String>, keyword: String): List<String> {
    val lower = keyword.trim().lowercase()
    return history.filterNot { it.lowercase() == lower }
}

fun hotSearchKeywords(categories: List<NavCategory>, limit: Int = 10): List<String> =
    categories
        .map { it.name.trim() }
        .filter { it.isNotBlank() }
        .distinct()
        .take(limit.coerceAtLeast(0))

fun applySearchKeyboardKey(value: String, key: String): String = when (key) {
    "BACKSPACE" -> value.dropLast(1)
    "CLEAR" -> ""
    "SPACE" -> value + " "
    else -> value + key
}
