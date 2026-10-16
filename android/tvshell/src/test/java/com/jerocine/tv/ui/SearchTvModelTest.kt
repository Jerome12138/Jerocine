package com.jerocine.tv.ui

import com.jerocine.tv.data.Card
import org.junit.Assert.assertEquals
import org.junit.Test

class SearchTvModelTest {
    @Test
    fun keyboardKeysKeepTvOrder() {
        assertEquals("A", searchKeyboardKeys().first())
        assertEquals(listOf("退格", "空格", "清空", "搜索"), searchKeyboardKeys().takeLast(4))
    }

    @Test
    fun derivesBoundedSearchTvModelAndEmptyCopy() {
        val model = deriveSearchTvModel(
            keyword = " 三体 ",
            hotKeywords = (1..10).map { "热词$it" },
            histories = (1..8).map { "历史$it" },
            results = (1..20).map { Card(mid = it.toLong(), name = "影片$it") },
        )

        assertEquals("三体", model.query)
        assertEquals(8, model.hotKeywords.size)
        assertEquals(6, model.histories.size)
        assertEquals(12, model.results.size)
        assertEquals("未查询到对应影片", model.emptyTitle)
        assertEquals("换一个关键词试试，或从首页分类发现内容", model.emptyDesc)
    }

    @Test
    fun derivesBlankSearchHintCopy() {
        val model = deriveSearchTvModel(
            keyword = " ",
            hotKeywords = emptyList(),
            histories = emptyList(),
            results = emptyList(),
        )

        assertEquals("", model.query)
        assertEquals("开始你的搜索", model.emptyTitle)
        assertEquals("输入片名或拼音首字母", model.emptyDesc)
    }
}
