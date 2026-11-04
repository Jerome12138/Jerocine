package com.jerocine.player.ui;

import org.junit.Test;

import static org.junit.Assert.assertEquals;

/**
 * JcServerUrlPrompt 归一化纯逻辑 spec（组件化二期 A2 下沉）。
 *
 * <p>两壳旧实现各写一份归一化，下沉时统一到这里：web 壳 promptServerUrl
 * （trim + 缺协议头补 http://）∪ 原生壳 withScheme + ServerUrl.normalize
 * （去首尾空白 + 去尾斜杠）。空白输入返回空串，由调用方弹「地址不能为空」。</p>
 */
public class JcServerUrlPromptTest {

    @Test
    public void null_or_blank_returns_empty() {
        assertEquals("", JcServerUrlPrompt.normalizeCandidate(null));
        assertEquals("", JcServerUrlPrompt.normalizeCandidate(""));
        assertEquals("", JcServerUrlPrompt.normalizeCandidate("   "));
    }

    @Test
    public void trims_and_strips_trailing_slashes() {
        assertEquals("https://example.com",
                JcServerUrlPrompt.normalizeCandidate(" https://example.com/// "));
    }

    @Test
    public void missing_scheme_gets_http_prefix() {
        assertEquals("http://example.com", JcServerUrlPrompt.normalizeCandidate("example.com"));
        assertEquals("http://1.2.3.4:8080", JcServerUrlPrompt.normalizeCandidate("1.2.3.4:8080"));
        assertEquals("http://1.2.3.4:8080", JcServerUrlPrompt.normalizeCandidate(" 1.2.3.4:8080 "));
    }

    @Test
    public void existing_scheme_untouched() {
        assertEquals("https://example.com",
                JcServerUrlPrompt.normalizeCandidate("https://example.com"));
        assertEquals("http://example.com",
                JcServerUrlPrompt.normalizeCandidate("http://example.com"));
        assertEquals("https://a.b/c", JcServerUrlPrompt.normalizeCandidate("https://a.b/c"));
    }

    @Test
    public void scheme_added_after_trailing_slash_stripped() {
        // 先 normalize（去尾斜杠）再补协议头 —— 与两壳旧顺序一致
        assertEquals("http://example.com", JcServerUrlPrompt.normalizeCandidate("example.com/"));
    }
}
