package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 本地/在线播放统一状态(2026-10-09 重构)的纯逻辑测试。
 *
 * 覆盖三层:
 *   ① {@link PlayerModes} 纯函数: 单集本地清单判据 / 播放本地判定 / 可切换判定 / 文案;
 *   ② {@link PlayerSession} 集成判定(JVM 实例 + 假 Host): isEpisodePlayingLocal /
 *      isEpisodeLocalAvailable / isEpisodeDownloadedOnSource / playbackUriFor 的本地优先与
 *      preferOnline 翻转;
 *   ③ 会话形态: 单集本地清单(bypassFilter + file://)整会话视为本地、无在线可切。
 */
public class PlayerModesTest {

    private static final String RAW = "https://cdn.example.com/video/01.m3u8";
    private static final String LOCAL = "file:///data/xx/download_cache/149293/0/playlist.m3u8";

    /** 最小 Host 假实现(与 PlayerSessionRelayTest 同款, 全部 no-op). */
    private static final class NoopHost implements PlayerSession.Host {
        @Override public android.content.Context context() { return null; }
        @Override public void showCenterToast(String msg, long durationMs) { }
        @Override public void showCenterIcon(int drawableRes, long durationMs) { }
        @Override public void showCenterIconPersistent(int drawableRes) { }
        @Override public void hideGestureToast() { }
        @Override public void finishPlayer() { }
        @Override public void updateTitleForCurrent() { }
        @Override public void emitEvent(String name, org.json.JSONObject payload) { }
        @Override public String filmId() { return ""; }
        @Override public void toggleAdFilter() { }
        @Override public void renderAdFilterSwitch(boolean on) { }
        @Override public void toggleNetworkMode() { }
        @Override public void toggleLocalOnline() { }
        @Override public void renderNetworkMode(boolean relay) { }
        @Override public androidx.media3.ui.PlayerView playerView() { return null; }
        @Override public void renderAdFilterBadge(AdFilterStatus status) { }
        @Override public void renderSpeedText(String label) { }
        @Override public void renderSpeedDot(boolean on) { }
        @Override public void renderSkipDot(boolean on) { }
        @Override public boolean dispatchToSuper(android.view.KeyEvent event) { return false; }
    }

    // ==================== ① PlayerModes 纯函数 ====================

    @Test
    public void singleLocalPlaylist_requiresBypassAndSingleFileUri() {
        assertTrue(PlayerModes.isSingleLocalPlaylist(
                true, Collections.singletonList("file:///x/playlist.m3u8")));
        // 大小写不敏感
        assertTrue(PlayerModes.isSingleLocalPlaylist(
                true, Collections.singletonList("FILE:///x/playlist.m3u8")));
        // 非 bypass → 否(在线多源会话也有一条 file:// 的可能被排除)
        assertFalse(PlayerModes.isSingleLocalPlaylist(
                false, Collections.singletonList("file:///x/playlist.m3u8")));
        // 多条列表 → 否
        assertFalse(PlayerModes.isSingleLocalPlaylist(
                true, Arrays.asList("file:///a.m3u8", "file:///b.m3u8")));
        // http 地址 → 否
        assertFalse(PlayerModes.isSingleLocalPlaylist(true, Collections.singletonList(RAW)));
        // 空/单 null → 否
        assertFalse(PlayerModes.isSingleLocalPlaylist(true, Collections.<String>emptyList()));
        assertFalse(PlayerModes.isSingleLocalPlaylist(
                true, Collections.singletonList((String) null)));
        assertFalse(PlayerModes.isSingleLocalPlaylist(true, null));
    }

    @Test
    public void isEpisodePlayingLocal_matrix() {
        Map<Integer, String> locals = new HashMap<>();
        locals.put(0, LOCAL);
        java.util.Set<Integer> preferOnline = ConcurrentHashMap.newKeySet();

        // 已下载集默认播本地
        assertTrue(PlayerModes.isEpisodePlayingLocal(false, false, locals, preferOnline, 0));
        // 未下载集在线
        assertFalse(PlayerModes.isEpisodePlayingLocal(false, false, locals, preferOnline, 1));
        // 用户切在线后 → 在线
        preferOnline.add(0);
        assertFalse(PlayerModes.isEpisodePlayingLocal(false, false, locals, preferOnline, 0));
        // 切回本地
        preferOnline.remove(0);
        assertTrue(PlayerModes.isEpisodePlayingLocal(false, false, locals, preferOnline, 0));
        // 单集本地会话: 恒为本地(哪怕表空)
        assertTrue(PlayerModes.isEpisodePlayingLocal(false, true, locals, preferOnline, 0));
        assertTrue(PlayerModes.isEpisodePlayingLocal(
                false, true, Collections.emptyMap(), preferOnline, 5));
        // 本地文件模式: 恒为本地
        assertTrue(PlayerModes.isEpisodePlayingLocal(true, false, locals, preferOnline, 0));
        // 非法索引
        assertFalse(PlayerModes.isEpisodePlayingLocal(false, false, locals, preferOnline, -1));
        // 空表
        assertFalse(PlayerModes.isEpisodePlayingLocal(
                false, false, Collections.emptyMap(), preferOnline, 0));
    }

    @Test
    public void isEpisodeLocalAvailable_requiresOnlineAlternative() {
        Map<Integer, String> locals = new HashMap<>();
        locals.put(0, LOCAL);
        // 完整会话: 已下载集可切
        assertTrue(PlayerModes.isEpisodeLocalAvailable(false, false, locals, 0));
        // 未下载集不可切
        assertFalse(PlayerModes.isEpisodeLocalAvailable(false, false, locals, 1));
        // 单集本地/本地文件模式: 没有在线可切 → 恒 false(按钮隐藏)
        assertFalse(PlayerModes.isEpisodeLocalAvailable(false, true, locals, 0));
        assertFalse(PlayerModes.isEpisodeLocalAvailable(true, false, locals, 0));
        // 非法索引/空表
        assertFalse(PlayerModes.isEpisodeLocalAvailable(false, false, locals, -1));
        assertFalse(PlayerModes.isEpisodeLocalAvailable(
                false, false, Collections.emptyMap(), 0));
    }

    @Test
    public void labels_areSingleSourced() {
        assertEquals("本集本地播放", PlayerModes.localBadgeText());
        assertEquals("切在线", PlayerModes.localToggleText(true));
        assertEquals("切本地", PlayerModes.localToggleText(false));
        // 选集条目: 标签文案(右侧灰色小字, 行本身是这一集 → 只说"已下载")
        assertEquals("已下载", PlayerModes.downloadedEpisodeTag());
        // 换源条目: 标签指明是"当前播放的这集"在该源有副本(用户拍板的措辞)
        assertEquals("已下载本集", PlayerModes.downloadedSourceTag());
        // 别源提示: 源名缺失时兜底
        assertEquals("本集已在「lz」源下载, 更多菜单可换源播本地",
                PlayerModes.otherSourceToastText("lz"));
        assertEquals("本集已在「其他」源下载, 更多菜单可换源播本地",
                PlayerModes.otherSourceToastText(""));
        assertEquals("本集已在「其他」源下载, 更多菜单可换源播本地",
                PlayerModes.otherSourceToastText(null));
    }

    // ==================== ② PlayerSession 集成判定 ====================

    @Test
    public void session_playbackUriFor_prefersLocalUnlessPreferOnline() {
        PlayerSession s = new PlayerSession(new NoopHost());
        s.setRawUrls(Arrays.asList(RAW, "https://cdn.example.com/video/02.m3u8"));
        s.localEpisodePlaylists = single(0, LOCAL);

        // 已下载集 → file:// 本地清单
        assertEquals(LOCAL, s.playbackUriFor(0, RAW));
        // 未下载集 → 在线路径(mediaUriFor): 过滤开+源可代理+代理地址 → 端侧直连原始地址
        assertEquals("https://cdn.example.com/video/02.m3u8", s.playbackUriFor(1, s.currentRawUrls.get(1)));
        // 用户切在线后 → 回到在线路径
        s.preferOnlineIdx.add(0);
        assertEquals(RAW, s.playbackUriFor(0, RAW));
    }

    @Test
    public void session_unifiedGates_followPlayingLocal() {
        PlayerSession s = new PlayerSession(new NoopHost());
        s.setRawUrls(Collections.singletonList(RAW));
        s.proxyBase = "https://example.com/api";
        s.localEpisodePlaylists = single(0, LOCAL);

        // 本地集: 无"线路"概念
        assertTrue(s.isEpisodePlayingLocal(0));
        assertFalse(s.isRelay(0));
        assertFalse(s.canSwitchNetworkMode(0));
        // 在线集(切 preferOnline 后): 线路语义恢复(过滤开+源可代理+代理地址+m3u8 → 可切)
        s.preferOnlineIdx.add(0);
        assertFalse(s.isEpisodePlayingLocal(0));
        assertTrue(s.canSwitchNetworkMode(0));
    }

    @Test
    public void session_downloadedOnSource_currentVsOther() {
        PlayerSession s = new PlayerSession(new NoopHost());
        PlayerSession.SourceData bf = new PlayerSession.SourceData();
        bf.id = "src_bf:bfzym3u8";
        bf.name = "bf源";
        PlayerSession.SourceData lz = new PlayerSession.SourceData();
        lz.id = "src_lz:lzm3u8";
        lz.name = "lz源";
        s.sourceList.add(bf);
        s.sourceList.add(lz);
        s.currentSourceIndex = 0; // 当前播 bf
        s.localEpisodePlaylists = single(0, LOCAL); // bf 的 E1 已下载
        s.otherSourceLocalEpisodes = new HashMap<>();
        com.jerocine.player.download.DownloadTask t =
                new com.jerocine.player.download.DownloadTask();
        t.episode = 1;
        t.sourceKey = "src_lz:lzm3u8";
        t.sourceName = "lz源";
        s.otherSourceLocalEpisodes.put(1, t); // lz 的 E2 已下载

        // 当前源: 看本地副本表
        assertTrue(s.isEpisodeDownloadedOnSource(0, 0));
        assertFalse(s.isEpisodeDownloadedOnSource(1, 0));
        // 别源: 看 otherSource 表(按 sourceKey 匹配)
        assertFalse(s.isEpisodeDownloadedOnSource(0, 1));
        assertTrue(s.isEpisodeDownloadedOnSource(1, 1));
        // 非法源下标
        assertFalse(s.isEpisodeDownloadedOnSource(0, -1));
        assertFalse(s.isEpisodeDownloadedOnSource(0, 9));
    }

    @Test
    public void session_singleLocalPlaylist_andLocalMapRegistration() {
        PlayerSession s = new PlayerSession(new NoopHost());
        s.setRawUrls(Collections.singletonList("file:///x/playlist.m3u8"));
        s.currentBypassFilter = true;
        // 单集本地形态: 判定命中
        assertTrue(s.isSingleLocalPlaylist());
        assertTrue(s.isEpisodePlayingLocal(0));
        // 无在线可切
        assertFalse(s.isEpisodeLocalAvailable(0));
        // Activity 注册本地副本表后, playbackUriFor(0) 也稳回 file://(重试路径不跑飞)
        s.localEpisodePlaylists = single(0, "file:///x/playlist.m3u8");
        assertEquals("file:///x/playlist.m3u8", s.playbackUriFor(0, "file:///x/playlist.m3u8"));
    }

    private static Map<Integer, String> single(int idx, String uri) {
        Map<Integer, String> m = new HashMap<>();
        m.put(idx, uri);
        return m;
    }
}
