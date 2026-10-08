package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.Arrays;
import java.util.List;

/**
 * 播放地址 / 线路策略的纯逻辑测试(不含 Android 依赖).
 *
 * 选路不变量(过滤开 + m3u8):
 *   默认 → **原始地址**(设备抓清单 + 端侧过滤, 与 web 播放页一致);
 *   只有"本集端侧过滤失败升级"(forceProxy) 或"要中转分片"(relay) 才包 /m3u8/proxy;
 *   服务端抓不到该源(proxyUsable=false) 时任何代理包装都无意义 → 仍给原始地址。
 */
public class PlayerUrlsTest {

    private static final String BASE = "https://example.com/api";

    @Test
    public void clientSideFilterIsTheDefaultPathSoNoProxyIsWrapped() {
        String raw = "https://cdn.example.com/video/01.m3u8";

        String playable = PlayerUrls.buildPlayableUrl(raw, true, false, false, false, true, BASE);

        assertEquals(raw, playable);
        // 关键: 必须是"非 proxy 清单", 端侧混合过滤才会被触发(proxy 清单会被短路跳过)
        assertTrue(PlayerUrls.needsClientSideFilter(playable));
    }

    @Test
    public void proxyIsWrappedWithProperEncodingAfterClientFilterFailure() {
        String url = PlayerUrls.buildPlayableUrl(
                "https://cdn.example.com/video/01.m3u8?token=a b",
                true, false, true, false, true, BASE + "/");

        assertEquals(
                BASE + "/v1/m3u8/proxy?src="
                        + "https%3A%2F%2Fcdn.example.com%2Fvideo%2F01.m3u8%3Ftoken%3Da+b"
                        + "&filterAds=1&proxyMedia=0",
                url);
        // 已由服务端过滤 → 端侧不再重复 POST
        assertFalse(PlayerUrls.needsClientSideFilter(url));
    }

    @Test
    public void relayUsesFullProxyMediaAndIsOnlyOnDemand() {
        String raw = "https://cdn.example.com/video/01.m3u8";

        // 没开中转也没升级代理 → 仍是原始地址(端侧过滤)
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, false, false, true, BASE));
        // 中转开(或本集直连自愈) → 包代理且 proxyMedia=1(分片也过服务器转发)
        assertTrue(PlayerUrls.buildPlayableUrl(raw, true, false, false, true, true, BASE)
                .endsWith("&filterAds=1&proxyMedia=1"));
    }

    /**
     * 服务端抓不到该源(adFilterOk=false)时任何代理包装都无意义 —— 升级代理与中转都不生效,
     * 只能"设备抓清单 + /v1/m3u8/filter 端侧过滤"(拿到的必须是非 proxy 清单)。
     */
    @Test
    public void unreachableSourceNeverGetsProxyWrapped() {
        String raw = "https://cdn.example.com/video/01.m3u8";

        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, false, false, false, BASE));
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, true, false, false, BASE));
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, false, true, false, BASE));
        assertTrue(PlayerUrls.needsClientSideFilter(
                PlayerUrls.buildPlayableUrl(raw, true, false, true, false, false, BASE)));
    }

    @Test
    public void originalUrlKeptWhenRawFallbackIsForced() {
        String raw = "https://cdn.example.com/video/01.m3u8";

        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, true, false, false, true, BASE));
        // forceRaw 优先于 forceProxy / relay(回退直连后不再包代理)
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, true, true, true, true, BASE));
    }

    @Test
    public void originalUrlKeptWhenAdFilterDisabledOrProxyMissing() {
        String raw = "https://cdn.example.com/video/01.m3u8";

        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, false, false, true, true, true, BASE));
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, true, false, true, ""));
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, true, false, true, null));
    }

    @Test
    public void nonM3u8IsNeverWrapped() {
        String mp4 = "https://cdn.example.com/video/01.mp4";

        assertEquals(mp4, PlayerUrls.buildPlayableUrl(mp4, true, false, true, true, true, BASE));
    }

    @Test
    public void alreadyProxiedUrlIsNotWrappedTwice() {
        String proxied = "https://example.com/api/v1/m3u8/proxy?src=x&proxyMedia=0";

        assertEquals(proxied, PlayerUrls.buildPlayableUrl(proxied, true, false, true, false, true, BASE));
    }

    /**
     * 中转自愈判据: 只要"失败的不是代理请求本身"且当前不在全量中转 → 可升级中转。
     * 与旧版的差别: 端侧过滤成为主路径后当前地址是**原始 m3u8**, 这种情况同样要能自愈。
     */
    @Test
    public void relayRetryTriggersFromDirectPlaybackToo() {
        String proxyManifest = "http://example.com/api/v1/m3u8/proxy?src=x&proxyMedia=0";
        String rawManifest = "https://cdn.example.com/video/index.m3u8";
        String seg = "https://cdn.example.com/video/seg01.ts";

        assertTrue(PlayerUrls.shouldRetryWithRelay(proxyManifest, seg));
        assertTrue(PlayerUrls.shouldRetryWithRelay(rawManifest, seg));
        assertTrue(PlayerUrls.shouldRetryWithRelay(rawManifest, rawManifest));
        // 已经是全量中转 → 不再升级(否则打转)
        assertFalse(PlayerUrls.shouldRetryWithRelay(
                proxyManifest.replace("proxyMedia=0", "proxyMedia=1"), seg));
        assertFalse(PlayerUrls.shouldRetryWithRelay(
                rawManifest.replace("index.m3u8", "index.m3u8?proxyMedia=1"), seg));
        // 挂的是代理本身 → 该走"回退原始", 不是中转
        assertFalse(PlayerUrls.shouldRetryWithRelay(proxyManifest, proxyManifest));
        // 没有具体失败请求(解码器/格式错误等) → 不误判为中转
        assertFalse(PlayerUrls.shouldRetryWithRelay(rawManifest, ""));
        assertFalse(PlayerUrls.shouldRetryWithRelay(rawManifest, null));
        assertFalse(PlayerUrls.shouldRetryWithRelay(null, seg));
    }

    @Test
    public void clientFilterSkipsManifestsAlreadyHandledByServerProxy() {
        assertFalse(PlayerUrls.needsClientSideFilter(
                "http://example.com/api/v1/m3u8/proxy?src=x&proxyMedia=1"));
        assertTrue(PlayerUrls.needsClientSideFilter("https://cdn.example.com/video/index.m3u8"));
        assertTrue(PlayerUrls.needsClientSideFilter(null));
    }

    @Test
    public void clientFilterSkipsLocalPlaylists() {
        // 离线/本地清单(下载时已过滤)不再付 POST; 在线源站仍需要端侧过滤
        assertFalse(PlayerUrls.needsClientSideFilter(
                "file:///data/user/0/art.jerocine.tv/cache/download_cache/149293/79/playlist.m3u8"));
        assertFalse(PlayerUrls.needsClientSideFilter(
                "content://com.android.externalstorage/raw/movie.m3u8"));
        assertFalse(PlayerUrls.needsClientSideFilter("FILE:///sdcard/a.m3u8"));
        assertTrue(PlayerUrls.needsClientSideFilter("https://cdn.example.com/video/index.m3u8"));
    }

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
}
