package com.jerocine.tv.data

import org.junit.Assert.assertEquals
import org.junit.Test

class LocalSkipSettingTest {
    @Test
    fun upsertLocalSkipSetting_replacesExistingFilmAndClampsValues() {
        val result = upsertLocalSkipSetting(
            settings = listOf(
                SkipSetting(mid = 1, intro = 90, outro = 60),
                SkipSetting(mid = 2, intro = 20, outro = 30)
            ),
            setting = SkipSetting(mid = 1, intro = -10, outro = 45)
        )

        assertEquals(listOf(1L, 2L), result.map { it.mid })
        assertEquals(0, result.first().intro)
        assertEquals(45, result.first().outro)
    }
}
