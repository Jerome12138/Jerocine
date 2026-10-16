package com.jerocine.tv.ui

import com.jerocine.tv.data.FilmFilters
import com.jerocine.tv.data.FilterTag
import org.junit.Assert.assertEquals
import org.junit.Test

class CategoryLibraryModelTest {
    @Test
    fun mapsServerOrderTitlesOptionsAndSelection() {
        val content = CategoryLibraryContent(
            filters = FilmFilters(
                titles = mapOf("Area" to "地区", "Year" to "年份"),
                tags = mapOf(
                    "Area" to listOf(FilterTag("全部", ""), FilterTag("大陆", "大陆")),
                    "Year" to listOf(FilterTag("2026", "2026")),
                ),
                sortList = listOf("Area", "Missing", "Year"),
            ),
            selections = mapOf("Area" to "大陆"),
        )

        val groups = categoryFilterGroups(content)

        assertEquals(listOf("Area", "Year"), groups.map { it.key })
        assertEquals(listOf("地区", "年份"), groups.map { it.title })
        assertEquals("大陆", groups.first().current)
        assertEquals(listOf("全部", "大陆"), groups.first().options.map { it.name })
    }
}
