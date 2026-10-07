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
    /** 失败原因(用户可读文案) */
    public String error;
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
            case STATE_FAILED:
                return to == STATE_QUEUED; // 用户重试
            default:
                return false;
        }
    }

    public boolean canTransitionTo(int to) {
        return canTransition(state, to);
    }

    /**
     * 导出文件名: {@code <片名>_E<集号>.ts}, 非法文件名字符(\\/:*?"&lt;&gt;|)清洗为下划线.
     * 空片名回退到 E&lt;集号&gt;.ts.
     */
    public static String exportFileName(String filmTitle, int episode) {
        String base = (filmTitle == null || filmTitle.trim().isEmpty())
                ? "" : filmTitle.trim().replaceAll("[\\\\/:*?\"<>|]", "_");
        String sep = base.isEmpty() ? "" : "_";
        return base + sep + "E" + (episode + 1) + ".ts";
    }
}
