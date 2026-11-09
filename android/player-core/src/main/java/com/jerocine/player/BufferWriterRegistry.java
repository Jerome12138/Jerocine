package com.jerocine.player;

import java.util.ArrayList;
import java.util.Collections;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Set;

/**
 * 在途写登记处: cancel 要掐<b>所有</b>在途, 而非单个。经 {@link Cancellable} 抽象可 JVM 单测。
 *
 * <p>buffer-perf 规格 D④: 并行下多个在途 CacheWriter, 旧 {@code activeWriter} 单引用只掐一个;
 * 本类替代单引用成为唯一登记处 —— 分片任务 register(写前) / unregister(写后 finally 对称注销),
 * cancel()/失败路径 cancelAll() 一次掐干净。
 *
 * <p>纯类无任何 Android 依赖(规格 §3.2 铁律); 线程安全(登记在写线程, 掐在 cancel/失败线程)。
 */
public final class BufferWriterRegistry {

    /** CacheWriter::cancel 的适配接口(避免纯类直接依赖 media3 类型)。 */
    public interface Cancellable {
        void cancel();
    }

    /** 身份集合(不重写 equals 的对象按引用判等): 同一实例重复 register 只算一份。 */
    private final Set<Cancellable> writers = Collections.newSetFromMap(new IdentityHashMap<>());

    /** 分片任务写前登记(与 finally 里的 {@link #unregister} 对称)。 */
    public synchronized void register(Cancellable w) {
        writers.add(w);
    }

    /** 分片任务 finally 对称注销; 未登记(如已被 cancelAll 清走)静默 no-op。 */
    public synchronized void unregister(Cancellable w) {
        writers.remove(w);
    }

    /**
     * 掐所有在途: 掐前取快照并清空 → 重复调用幂等(同一实例不会被重复掐), 空集安全。
     * 单个 cancel 抛异常吞掉继续其余(对齐 BufferPrefetcher 旧 cancel 的吞异常口径)。
     */
    public void cancelAll() {
        final List<Cancellable> snapshot;
        synchronized (this) {
            if (writers.isEmpty()) {
                return;
            }
            snapshot = new ArrayList<>(writers);
            writers.clear();
        }
        for (Cancellable w : snapshot) {
            try {
                w.cancel();
            } catch (Exception ignore) {
                // 单个在途写取消失败不影响其余(对齐 :236-239)
            }
        }
    }
}
