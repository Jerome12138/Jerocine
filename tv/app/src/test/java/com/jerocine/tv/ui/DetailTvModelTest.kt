package com.jerocine.tv.ui

import com.jerocine.tv.data.FilmDetail
import org.junit.Assert.assertEquals
import org.junit.Test

class DetailTvModelTest {
    @Test
    fun derivesWebTvDetailChipsMetaAndCleanSummary() {
        val detail = FilmDetail(
            year = 2026,
            cName = "电影",
            area = "大陆",
            classTag = "动作, 悬疑 / 犯罪 科幻 战争 喜剧",
            remarks = "更新至10集",
            language = "国语",
            director = "张三,李四/王五",
            actor = "演员一,演员二",
            content = "<p>第一行&nbsp;</p><br>第二行&amp;更多"
        )

        val model = deriveDetailTvModel(detail)

        assertEquals(listOf("2026", "电影", "大陆", "动作", "悬疑", "犯罪"), model.chips)
        assertEquals(
            listOf(
                DetailTvMeta("状态", "更新至10集"),
                DetailTvMeta("语言", "国语"),
                DetailTvMeta("导演", "张三 / 李四 / 王五"),
            ),
            model.meta
        )
        assertEquals("第一行 第二行&更多", model.summary)
    }
}
