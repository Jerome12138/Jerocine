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
 * <p>背景: 入队只落 QUEUED 行(全部立即可见), 广告过滤是<b>队列的前置步骤</b>(2026-10-10 三轮
 * 用户拍板): {@link com.jerocine.player.download.DownloadEngine#activateNext} 按
 * {@link #pickActivatable} 选出的顺序出队, 出队任务先跑过滤管线(FILTERING)再交给 media3 下载;
 * <b>过滤中也占一个名额</b>(见 {@link #countOccupied}), 所以整条链路并发恒 ≤ MAX_PARALLEL。
 * media3 内部队列同批加入按 id 序、跨批按加入序, 都不受集序控制 —— 顺序必须在引擎侧控制。
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

    /**
     * 占用名额的任务数(激活名额计算用): 下载中(DOWNLOADING) + 过滤中(FILTERING)。
     *
     * <p>过滤中也占名额是三轮重设计(2026-10-10)的语义: 过滤是队列前置步骤, 出队即占坑,
     * 否则 3 个名额会同时被 3 个过滤中的任务占住线程却不开下, 下载反而饿死 ——
     * 反过来过滤也计入, 保证"同时最多 3 条链路(过滤或下载)在跑", 恒定可控。
     */
    public static int countOccupied(List<DownloadTask> all) {
        int n = 0;
        if (all == null) return 0;
        for (DownloadTask t : all) {
            if (t == null) continue;
            if (t.state == DownloadTask.STATE_DOWNLOADING
                    || t.state == DownloadTask.STATE_FILTERING) n++;
        }
        return n;
    }
}
