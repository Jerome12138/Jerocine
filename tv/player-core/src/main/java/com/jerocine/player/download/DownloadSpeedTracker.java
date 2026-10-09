package com.jerocine.player.download;

import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;

/**
 * 下载速度跟踪器(纯逻辑, 可单测) — 2026-10-10 用户要求"下载页面显示总体下载速度和分集下载速度"。
 *
 * <p>原理: 引擎的进度轮询({@code DownloadEngine.pollProgress}, 2s 一拍)对每个下载中任务采样
 * {@code (bytesDownloaded, nowMs)}, 这里按差分算瞬时速度 bytes/s。无跨拍平滑 —— 轮询粒度 2s
 * 本身就是天然平滑, UI 刷新粒度与之相同, 再平滑也看不见。
 *
 * <p>线程模型: 引擎只在进度轮询线程写、UI 线程读 → 全部方法 synchronized(this)。
 * 总速度维护增量缓存(totalBpsCache), 避免每次读取遍历全表。
 */
public final class DownloadSpeedTracker {

    private static final class Sample {
        long bytes;
        long atMs;
        long bps;
    }

    private final Map<String, Sample> samples = new HashMap<>();
    /** 全表速度和(增量维护, 见方法注释)。 */
    private long totalBpsCache;

    /**
     * 记录一拍采样。
     *
     * @return 该任务瞬时速度 bytes/s; 首拍(无差分基准)或字节数回退(删除重下等)返回 0
     */
    public synchronized long onSample(String taskId, long bytes, long nowMs) {
        if (taskId == null || bytes < 0 || nowMs <= 0) return 0;
        Sample s = samples.get(taskId);
        if (s == null) {
            s = new Sample();
            s.bytes = bytes;
            s.atMs = nowMs;
            s.bps = 0;
            samples.put(taskId, s);
            return 0;
        }
        long dtMs = nowMs - s.atMs;
        long dBytes = bytes - s.bytes;
        // 基准刷新: 时钟回拨(dt<=0)或字节回退(重下)都不产速度, 只把基准推到现在
        if (dtMs <= 0 || dBytes < 0) {
            s.bytes = bytes;
            s.atMs = nowMs;
            if (s.bps != 0) {
                totalBpsCache -= s.bps;
                s.bps = 0;
            }
            return 0;
        }
        long bps = dBytes * 1000L / dtMs;
        totalBpsCache += bps - s.bps;
        s.bps = bps;
        s.bytes = bytes;
        s.atMs = nowMs;
        return bps;
    }

    /** 任务退出下载中(暂停/完成/失败/删除)时移除, 免得其速度残留在总数里。 */
    public synchronized void remove(String taskId) {
        if (taskId == null) return;
        Sample s = samples.remove(taskId);
        if (s != null) {
            totalBpsCache -= s.bps;
        }
    }

    /**
     * 只保留 seen 中的任务 — 轮询兜底清理: 正常路径由 {@link #remove} 覆盖,
     * 但崩溃恢复/遗漏回调等场景可能有僵尸条目, 每拍对账一次最稳。
     */
    public synchronized void retainAll(Set<String> seen) {
        samples.keySet().retainAll(seen == null ? Collections.emptySet() : seen);
        long t = 0;
        for (Sample s : samples.values()) t += s.bps;
        totalBpsCache = t;
    }

    /** 某任务当前速度 bytes/s; 无采样(未下载中/首拍)返回 0。 */
    public synchronized long speedBps(String taskId) {
        if (taskId == null) return 0;
        Sample s = samples.get(taskId);
        return s == null ? 0 : s.bps;
    }

    /** 全部下载中任务的总速度 bytes/s。 */
    public synchronized long totalBps() {
        return totalBpsCache;
    }

    public synchronized void clear() {
        samples.clear();
        totalBpsCache = 0;
    }
}
