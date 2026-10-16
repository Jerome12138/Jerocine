package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * {@link AdFilterTrace} 的格式契约 — 日志行一旦漂移, 排障时用的 grep/awk 会**静默**失效
 * (取不到 ms=/ads=, 或把 token 打进 logcat), 所以格式与脱敏都要钉住。
 */
public class AdFilterTraceTest {

    // ============================ srcTag: 脱敏 ============================

    @Test
    public void srcTag_dropsSchemeDirectoriesAndQuery() {
        assertEquals("v.lfthirtytwo.com/mixed.m3u8", AdFilterTrace.srcTag(
                "https://v.lfthirtytwo.com/20260912/10442_b4cd5ef0/2000k/hls/mixed.m3u8"));
    }

    @Test
    public void srcTag_neverLeaksQueryCredentials() {
        String tag = AdFilterTrace.srcTag(
                "https://cdn.example.com/a/b/index.m3u8?token=SECRET123&expire=1700000000");
        assertEquals("cdn.example.com/index.m3u8", tag);
        assertFalse("query 可能含签名凭据, 不得进日志", tag.contains("SECRET123"));
        assertFalse(tag.contains("?"));
    }

    @Test
    public void srcTag_stripsFragment() {
        assertEquals("cdn.example.com/x.m3u8",
                AdFilterTrace.srcTag("https://cdn.example.com/x.m3u8#frag"));
    }

    @Test
    public void srcTag_hostOnlyWhenNoPath() {
        assertEquals("cdn.example.com", AdFilterTrace.srcTag("https://cdn.example.com"));
        assertEquals("cdn.example.com", AdFilterTrace.srcTag("https://cdn.example.com/"));
    }

    @Test
    public void srcTag_worksWithoutScheme() {
        assertEquals("cdn.example.com/x.m3u8", AdFilterTrace.srcTag("cdn.example.com/x.m3u8"));
    }

    @Test
    public void srcTag_nullOrEmptyIsDash() {
        assertEquals("-", AdFilterTrace.srcTag(null));
        assertEquals("-", AdFilterTrace.srcTag(""));
    }

    @Test
    public void srcTag_clipsLongHostkeepingHeadAndLeaf() {
        // 触发截断的现实组合: CDN 边缘节点 host 很长, 而 srcTag 保留的 host + 末段文件名拼起来超长
        String longUrl = "https://v-very-long-cdn-edge-node-name-1234.example.com/"
                + "2000k/hls/index.m3u8";
        String tag = AdFilterTrace.srcTag(longUrl);
        assertTrue("超长标识要截断到 48 字符内, 实得 " + tag.length(), tag.length() <= 48);
        assertTrue(tag.startsWith("v-very-long-cdn-edge-node-name"));
        assertTrue("截断不得丢掉末段文件名(分 master/子表靠它)", tag.endsWith("index.m3u8"));
        assertTrue(tag.contains(".."));
    }

    // ============================ attempt: 每次尝试 ============================

    @Test
    public void attempt_okCarriesMsHttpOutAdsAndResult() {
        String line = AdFilterTrace.attempt(2, 8123L, 200, null, 77012, 21, "cdn.example.com/mixed.m3u8");
        assertEquals("attempt=2 ms=8123 src=cdn.example.com/mixed.m3u8 http=200 out=77012 ads=21 result=ok",
                line);
    }

    @Test
    public void attempt_failNamesCauseAndOmitsSizeFields() {
        String line = AdFilterTrace.attempt(1, 8005L, -1, "NETWORK", -1, -1, "cdn.example.com/x.m3u8");
        assertEquals("attempt=1 ms=8005 src=cdn.example.com/x.m3u8 result=fail cause=NETWORK", line);
        assertFalse("失败时不该出现体积字段", line.contains("out="));
        assertFalse(line.contains("ads="));
        assertFalse("未拿到响应就不该有 http 字段", line.contains("http="));
    }

    @Test
    public void attempt_failKeepsHttpCodeWhenResponseArrived() {
        String line = AdFilterTrace.attempt(1, 120L, 404, "REJECTED", -1, -1, "cdn.example.com/x.m3u8");
        assertTrue(line.contains("http=404"));
        assertTrue(line.endsWith("result=fail cause=REJECTED"));
    }

    @Test
    public void attempt_zeroAdsIsStillPrinted() {
        // master 表层剔除恒 0: 是"过滤成功但本层无广告", 与"失败"必须能区分出来
        String line = AdFilterTrace.attempt(1, 900L, 200, null, 120, 0, "-");
        assertTrue(line.contains("ads=0"));
        assertTrue(line.endsWith("result=ok"));
    }

    // ============================ end: 整次汇总 ============================

    @Test
    public void end_reportsAttemptCountAndTotalMs() {
        String line = AdFilterTrace.end("cdn.example.com/mixed.m3u8", 2, 16233L, null, 77012, 21);
        assertEquals("end src=cdn.example.com/mixed.m3u8 attempts=2 totalMs=16233 out=77012 ads=21 result=ok",
                line);
    }

    @Test
    public void end_failureKeepsCause() {
        String line = AdFilterTrace.end("cdn.example.com/x.m3u8", 2, 16233L, "NETWORK", -1, -1);
        assertTrue(line.contains("attempts=2"));
        assertTrue(line.endsWith("result=fail cause=NETWORK"));
    }

    @Test
    public void end_zeroAttemptsForLocalReject() {
        // NO_FILTER 在发请求前就返回: attempts=0 表示"根本没发", 不该被读成"试了 0 次还失败"
        String line = AdFilterTrace.end("-", 0, 0L, "NO_FILTER", -1, -1);
        assertTrue(line.contains("attempts=0 totalMs=0"));
        assertTrue(line.contains("cause=NO_FILTER"));
    }

    // ============================ begin / layer ============================

    @Test
    public void begin_reportsUplinkBytes() {
        assertEquals("begin src=cdn.example.com/x.m3u8 in=25771",
                AdFilterTrace.begin("cdn.example.com/x.m3u8", 25771));
    }

    @Test
    public void layer_reportsPhaseDecisionAndElapsed() {
        assertEquals("layer=parse decision=prefetch-hit ms=0 src=cdn.example.com/x.m3u8",
                AdFilterTrace.layer("parse", "prefetch-hit", 0L, "cdn.example.com/x.m3u8"));
        assertEquals("layer=escalate decision=proxy idx=3 streak=2 ms=0 src=-",
                AdFilterTrace.layer("escalate", "proxy idx=3 streak=2", 0L, "-"));
    }

    /** tag 三处调用点共用, 改 tag 等于改排障命令 — 钉住。 */
    @Test
    public void tagIsStable() {
        assertEquals("JcAdFilter", AdFilterTrace.TAG);
    }
}
