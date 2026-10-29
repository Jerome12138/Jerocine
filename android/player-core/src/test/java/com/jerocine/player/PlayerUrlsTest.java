package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.Arrays;
import java.util.List;

/**
 * 播放地址工具的纯逻辑测试(不含 Android 依赖).
 *
 * 服务端 m3u8 代理/中转已下线, 端侧本地过滤是唯一路径;
 * 保留的判形(isM3u8)/master 识别(hasMediaSegments)/绝对化(absolutize)/
 * 子表提取(childPlaylistUrls)服务于本地过滤与预取下钻。
 */
public class PlayerUrlsTest {

@Test
    public void m3u8DetectionCoversQueryAndFragment() {
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/a.m3u8"));
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/a.m3u8?x=1"));
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/a.M3U8#t"));
        assertFalse(PlayerUrls.isM3u8("https://cdn.example.com/a.mp4"));
        assertFalse(PlayerUrls.isM3u8(null));
    }

    // 放宽 isM3u8 的回归: 大量真实 HLS 源不以 .m3u8 结尾, 旧判据一律 false →
    // 中转/端侧失败升级/角标/子表预取四处连锁失效。
    @Test
    public void m3u8DetectionCoversRealWorldShapes() {
        // 尾部斜杠: 部分 CDN 的 /playlist.m3u8/ 同样返回清单
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/live/playlist.m3u8/"));
        // 无扩展名
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/hls/master"));
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/hls/master#frag"));
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/live/index.HLS"));
        // m3u8 写在查询串里
        assertTrue(PlayerUrls.isM3u8("https://api.example.com/play?type=m3u8&url=abc"));
        // 组合: query + fragment
        assertTrue(PlayerUrls.isM3u8("https://cdn.example.com/video/main.m3u8?token=1#frag"));
        // 不能放宽过头
        assertFalse(PlayerUrls.isM3u8("https://cdn.example.com/movie.mp4"));
        assertFalse(PlayerUrls.isM3u8("https://cdn.example.com/"));
        assertFalse(PlayerUrls.isM3u8(""));
    }

    // ==================== 子清单提取(端侧预取下钻一层) ====================
    // 端侧路径每集要过滤两次(master + 子表): 服务端 FilterText 只绝对化、不改写子表地址,
    // 所以预取必须把子表也一并做掉, 否则切集仍卡在子表那次 POST 上。

    @Test
    public void childPlaylistUrlsPicksStreamInfAndMediaUris() {
        String master = String.join("\n",
                "#EXTM3U",
                "#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",NAME=\"中文\",URI=\"audio/cn.m3u8\"",
                "#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=1280x720",
                "720p/index.m3u8",
                "#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1920x1080",
                "https://cdn.example.com/1080p/index.m3u8?token=x");

        List<String> children = PlayerUrls.childPlaylistUrls(master, 5);

        assertEquals(Arrays.asList(
                "audio/cn.m3u8",
                "720p/index.m3u8",
                "https://cdn.example.com/1080p/index.m3u8?token=x"), children);
    }

    @Test
    public void childPlaylistUrlsRespectsMaxAndDedupes() {
        String master = String.join("\n",
                "#EXT-X-STREAM-INF:BANDWIDTH=1",
                "a.m3u8",
                "#EXT-X-STREAM-INF:BANDWIDTH=2",
                "b.m3u8",
                "#EXT-X-STREAM-INF:BANDWIDTH=3",
                "a.m3u8",
                "#EXT-X-STREAM-INF:BANDWIDTH=4",
                "c.m3u8");

        assertEquals(Arrays.asList("a.m3u8", "b.m3u8"), PlayerUrls.childPlaylistUrls(master, 2));
        assertEquals(Arrays.asList("a.m3u8", "b.m3u8", "c.m3u8"),
                PlayerUrls.childPlaylistUrls(master, 9));
    }

    @Test
    public void childPlaylistUrlsSkipsCommentsAndBlankLines() {
        String master = String.join("\n",
                "#EXT-X-STREAM-INF:BANDWIDTH=1",
                "# 中间还夹了注释",
                "",
                "  sub/index.m3u8  ");

        assertEquals(Arrays.asList("sub/index.m3u8"), PlayerUrls.childPlaylistUrls(master, 5));
    }

