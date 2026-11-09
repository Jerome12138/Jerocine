package com.jerocine.player;

import java.util.List;

/**
 * 源测速择优 — 纯函数, 无任何 Android 依赖, JVM 可测(buffer-perf 规格 D⑧)。
 *
 * <p>规则({@link #pickBest}):
 * <ul>
 *   <li>空表 / 全失败 → {@code -1}(不切);</li>
 *   <li>成功者中耗时最小者胜(并列取序号靠前者, 确定性);</li>
 *   <li>当前源成功时, 最优须<b>严格</b> {@code < 0.8×current} 才返回 —— 恰 0.8×current
 *       不切(抗单次采样噪声; 单会话一次性决策, 无振荡面);</li>
 *   <li>当前源失败(或无结果)且存在成功者 → 最快成功者;</li>
 *   <li>最优 == 当前 → {@code -1}。</li>
 * </ul>
 *
 * <p>守卫(≥2 源 / 未手动选源 / 非直播 / 非缓冲中 / 无本地副本)不在本类 ——
 * 由调用方(PlayerActivity)在探测完成时重查, 本类只做择优数学。
 */
public final class SourceSpeedRanker {

    /** 「无更优源」哨兵值。 */
    public static final int NONE = -1;

    /** 当前源成功时的切换门槛: 最优耗时须严格小于 current × 0.8(D⑧)。 */
    static final double SWITCH_MARGIN = 0.8d;

    private SourceSpeedRanker() {}

    /**
     * 每源测速结果(D⑦): {@code {sourceIndex, ok, elapsedMs}}。
     * 探测引擎产出、本类消费的共享值对象 —— 放这里让择优口径与结果形态同处声明。
     */
    public static final class Result {
        public final int sourceIndex;
        public final boolean ok;
        public final long elapsedMs;

        public Result(int sourceIndex, boolean ok, long elapsedMs) {
            this.sourceIndex = sourceIndex;
            this.ok = ok;
            this.elapsedMs = elapsedMs;
        }
    }

    /**
     * 从探测结果里挑应切换到的源下标; 不该切返回 {@link #NONE}。
     *
     * @param results      每源结果(全源探测, 含当前源 —— margin 比较需要)
     * @param currentIndex 当前源下标(探测完成时刻的实况值, 由调用方传入)
     */
    public static int pickBest(List<Result> results, int currentIndex) {
        if (results == null || results.isEmpty()) return NONE;
        Result current = null;
        Result fastest = null;
        for (Result r : results) {
            if (!r.ok) continue;
            if (r.sourceIndex == currentIndex) current = r;
            // 严格更快才替换 → 并列时先到者(序号靠前)胜, 确定性
            if (fastest == null || r.elapsedMs < fastest.elapsedMs) fastest = r;
        }
        if (fastest == null) return NONE;              // 全失败
        if (fastest.sourceIndex == currentIndex) return NONE; // 最优就是当前
        if (current != null && fastest.elapsedMs >= current.elapsedMs * SWITCH_MARGIN) {
            return NONE;                               // 当前成功: 未严格快超 20% 不切
        }
        return fastest.sourceIndex;                    // 当前失败/缺结果: 最快成功者
    }
}
