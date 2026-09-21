package com.jerocine.player;

import android.content.Intent;
import android.view.View;

import androidx.media3.common.MediaItem;

import java.util.ArrayList;
import java.util.List;

/**
 * 源管理 + Intent 解析 — 多源/单源/单URL 三种启动模式。
 */
public class PlayerSourceHelper {

    /** 多源模式 state (单源模式时 sourceList 为空) */
    static class SourceData {
        String id;
        String name;
        ArrayList<String> urls = new ArrayList<>();
        ArrayList<String> titles = new ArrayList<>();
    }

    private final PlayerActivity activity;

    final ArrayList<SourceData> sourceList = new ArrayList<>();
    int currentSourceIndex = 0;
    List<String> playlistTitles = new ArrayList<>();
    int lastMediaItemIndex = 0;

    public PlayerSourceHelper(PlayerActivity activity) {
        this.activity = activity;
    }

    /** 解析 Intent 启动: 优先级 sources_json (v3 多源) > playlist (v2 单源) > 单 URL */
    void startFromIntent(Intent intent) {
        int startIndex = intent.getIntExtra(PlayerActivity.EXTRA_START_INDEX, 0);
        long resumeMs = intent.getLongExtra(PlayerActivity.EXTRA_RESUME_MS, 0L);
        lastMediaItemIndex = startIndex;

        // v3: 多源模式
        String sourcesJson = intent.getStringExtra(PlayerActivity.EXTRA_SOURCES_JSON);
        if (sourcesJson != null && !sourcesJson.isEmpty()) {
            try {
                sourceList.clear();
                org.json.JSONArray arr = new org.json.JSONArray(sourcesJson);
                String currentId = intent.getStringExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID);
                int curIdx = 0;
                for (int i = 0; i < arr.length(); i++) {
                    org.json.JSONObject src = arr.getJSONObject(i);
                    SourceData sd = new SourceData();
                    sd.id = src.optString("id", "");
                    sd.name = src.optString("name", "源 " + (i + 1));
                    org.json.JSONArray eps = src.optJSONArray("episodes");
                    if (eps != null) {
                        for (int j = 0; j < eps.length(); j++) {
                            org.json.JSONObject ep = eps.getJSONObject(j);
                            String u = ep.optString("url", "");
                            if (u.isEmpty()) continue;
                            sd.urls.add(u);
                            sd.titles.add(ep.optString("title", ""));
                        }
                    }
                    sourceList.add(sd);
                    if (currentId != null && currentId.equals(sd.id)) curIdx = i;
                }
                if (sourceList.isEmpty()) { activity.finish(); return; }
                currentSourceIndex = Math.max(0, Math.min(curIdx, sourceList.size() - 1));
                loadSourceIntoPlayer(currentSourceIndex, startIndex, resumeMs);
                return;
            } catch (Exception e) {
                // JSON 异常 fallback 单 URL
            }
        }

        // v2: 单源 playlist
        ArrayList<String> urls = intent.getStringArrayListExtra(PlayerActivity.EXTRA_PLAYLIST_URLS);
        ArrayList<String> titles = intent.getStringArrayListExtra(PlayerActivity.EXTRA_PLAYLIST_TITLES);
        if (urls != null && !urls.isEmpty()) {
            playlistTitles = (titles != null) ? titles : new ArrayList<>();
            activity.adFilterHelper.currentRawUrls = new ArrayList<>(urls);
            activity.adFilterHelper.forceRawIdx.clear();
            List<MediaItem> items = new ArrayList<>(urls.size());
            for (int i = 0; i < urls.size(); i++) items.add(MediaItem.fromUri(urls.get(i)));
            activity.player.setMediaItems(items, Math.max(0, Math.min(startIndex, items.size() - 1)), resumeMs);
            activity.player.prepare();
            activity.player.setPlayWhenReady(true);
            updateTitleForCurrent();
            return;
        }

        // 兼容单 URL
        String url = intent.getStringExtra(PlayerActivity.EXTRA_URL);
        String title = intent.getStringExtra(PlayerActivity.EXTRA_TITLE);
        if (url == null || url.isEmpty()) {
            activity.finish();
            return;
        }
        playlistTitles = new ArrayList<>();
        playlistTitles.add(title != null ? title : "");
        activity.player.setMediaItem(MediaItem.fromUri(url));
        if (resumeMs > 0) activity.player.seekTo(resumeMs);
        activity.player.prepare();
        activity.player.setPlayWhenReady(true);
        activity.titleText.setText(title != null ? title : "");
    }

    /** 把指定 source 的 episodes 装入 player; 从 startEpisodeIndex 位置开始, resumeMs 续播 */
    void loadSourceIntoPlayer(int sourceIdx, int startEpisodeIndex, long resumeMs) {
        if (sourceIdx < 0 || sourceIdx >= sourceList.size()) return;
        SourceData src = sourceList.get(sourceIdx);
        playlistTitles = new ArrayList<>(src.titles);
        activity.adFilterHelper.currentRawUrls = new ArrayList<>(src.urls);
        activity.adFilterHelper.forceRawIdx.clear();
        List<MediaItem> items = new ArrayList<>(src.urls.size());
        for (int i = 0; i < src.urls.size(); i++) {
            items.add(MediaItem.fromUri(src.urls.get(i)));
        }
        int safeStart = Math.max(0, Math.min(startEpisodeIndex, items.size() - 1));
        activity.skipHelper.introSkippedForCurrent = false;
        activity.skipHelper.outroPromptShown = false;
        activity.player.setMediaItems(items, safeStart, resumeMs);
        activity.player.prepare();
        activity.player.setPlayWhenReady(true);
        updateTitleForCurrent();
    }

    /** 当前片源 id(多源时 = sourceList 选中项 id), 供回传 web 更新历史片源。 */
    String currentSourceLabel() {
        if (currentSourceIndex >= 0 && currentSourceIndex < sourceList.size()) {
            return sourceList.get(currentSourceIndex).id;
        }
        return "";
    }

    void updateTitleForCurrent() {
        int idx = activity.player.getCurrentMediaItemIndex();
        if (idx >= 0 && idx < playlistTitles.size()) {
            activity.titleText.setText(playlistTitles.get(idx));
        }
        if (activity.episodesCount != null) {
            int total = playlistTitles.size();
            if (total > 0) {
                activity.episodesCount.setText("共 " + total + " 集");
                activity.episodesCount.setVisibility(View.VISIBLE);
            } else {
                activity.episodesCount.setVisibility(View.GONE);
            }
        }
    }
}
