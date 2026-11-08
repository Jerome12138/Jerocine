package com.jerocine.player;

/**
 * DefaultLoadControl 缓冲参数表 — 纯函数, 无任何 Android 依赖, JVM 可测(buffer-perf 规格 D⑥)。
 *
 * <p>两档(D⑥/§3.3):
 * <pre>
 *   档位                        minBuffer  maxBuffer  playback  rebuffer  backBuffer
 *   非高峰在线(= 现状 = 本地恒定)  30_000    120_000     1_500     2_500     30_000
 *   高峰在线                       60_000    150_000     3_000     5_000     30_000
 * </pre>
 * <ul>
 *   <li>非高峰档 = 改造前 PlayerActivity 两处装配的逐字现状(golden, 测试锁定
 *       「非高峰与现状完全一致」);</li>
 *   <li>本地文件模式恒现状值(peak 不影响);</li>
 *   <li>max 刻意不上 180s: {@code setPrioritizeTimeOverSizeThresholds(true)} 下字节阈值
 *       不构成闸门, 缓冲样本全在 Java 堆, max 放大即堆顶格同步放大(全仓无 largeHeap,
 *       DownloadEngine 有低配机 OOM 事故先例);</li>
 *   <li>backBuffer 与高峰正向卡顿正交, 不动。</li>
 * </ul>
 *
 * <p>高峰判定在装配时刻取一次(PeakWindow), 会话内不热切换 —— 跨峰边界的会话沿用
 * 装配时档位, 为已知限制(规格 D⑥「不做」列)。
 */
public final class PlayerLoadControlPolicy {

    /** 回看缓冲: 两档共用, 与现状一致(D⑥)。 */
    public static final int BACK_BUFFER_MS = 30_000;

    // 非高峰在线档 = 现状逐值(golden)
    private static final int STD_MIN_MS = 30_000;
    private static final int STD_MAX_MS = 120_000;
    private static final int STD_PLAYBACK_MS = 1_500;
    private static final int STD_REBUFFER_MS = 2_500;

    // 高峰在线档(D⑥)
    private static final int PEAK_MIN_MS = 60_000;
    private static final int PEAK_MAX_MS = 150_000;
    private static final int PEAK_PLAYBACK_MS = 3_000;
    private static final int PEAK_REBUFFER_MS = 5_000;

    private final int minBufferMs;
    private final int maxBufferMs;
    private final int bufferForPlaybackMs;
    private final int bufferForPlaybackAfterRebufferMs;

    private PlayerLoadControlPolicy(int minBufferMs, int maxBufferMs,
                                    int bufferForPlaybackMs,
                                    int bufferForPlaybackAfterRebufferMs) {
        this.minBufferMs = minBufferMs;
        this.maxBufferMs = maxBufferMs;
        this.bufferForPlaybackMs = bufferForPlaybackMs;
        this.bufferForPlaybackAfterRebufferMs = bufferForPlaybackAfterRebufferMs;
    }

    /**
     * 按模式与高峰态解析参数档。
     *
     * @param localMode true = 本地文件播放(恒现状值, peak 不影响)
     * @param peak      true = 高峰窗内(PeakWindow.isPeak, 装配时刻取一次)
     */
    public static PlayerLoadControlPolicy resolve(boolean localMode, boolean peak) {
        if (localMode || !peak) {
            return new PlayerLoadControlPolicy(STD_MIN_MS, STD_MAX_MS,
                    STD_PLAYBACK_MS, STD_REBUFFER_MS);
        }
        return new PlayerLoadControlPolicy(PEAK_MIN_MS, PEAK_MAX_MS,
                PEAK_PLAYBACK_MS, PEAK_REBUFFER_MS);
    }

    /** 稳态缓冲下限(ms)。 */
    public int minBufferMs() { return minBufferMs; }

    /** 稳态缓冲上限(ms)。 */
    public int maxBufferMs() { return maxBufferMs; }

    /** 起播前缓冲阈值(ms)。 */
    public int bufferForPlaybackMs() { return bufferForPlaybackMs; }

    /** 重缓冲恢复阈值(ms)。 */
    public int bufferForPlaybackAfterRebufferMs() { return bufferForPlaybackAfterRebufferMs; }

    /** 回看缓冲(ms), 恒 {@link #BACK_BUFFER_MS}。 */
    public int backBufferMs() { return BACK_BUFFER_MS; }
}
