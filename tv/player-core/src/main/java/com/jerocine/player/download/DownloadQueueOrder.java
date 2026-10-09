package com.jerocine.player.download;

import androidx.annotation.Nullable;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Set;

/**
 * 下载队列激活顺序(纯逻辑, 可单测) — 2026-10-10 用户需求:
 * "全部入队并显示, 然后并发同时下载几个, 后面的显示排队中, 队列顺序按原集序号加入, 按队列激活"。
 *
 * <p>背景: 入队管线(拉清单+过滤, 见 {@link DownloadEngine#enqueue})由 4 线程池并行执行,
 * 各任务到达"过滤完成"的先后与集序无关; media3 内部队列同批加入按 id 序、跨批按加入序,
 * 都不受集序控制。改为引擎门控: 过滤完成只落 QUEUED 行, 由 {@link DownloadEngine#activateNext}
 * 按 {@link #pickActivatable} 选出的顺序逐个交给 media3, 容量 = {@code MAX_PARALLEL - 下载中数}。
 *
 * <p>排序键: episode 升序(用户要求"按原集序号") → createdAt 升序(同集号跨批次/跨片先来先下)
 * → id 升序(全并列时稳定排序, 保证两次调用结果一致, 不在同一拍内来回换序)。
 */
public final class DownloadQueueOrder {

    private DownloadQueueOrder() {
    }

    private static final Comparator<DownloadTask> ORDER = Comparator
            .comparingInt((DownloadTask t) -> t.episode)
            .thenComparingLong(t -> t.createdAt)
            .thenComparing(t -> t.id);

    /**
     * 从全部任务中选出本轮可激活的 QUEUED 任务(有序)。
     *
     * @param all        业务表全量任务(任一线程快照, 方法不改它)
     * @param trackedIds 已在 media3 或已提交激活(在途)的任务 id — 这些一律跳过,
     *                   防止重复 {@code sendAddDownload} 触发"删旧缓存全量重下"
     * @param slots      本轮可激活的名额(= MAX_PARALLEL - 下载中数); &lt;=0 返回空
     * @return 待激活任务, 已按集序排列; 无候选/无名额返回空列表(永不 null)
     */
    public static List<DownloadTask> pickActivatable(List<DownloadTask> all,
                                                     Set<String> trackedIds, int slots) {
        List<DownloadTask> out = new ArrayList<>();
        if (all == null || slots <= 0) return out;
        List<DownloadTask> candidates = new ArrayList<>();
        for (DownloadTask t : all) {
            if (t == null || t.state != DownloadTask.STATE_QUEUED) continue;
            if (t.id == null) continue;
            if (trackedIds != null && trackedIds.contains(t.id)) continue;
            candidates.add(t);
        }
        candidates.sort(ORDER);
        for (int i = 0; i < candidates.size() && out.size() < slots; i++) {
            out.add(candidates.get(i));
        }
        return out;
    }

    /** 下载中任务数(激活名额计算用): 只认业务表 DOWNLOADING 态。 */
    public static int countDownloading(List<DownloadTask> all) {
        int n = 0;
        if (all == null) return 0;
        for (DownloadTask t : all) {
            if (t != null && t.state == DownloadTask.STATE_DOWNLOADING) n++;
        }
        return n;
    }
}
