package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * 在途写注册表(buffer-perf 规格 D④)纯逻辑测试。
 *
 * <p>并行写分片下 cancel 要掐<b>所有</b>在途 CacheWriter(单引用只掐一个的教训),
 * 经 {@link BufferWriterRegistry.Cancellable} 抽象承载, JVM 可测:
 *   ① register → cancelAll 掐所有;
 *   ② finally 对称注销后不再被掐;
 *   ③ 单个 cancel 抛异常吞掉继续其余(对齐 BufferPrefetcher :236-239);
 *   ④ 空集安全 / 重复 cancelAll 幂等。
 */
public class BufferWriterRegistryTest {

    /** 计数型假在途写: 记录 cancel 次数, 可指定抛异常/是否必须被掐到。 */
    private static final class FakeWriter implements BufferWriterRegistry.Cancellable {
        final AtomicInteger cancelCalls = new AtomicInteger();
        final boolean throwOnCancel;
        /** true = 注册后从未注销(在途), 最终必须有某次 cancelAll 掐到它(≥1 次)。 */
        boolean expectCancelled;

        FakeWriter() { this(false); }

        FakeWriter(boolean throwOnCancel) { this.throwOnCancel = throwOnCancel; }

        @Override
        public void cancel() {
            cancelCalls.incrementAndGet();
            if (throwOnCancel) throw new RuntimeException("cancel boom");
        }
    }

    @Test
    public void cancelAll_cancelsAllRegistered() {
        BufferWriterRegistry registry = new BufferWriterRegistry();
        FakeWriter w1 = new FakeWriter();
        FakeWriter w2 = new FakeWriter();
        FakeWriter w3 = new FakeWriter();
        registry.register(w1);
        registry.register(w2);
        registry.register(w3);
        registry.cancelAll();
        assertEquals(1, w1.cancelCalls.get());
        assertEquals(1, w2.cancelCalls.get());
        assertEquals(1, w3.cancelCalls.get());
    }

    @Test
    public void unregistered_notCancelledByLaterCancelAll() {
        BufferWriterRegistry registry = new BufferWriterRegistry();
        FakeWriter done = new FakeWriter();
        FakeWriter inFlight = new FakeWriter();
        registry.register(done);
        registry.register(inFlight);
        registry.unregister(done); // 写完/写炸的 finally 对称注销
        registry.cancelAll();
        assertEquals(0, done.cancelCalls.get());
        assertEquals(1, inFlight.cancelCalls.get());
    }

    @Test
    public void oneCancelThrowing_doesNotStopOthers() {
        BufferWriterRegistry registry = new BufferWriterRegistry();
        FakeWriter w1 = new FakeWriter(true); // 先注册的会抛
        FakeWriter w2 = new FakeWriter();
        FakeWriter w3 = new FakeWriter();
        registry.register(w1);
        registry.register(w2);
        registry.register(w3);
        registry.cancelAll(); // 不得向调用方抛出
        assertEquals(1, w1.cancelCalls.get());
        assertEquals(1, w2.cancelCalls.get());
        assertEquals(1, w3.cancelCalls.get());
    }

    @Test
    public void emptyRegistry_cancelAllIsSafe() {
        BufferWriterRegistry registry = new BufferWriterRegistry();
        registry.cancelAll(); // 空集: 不抛即安全
    }

    @Test
    public void repeatedCancelAll_idempotent() {
        BufferWriterRegistry registry = new BufferWriterRegistry();
        FakeWriter w = new FakeWriter();
        registry.register(w);
        registry.cancelAll();
        registry.cancelAll();
        registry.cancelAll();
        assertEquals("重复 cancelAll 不得重复掐同一在途写", 1, w.cancelCalls.get());
        // cancelAll 之后再注销: 对称注销不得因已被 cancelAll 清走而出错(静默 no-op)
        registry.unregister(w);
    }

    @Test
    public void registerAfterCancelAll_thenCancelAgain_stillCancelled() {
        // 失败/取消后的迟到登记(入口查标志前的竞态片): cancelAll 清空后新登记者
        // 必须能被下一次 cancelAll 掐到, 不能因清空而漏掐。
        BufferWriterRegistry registry = new BufferWriterRegistry();
        registry.register(new FakeWriter());
        registry.cancelAll();
        FakeWriter late = new FakeWriter();
        registry.register(late);
        registry.cancelAll();
        assertEquals(1, late.cancelCalls.get());
    }

    /** 并发 register/unregister/cancelAll 混压: 不抛异常、每个在途写最终被掐 ≥1 次或已注销。 */
    @Test
    public void concurrentMix_noExceptionAndNoMiss() throws Exception {
        final BufferWriterRegistry registry = new BufferWriterRegistry();
        final List<FakeWriter> writers = Collections.synchronizedList(new ArrayList<>());
        final int threads = 6;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        final CountDownLatch startGate = new CountDownLatch(1);
        List<java.util.concurrent.Future<?>> futures = new ArrayList<>();
        for (int t = 0; t < threads; t++) {
            final int id = t;
            futures.add(pool.submit(() -> {
                try {
                    startGate.await();
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    return;
                }
                    try {
                        if (id % 2 == 0) {
                            // 偶数线程: 注册 → 注册 → 注销(在途窗口极短, 允许错过全部 cancelAll)
                            for (int i = 0; i < 50; i++) {
                                FakeWriter w = new FakeWriter();
                                writers.add(w);
                                registry.register(w);
                                registry.register(w);
                                registry.unregister(w);
                            }
                        } else if (id % 2 == 1 && id != threads - 1) {
                            // 奇数线程: 注册后不注销(在途) —— 必须被某次 cancelAll 掐到
                            for (int i = 0; i < 50; i++) {
                                FakeWriter w = new FakeWriter();
                                w.expectCancelled = true;
                                writers.add(w);
                                registry.register(w);
                            }
                        } else {
                            // 最后一个奇数线程: 反复 cancelAll
                            for (int i = 0; i < 100; i++) registry.cancelAll();
                        }
                    } catch (RuntimeException e) {
                        throw new AssertionError("并发混压抛异常: " + e, e);
                    }
            }));
        }
        startGate.countDown();
        for (java.util.concurrent.Future<?> f : futures) f.get(15, TimeUnit.SECONDS);
        pool.shutdown();
        assertTrue(pool.awaitTermination(15, TimeUnit.SECONDS));
        // 收尾一次 cancelAll: 此后"注册后从未注销"的在途写必须全部被掐 ≥1 次
        // (要么被混压中的某次 cancelAll, 要么被这次收尾); 注销过的允许 0 次。
        registry.cancelAll();
        synchronized (writers) {
            for (FakeWriter w : writers) {
                if (w.expectCancelled) {
                    assertTrue("在途写被 cancelAll 漏掐", w.cancelCalls.get() >= 1);
                }
            }
        }
        assertFalse(writers.isEmpty());
    }
}
