package com.jerocine.player.ui;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * {@link SettingsDrawerController.AccountInfo} 的 JVM 单测（本机可跑，不需要设备/模拟器）：
 *   cd web/android && ./gradlew :player-core:testDebugUnitTest
 *
 * 为什么要单测一个"字符串工厂"：设置抽屉是**两壳共用**的，账号行文案只在 core 定义一次，
 * 一旦有人把它改回两边各拼各的，或改了字面量而只改一边，同一个抽屉就会在两种模式下显示不同的字。
 * 这里把字面量钉住，任何一侧漂移都会让用例变红。
 */
public class AccountInfoTest {

    @Test
    public void loggedOut_hasSingleCanonicalCopy() {
        SettingsDrawerController.AccountInfo a = SettingsDrawerController.AccountInfo.loggedOut();
        assertFalse(a.loggedIn);
        assertEquals("未登录", a.title);
        assertEquals("登录后收藏 / 历史 / 跳过设置多端同步", a.subtitle);
        assertEquals("登录", a.actionText);
    }

    @Test
    public void loggedIn_usesNicknameAsTitle() {
        SettingsDrawerController.AccountInfo a =
                SettingsDrawerController.AccountInfo.loggedIn("老王");
        assertTrue(a.loggedIn);
        assertEquals("老王", a.title);
        assertEquals("收藏 / 历史 / 跳过设置多端同步", a.subtitle);
        assertEquals("退出", a.actionText);
    }

    @Test
    public void loggedIn_fallsBackWhenNameMissing() {
        // web 壳登录只存 token（昵称在前端 localStorage），喂空串是常态 —— 不能渲染成空标题
        for (String blank : new String[] {null, "", "   "}) {
            SettingsDrawerController.AccountInfo a =
                    SettingsDrawerController.AccountInfo.loggedIn(blank);
            assertTrue("blank=" + blank, a.loggedIn);
            assertEquals("blank=" + blank, "已登录", a.title);
        }
    }

    @Test
    public void loggedIn_trimsNickname() {
        assertEquals("小明",
                SettingsDrawerController.AccountInfo.loggedIn("  小明  ").title);
    }

    @Test
    public void loggedInAndLoggedOut_differInEveryField() {
        // 两态必须可用同一个渲染路径区分（标题/副标题/按钮/登录位 全不同）
        SettingsDrawerController.AccountInfo in =
                SettingsDrawerController.AccountInfo.loggedIn("x");
        SettingsDrawerController.AccountInfo out = SettingsDrawerController.AccountInfo.loggedOut();
        assertFalse(in.subtitle.equals(out.subtitle));
        assertFalse(in.actionText.equals(out.actionText));
        assertTrue(in.loggedIn != out.loggedIn);
    }
}
