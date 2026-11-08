package com.jerocine.player.ui;

import org.junit.Test;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

/**
 * JcDeviceDiagnostics.formatWebViewport spec（2026-11-08 并回 web 侧视口快照）。
 *
 * <p>输入是 web 壳 evaluateJavascript 的回调原值 —— JS 返回的 JSON 字符串又被
 * JSON 引号包一层（内部引号转义 \"）。解析不到关键字段一律返回 null（诊断页
 * 不显示该行），绝不抛异常 —— 诊断是兜底功能，不能再炸出第二个兜底。</p>
 */
public class JcDeviceDiagnosticsWebViewportTest {

    /** 1080p 电视典型值: dpr=2 → CSS 视口 960, 内容无溢出 */
    @Test
    public void dpr2_tv_viewport() {
        String raw = "\"{\\\"innerW\\\":960,\\\"innerH\\\":540,\\\"docW\\\":960," +
                "\\\"docH\\\":1180,\\\"dpr\\\":2,\\\"mode\\\":\\\"tv\\\"}\"";
        String line = JcDeviceDiagnostics.formatWebViewport(raw);
        assertTrue(line, line.contains("Web 视口: 960×540"));
        assertTrue(line, line.contains("dpr=2"));
        assertTrue(line, line.contains("mode=tv"));
        assertTrue(line, line.contains("内容 960×1180"));
        assertFalse(line, line.contains("溢出"));
    }

    /** 内容比视口宽 ⇒ 标出横向溢出量（排「页面太宽」的核心信号） */
    @Test
    public void horizontal_overflow_flagged() {
        String raw = "\"{\\\"innerW\\\":960,\\\"innerH\\\":540,\\\"docW\\\":1080," +
                "\\\"docH\\\":1200,\\\"dpr\\\":1,\\\"mode\\\":\\\"desktop\\\"}\"";
        String line = JcDeviceDiagnostics.formatWebViewport(raw);
        assertTrue(line, line.contains("⚠ 横向溢出 120px"));
    }

    /** dpr 是小数(1.5)也原样保留 */
    @Test
    public void fractional_dpr_preserved() {
        String raw = "\"{\\\"innerW\\\":1280,\\\"innerH\\\":720,\\\"docW\\\":1280," +
                "\\\"docH\\\":900,\\\"dpr\\\":1.5,\\\"mode\\\":\\\"tv\\\"}\"";
        assertTrue(JcDeviceDiagnostics.formatWebViewport(raw).contains("dpr=1.5"));
    }

    /** 页面端 JS 抛异常 → 网页快照回传 err, 显示失败原因而不是吞掉 */
    @Test
    public void page_error_reported() {
        String raw = "\"{\\\"err\\\":\\\"ReferenceError: xxx is not defined\\\"}\"";
        String line = JcDeviceDiagnostics.formatWebViewport(raw);
        assertTrue(line, line.contains("取数失败"));
        assertTrue(line, line.contains("ReferenceError"));
    }

    @Test
    public void garbage_or_missing_returns_null() {
        assertNull(JcDeviceDiagnostics.formatWebViewport(null));
        assertNull(JcDeviceDiagnostics.formatWebViewport(""));
        assertNull(JcDeviceDiagnostics.formatWebViewport("null"));
        assertNull(JcDeviceDiagnostics.formatWebViewport("not json at all"));
        // 缺字段(如只回传了部分 key) → null, 不显示半截数据
        assertNull(JcDeviceDiagnostics.formatWebViewport(
                "\"{\\\"innerW\\\":960,\\\"mode\\\":\\\"tv\\\"}\""));
    }
}
