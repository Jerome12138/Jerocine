package com.jerocine.player.download;

/**
 * 下载任务模型 — 纯 Java, 不依赖 Android, 可直接单测.
 *
 * <p>id = {@code filmId:sourceKey:episode}(幂等键, DB 层 UNIQUE 兜底):
 * 同一影片同一源同一集只允许一个任务, 重复入队被 {@link DownloadTaskStore#insertIgnore} 忽略。
 *
 * <p>状态机(非法迁移直接返回 false, 由调用方提示或忽略):
 * <pre>
 *   QUEUED ──→ DOWNLOADING ──→ COMPLETED ──→ EXPORTING ──→ COMPLETED(exportedPath 非空)
 *     │              │
 *     ├──→ PAUSED ←──┤              (用户暂停; PAUSED ──→ QUEUED 续传)
 *     └──→ FAILED ←──┘              (失败可重试; FAILED ──→ QUEUED 重试)
 *
 *   FILTERING ──→ QUEUED | PAUSED | FAILED   (入队起点: 拉清单+广告过滤阶段, 见 DownloadEngine.enqueue)
 * </pre>
 * 删除不走状态机(任意态都可删除, 见 DownloadActivity)。
 */
public final class DownloadTask {

    public static final int STATE_QUEUED = 0;
    public static final int STATE_DOWNLOADING = 1;
    public static final int STATE_PAUSED = 2;
    public static final int STATE_COMPLETED = 3;
    public static final int STATE_FAILED = 4;
    public static final int STATE_EXPORTING = 5;
    /**
     * 拉清单+广告过滤阶段(2026-10-10): 入队后先落库此状态, 让"下载中"列表立即可见
     * (过滤是网络操作, 含最多 3 轮重试, 可达数十秒; 旧行为过滤完才入库 → 用户看空白)。
     * 过滤结束 → QUEUED(成功/原始流兜底) 或 FAILED; 用户可在过滤期间暂停。
     */
    public static final int STATE_FILTERING = 6;

    /**
     * 原始流兜底任务的常驻角标文案(2026-10-10 用户要求可见): 该集离线播放/导出含潜在广告段。
     * 持久化在 {@code rawFallback} 列, 下载中/已完成行都要带。
     */
    public static final String RAW_FALLBACK_BADGE = "原始流(含广告)";

    /** 幂等键 = filmId:sourceKey:episode */
    public String id;
    public String filmId;
    public String filmTitle;
    /** 源标识(如 src_lz:lzm3u8), 用于区分同一影片不同源的任务 */
    public String sourceKey;
    public String sourceName;
    /** 集号(从 0 开始, 与播放器 playlist 下标一致) */
    public int episode;
    /** 集名(综艺多为日期/期名, 可空) */
    public String episodeTitle;
    /** 源站 m3u8 原始 URL — Media3 下载的 uri 与缓存键根 */
    public String srcUrl;
    /** 过滤后清单全文(下载时保存, 离线播放用) */
    public String filteredPlaylist;
    public int state = STATE_QUEUED;
    public long progressBytes;
    public long totalBytes;
    /** 失败原因(用户可读文案); FILTERING 阶段临时承载过程文案(如"广告过滤中 · 重试 2/3") */
    public String error;
    /** true = 过滤失败按策略原始流兜底(见 DownloadFilterFallbackPolicy), 离线播放含潜在广告段 */
    public boolean rawFallback;
    /** 下载缓存区目录 download_cache/&lt;filmId&gt;/&lt;episode&gt; */
    public String cacheDir;
    /** 导出 .ts 后的系统下载目录路径(可空) */
    public String exportedPath;
    public long createdAt;
    public long updatedAt;

    public static String idFor(String filmId, String sourceKey, int episode) {
        // 注意: 这里**不做路径净化** —— id 是持久化主键, 清洗会改变既有任务的 id
        // (旧库里已入库的行再也查不到)。路径侧的防护在 DownloadEngine.episodeCacheDir。
        return (filmId == null ? "" : filmId) + ":" + (sourceKey == null ? "" : sourceKey)
                + ":" + episode;
    }

    /**
     * 路径片段净化 — 只保留 {@code [A-Za-z0-9._-]}, 其余(含 {@code /} 与 {@code \})全替换为下划线。
     *
     * <p><b>为什么必须有</b>: {@code filmId} 来源是 Intent extra,而 PlayerActivity 已是
     * {@code exported="true"}(为接ACTION_VIEW 的本地文件), 任意第三方 App 都能注入
     * {@code film_id=../../databases/x}。它一旦进入路径拼接,配合
     * {@link DownloadEngine#remove()} 的 {@code deleteRecursive} 就是<b>递归删除应用私有目录树里
     * 任意路径</b> —— 零成本的实际破坏。片名/源名等同样入路径的值走同一道清洗。
     *
     * <p>保留 {@code .} 与 {@code -} 是为了不破坏正常形态(如 "3.10"、"film-2");
     * 但 {@code ..} 这类"纯点"必须单独处理, 否则清洗后仍可能是 {@code ..} 或 {@code _.._}。
     */
    public static String safeSegment(String raw) {
        if (raw == null || raw.isEmpty()) return "_";
        String s = raw.replaceAll("[^A-Za-z0-9._-]", "_");
        // 纯点/点点的变体: "..", ".", "...", "._." 等都无意义且可能参与上跳
        if (s.chars().allMatch(c -> c == '.')) return "_";
        return s;
    }

