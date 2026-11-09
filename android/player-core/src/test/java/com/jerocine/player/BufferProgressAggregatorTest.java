package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

/**
 * 缓冲进度聚合器(buffer-perf 规格 D③)纯逻辑测试。
 *
 * <p>并行写分片下完成事件乱序到达, 聚合器必须:
 *   ① 单调不减(重复上报不虚增);
 *   ② 按窗口序号<b>升序重算</b> cached —— 任意完成顺序结果逐位一致,
 *      全完成时与 target(窗口分片时长和, 升序求和)精确相等;
 *   ③ 线程安全。
 */
public class BufferProgressAggregatorTest {

    /** 乱序完成: 每次查询输出单调不减。 */
    @Test
    public void outOfOrderCompletion_isMonotonicNonDecreasing() {
        BufferProgressAggregator a = new BufferProgressAggregator(0.1 + 0.2 + 0.3);
        a.onSegmentDone(3, 0.3);
        double c1 = a.cachedSeconds();
        a.onSegmentDone(1, 0.1);
        double c2 = a.cachedSeconds();
        a.onSegmentDone(2, 0.2);
        double c3 = a.cachedSeconds();
        assertTrue(c1 <= c2);
        assertTrue(c2 <= c3);
    }

    /** 乱序完成与升序重算逐位一致(double 求和非结合性的消解凭据)。 */
    @Test
    public void anyCompletionOrder_isBitIdenticalToAscendingResum() {
        // 0.1/0.2/0.3 的 double 求和顺序不同结果不同位型: (0.1+0.2)+0.3 = 0.6000000000000001
        // 而 (0.3+0.2)+0.1 = 0.6 —— 到达序累加会在此类序列上分叉, 升序重算必须抹平。
        double[] durations = {0.1, 0.2, 0.3};
        int[][] orders = {
                {1, 2, 3}, // 升序到达(=重算序)
                {3, 2, 1}, // 逆序到达
                {2, 3, 1}, // 乱序到达
        };
        long firstBits = -1;
        for (int[] order : orders) {
            BufferProgressAggregator a = new BufferProgressAggregator(0.1 + 0.2 + 0.3);
            for (int idx : order) a.onSegmentDone(idx, durations[idx - 1]);
            long bits = Double.doubleToLongBits(a.cachedSeconds());
            if (firstBits == -1) firstBits = bits;
            assertEquals("order " + java.util.Arrays.toString(order) + " 求和位型不一致",
                    firstBits, bits);
        }
        // 升序重算的结果 = 按序号升序手写累加的逐位结果
        assertEquals(Double.doubleToLongBits((0.1 + 0.2) + 0.3), firstBits);
    }

    /** 重复窗口序号只计首次, 二次上报不虚增进度。 */
    @Test
    public void duplicateWindowIndex_countedOnlyOnce() {
        BufferProgressAggregator a = new BufferProgressAggregator(0.5 + 0.5);
        a.onSegmentDone(1, 0.5);
        a.onSegmentDone(2, 0.5);
        double full = a.cachedSeconds();
        for (int i = 0; i < 5; i++) {
            a.onSegmentDone(2, 0.5); // 同一分片的重复完成事件(并行竞态/重试残留口径)
            a.onSegmentDone(1, 0.5);
        }
        assertEquals(full, a.cachedSeconds(), 0.0);
        assertEquals(1.0, a.cachedSeconds(), 0.0);
    }

    /** target 恒等于构造传入的窗口分片时长和。 */
    @Test
    public void target_isSumPassedIn() {
        assertEquals(12.75, new BufferProgressAggregator(12.75).targetSeconds(), 0.0);
        assertEquals(0.0, new BufferProgressAggregator(0.0).targetSeconds(), 0.0);
    }

    /** 全部窗口完成 → cached 与 target 精确相等(升序重算与 target 同序求和, exact 合法)。 */
    @Test
    public void allDone_cachedExactlyEqualsTarget() {
        double[] durations = {4.08, 3.96, 5.02, 0.13, 9.87, 6.5};
        double target = 0;
        for (double d : durations) target += d; // 与 BufferPrefetcher targetSec 同款升序累加
        BufferProgressAggregator a = new BufferProgressAggregator(target);
        // 完全倒序完成(并行下最坏乱序)
        for (int i = durations.length; i >= 1; i--) a.onSegmentDone(i, durations[i - 1]);
        assertEquals(target, a.cachedSeconds(), 0.0);
    }

    /** duration=0 的分片合法(清单缺 EXTINF 兜底口径), 不影响单调与终值。 */
    @Test
    public void zeroDurationSegment_isLegal() {
        BufferProgressAggregator a = new BufferProgressAggregator(0.5 + 0.0 + 0.5);
        a.onSegmentDone(2, 0.0);
        assertEquals(0.0, a.cachedSeconds(), 0.0);
        a.onSegmentDone(1, 0.5);
        assertEquals(0.5, a.cachedSeconds(), 0.0);
        a.onSegmentDone(3, 0.5);
        assertEquals(1.0, a.cachedSeconds(), 0.0);
    }

    /** 多线程争调后终值正确(无丟事件、无重复计数)。 */
    @Test
    public void concurrentHammer_finalValueCorrect() throws Exception {
        final int total = 200;
        double[] durations = new double[total];
        double target = 0;
        for (int i = 0; i < total; i++) {
            durations[i] = 0.5; // 2 的幂, 升序求和精确
            target += durations[i];
        }
        final BufferProgressAggregator a = new BufferProgressAggregator(target);
        final int threads = 8;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        final CountDownLatch startGate = new CountDownLatch(1);
        List<java.util.concurrent.Future<?>> futures = new ArrayList<>();
        for (int t = 0; t < threads; t++) {
            final int lo = t * (total / threads);
            final int hi = (t == threads - 1) ? total : lo + (total / threads);
            futures.add(pool.submit(() -> {
                try {
                    startGate.await();
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    return;
                }
                for (int i = lo; i < hi; i++) a.onSegmentDone(i + 1, durations[i]);
            }));
        }
        startGate.countDown();
        for (java.util.concurrent.Future<?> f : futures) f.get(10, TimeUnit.SECONDS);
        pool.shutdown();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS));
        assertEquals(target, a.cachedSeconds(), 0.0);
    }

    /** 重复事件与并发混压: 同一序号被多线程各报一次仍只计一次。 */
    @Test
    public void concurrentDuplicateReports_countedOnce() throws Exception {
        final int total = 100;
        double target = total * 0.25;
        final BufferProgressAggregator a = new BufferProgressAggregator(target);
        final int threads = 4;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        final CountDownLatch startGate = new CountDownLatch(1);
        List<java.util.concurrent.Future<?>> futures = new ArrayList<>();
        for (int t = 0; t < threads; t++) {
            futures.add(pool.submit(() -> {
                try {
                    startGate.await();
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    return;
                }
                // 每个线程把全部序号各报一遍(乱序): 4 倍重复上报
                List<Integer> idx = new ArrayList<>();
                for (int i = 1; i <= total; i++) idx.add(i);
                Collections.shuffle(idx);
                for (int i : idx) a.onSegmentDone(i, 0.25);
            }));
        }
        startGate.countDown();
        for (java.util.concurrent.Future<?> f : futures) f.get(10, TimeUnit.SECONDS);
        pool.shutdown();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS));
        assertEquals(target, a.cachedSeconds(), 0.0);
    }
}
