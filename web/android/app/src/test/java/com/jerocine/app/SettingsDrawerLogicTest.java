package com.jerocine.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

/**
 * SettingsDrawerLogic 的 JVM 单测(本机可跑, 不需要设备/模拟器):
 *   cd web/android && ./gradlew :app:testDebugUnitTest
 */
public class SettingsDrawerLogicTest {

    // ============ 显示模式三态循环 ============

    @Test
    public void nextDisplayMode_tv_to_desktop() {
        assertEquals(SettingsDrawerLogic.MODE_DESKTOP, SettingsDrawerLogic.nextDisplayMode("tv"));
    }

    @Test
    public void nextDisplayMode_desktop_to_auto() {
        assertNull(SettingsDrawerLogic.nextDisplayMode("desktop"));
    }

    @Test
    public void nextDisplayMode_auto_to_tv() {
        assertEquals(SettingsDrawerLogic.MODE_TV, SettingsDrawerLogic.nextDisplayMode(null));
        assertEquals(SettingsDrawerLogic.MODE_TV, SettingsDrawerLogic.nextDisplayMode(""));
        // 脏值(比如手改 localStorage 塞了别的字符串)一律回 TV, 不卡死循环
        assertEquals(SettingsDrawerLogic.MODE_TV, SettingsDrawerLogic.nextDisplayMode("weird"));
    }

    @Test
    public void nextDisplayMode_is_a_closed_3_cycle() {
        String m = null;
        StringBuilder seen = new StringBuilder();
        for (int i = 0; i < 3; i++) {
            m = SettingsDrawerLogic.nextDisplayMode(m);
            seen.append(m == null ? "auto" : m).append(',');
        }
        // 起点自动 → tv → desktop → auto
        assertEquals("tv,desktop,auto,", seen.toString());
        m = SettingsDrawerLogic.nextDisplayMode(m); // auto → tv, 回到起点
        assertEquals(SettingsDrawerLogic.MODE_TV, m);
    }

    @Test
    public void displayModeLabel() {
        assertEquals("TV", SettingsDrawerLogic.displayModeLabel("tv"));
        assertEquals("桌面", SettingsDrawerLogic.displayModeLabel("desktop"));
        assertEquals("自动", SettingsDrawerLogic.displayModeLabel(null));
        assertEquals("自动", SettingsDrawerLogic.displayModeLabel(""));
        assertEquals("自动", SettingsDrawerLogic.displayModeLabel("weird"));
    }

    @Test
    public void displayModeToast() {
        assertEquals("已设为 TV 模式", SettingsDrawerLogic.displayModeToast("tv"));
        assertEquals("已设为桌面模式", SettingsDrawerLogic.displayModeToast("desktop"));
        assertEquals("已清除模式 · 自动检测", SettingsDrawerLogic.displayModeToast(null));
    }

    // ============ 下发给前端 window.__jcSetMode 的入参 ============
    // 契约: 只有 tv / desktop 是有效值; "自动"(=null/空/脏值) 必须下发 "auto",
    // 前端 normalizeNativeMode 收到非 tv/desktop 才会清除覆盖、回到壳内默认 TV。
    // 两边只要有一边把 null 当成 "null" 字符串之类, 抽屉的"自动"就会失效。

    @Test
    public void jsModeArg_valid_values_pass_through() {
        assertEquals("tv", SettingsDrawerLogic.jsModeArg(SettingsDrawerLogic.MODE_TV));
        assertEquals("desktop", SettingsDrawerLogic.jsModeArg(SettingsDrawerLogic.MODE_DESKTOP));
    }

    @Test
    public void jsModeArg_auto_and_garbage_become_auto() {
        assertEquals("auto", SettingsDrawerLogic.jsModeArg(null));
        assertEquals("auto", SettingsDrawerLogic.jsModeArg(""));
        assertEquals("auto", SettingsDrawerLogic.jsModeArg("TV"));      // 大小写不宽容
        assertEquals("auto", SettingsDrawerLogic.jsModeArg("mobile"));  // 壳内不提供 mobile
        assertEquals("auto", SettingsDrawerLogic.jsModeArg("weird"));
    }

    @Test
    public void jsModeArg_round_trips_with_nextDisplayMode() {
        // 三态循环产生的每个值都必须能安全下发
        String m = null;
        for (int i = 0; i < 3; i++) {
            m = SettingsDrawerLogic.nextDisplayMode(m);
            String arg = SettingsDrawerLogic.jsModeArg(m);
            assertEquals(m == null ? "auto" : m, arg);
        }
    }

    // ============ SPA 深链拼接 ============

    @Test
    public void spaSettingsUrl_plain() {
        assertEquals("https://jerocine.art/settings?group=account",
                SettingsDrawerLogic.spaSettingsUrl("https://jerocine.art", "account"));
    }

    @Test
    public void spaSettingsUrl_trailing_slash_normalized() {
        assertEquals("https://jerocine.art/settings?group=account",
                SettingsDrawerLogic.spaSettingsUrl("https://jerocine.art/", "account"));
        assertEquals("http://192.168.1.9:8080/settings",
                SettingsDrawerLogic.spaSettingsUrl("http://192.168.1.9:8080///", ""));
    }

    @Test
    public void spaSettingsUrl_no_group() {
        assertEquals("https://jerocine.art/settings",
                SettingsDrawerLogic.spaSettingsUrl("https://jerocine.art", null));
        assertEquals("https://jerocine.art/settings",
                SettingsDrawerLogic.spaSettingsUrl("https://jerocine.art", ""));
    }

    @Test
    public void spaSettingsUrl_handles_null_and_blank_base() {
        assertEquals("/settings?group=play", SettingsDrawerLogic.spaSettingsUrl(null, "play"));
        assertEquals("/settings", SettingsDrawerLogic.spaSettingsUrl("   ", ""));
    }
}
