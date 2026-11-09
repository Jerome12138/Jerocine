package com.jerocine.player;

/**
 * 卡顿看门狗跨源 failover 决策(纯函数, JVM 可测) —— 高峰慢源治标
 * (web/src/utils/stallFailover.ts 同口径的 Android 版; 方案-高峰卡顿单机缓解-2026-11-09 §三 P3)。
 *
 * <p>背景: 高峰第三方源 CDN 拥堵的体感是「长时间 {@code STATE_BUFFERING} 但不报错」——
 * {@code onPlayerError} 只接真错误, 覆盖不到慢速卡顿。看门狗在连续缓冲超阈值时评估换源,
 * 目标取后台测速({@link SourceProbeHelper}/{@link SourceSpeedRanker})里显著更快的源
 * (P2b 已在起播 5s 后探测); 换无可换就不折腾 —— 盲轮换到同样慢的源只是换一种卡。</p>
 *
 * <p>守卫口径与 web 版一致(阈值 12s / 冷却 30s / 单集上限 2), 差异:
 * Android 侧的 seek 不单独排除 —— seek 后在拥塞源上的持续缓冲恰是本案要治的场景,
 * 12s 阈值天然盖过正常 seek 起播; 择优 margin 复用 P2b ranker 的 0.8
 * (web 看门狗自用 0.6, 各端口径独立、互不强求一致)。</p>
 */
public final class StallFailoverPolicy {

    /** 连续缓冲多久算 stall: 盖过 seek 正常起播与高峰档放大后的 3s 起播缓冲、短抖动。 */
    public static final long STALL_THRESHOLD_MS = 12_000L;
    /** 触发一次换源后的冷却: 新源也可能慢, 防 A→B→A 横跳。 */
    public static final long COOLDOWN_MS = 30_000L;
    /** 单集内 failover 上限(与 P2b 择优「每会话至多一次」各自独立计数)。 */
    public static final int MAX_PER_EPISODE = 2;

    private StallFailoverPolicy() {}

    /**
     * 守卫全过才发起换源评估(各守卫独立可测, 见 StallFailoverPolicyTest)。
     *
     * @param bufferingMs          本轮连续缓冲时长(STATE_BUFFERING 起算)
     * @param msSinceLastFailover  距上次看门狗换源; 从未触发传 {@link Long#MAX_VALUE}
     * @param failoverCount        本集内已发生的看门狗换源次数(切集归零, 由接线方管理)
     * @param playWhenReady        暂停中的等待缓冲不算卡顿
     * @param otherSourceCount     除当前源外的可换源数量(单源无从换)
     * @param switching            切集互斥窗口(episodeSwitching)或缓冲预取进行中
     */
    public static boolean shouldFailover(long bufferingMs, long msSinceLastFailover,
                                         int failoverCount, boolean playWhenReady,
                                         int otherSourceCount, boolean switching) {
        if (!playWhenReady) return false;
        if (switching) return false;
        if (otherSourceCount <= 0) return false;
        if (failoverCount >= MAX_PER_EPISODE) return false;
        if (msSinceLastFailover < COOLDOWN_MS) return false;
        return bufferingMs >= STALL_THRESHOLD_MS;
    }
}