    @Test
    public void childPlaylistUrlsIgnoresMediaPlaylistAndNonM3u8() {
        // 媒体清单(只有分片) → 没有子清单
        String media = String.join("\n",
                "#EXTM3U",
                "#EXTINF:6.0,",
                "seg001.ts",
                "#EXTINF:6.0,",
                "seg002.ts");
        assertTrue(PlayerUrls.childPlaylistUrls(media, 5).isEmpty());

        // 标了 #EXT-X-STREAM-INF 但目标不是 m3u8 → 不收(别把分片/MPD 当清单预取)
        String odd = String.join("\n",
                "#EXT-X-STREAM-INF:BANDWIDTH=1",
                "https://cdn.example.com/video/playlist.mpd");
        assertTrue(PlayerUrls.childPlaylistUrls(odd, 5).isEmpty());

        assertTrue(PlayerUrls.childPlaylistUrls(null, 5).isEmpty());
        assertTrue(PlayerUrls.childPlaylistUrls("#EXTM3U", 0).isEmpty());
    }

    // ==================== master 识别与子表绝对化(免 POST 加速, 2026-10-19) ====================
    // master 主表无分片可剔, 端侧过滤对它只是 URI 绝对化(ExoPlayer 本来就会做)
    // ⇒ 整层免 POST: 播放路径(parse)与预取器共用 hasMediaSegments 判据。

    @Test
    public void hasMediaSegmentsDistinguishesMediaFromMaster() {
        // master: 只有变体声明 → false(免 POST)
        String master = String.join("\n",
                "#EXTM3U",
                "#EXT-X-STREAM-INF:BANDWIDTH=800000",
                "720p/index.m3u8");
        assertFalse(PlayerUrls.hasMediaSegments(master));
        assertFalse(PlayerUrls.hasMediaSegments("#EXTM3U"));
        assertFalse(PlayerUrls.hasMediaSegments(null));
        assertFalse(PlayerUrls.hasMediaSegments(""));

        // 媒体列表: 有 #EXTINF → true(要 POST 过滤)
        String media = String.join("\n",
                "#EXTM3U",
                "#EXT-X-TARGETDURATION:6",
                "#EXTINF:6.0,",
                "seg001.ts");
        assertTrue(PlayerUrls.hasMediaSegments(media));
        // 带 \r\n 行尾(有的源站用 CRLF)也要认出来
        assertTrue(PlayerUrls.hasMediaSegments("#EXTM3U\r\n#EXTINF:6.0,\r\nseg001.ts"));
        // 小写 extinf 不是合法标签, 但判据按宽松前缀走(源站行为不可控, 误判方向是"多过滤一层", 安全)
        assertTrue(PlayerUrls.hasMediaSegments("#extinf:6.0,"));
    }

    @Test
    public void absolutizePassesThroughAbsoluteUrls() {
        assertEquals("https://cdn.com/a.m3u8",
                PlayerUrls.absolutize("https://cdn.com/a.m3u8", "https://src.com/x/master.m3u8"));
        assertEquals("http://cdn.com/a.m3u8",
                PlayerUrls.absolutize("http://cdn.com/a.m3u8", "https://src.com/x/master.m3u8"));
    }

    @Test
    public void absolutizeResolvesRelativeRefsAgainstBase() {
        String base = "https://src.com/vod/ep1/master.m3u8?token=abc";
        // 路径相对
        assertEquals("https://src.com/vod/ep1/720p/index.m3u8",
                PlayerUrls.absolutize("720p/index.m3u8", base));
        // 根相对
        assertEquals("https://src.com/hls/ep1.m3u8",
                PlayerUrls.absolutize("/hls/ep1.m3u8", base));
        // 协议相对
        assertEquals("https://cdn.src.com/hls/ep1.m3u8",
                PlayerUrls.absolutize("//cdn.src.com/hls/ep1.m3u8", base));
        // ".." 消化
        assertEquals("https://src.com/vod/ep0/index.m3u8",
                PlayerUrls.absolutize("../ep0/index.m3u8", base));
        // 标准 RFC 3986 由 java.net.URI 承担: query 里的空格不让 URI 拒绝解析的场景走手工兜底
        assertEquals("https://src.com/vod/ep1/sub.m3u8",
                PlayerUrls.absolutize("sub.m3u8", "https://src.com/vod/ep1/master.m3u8?a=b c"));
    }

    @Test
    public void absolutizeReturnsNullForUnresolvableInput() {
        assertNull(PlayerUrls.absolutize(null, "https://src.com/a.m3u8"));
        assertNull(PlayerUrls.absolutize("", "https://src.com/a.m3u8"));
        assertNull(PlayerUrls.absolutize("b.m3u8", null));
        assertNull(PlayerUrls.absolutize("b.m3u8", ""));
        // base 连 scheme 都没有 + ref 带非法字符(java.net.URI 拒绝, 手工兜底也找不到 "://")
        assertNull(PlayerUrls.absolutize("a b.m3u8", "not-a-url"));
    }
}
