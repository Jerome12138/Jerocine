package com.jerocine.player

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Q⑤(player-fix 2026-11-05) — 直播媒体表判定: 无 `#EXT-X-ENDLIST` 且含 `#EXT-X-MEDIA-SEQUENCE`。
 *
 * 命中 ⇒ 播放路径跳过 M3u8FilterCore 剔段(仅 URI 绝对化): 剔段三维启发
 * (cross/num/short)都基于"窗口内容静态"假设, 对滚动滑窗是纯干扰面。
 * master 多变体表无 MEDIA-SEQUENCE, 不会命中(且本就走 master-direct 直通)。
 */
class LiveMediaPlaylistDetectionTest {

    @Test
    fun liveWindow_playlistDetected() {
        val body = """
            #EXTM3U
            #EXT-X-VERSION:3
            #EXT-X-TARGETDURATION:8
            #EXT-X-MEDIA-SEQUENCE:12345
            #EXTINF:8.0,
            seg1.ts
            #EXTINF:8.0,
            seg2.ts
        """.trimIndent()
        assertTrue(M3u8FilterCore.isLiveMediaPlaylist(body))
    }

    @Test
    fun eventEnded_withEndList_notLive() {
        // 直播结束定档(ENDLIST 落盘): 已是静态表, 剔段照常
        val body = """
            #EXTM3U
            #EXT-X-TARGETDURATION:8
            #EXT-X-MEDIA-SEQUENCE:0
            #EXTINF:8.0,
            seg1.ts
            #EXT-X-ENDLIST
        """.trimIndent()
        assertFalse(M3u8FilterCore.isLiveMediaPlaylist(body))
    }

    @Test
    fun vodWithoutMediaSequence_notLive() {
        // 缺 MEDIA-SEQUENCE 的普通 VOD(哪怕也缺 ENDLIST)不判直播 —— 两个条件必须同时满足
        val body = """
            #EXTM3U
            #EXT-X-TARGETDURATION:8
            #EXTINF:8.0,
            seg1.ts
        """.trimIndent()
        assertFalse(M3u8FilterCore.isLiveMediaPlaylist(body))
    }

    @Test
    fun masterPlaylist_notLive() {
        // master 多变体表没有 MEDIA-SEQUENCE —— 不得误命中
        val body = """
            #EXTM3U
            #EXT-X-STREAM-INF:BANDWIDTH=1280000
            720p.m3u8
            #EXT-X-STREAM-INF:BANDWIDTH=640000
            480p.m3u8
        """.trimIndent()
        assertFalse(M3u8FilterCore.isLiveMediaPlaylist(body))
    }

    @Test
    fun endListWins_overMediaSequence() {
        // MEDIA-SEQUENCE 与 ENDLIST 并存(直播结束) → 以 ENDLIST 为准, 判非直播
        val body = """
            #EXT-X-MEDIA-SEQUENCE:100
            #EXTINF:6.0,
            a.ts
            #EXT-X-ENDLIST
        """.trimIndent()
        assertFalse(M3u8FilterCore.isLiveMediaPlaylist(body))
    }

    @Test
    fun emptyBody_notLive() {
        assertFalse(M3u8FilterCore.isLiveMediaPlaylist(""))
    }

    @Test
    fun whitespaceAroundTag_tolerated() {
        // 与内核其它行判定一致: trim 后比对前缀
        val body = "  #EXT-X-MEDIA-SEQUENCE:5  \n\t#EXTINF:6.0,\na.ts\n"
        assertTrue(M3u8FilterCore.isLiveMediaPlaylist(body))
    }
}
