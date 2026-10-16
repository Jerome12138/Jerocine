package com.jerocine.tv.data

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PlayerEventTest {
    @Test
    fun playerHistoryEventsIncludeEpisodeChange() {
        assertTrue(isPlayerHistoryEvent("playerProgress"))
        assertTrue(isPlayerHistoryEvent("playerClosed"))
        assertTrue(isPlayerHistoryEvent("playerEpisodeChange"))
        assertFalse(isPlayerHistoryEvent("skipSettingChanged"))
    }
}
