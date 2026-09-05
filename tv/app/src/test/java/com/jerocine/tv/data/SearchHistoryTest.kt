package com.jerocine.tv.data

import org.junit.Assert.assertEquals
import org.junit.Test

class SearchHistoryTest {
    @Test
    fun addSearchHistory_trimsDeduplicatesIgnoringCaseAndMovesToFront() {
        val result = addSearchHistory(
            history = listOf("三体", "Matrix", "流浪地球"),
            keyword = " matrix ",
            limit = 3
        )

        assertEquals(listOf("matrix", "三体", "流浪地球"), result)
    }

    @Test
    fun addSearchHistory_ignoresBlankAndLimitsItems() {
        val unchanged = addSearchHistory(listOf("A"), "   ")
        val limited = addSearchHistory(listOf("A", "B"), "C", limit = 2)

        assertEquals(listOf("A"), unchanged)
        assertEquals(listOf("C", "A"), limited)
    }

    @Test
    fun removeSearchHistory_removesIgnoringCase() {
        val result = removeSearchHistory(listOf("三体", "Matrix"), "matrix")

        assertEquals(listOf("三体"), result)
    }

    @Test
    fun hotSearchKeywords_usesCategoryNamesAndLimitsItems() {
        val result = hotSearchKeywords(
            categories = listOf(
                NavCategory(name = "电影"),
                NavCategory(name = " "),
                NavCategory(name = "剧集"),
                NavCategory(name = "电影")
            ),
            limit = 2
        )

        assertEquals(listOf("电影", "剧集"), result)
    }

    @Test
    fun applySearchKeyboardKeyHandlesCharacterAndControls() {
        assertEquals("AB", applySearchKeyboardKey("A", "B"))
        assertEquals("A", applySearchKeyboardKey("AB", "BACKSPACE"))
        assertEquals("A ", applySearchKeyboardKey("A", "SPACE"))
        assertEquals("", applySearchKeyboardKey("AB", "CLEAR"))
    }
}
