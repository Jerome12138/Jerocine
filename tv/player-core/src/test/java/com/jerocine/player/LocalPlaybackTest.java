package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/** LocalPlayback 纯逻辑单测 — ACTION_VIEW 判定与文件标题提取. */
public class LocalPlaybackTest {

    @Test
    public void isLocalAction_matchesViewAction() {
        assertTrue(LocalPlayback.isLocalAction("android.intent.action.VIEW"));
        assertFalse(LocalPlayback.isLocalAction("android.intent.action.MAIN"));
        assertFalse(LocalPlayback.isLocalAction(null));
    }

    @Test
    public void displayTitle_extractsFileNameFromPath() {
        assertEquals("movie", LocalPlayback.displayTitle("/storage/emulated/0/Movies/movie.mp4", "x"));
        assertEquals("心动的信号", LocalPlayback.displayTitle("content://com.android.externalstorage/raw/心动的信号.ts", "x"));
        assertEquals("a.b.c", LocalPlayback.displayTitle("a.b.c.mkv", "x")); // 只去最后一个扩展名
        assertEquals("noext", LocalPlayback.displayTitle("noext", "x"));
    }

    @Test
    public void displayTitle_handlesEmptyAndFallback() {
        assertEquals("回退名", LocalPlayback.displayTitle(null, "回退名"));
        assertEquals("回退名", LocalPlayback.displayTitle("", "回退名"));
        assertEquals("本地视频", LocalPlayback.displayTitle(null, ""));
        assertEquals("本地视频", LocalPlayback.displayTitle("/", ""));
    }

    @Test
    public void displayTitle_urlDecodes() {
        // 文件管理器 URI 常带 percent-encoding
        assertEquals("我的 视频", LocalPlayback.displayTitle("我的%20视频.mp4", "x"));
    }
}
