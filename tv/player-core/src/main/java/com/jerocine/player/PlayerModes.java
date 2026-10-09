package com.jerocine.player;

import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

/**
 * 本地/在线播放的<b>统一判据与文案</b>(纯函数, 无 Android 依赖, 可 JVM 单测)。
 *
 * <p>背景(2026-10-09 重构): 原先"本地文件播放(localPlayback) / 离线播放(offlinePlayback,
 * 下载页播单集) / 本地优先(localEpisodePlaylists)"三套状态各管一段, 判据散落在
 * PlayerActivity/PlayerSession/各 helper 里, 出现过"离线播放走中转自愈""本地集弹
 * 已过滤广告""别源下载的集菜单无提示"等串味 bug。重构后收敛为本类的一个核心问题:
 * <b>"第 idx 集现在是不是在播本地?"</b>(见 {@link #isEpisodePlayingLocal})与
 * <b>"第 idx 集有没有本地副本可切?"</b>(见 {@link #isEpisodeLocalAvailable}),
 * 角标/底栏按钮/线路自愈/预取/过滤提示全部只看这两个答案。
 *
 * <p>状态模型(重构后仅两种会话形态, 不再有独立的"离线模式"):
 * <ul>
 *   <li><b>完整会话</b>(在线装载, 已下载集自动播 file:// 本地清单): 本地优先按集生效,
 *       底栏「切本地/切在线」按集切换(preferOnlineIdx);</li>
 *   <li><b>单集本地清单</b>(下载页无影片上下文播单集): 整个会话只有一条 file:// 清单,
 *       无在线可切 —— 判据见 {@link #isSingleLocalPlaylist}。</li>
 * </ul>
 * localPlayback(文件管理器 ACTION_VIEW 打开本地文件)保持独立: 它连"集"的概念都没有。
 */
public final class PlayerModes {

    private PlayerModes() {
    }

    /**
     * 播放列表是否是"单条本地清单"(下载管理页无影片上下文时的播放入口)。
     * 判据: 单 URL 兼容模式(bypassFilter) + 列表仅 1 条 + 是 file:// 地址。
     */
    public static boolean isSingleLocalPlaylist(boolean bypassFilter, List<String> rawUrls) {
        if (!bypassFilter || rawUrls == null || rawUrls.size() != 1) return false;
        String u = rawUrls.get(0);
        return u != null && u.toLowerCase(Locale.US).startsWith("file://");
    }

    /**
     * 核心判据: 第 idx 集现在是否在播本地(file:// 清单, 分片来自下载缓存/本地盘)。
     *
     * @param localPlayback          本地文件模式(ACTION_VIEW/SAF): 恒为本地
     * @param singleLocal            会话是单集本地清单({@link #isSingleLocalPlaylist})
     * @param localEpisodePlaylists  本地副本表(episode → file:// 清单)
     * @param preferOnlineIdx        用户显式切回在线的集
     */
    public static boolean isEpisodePlayingLocal(
            boolean localPlayback,
            boolean singleLocal,
            Map<Integer, ?> localEpisodePlaylists,
            Set<Integer> preferOnlineIdx,
            int idx) {
        if (localPlayback || singleLocal) return true;
        if (idx < 0 || localEpisodePlaylists == null) return false;
        return localEpisodePlaylists.containsKey(idx) && !preferOnlineIdx.contains(idx);
    }

    /**
     * 第 idx 集有没有本地副本<b>且存在在线替代</b>(完整会话才成立) ——
     * 决定底栏「切本地/切在线」按钮的显隐: 单集本地清单/本地文件模式没有在线可切, 恒 false。
     */
    public static boolean isEpisodeLocalAvailable(
            boolean localPlayback,
            boolean singleLocal,
            Map<Integer, ?> localEpisodePlaylists,
            int idx) {
        if (localPlayback || singleLocal) return false;
        if (idx < 0 || localEpisodePlaylists == null) return false;
        return localEpisodePlaylists.containsKey(idx);
    }

    // ============================ 文案(单一出处) ============================

    /** 右上角角标: 本地播放时的文案(顶替原"去广告"角标位; 在线集维持过滤角标不动)。 */
    public static String localBadgeText() {
        return "本集本地播放";
    }

    /** 底栏切换按钮文案: 正在播本地 → 提供切在线; 正在播在线 → 提供切本地。 */
    public static String localToggleText(boolean playingLocal) {
        return playingLocal ? "切在线" : "切本地";
    }

    /** 选集弹窗条目的已下载标签(列表行右侧灰色小字; 该行本身就是这一集, 只说"已下载")。 */
    public static String downloadedEpisodeTag() {
        return "已下载";
    }

    /**
     * 换源弹窗条目的已下载标签(列表行右侧灰色小字)。
     * 用"已下载本集"而非"已下载" —— 换源列表标记的是「当前播放的这集」在该源有副本,
     * 不是整部片都下过, 用户拍板(2026-10-09)这个措辞更准确。
     */
    public static String downloadedSourceTag() {
        return "已下载本集";
    }

    /** 播到"本集在别的源已下载"的在线集时的一次性提示。 */
    public static String otherSourceToastText(String sourceName) {
        String n = sourceName == null || sourceName.isEmpty() ? "其他" : sourceName;
        return "本集已在「" + n + "」源下载, 更多菜单可换源播本地";
    }
}