    /** 状态机迁移表 — 非法迁移返回 false. */
    public static boolean canTransition(int from, int to) {
        switch (from) {
            case STATE_QUEUED:
                return to == STATE_DOWNLOADING || to == STATE_PAUSED || to == STATE_FAILED;
            case STATE_DOWNLOADING:
                return to == STATE_COMPLETED || to == STATE_PAUSED || to == STATE_FAILED;
            case STATE_PAUSED:
                return to == STATE_QUEUED; // 用户续传(重试)
            case STATE_COMPLETED:
                return to == STATE_EXPORTING;
            case STATE_EXPORTING:
                return to == STATE_COMPLETED; // 导出完成, 回 COMPLETED 并带 exportedPath
            case STATE_FILTERING:
                // 过滤结束: 成功/兜底 → QUEUED; 用户暂停 → PAUSED(管线末端注册 media3 后回挂 stopReason);
                // 失败(重试耗尽防御路径/清单拉取失败) → FAILED。不直接到 DOWNLOADING/COMPLETED。
                return to == STATE_QUEUED || to == STATE_PAUSED || to == STATE_FAILED;
            case STATE_FAILED:
                return to == STATE_QUEUED; // 用户重试
            default:
                return false;
        }
    }

    /**
     * 行标题的片源标识(2026-10-10 用户要求): 同一片常从多个源下载, 标题不带源分不清是哪条线的缓存。
     * sourceName 优先(如"量子"), 空 则退 sourceKey(如"src_lz:lzm3u8"); 都空返回 ""。
     */
    public static String sourceTag(String sourceName, String sourceKey) {
        String s = (sourceName == null || sourceName.trim().isEmpty())
                ? sourceKey : sourceName;
        return s == null ? "" : s.trim();
    }

    /**
     * 给行内信息文案追加原始流兜底角标 — rawFallback 任务的常驻标记(下载中/已完成行都带)。
     */
    public static String withRawBadge(boolean rawFallback, String base) {
        if (base == null) return null;
        if (!rawFallback) return base;
        // 空基础文案不带" · "前缀(行信息为空时角标单独成词)
        return base.isEmpty() ? RAW_FALLBACK_BADGE : base + " · " + RAW_FALLBACK_BADGE;
    }

    public boolean canTransitionTo(int to) {
        return canTransition(state, to);
    }

    /**
     * 导出文件名: {@code <片名>_E<集号>.ts}, 非法文件名字符(\\/:*?"&lt;&gt;|)清洗为下划线.
     * 空片名回退到 E&lt;集号&gt;.ts.
     */
    /**
     * 导出文件名: {@code <片名>_E<集号>_<集名>.ts}(集名为空退回 {@code <片名>_E<集号>.ts}),
     * 非法文件名字符(\\/:*?"<>|及括号方括号)清洗为下划线. 空片名回退到 E<集号>.ts.
     *
     * <p>2026-10-08: 按用户要求文件名加该集名称(episodeTitle 建任务时已落库)。
     */
    public static String exportFileName(String filmTitle, int episode, String episodeTitle) {
        String base = cleanNamePart(filmTitle);
        String epName = episodeTitle == null ? "" : episodeTitle.trim();
        // 源返回的集标题常自带"片名 · "前缀(真机实锤: <片名>_E1_<片名>_xxx.ts 重复),
        // 以片名开头时剥掉前缀和紧随的分隔符(·/-/_/空格), 得到纯集名。
        String rawTitle = filmTitle == null ? "" : filmTitle.trim();
        if (!base.isEmpty() && !rawTitle.isEmpty() && epName.startsWith(rawTitle)) {
            epName = epName.substring(rawTitle.length()).replaceFirst("^[\\s·・\\-_·]+", "");
        }
        epName = cleanNamePart(epName);
        StringBuilder sb = new StringBuilder();
        if (!base.isEmpty()) sb.append(base).append('_');
        sb.append('E').append(episode + 1);
        if (!epName.isEmpty()) sb.append('_').append(epName);
        return sb.append(".ts").toString();
    }

    /** 文件名片段清洗: 去首尾空白 + 非法字符(含括号)替换为下划线。 */
    private static String cleanNamePart(String raw) {
        if (raw == null) return "";
        return raw.trim().replaceAll("[\\\\/:*?\"<>|()\\[\\]]", "_");
    }
}
