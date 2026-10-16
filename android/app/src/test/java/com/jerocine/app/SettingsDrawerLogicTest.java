package com.jerocine.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

/**
 * SettingsDrawerLogic 的 JVM 单测(本机可跑, 不需要设备/模拟器):
 *   cd android && ./gradlew :app:testDebugUnitTest
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

    // ============ 账号行: __jcAuth.user() 报文解析 ============
    // 只覆盖"解析"(JSON → 登录态 + 昵称)。
    // 账号行文案(标题/副标题/按钮)已收敛到 core 的 JcSettingsDrawer.AccountInfo,
    // 由 player-core 的 AccountInfoTest 覆盖 —— 两壳共用同一份字面量。

    @Test
    public void parseAccountSnapshot_logged_in() {
        SettingsDrawerLogic.AccountSnapshot s =
                SettingsDrawerLogic.parseAccountSnapshot("{\"loggedIn\":true,\"name\":\"老王\"}");
        org.junit.Assert.assertTrue(s.loggedIn);
        assertEquals("老王", s.name);
    }

    @Test
    public void parseAccountSnapshot_logged_out() {
        SettingsDrawerLogic.AccountSnapshot s =
                SettingsDrawerLogic.parseAccountSnapshot("{\"loggedIn\":false,\"name\":\"\"}");
        org.junit.Assert.assertFalse(s.loggedIn);
        assertEquals("", s.name);
    }

    @Test
    public void parseAccountSnapshot_null_and_junk_become_logged_out() {
        // evaluateJavascript 的回调可能是 null / "null" / 空串 / 报错文本 —— 一律归一成未登录
        for (String junk : new String[] {null, "null", "", "   ", "not-json", "{\"a\":1}"}) {
            SettingsDrawerLogic.AccountSnapshot s = SettingsDrawerLogic.parseAccountSnapshot(junk);
            org.junit.Assert.assertFalse("junk=" + junk, s.loggedIn);
            assertEquals("", s.name);
        }
    }

    @Test
    public void parseAccountSnapshot_handles_escaped_quotes_in_name() {
        // 昵称理论上可含引号 —— JSON.stringify 会转义, 解析端要还原
        SettingsDrawerLogic.AccountSnapshot s = SettingsDrawerLogic.parseAccountSnapshot(
                "{\"loggedIn\":true,\"name\":\"a\\\"b\\\\c\"}");
        org.junit.Assert.assertTrue(s.loggedIn);
        assertEquals("a\"b\\c", s.name);
    }

    @Test
    public void parseAccountSnapshot_field_order_and_whitespace_tolerant() {
        // 字段顺序/空白变化不该影响解析(报文出自 JSON.stringify, 但别把实现绑死在形状上)
        SettingsDrawerLogic.AccountSnapshot s = SettingsDrawerLogic.parseAccountSnapshot(
                "{ \"name\" : \"小明\" , \"loggedIn\" : true }");
        org.junit.Assert.assertTrue(s.loggedIn);
        assertEquals("小明", s.name);
    }

    @Test
    public void parseAccountSnapshot_logged_in_without_name() {
        // 登录了但没有 name 字段 → name 归一成空串(由 core 的 loggedIn("") 渲染成"已登录")
        SettingsDrawerLogic.AccountSnapshot weird =
                SettingsDrawerLogic.parseAccountSnapshot("{\"loggedIn\":true}");
        org.junit.Assert.assertTrue(weird.loggedIn);
        assertEquals("", weird.name);
    }
}
