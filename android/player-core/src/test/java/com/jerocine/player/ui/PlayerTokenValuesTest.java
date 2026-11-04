package com.jerocine.player.ui;

import org.junit.Test;

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

/**
 * 播放器 UI 令牌收敛值锁定(组件化二期 A6 / 批次 C5)。
 *
 * hex→R.color 收敛的铁律是「值逐字不变」。本测试直接读 src/main/res 资源源文件与被改 Java 源
 * (单测工作目录 = 模块目录, 向上回溯兜底, 同 M3u8FilterCoreTest 的夹具定位法), 把本批每一处
 * 「旧字面量 = 新令牌值」的等式与两处无令牌保留项锁进测试:
 * <ul>
 *   <li>新增令牌 jc_step_btn_press 必须恰等于 jc_step_btn_bg.xml 的旧字面量 #FF2BB8CC;</li>
 *   <li>被引令牌 jc_text/jc_bg/jc_accent/jc_scrim 的值必须等于被替换的旧字面量
 *       (0xFFFFFFFF / 0xFF0B0B0F / #FF4AD1E5 / #99000000);</li>
 *   <li>jc_step_btn_bg / jc_checkbox_on / jc_badge_bg 已引令牌、旧 hex 清零;</li>
 *   <li>无对应令牌的两处(jc_checkbox_off #66FFFFFF、BrandSplashView 0x99FFFFFF)字面量保留;</li>
 *   <li>dimens 新条目 jc_toast_bottom_offset = 96dp(GlassToast 旧散写值);</li>
 *   <li>DownloadActivity / DownloadEngine 不再裸调系统 Toast。</li>
 * </ul>
 */
public class PlayerTokenValuesTest {

    /** 单测工作目录 = 模块目录; 保险起见向上回溯(同 M3u8FilterCoreTest 的夹具定位法)。 */
    private static File moduleDir() {
        File dir = new File(".").getAbsoluteFile();
        for (int i = 0; i < 4 && dir != null; i++) {
            if (new File(dir, "src/main/res/values/colors.xml").exists()) return dir;
            dir = dir.getParentFile();
        }
        throw new AssertionError("找不到 player-core 模块目录(colors.xml 向上回溯失败)");
    }

    private static String fileText(String relPath) throws Exception {
        File f = new File(moduleDir(), relPath);
        assertTrue("缺文件: " + relPath, f.exists());
        return new String(Files.readAllBytes(f.toPath()), StandardCharsets.UTF_8);
    }

    private static String colorValue(String colorsXml, String name) {
        Matcher m = Pattern.compile("<color name=\"" + name + "\">([^<]+)</color>")
                .matcher(colorsXml);
        assertTrue("colors.xml 缺令牌: " + name, m.find());
        return m.group(1);
    }

    @Test
    public void new_token_jc_step_btn_press_equals_old_literal() throws Exception {
        String colors = fileText("src/main/res/values/colors.xml");
        assertEquals("#FF2BB8CC", colorValue(colors, "jc_step_btn_press"));
    }

    @Test
    public void referenced_tokens_equal_replaced_literals() throws Exception {
        String colors = fileText("src/main/res/values/colors.xml");
        // PlayerDialogHelper ×3: 0xFFFFFFFF; BrandSplashView JC_BG: 0xFF0B0B0F;
        // BrandSplashView JC_ACCENT 与 jc_checkbox_on: #FF4AD1E5; jc_badge_bg: #99000000
        assertEquals("#FFFFFFFF", colorValue(colors, "jc_text"));
        assertEquals("#0B0B0F", colorValue(colors, "jc_bg"));
        assertEquals("#FF4AD1E5", colorValue(colors, "jc_accent"));
        assertEquals("#99000000", colorValue(colors, "jc_scrim"));
    }

    @Test
    public void drawables_reference_tokens_and_old_hex_gone() throws Exception {
        String stepBtn = fileText("src/main/res/drawable/jc_step_btn_bg.xml");
        assertTrue(stepBtn.contains("@color/jc_step_btn_press"));
        assertFalse(stepBtn.contains("#FF2BB8CC"));

        String checkboxOn = fileText("src/main/res/drawable/jc_checkbox_on.xml");
        assertTrue(checkboxOn.contains("@color/jc_accent"));
        assertFalse(checkboxOn.contains("#FF4AD1E5"));

        String badgeBg = fileText("src/main/res/drawable/jc_badge_bg.xml");
        assertTrue(badgeBg.contains("@color/jc_scrim"));
        assertFalse(badgeBg.contains("#99000000"));
    }

    @Test
    public void literals_without_token_kept_verbatim() throws Exception {
        // jc_checkbox_off 描边 #66FFFFFF: jc_stroke #22 / faint #14 / buffered #55 均不等值
        assertTrue(fileText("src/main/res/drawable/jc_checkbox_off.xml").contains("#66FFFFFF"));
        // BrandSplashView 品牌名 0x99FFFFFF(白60%): secondary #B3 / tertiary #80 均不等值
        assertTrue(fileText("src/main/java/com/jerocine/player/ui/BrandSplashView.java")
                .contains("0x99FFFFFF"));
    }

    @Test
    public void glassToast_bottom_offset_in_dimens_value_unchanged() throws Exception {
        String dimens = fileText("src/main/res/values/dimens.xml");
        Matcher m = Pattern.compile("<dimen name=\"jc_toast_bottom_offset\">([^<]+)</dimen>")
                .matcher(dimens);
        assertTrue("dimens.xml 缺 jc_toast_bottom_offset", m.find());
        assertEquals("96dp", m.group(1));
        String src = fileText("src/main/java/com/jerocine/player/ui/GlassToast.java");
        assertTrue(src.contains("jc_toast_bottom_offset"));
        assertFalse("GlassToast 仍散写 96dp 换算", src.contains("96 *"));
    }

    @Test
    public void player_dialog_helper_text_color_via_token() throws Exception {
        String helper = fileText("src/main/java/com/jerocine/player/PlayerDialogHelper.java");
        assertFalse("PlayerDialogHelper 仍散写 0xFFFFFFFF", helper.contains("0xFFFFFFFF"));
        assertTrue(helper.contains("R.color.jc_text"));
    }

    @Test
    public void brand_splash_bg_and_accent_via_token() throws Exception {
        String src = fileText("src/main/java/com/jerocine/player/ui/BrandSplashView.java");
        assertTrue(src.contains("R.color.jc_bg"));
        assertTrue(src.contains("R.color.jc_accent"));
        assertFalse(src.contains("0xFF0B0B0F"));
        assertFalse(src.contains("0xFF4AD1E5"));
    }

    @Test
    public void download_pages_no_raw_system_toast() throws Exception {
        assertFalse(fileText("src/main/java/com/jerocine/player/download/DownloadActivity.java")
                .contains("Toast.makeText"));
        assertFalse(fileText("src/main/java/com/jerocine/player/download/DownloadEngine.java")
                .contains("Toast.makeText"));
    }
}
