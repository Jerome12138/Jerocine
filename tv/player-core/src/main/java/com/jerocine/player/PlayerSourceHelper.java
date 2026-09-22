package com.jerocine.player;

import android.content.Intent;

import androidx.media3.common.MediaItem;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * Intent 解析 — 多源(v3) / 单源 playlist(v2) / 单 URL 三种启动模式.
 *
 * 只负责"把 Intent 变成 session 里的片源与播放列表", 实际装载交给
 * {@link PlayerSession#loadSourceIntoPlayer}. 与其它 helper 无任何互相引用.
 */
public class PlayerSourceHelper {

    private static final int SOURCES_INVALID = 0;
    private static final int SOURCES_LOADED = 1;
    private static final int SOURCES_EMPTY = 2;

    private final PlayerSession session;

    public PlayerSourceHelper(PlayerSession session) {
        this.session = session;
    }

    /** 解析 Intent 启动: 优先级 sources_json (v3 多源) > playlist (v2 单源) > 单 URL. */
    void startFromIntent(Intent intent) {
        int startIndex = intent.getIntExtra(PlayerActivity.EXTRA_START_INDEX, 0);
        long resumeMs = intent.getLongExtra(PlayerActivity.EXTRA_RESUME_MS, 0L);
        session.lastMediaItemIndex = startIndex;

        // v3: 多源模式
        String sourcesJson = intent.getStringExtra(PlayerActivity.EXTRA_SOURCES_JSON);
        if (sourcesJson != null && !sourcesJson.isEmpty()) {
            int result = parseSources(sourcesJson,
                    intent.getStringExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID));
            if (result == SOURCES_LOADED) {
                session.loadSourceIntoPlayer(session.currentSourceIndex, startIndex, resumeMs);
                return;
            }
            if (result == SOURCES_EMPTY) {
                session.host().finishPlayer();
                return;
            }
            // SOURCES_INVALID: JSON 异常 → 继续按 v2 / 单 URL 兼容处理
        }

        // v2: 单源 playlist
        ArrayList<String> urls = intent.getStringArrayListExtra(PlayerActivity.EXTRA_PLAYLIST_URLS);
        ArrayList<String> titles = intent.getStringArrayListExtra(PlayerActivity.EXTRA_PLAYLIST_TITLES);
        if (urls != null && !urls.isEmpty()) {
            session.playlistTitles = (titles != null) ? titles : new ArrayList<>();
            session.setRawUrls(urls);
            session.resetLineState();
            List<MediaItem> items = new ArrayList<>(urls.size());
            for (int i = 0; i < urls.size(); i++) {
                items.add(MediaItem.fromUri(session.mediaUriFor(i, urls.get(i))));
            }
            int safeStart = Math.max(0, Math.min(startIndex, items.size() - 1));
            session.player.setMediaItems(items, safeStart, Math.max(0L, resumeMs));
            session.player.prepare();
            session.player.setPlayWhenReady(true);
            session.host().updateTitleForCurrent();
            session.updateNetworkModeUi();
            return;
        }

        // 兼容单 URL
        String url = intent.getStringExtra(PlayerActivity.EXTRA_URL);
        String title = intent.getStringExtra(PlayerActivity.EXTRA_TITLE);
        if (url == null || url.isEmpty()) {
            session.host().finishPlayer();
            return;
        }
        session.playlistTitles = Collections.singletonList(title != null ? title : "");
        session.setRawUrls(Collections.singletonList(url));
        session.resetLineState();
        session.player.setMediaItem(MediaItem.fromUri(url));
        if (resumeMs > 0) session.player.seekTo(resumeMs);
        session.player.prepare();
        session.player.setPlayWhenReady(true);
        session.host().updateTitleForCurrent();
    }

    /** 解析多源 JSON. 返回 SOURCES_LOADED / SOURCES_EMPTY / SOURCES_INVALID. */
    private int parseSources(String sourcesJson, String currentId) {
        try {
            session.sourceList.clear();
            JSONArray arr = new JSONArray(sourcesJson);
            int curIdx = 0;
            for (int i = 0; i < arr.length(); i++) {
                JSONObject src = arr.getJSONObject(i);
                PlayerSession.SourceData sd = new PlayerSession.SourceData();
                sd.id = src.optString("id", "");
                sd.name = src.optString("name", "源 " + (i + 1));
                JSONArray eps = src.optJSONArray("episodes");
                if (eps != null) {
                    for (int j = 0; j < eps.length(); j++) {
                        JSONObject ep = eps.getJSONObject(j);
                        String u = ep.optString("url", "");
                        if (u.isEmpty()) continue;
                        sd.urls.add(u);
                        sd.titles.add(ep.optString("title", ""));
                    }
                }
                session.sourceList.add(sd);
                if (currentId != null && currentId.equals(sd.id)) curIdx = i;
            }
            if (session.sourceList.isEmpty()) return SOURCES_EMPTY;
            session.currentSourceIndex = Math.max(0, Math.min(curIdx, session.sourceList.size() - 1));
            return SOURCES_LOADED;
        } catch (Exception e) {
            return SOURCES_INVALID;
        }
    }
}
