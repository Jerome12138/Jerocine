package com.jerocine.player;

import java.util.Map;
import java.util.TreeMap;

/**
 * 窗口进度聚合: 乱序完成仍单调; 重复窗口序号只计首次; 线程安全。JVM 可测。
 *
 * <p>buffer-perf 规格 D③: 缓冲进度口径改「已完成窗口时长/窗口总时长」。顺序累加天然单调,
 * 并行完成乱序后必须经本类保单调 + 去重。cached **按窗口序号升序重算**(非到达序累加)——
 * 消除乱序下 double 求和的非结合性: 任意完成顺序结果逐位一致, 全完成时与 target
 * (窗口分片时长和, 同序求和)精确相等。
 *
 * <p>纯类无任何 Android 依赖(规格 §3.2 铁律), 顺序/并行两模式统一走本类。
 */
public final class BufferProgressAggregator {

    /** 窗口分片时长和(与 BufferPrefetcher targetSec 同口径, 不在本类重算)。 */
    private final double targetSeconds;

    /** 窗口序号 → 已完成分片时长。TreeMap 使 cachedSeconds 恒按序号升序重算。 */
    private final TreeMap<Integer, Double> done = new TreeMap<>();

    public BufferProgressAggregator(double targetSeconds) {
        this.targetSeconds = targetSeconds;
    }

    /**
     * 分片完成事件。
     *
     * @param windowIndex     窗口内序号(1-based, 对齐「分片 x/N」口径); 同序号重复上报只计首次
     * @param durationSeconds 该分片时长(秒), 0 合法
     */
    public synchronized void onSegmentDone(int windowIndex, double durationSeconds) {
        done.putIfAbsent(windowIndex, durationSeconds);
    }

    /** cached = 已完成分片时长和, **按窗口序号升序重算**(每次查询重放, 恒单调不减)。 */
    public synchronized double cachedSeconds() {
        double sum = 0;
        for (Map.Entry<Integer, Double> e : done.entrySet()) {
            sum += e.getValue();
        }
        return sum;
    }

    /** 目标进度 = 窗口分片时长和(计算在 BufferPrefetcher, 与现状口径一致)。 */
    public double targetSeconds() {
        return targetSeconds;
    }
}
