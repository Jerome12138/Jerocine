package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * {@link DownloadEngine#redactUrls} 测试 — 落库前的签名 URL 脱敏。
 *
 * <p>为什么重要: 这个返回值会写进 {@code DownloadTask.error} 并显示在下载列表的错误列。
 * 源站 URL 常带时效签名({@code ?token=} {@code &sign=}), 落库 =持久化扩散。
 *
 * <p>本类是纯逻辑测试, 不加载 Android(故不直接测 ErrorDiag.safeUrl —— 它用 android.net.Uri)。
 */
public class RedactUrlsTest {

    @Test
    public void redactsBareSignedUrl() {
        String out = DownloadEngine.redactUrls("广告过滤失败: https://cdn.y.com/p/a.m3u8?token=SECRET");
        assertFalse(out.contains("SECRET"));
        assertFalse(out.contains("token="));
    }

    @Test
    public void redactsMultipleUrlsInOneMessage() {
        String out = DownloadEngine.redactUrls("failed https://a.b/c?sign=X and https://d.e/f?token=Y");
        assertFalse(out.contains("sign=X"));
        assertFalse(out.contains("token=Y"));
    }

    @Test
    public void keepsNonUrlText() {
        String msg = "清单获取失败: java.net.UnknownHostException: cdn.y.com";
        assertEquals(msg, DownloadEngine.redactUrls(msg));
    }

    @Test
    public void redactsWhenWrappedInParensOrQuotes() {
        assertFalse(DownloadEngine.redactUrls("地址 (https://cdn.y.com/a.m3u8?token=S) 结束")
                .contains("token="));
        assertFalse(DownloadEngine.redactUrls("地址\"https://cdn.y.com/a.m3u8?t=1\"结束")
                .contains("t=1"));
    }

    // URL 紧贴中文前缀时回溯会被截断, 早期实现会切出不含 http 的残段而泄漏 token。
    @Test
    public void redactsWhenGluedToChineseText() {
        String out = DownloadEngine.redactUrls("无分隔https://cdn.y.com/a?token=1然后很长的中文后缀");
        assertFalse("不能残留签名: " + out, out.contains("token="));
    }

    @Test
    public void keepsNonHttpScheme() {
        // ftp/magnet 不带签名, 原样保留(过度脱敏会让诊断信息变得难以阅读)
        String msg = "ftp://x.y/z";
        assertEquals(msg, DownloadEngine.redactUrls(msg));
    }

    @Test
    public void handlesNullAndEmpty() {
        assertNull(DownloadEngine.redactUrls(null));
        assertEquals("", DownloadEngine.redactUrls(""));
    }
}