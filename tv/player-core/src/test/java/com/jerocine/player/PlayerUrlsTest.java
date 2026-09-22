package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * 播放地址 / 线路策略的纯逻辑测试(不含 Android 依赖).
 */
public class PlayerUrlsTest {

    @Test
    public void m3u8IsWrappedWithProxyWhenAdFilterEnabled() {
        String url = PlayerUrls.buildPlayableUrl(
                "https://cdn.example.com/video/01.m3u8?token=a b",
                true, false, false, "https://example.com/api/");

        assertEquals(
                "https://example.com/api/v1/m3u8/proxy?src="
                        + "https%3A%2F%2Fcdn.example.com%2Fvideo%2F01.m3u8%3Ftoken%3Da+b"
                        + "&filterAds=1&proxyMedia=0",
                url);
    }

    @Test
    public void fullRelayOnlyForCompatibilityRetry() {
        String url = PlayerUrls.buildPlayableUrl(
                "https://cdn.example.com/video/01.m3u8", true, false, true,
                "http://example.com/api");

        assertTrue(url.endsWith("&filterAds=1&proxyMedia=1"));
    }

    @Test
    public void originalUrlKeptWhenProxyFallbackIsForced() {
        String raw = "https://cdn.example.com/video/01.m3u8";

        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, true, false, "https://example.com/api"));
    }

    @Test
    public void originalUrlKeptWhenAdFilterDisabledOrProxyMissing() {
        String raw = "https://cdn.example.com/video/01.m3u8";

        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, false, false, false, "https://example.com/api"));
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, false, ""));
        assertEquals(raw, PlayerUrls.buildPlayableUrl(raw, true, false, false, null));
    }

    @Test
    public void nonM3u8IsNeverWrapped() {
        String mp4 = "https://cdn.example.com/video/01.mp4";

        assertEquals(mp4, PlayerUrls.buildPlayableUrl(mp4, true, false, false, "https://example.com/api"));
    }

    @Test
    public void alreadyProxiedUrlIsNotWrappedTwice() {
        String proxied = "https://example.com/api/v1/m3u8/proxy?src=x&proxyMedia=0";

        assertEquals(proxied, PlayerUrls.buildPlayableUrl(proxied, true, false, false, "https://example.com/api"));
    }

    @Test
    public void relayRetryRequiresProxyManifestAndDirectCdnFailure() {
        String manifest = "http://example.com/api/v1/m3u8/proxy?src=x&proxyMedia=0";

        assertTrue(PlayerUrls.shouldRetryWithRelay(manifest, "https://cdn.example.com/video/seg01.ts"));
        assertFalse(PlayerUrls.shouldRetryWithRelay(manifest, manifest));
        assertFalse(PlayerUrls.shouldRetryWithRelay(
                manifest.replace("proxyMedia=0", "proxyMedia=1"),
                "https://cdn.example.com/video/seg01.ts"));
        assertFalse(PlayerUrls.shouldRetryWithRelay(
                "https://cdn.example.com/video/index.m3u8",
                "https://cdn.example.com/video/seg01.ts"));
    }

    @Test
    public void clientFilterSkipsManifestsAlreadyHandledByServerProxy() {
        assertFalse(PlayerUrls.needsClientSideFilter(
                "http://example.com/api/v1/m3u8/proxy?src=x&proxyMedia=1"));
        assertTrue(PlayerUrls.needsClientSideFilter("https://cdn.example.com/video/index.m3u8"));
        assertTrue(PlayerUrls.needsClientSideFilter(null));
    }
}
