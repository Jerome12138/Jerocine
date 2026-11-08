package com.jerocine.player;

/**
 * 进程级探测节流/熔断 — 判定逻辑纯函数可 JVM 单测(buffer-perf 规格 D⑦/§3.4)。
 *
 * <p>两条规则:
 * <ul>
 *   <li><b>节流</b>: 距上次探测 &lt;30 分钟跳过 —— 长追剧每集 recreate 也不至于每集
 *       对全部源各轰 1-3 个无 Referer 清单 GET(频率/UA 敏感源兜底, 豆瓣 418 反爬先例
 *       ApiClient 注释);</li>
 *   <li><b>熔断</b>: 连续 2 轮探测全败 → 进程内禁探至进程重启(源站整体不可达/反爬时
 *       彻底停手); 任一轮出现成功即清零全败计数。</li>
 * </ul>
 *
 * <p>时间为纯入参 {@code nowMs}(单调时钟语义由调用方保证, 生产用
 * SystemClock.elapsedRealtime), 本类不做任何系统取时 —— 可测性优先。
 * 进程级状态 = {@link #processWide()} 持有的单例(静态字段随进程存活, 天然
 * 跨 Activity 实例/recreate 共享); 测试直接 new 实例, 逻辑与进程单例同码。
 */
public final class SourceProbeThrottle {

    /** 两次探测的最小间隔。 */
    static final long MIN_INTERVAL_MS = 30L * 60L * 1000L;
    /** 连续全败轮数达到该值即熔断。 */
    static final int MAX_CONSECUTIVE_ALL_FAIL_ROUNDS = 2;

    private static final SourceProbeThrottle PROCESS = new SourceProbeThrottle();

    /** 上次探测轮的落点(0 = 冷启动无记录)。 */
    private long lastRoundAtMs;
    private boolean hasRound;
    /** 连续全败轮数(任一轮出现成功即清零)。 */
    private int consecutiveAllFailRounds;

    /** 进程级单例(PlayerActivity 持有引用到进程结束, 状态天然跨会话共享)。 */
    public static SourceProbeThrottle processWide() {
        return PROCESS;
    }

    /**
     * 现在是否允许发起一轮探测。
     * 熔断优先判定(先于节流), 熔断后任何时间都返回 false。
     */
    public synchronized boolean shouldProbe(long nowMs) {
        if (consecutiveAllFailRounds >= MAX_CONSECUTIVE_ALL_FAIL_ROUNDS) return false;
        if (!hasRound) return true; // 冷启动(无记录)允许
        return nowMs - lastRoundAtMs >= MIN_INTERVAL_MS;
    }

    /**
     * 一轮探测批次结束(无论决策切不切都记录 —— 探测流量本身已发生)。
     *
     * @param nowMs      本轮结束时刻(单调毫秒)
     * @param anySuccess 本轮是否存在至少一个成功源
     */
    public synchronized void recordRound(long nowMs, boolean anySuccess) {
        hasRound = true;
        lastRoundAtMs = nowMs;
        consecutiveAllFailRounds = anySuccess ? 0 : consecutiveAllFailRounds + 1;
    }
}
