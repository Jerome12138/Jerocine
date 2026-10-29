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
                "adFilter=1;autoPlayNext=1");
        assertEquals("1", m.get(WebSettingsContract.KEY_AD_FILTER));
        assertEquals("1", m.get(WebSettingsContract.KEY_AUTO_PLAY_NEXT));
    }

    @Test
    public void parse_toleratesJunk() {
        Map<String, String> m = WebSettingsContract.parse(
                "  adFilter = 0 ;;novalue;=novalue;;x=y");
        assertEquals("0", m.get(WebSettingsContract.KEY_AD_FILTER)); // 键值两侧空白被裁
        assertEquals("y", m.get("x"));
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
        assertFalse(WebSettingsContract.bool(m, WebSettingsContract.KEY_AUTO_PLAY_NEXT, false));
        assertFalse(WebSettingsContract.bool(WebSettingsContract.parse(null),
                WebSettingsContract.KEY_AD_FILTER, false));
    }

    @Test
    public void bool_acceptsNumericAndWordForms() {
        Map<String, String> m = WebSettingsContract.parse(
                "adFilter=1;autoPlayNext=true;extra=false");
        assertTrue(WebSettingsContract.bool(m, WebSettingsContract.KEY_AD_FILTER, false));
        assertTrue(WebSettingsContract.bool(m, WebSettingsContract.KEY_AUTO_PLAY_NEXT, false));
        assertFalse(WebSettingsContract.bool(m, "extra", true));
    }
}
