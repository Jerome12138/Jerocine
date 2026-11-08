package com.jerocine.player;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.util.TimeZone;

/**
 * 高峰窗判定(buffer-perf 规格 D⑤)纯逻辑测试。
 *
 * <p>窗口: 每日 [19:00, 23:30) ∪ 周六/周日 [09:00, 23:30), 左闭右开, 按注入
 * {@link TimeZone} 的本地墙上钟计算。
 *
 * <p>锚点日期(已用 date 命令核对): 2026-11-09 周一 / 11-13 周五 / 11-14 周六 /
 * 11-15 周日。测试自身运行在桌面 JVM, 用 java.time 构造锚点时刻 —— 被测类
 * (生产代码, minSdk 21 无 desugaring)只用 java.util.Calendar/TimeZone, 禁 java.time。
 */
public class PeakWindowTest {

    private static final ZoneId SH = ZoneId.of("Asia/Shanghai");
    private static final TimeZone SH_TZ = TimeZone.getTimeZone("Asia/Shanghai");

    /** 上海时区本地墙上钟 → epoch 毫秒。 */
    private static long at(int year, int month, int day,
                           int hour, int minute, int second, int millis) {
        return ZonedDateTime.of(year, month, day, hour, minute, second, millis * 1_000_000, SH)
                .toInstant().toEpochMilli();
    }

    // ===== 平日(周一 2026-11-09): 每日窗 [19:00, 23:30) =====

    @Test
    public void weekday_185959999_notPeak() {
        assertFalse(PeakWindow.isPeak(at(2026, 11, 9, 18, 59, 59, 999), SH_TZ));
    }

    @Test
    public void weekday_190000_isPeak() {
        assertTrue(PeakWindow.isPeak(at(2026, 11, 9, 19, 0, 0, 0), SH_TZ)); // 左闭
    }

    @Test
    public void weekday_232959999_isPeak() {
        assertTrue(PeakWindow.isPeak(at(2026, 11, 9, 23, 29, 59, 999), SH_TZ));
    }

    @Test
    public void weekday_233000_notPeak() {
        assertFalse(PeakWindow.isPeak(at(2026, 11, 9, 23, 30, 0, 0), SH_TZ)); // 右开
    }

    // ===== 周末(周六 2026-11-14 / 周日 2026-11-15): 窗提前到 [09:00, 23:30) =====

    @Test
    public void weekend_085959999_notPeak() {
        assertFalse(PeakWindow.isPeak(at(2026, 11, 14, 8, 59, 59, 999), SH_TZ)); // 周六
    }

    @Test
    public void weekend_090000_isPeak() {
        assertTrue(PeakWindow.isPeak(at(2026, 11, 14, 9, 0, 0, 0), SH_TZ)); // 周六, 左闭
        assertTrue(PeakWindow.isPeak(at(2026, 11, 15, 9, 0, 0, 0), SH_TZ)); // 周日
    }

    @Test
    public void sunday_233000_notPeak() {
        assertFalse(PeakWindow.isPeak(at(2026, 11, 15, 23, 30, 0, 0), SH_TZ)); // 右开
    }

    @Test
    public void monday_090000_notPeak() {
        // 每日窗 19:00 起 —— 周一 09:00 不在窗内(周末窗不外溢到工作日)
        assertFalse(PeakWindow.isPeak(at(2026, 11, 9, 9, 0, 0, 0), SH_TZ));
    }

    @Test
    public void friday_090000_notPeak() {
        assertFalse(PeakWindow.isPeak(at(2026, 11, 13, 9, 0, 0, 0), SH_TZ)); // 周五
    }

    @Test
    public void weekend_windowDoesNotCrossMidnight() {
        assertFalse(PeakWindow.isPeak(at(2026, 11, 14, 23, 45, 0, 0), SH_TZ)); // 周六 23:45: 23:30 后不顺延
        assertFalse(PeakWindow.isPeak(at(2026, 11, 15, 0, 30, 0, 0), SH_TZ));  // 周日 00:30: 跨午夜即出窗
    }

    @Test
    public void saturday_evening_stillPeak() {
        assertTrue(PeakWindow.isPeak(at(2026, 11, 14, 19, 0, 0, 0), SH_TZ)); // 周六晚 = 两窗并集内
    }

    // ===== 时区注入: 同一 epoch 换时区结论翻转(「本机时区」语义可测而非藏进实现) =====

    @Test
    public void sameEpoch_timeZoneFlip_shanghaiVsUtc() {
        // 2026-11-09(周一) 19:30 上海 = 11:30 UTC: 上海在每日高峰窗, UTC 不在
        long epoch = at(2026, 11, 9, 19, 30, 0, 0);
        assertTrue(PeakWindow.isPeak(epoch, TimeZone.getTimeZone("Asia/Shanghai")));
        assertFalse(PeakWindow.isPeak(epoch, TimeZone.getTimeZone("UTC")));
        // 反向: UTC 2026-11-09 20:00 = 上海 11-10(周二) 04:00: UTC 在每日高峰窗, 上海不在
        long epoch2 = ZonedDateTime.of(2026, 11, 9, 20, 0, 0, 0, ZoneId.of("UTC"))
                .toInstant().toEpochMilli();
        assertTrue(PeakWindow.isPeak(epoch2, TimeZone.getTimeZone("UTC")));
        assertFalse(PeakWindow.isPeak(epoch2, SH_TZ));
    }
}
