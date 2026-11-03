package com.jerocine.tv.data

import com.jerocine.tv.data.LiveLastViewed.Selection
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class LiveLastViewedTest {

    @Test
    fun roundTripsSelectionWithNamesAndTimestamp() {
        val sel = Selection(sourceId = 3, index = 42, sourceName = "iptv-org", channelName = "CCTV-1", ts = 1_700_000_000_000L)
        assertEquals(sel, LiveLastViewed.decode(LiveLastViewed.encode(sel)))
    }

    @Test
    fun decodesMissingOptionalNamesAndTimestamp() {
        assertEquals(Selection(2, 0), LiveLastViewed.decode("""{"sourceId":2,"index":0}"""))
    }

    @Test
    fun rejectsMalformedOrInvalidValues() {
        assertNull(LiveLastViewed.decode(null))
        assertNull(LiveLastViewed.decode(""))
        assertNull(LiveLastViewed.decode("not json"))
        assertNull(LiveLastViewed.decode("""{"sourceId":0,"index":1}""")) // 非正源 id
        assertNull(LiveLastViewed.decode("""{"sourceId":1,"index":-1}""")) // 负下标
    }

    @Test
    fun acceptsBoundaryValues() {
        assertEquals(Selection(1, 0), LiveLastViewed.decode("""{"sourceId":1,"index":0}"""))
    }
}
