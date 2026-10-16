package com.jerocine.player.ui;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNotEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * 壳模式取值契约单测。
 *
 * <p>背景(2026-10-12 真机回归): 写入端(设置抽屉 / web 壳 WebView 降级)统一存小写
 * {@code auto/web/native}, 读取端 {@code DeviceProbe.readUserMode} 却只认枚举大写名 ——
 * 用户选「Web」/「原生」被静默忽略。本测试锁住"抽屉侧"的取值口径, 与
 * {@code DeviceProbeTest.readUserMode_*} 一起构成两端契约的双向保护。</p>
 */
public class ShellModeContractTest {

    // ---- nextShellMode: 三态循环 ----

    @Test
    public void nextShellMode_cyclesAutoWebNative() {
        assertEquals("web", JcSettingsDrawer.nextShellMode("auto"));
        assertEquals("native", JcSettingsDrawer.nextShellMode("web"));
        assertEquals("auto", JcSettingsDrawer.nextShellMode("native"));
    }

    @Test
    public void nextShellMode_threeClicksReturnsToStart() {
        String mode = "auto";
        for (int i = 0; i < 3; i++) {
            mode = JcSettingsDrawer.nextShellMode(mode);
        }
        assertEquals("auto", mode);
    }

    // ---- 归一: 历史大写值 / 空白 / null ----

    @Test
    public void nextShellMode_normalizesLegacyUppercase() {
        assertEquals("auto", JcSettingsDrawer.nextShellMode("NATIVE"));
        assertEquals("native", JcSettingsDrawer.nextShellMode("WEB"));
        assertEquals("web", JcSettingsDrawer.nextShellMode("AUTO"));
    }

    @Test
    public void nextShellMode_unknownOrNullActsAsAuto() {
        assertEquals("web", JcSettingsDrawer.nextShellMode(null));
        assertEquals("web", JcSettingsDrawer.nextShellMode(""));
        assertEquals("web", JcSettingsDrawer.nextShellMode("   "));
        assertEquals("web", JcSettingsDrawer.nextShellMode("bogus"));
    }

    // ---- shellModeLabel: 展示文案 ----

    @Test
    public void shellModeLabel_mapsLowercaseValues() {
        assertEquals("自动", JcSettingsDrawer.shellModeLabel("auto"));
        assertEquals("Web", JcSettingsDrawer.shellModeLabel("web"));
        assertEquals("原生", JcSettingsDrawer.shellModeLabel("native"));
    }

    @Test
    public void shellModeLabel_normalizesLegacyUppercase() {
        assertEquals("Web", JcSettingsDrawer.shellModeLabel("WEB"));
        assertEquals("原生", JcSettingsDrawer.shellModeLabel("NATIVE"));
        assertEquals("自动", JcSettingsDrawer.shellModeLabel("AUTO"));
    }

    @Test
    public void shellModeLabel_unknownOrNullIsAuto() {
        assertEquals("自动", JcSettingsDrawer.shellModeLabel(null));
        assertEquals("自动", JcSettingsDrawer.shellModeLabel(""));
        assertEquals("自动", JcSettingsDrawer.shellModeLabel("bogus"));
    }

    /**
     * 写入端值域恒定 —— 若将来有人改 nextShellMode 的值域而忘了同步读取端,
     * 这里先变红(而不是等真机上出现"点了没反应")。
     */
    @Test
    public void nextShellMode_writesOnlyKnownValues() {
        String mode = "auto";
        for (int i = 0; i < 3; i++) {
            mode = JcSettingsDrawer.nextShellMode(mode);
            boolean known = "auto".equals(mode) || "web".equals(mode) || "native".equals(mode);
            assertTrue("第 " + (i + 1) + " 次循环写出非法值: " + mode, known);
        }
    }

    // ---- shellModeSwitchMessage: 切壳后的后果说明(2026-10-12 需求) ----

    /**
     * 三档都要说清"会变成什么样", 且互不相同 —— 否则用户切了壳也看不出差别,
     * 弹窗就失去了"告知后果"的意义。
     */
    @Test
    public void shellModeSwitchMessage_describesEachModeDistinctly() {
        String web = JcSettingsDrawer.shellModeSwitchMessage("web");
        String nat = JcSettingsDrawer.shellModeSwitchMessage("native");
        String auto = JcSettingsDrawer.shellModeSwitchMessage("auto");
        assertTrue("Web 档应说明界面由网页渲染", web.contains("网页"));
        assertTrue("原生档应说明界面由本机渲染", nat.contains("本机"));
        assertTrue("自动档应说明按设备能力选择", auto.contains("设备"));
        assertNotEquals(web, nat);
        assertNotEquals(web, auto);
        assertNotEquals(nat, auto);
    }

    /** 与标签/循环共用同一套归一: 历史大写值、null、空白都落到同一档文案。 */
    @Test
    public void shellModeSwitchMessage_normalizesLegacyAndNull() {
        assertEquals(JcSettingsDrawer.shellModeSwitchMessage("web"),
                JcSettingsDrawer.shellModeSwitchMessage("WEB"));
        assertEquals(JcSettingsDrawer.shellModeSwitchMessage("native"),
                JcSettingsDrawer.shellModeSwitchMessage("NATIVE"));
        assertEquals(JcSettingsDrawer.shellModeSwitchMessage("auto"),
                JcSettingsDrawer.shellModeSwitchMessage(null));
        assertEquals(JcSettingsDrawer.shellModeSwitchMessage("auto"),
                JcSettingsDrawer.shellModeSwitchMessage("   "));
    }
}
