package com.jerocine.player;

import java.util.Calendar;
import java.util.Locale;
import java.util.TimeZone;

/**
 * 高峰窗判定 — 纯函数, 无任何 Android 依赖, JVM 可测(buffer-perf 规格 D⑤)。
 *
 * <p>窗口: 每日 <b>[19:00, 23:30)</b> ∪ 周六/周日 <b>[09:00, 23:30)</b>, <b>左闭右开</b>
 * (23:30:00 整点即出窗, 消端点歧义)。按注入 {@link TimeZone} 的本地墙上钟计算 ——
 * 生产传 {@code TimeZone.getDefault()}(本机时区), 时区显式注入让「本机时区」语义可测。
 *
 * <p><b>禁用 java.time(ZoneId/LocalTime 等)</b>: minSdk 21 且全仓无
 * coreLibraryDesugaring, java.time 是 API 26+, Android 5-7 真机会
 * NoClassDefFoundError(2026-10-15 事故同类; lint NewApi fatal 只拦 lintVitalRelease,
 * 门禁单测不跑 lint 发现不了)。本类只用 java.util.Calendar/TimeZone。
 * 同类先例: DownloadQueueOrder / JcSettingsDrawer。
 *
 * <p>不做: 节假日/工作日历、用户可配时段、按服务端时钟、跨日合并(23:30 后不顺延)。
 */
public final class PeakWindow {

    private PeakWindow() {}

    /**
     * 判定 {@code epochMillis} 是否落在高峰窗内。
     *
     * @param epochMillis 时刻(Unix epoch 毫秒)
     * @param zone        计算本地墙上钟的时区(生产传 {@code TimeZone.getDefault()})
     * @return true = 高峰窗内( PlayerLoadControlPolicy 据此放大缓冲参数)
     */
    public static boolean isPeak(long epochMillis, TimeZone zone) {
        Calendar cal = Calendar.getInstance(zone, Locale.US);
        cal.setTimeInMillis(epochMillis);
        int minutesOfDay = cal.get(Calendar.HOUR_OF_DAY) * 60 + cal.get(Calendar.MINUTE);
        int dayOfWeek = cal.get(Calendar.DAY_OF_WEEK);
        boolean weekend = dayOfWeek == Calendar.SATURDAY || dayOfWeek == Calendar.SUNDAY;
        int startMinutes = (weekend ? 9 : 19) * 60;
        int endMinutes = 23 * 60 + 30;
        return minutesOfDay >= startMinutes && minutesOfDay < endMinutes;
    }
}
