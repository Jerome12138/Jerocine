package com.jerocine.player;

import org.junit.Test;

import java.util.Map;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

/**
 * {@link WebSettingsContract} 纯逻辑单测 —— k=v; 快照契约与前端
 * useWebSettings.webSettingsSnapshot() 的双向一致(2026-10-18 web 播放设置联动)。
 */
public class WebSettingsContractTest {

    // ---- parse ----

    @Test
    public void parse_standardSnapshot() {
        Map<String, String> m = WebSettingsContract.parse(
                "adFilter=1;filterMode=server;relayCache=0;autoPlayNext=1");
        assertEquals("1", m.get(WebSettingsContract.KEY_AD_FILTER));
        assertEquals("server", m.get(WebSettingsContract.KEY_FILTER_MODE));
        assertEquals("0", m.get(WebSettingsContract.KEY_RELAY_CACHE));
        assertEquals("1", m.get(WebSettingsContract.KEY_AUTO_PLAY_NEXT));
    }

    @Test
    public void parse_toleratesJunk() {
        Map<String, String> m = WebSettingsContract.parse(
                "  adFilter = 0 ;;novalue;=novalue;;filterMode=server;x=");
        assertEquals("0", m.get(WebSettingsContract.KEY_AD_FILTER)); // 键值两侧空白被裁
        assertEquals("server", m.get(WebSettingsContract.KEY_FILTER_MODE));
        assertEquals(2, m.size()); // 无等号/空值段被丢弃
    }

    @Test
    public void parse_nullAndEmpty_giveEmptyMap() {
        assertTrue(WebSettingsContract.parse(null).isEmpty());
        assertTrue(WebSettingsContract.parse("").isEmpty());
    }

    // ---- bool ----

    @Test
    public void bool_usesDefaultForMissingOrInvalid() {
        Map<String, String> m = WebSettingsContract.parse("adFilter=bogus");
        assertTrue(WebSettingsContract.bool(m, WebSettingsContract.KEY_AD_FILTER, true));
        assertFalse(WebSettingsContract.bool(m, WebSettingsContract.KEY_RELAY_CACHE, false));
        assertFalse(WebSettingsContract.bool(WebSettingsContract.parse(null),
                WebSettingsContract.KEY_AD_FILTER, false));
    }

    @Test
    public void bool_acceptsNumericAndWordForms() {
        Map<String, String> m = WebSettingsContract.parse(
                "adFilter=1;relayCache=0;autoPlayNext=true;filterMode=client;extra=false");
        assertTrue(WebSettingsContract.bool(m, WebSettingsContract.KEY_AD_FILTER, false));
        assertFalse(WebSettingsContract.bool(m, WebSettingsContract.KEY_RELAY_CACHE, true));
        assertTrue(WebSettingsContract.bool(m, WebSettingsContract.KEY_AUTO_PLAY_NEXT, false));
        assertFalse(WebSettingsContract.bool(m, "extra", true));
    }

    // ---- filterMode ----

    @Test
    public void filterMode_normalizeDefaultsToClient() {
        assertEquals(WebSettingsContract.FILTER_MODE_CLIENT,
                WebSettingsContract.normalizeFilterMode(null));
        assertEquals(WebSettingsContract.FILTER_MODE_CLIENT,
                WebSettingsContract.normalizeFilterMode(""));
        assertEquals(WebSettingsContract.FILTER_MODE_CLIENT,
                WebSettingsContract.normalizeFilterMode("cloud"));
        assertEquals(WebSettingsContract.FILTER_MODE_SERVER,
                WebSettingsContract.normalizeFilterMode("server"));
        assertEquals(WebSettingsContract.FILTER_MODE_SERVER,
                WebSettingsContract.normalizeFilterMode("SERVER"));
    }

    @Test
    public void filterMode_nextCyclesBothWays() {
        assertEquals(WebSettingsContract.FILTER_MODE_SERVER,
                WebSettingsContract.nextFilterMode("client"));
        assertEquals(WebSettingsContract.FILTER_MODE_CLIENT,
                WebSettingsContract.nextFilterMode("server"));
        assertEquals(WebSettingsContract.FILTER_MODE_SERVER,
                WebSettingsContract.nextFilterMode(null)); // 未知值按 client → 下一个是 server
    }

    @Test
    public void filterMode_labels() {
        assertEquals("端侧取源", WebSettingsContract.filterModeLabel("client"));
        assertEquals("云端取源", WebSettingsContract.filterModeLabel("server"));
        assertEquals("端侧取源", WebSettingsContract.filterModeLabel(null));
        assertFalse(WebSettingsContract.isServerMode("client"));
        assertTrue(WebSettingsContract.isServerMode("server"));
    }
}
