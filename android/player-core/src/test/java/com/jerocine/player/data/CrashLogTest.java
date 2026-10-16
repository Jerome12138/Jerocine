package com.jerocine.player.data;

import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

import java.io.File;
import java.io.IOException;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

/**
 * CrashLog 纯逻辑单测（报告格式 / 环形裁剪选择 / 摘要提取 / 落盘环形保留）。
 * 不碰 Android 类 —— Context 相关走 File 层重载。
 */
public class CrashLogTest {

    private static final String STACK =
            "java.lang.RuntimeException: boom\n"
            + "  at com.jerocine.app.JerocineApp.onCreate(JerocineApp.kt:20)\n"
            + "  at android.app.Instrumentation.callApplicationOnCreate(Instrumentation.java)";

    @Rule
    public TemporaryFolder tmp = new TemporaryFolder();

    // ---- formatReport ----

    @Test
    public void formatReport_containsAllSections() {
        String report = CrashLog.formatReport(
                1728990000000L, "main",
                "JMGO J9 (API 23)", "art.jerocine.app 1.0.10(1110)", STACK);
        assertTrue(report.startsWith("Jerocine 崩溃报告\n"));
        assertTrue(report.contains("时间: "));
        assertTrue(report.contains("线程: main\n"));
        assertTrue(report.contains("设备: JMGO J9 (API 23)\n"));
        assertTrue(report.contains("应用: art.jerocine.app 1.0.10(1110)\n"));
        assertTrue(report.contains("---- 栈 ----\n" + STACK));
    }

    @Test
    public void formatReport_nullOptionalsStillRenders() {
        String report = CrashLog.formatReport(1728990000000L, null, null, null, null);
        assertFalse(report.contains("设备: "));
        assertFalse(report.contains("应用: "));
        assertTrue(report.contains("线程: ?\n"));
        assertTrue(report.contains("(无栈)"));
    }

    // ---- pruneSelection ----

    @Test
    public void pruneSelection_keepsNewestFive() {
        String[] names = {
                "crash-1000.txt", "crash-1001.txt", "crash-1002.txt",
                "crash-1003.txt", "crash-1004.txt", "crash-1005.txt", "crash-1006.txt"};
        // 升序排好 → 最老的两份(1000/1001)该删, 最新 5 份保留
        assertArrayEquals(new String[]{"crash-1000.txt", "crash-1001.txt"},
                CrashLog.pruneSelection(names, 5));
    }

    @Test
    public void pruneSelection_underLimitDeletesNothing() {
        String[] names = {"crash-1000.txt", "crash-1001.txt"};
        assertEquals(0, CrashLog.pruneSelection(names, 5).length);
    }

    @Test
    public void pruneSelection_ignoresForeignFiles() {
        String[] names = {
                "crash-1000.txt", "crash-1001.txt", "crash-1002.txt",
                "crash-1003.txt", "crash-1004.txt", "crash-1005.txt",
                "other.txt", "subdir"};
        // other.txt / subdir 不算崩溃报告 → 只该删 crash-1000
        assertArrayEquals(new String[]{"crash-1000.txt"},
                CrashLog.pruneSelection(names, 5));
    }

    // ---- summaryFrom ----

    @Test
    public void summaryFrom_extractsTimeAndException() {
        String report = CrashLog.formatReport(
                1792713600000L, "main",
                "JMGO J9 (API 23)", "art.jerocine.app 1.0.10(1110)", STACK);
        String summary = CrashLog.summaryFrom(report);
        assertTrue(summary, summary.startsWith("2026-"));
        assertTrue(summary, summary.contains("java.lang.RuntimeException: boom"));
        assertFalse(summary, summary.contains("at com.jerocine"));
    }

    @Test
    public void summaryFrom_garbageReturnsNull() {
        assertNull(CrashLog.summaryFrom(null));
        assertNull(CrashLog.summaryFrom(""));
        assertNull(CrashLog.summaryFrom("随便什么不是报告的文本"));
    }

    @Test
    public void summaryFrom_longExceptionTruncated() {
        StringBuilder bigStack = new StringBuilder("java.lang.RuntimeException: ");
        for (int i = 0; i < 400; i++) {
            bigStack.append('x');
        }
        String report = CrashLog.formatReport(
                1728990000000L, "main", "d", "a", bigStack.toString());
        String summary = CrashLog.summaryFrom(report);
        assertTrue(summary, summary.length() <= 161 && summary.endsWith("…"));
    }

    // ---- writeReport(文件层) + 环形保留 ----

    @Test
    public void writeReport_createsFileAndPrunesRing() throws IOException {
        File dir = tmp.newFolder("crashes");
        for (long ts = 1000; ts < 1007; ts++) {
            CrashLog.writeReport(dir, ts, "main", "d", "a", STACK);
        }
        String[] names = dir.list();
        assertEquals(5, names.length);
        // 最老的一份(crash-1000/1001)被裁掉, 最新 5 份在
        for (long ts = 1002; ts < 1007; ts++) {
            assertTrue(new File(dir, "crash-" + ts + ".txt").exists());
        }
        assertFalse(new File(dir, "crash-1000.txt").exists());
        assertFalse(new File(dir, "crash-1001.txt").exists());
    }

    @Test
    public void writeReport_contentRoundTrip() throws IOException {
        File dir = tmp.newFolder("crashes2");
        CrashLog.writeReport(dir, 1728990000000L, "main",
                "JMGO J9 (API 23)", "art.jerocine.app 1.0.10(1110)", STACK);
        File report = new File(dir, "crash-1728990000000.txt");
        assertTrue(report.exists());
        String summary = CrashLog.summaryFrom(
                new String(java.nio.file.Files.readAllBytes(report.toPath()),
                        java.nio.charset.StandardCharsets.UTF_8));
        assertTrue(summary, summary.contains("java.lang.RuntimeException: boom"));
    }
}
