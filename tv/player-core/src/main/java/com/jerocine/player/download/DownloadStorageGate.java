package com.jerocine.player.download;

import java.io.File;

/**
 * 下载空间预检(2026-10-10 用户拍板): 下载缓存分区可用空间低于 {@link #MIN_FREE_BYTES} 时
 * 不发新任务、暂停在途任务, 并提示用户清理。
 *
 * <p>为什么必须有闸: 下载缓存用 {@code NoOpCacheEvictor}(主动下载永不清), 只增不减;
 * 磁盘被写满会拖垮整个系统(在线播放缓存/导出/其它应用全受影响), 不能等系统层面报错。
 * 红线留 2GB 是给系统与播放缓存(video_cache, 可再生 LRU)留余量。
 *
 * <p>纯逻辑类(可单测): 字节判定、分区探测、文案在这里; {@link DownloadEngine} 只负责
 * 查分区、改状态、弹提示。
 */
final class DownloadStorageGate {

    /** 可用空间红线: 2GB。 */
    static final long MIN_FREE_BYTES = 2L * 1024L * 1024L * 1024L;

    private DownloadStorageGate() {
    }

    /** 可用字节数是否低于红线(恰好等于红线 = 放行)。 */
    static boolean belowFloor(long usableBytes) {
        return usableBytes < MIN_FREE_BYTES;
    }

    /**
     * 目录所在分区的可用字节数。
     *
     * <p>{@link File#getUsableSpace()} 对不存在的路径返回 0(会被误判为"没空间"),
     * 这里向上找最多 3 级父目录取真实分区; 全部落空返回 {@link Long#MAX_VALUE}(放行 ——
     * 探测不到分区时不能把下载永久卡死, 轮询下一拍会再试)。
     */
    static long usableBytes(File dir) {
        File probe = dir;
        for (int i = 0; i < 3 && probe != null && !probe.isDirectory(); i++) {
            probe = probe.getParentFile();
        }
        return probe != null ? probe.getUsableSpace() : Long.MAX_VALUE;
    }

    /** 暂停原因文案(写任务行 + 上屏, 告诉用户为什么停、该干什么)。 */
    static String pauseReason() {
        return "存储空间不足(剩余<2GB) · 清理后点继续";
    }

    /** toast 文案(引擎侧一次性提醒)。 */
    static String toastText() {
        return "存储空间不足，已暂停下载，请清理空间后重试";
    }
}
