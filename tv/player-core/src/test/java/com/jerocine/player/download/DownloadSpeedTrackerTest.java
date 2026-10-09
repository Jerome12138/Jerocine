package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.HashSet;

/**
 * {@link DownloadSpeedTracker} 单测 — 下载速度显示(2026-10-10 用户要求: 总体+分集速度)。
 * 模拟 2s 轮询采样节奏: 速度 = 字节差分 / 时间差。
 */
public class DownloadSpeedTrackerTest {

    @Test
    public void firstSampleYieldsZero() {
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        assertEquals(0, t.onSample("a", 1000L, 10_000L));
        assertEquals(0, t.speedBps("a"));
        assertEquals(0, t.totalBps());
    }

    @Test
    public void secondSampleComputesDelta() {
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("a", 1_000L, 10_000L);
        // 2s 内涨了 2_000_000 字节 → 1_000_000 B/s
        assertEquals(1_000_000L, t.onSample("a", 2_001_000L, 12_000L));
        assertEquals(1_000_000L, t.speedBps("a"));
        assertEquals(1_000_000L, t.totalBps());
    }

    @Test
    public void totalSumsAcrossTasks() {
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("a", 0L, 10_000L);
        t.onSample("b", 0L, 10_000L);
        t.onSample("a", 500_000L, 12_000L);  // 250_000 B/s
        t.onSample("b", 1_500_000L, 12_000L); // 750_000 B/s
        assertEquals(250_000L, t.speedBps("a"));
        assertEquals(750_000L, t.speedBps("b"));
        assertEquals(1_000_000L, t.totalBps());
    }

    @Test
    public void noProgressYieldsZeroNotStale() {
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("a", 0L, 10_000L);
        t.onSample("a", 400_000L, 12_000L); // 200_000 B/s
        assertEquals(200_000L, t.speedBps("a"));
        // 字节不动(轮询间无新数据) → 速度归零, 不残留上一拍的速度
        assertEquals(0, t.onSample("a", 400_000L, 14_000L));
        assertEquals(0, t.speedBps("a"));
        assertEquals(0, t.totalBps());
    }

    @Test
    public void byteRegressionResetsBaselineOnly() {
        // 删除重下/重试: 字节回退不产速度, 只刷新基准, 下一拍恢复正常差分
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("a", 0L, 10_000L);
        t.onSample("a", 400_000L, 12_000L);
        assertEquals(0, t.onSample("a", 100L, 14_000L)); // 回退
        assertEquals(0, t.speedBps("a"));
        // 基准已刷新为 (100, 14s): 涨 400_100-100=400_000 字节 / 2s → 200_000 B/s
        assertEquals(200_000L, t.onSample("a", 400_100L, 16_000L));
    }

    @Test
    public void removeDropsFromTotal() {
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("a", 0L, 10_000L);
        t.onSample("b", 0L, 10_000L);
        t.onSample("a", 500_000L, 12_000L);
        t.onSample("b", 1_000_000L, 12_000L);
        assertEquals(750_000L, t.totalBps()); // 250k + 500k
        t.remove("a");
        assertEquals(500_000L, t.totalBps());
        assertEquals(0, t.speedBps("a"));
        t.remove("不存在的id"); // 幂等
        assertEquals(500_000L, t.totalBps());
    }

    @Test
    public void retainAllDropsZombieEntries() {
        // 轮询对账兜底: 崩溃恢复/遗漏回调留下的僵尸条目不能污染总速度
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("live", 0L, 10_000L);
        t.onSample("zombie", 0L, 10_000L);
        t.onSample("live", 500_000L, 12_000L);
        t.onSample("zombie", 500_000L, 12_000L);
        assertEquals(500_000L, t.totalBps());
        t.retainAll(new HashSet<>(java.util.Collections.singletonList("live")));
        assertEquals(250_000L, t.totalBps());
        assertEquals(0, t.speedBps("zombie"));
        t.retainAll(null); // 防御: null 等于清空
        assertEquals(0, t.totalBps());
    }

    @Test
    public void nullAndInvalidSamplesIgnored() {
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        assertEquals(0, t.onSample(null, 100L, 10_000L));
        assertEquals(0, t.onSample("a", -1L, 10_000L));
        assertEquals(0, t.onSample("a", 100L, 0L));
        assertEquals(0, t.onSample("a", 100L, -5L));
        assertEquals(0, t.speedBps(null));
        assertTrue(t.totalBps() == 0);
    }

    @Test
    public void clockAnomalyRefreshesBaseline() {
        // 时钟回拨(dt<=0): 不产速度也不崩, 基准推到现在
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("a", 0L, 10_000L);
        t.onSample("a", 500_000L, 12_000L);
        assertEquals(0, t.onSample("a", 600_000L, 12_000L)); // dt=0
        assertEquals(0, t.onSample("a", 700_000L, 11_000L)); // dt<0
        assertEquals(0, t.totalBps());
        // 基准 = (700_000, 11_000): 涨 300_000 字节 / 2s
        assertEquals(150_000L, t.onSample("a", 1_000_000L, 13_000L));
    }

    @Test
    public void clearResetsEverything() {
        DownloadSpeedTracker t = new DownloadSpeedTracker();
        t.onSample("a", 0L, 10_000L);
        t.onSample("a", 500_000L, 12_000L);
        t.clear();
        assertEquals(0, t.totalBps());
        assertEquals(0, t.speedBps("a"));
        // 清空后重新采样从首拍开始
        assertEquals(0, t.onSample("a", 600_000L, 14_000L));
    }
}
