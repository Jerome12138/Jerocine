package com.jerocine.player.download;

import android.content.Intent;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.os.Bundle;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.BaseAdapter;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.HorizontalScrollView;
import android.widget.LinearLayout;
import android.widget.ListView;
import android.widget.ProgressBar;
import android.widget.TextView;

import androidx.annotation.Nullable;
import androidx.appcompat.app.AppCompatActivity;

import com.jerocine.player.JerocinePlayer;
import com.jerocine.player.PlayerActivity;
import com.jerocine.player.PlayerSession;
import com.jerocine.player.R;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.File;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * 下载管理页(原生) — 三个 tab: 选集下载 / 下载中 / 已完成.
 *
 * <p>入口(两处): 播放器「⋮ 更多 → 下载管理」(带影片上下文: filmId/filmName/sourcesJson)
 * 与 设置页(无上下文 → 只展示"下载中/已完成", 选集 tab 提示从播放器进入).
 *
 * <p>离线播放复用 {@link PlayerActivity}: EXTRA_URL=本地过滤后清单(file://) +
 * EXTRA_CACHE_DIR=下载缓存根目录 → 播放器 CacheDataSource 命中下载分片, 飞行模式可完整播放.
 */
public class DownloadActivity extends AppCompatActivity {

    private static final int EPISODE_SEG = 30;

    private DownloadEngine engine;
    private String filmId = "";
    private String filmName = "";
    private String proxyBase = "";
    private List<PlayerSession.SourceData> sources = new ArrayList<>();
    private int currentSource = 0;
    private int currentSegment = 0;

    private TextView tabEpisodes, tabActive, tabDone;
    private View llEpisodes;
    private ListView lvEpisodes, lvActive, lvDone;
    private HorizontalScrollView hsvSources, hsvSegments;
    private LinearLayout llSources, llSegments;
    private TextView tvSelectedCount;

    private final Set<Integer> selected = new HashSet<>();
    private final Set<String> inProgress = new HashSet<>();

    private final List<DownloadTask> activeTasks = new ArrayList<>();
    private final List<DownloadTask> doneTasks = new ArrayList<>();

    private int dp(float v) {
        return (int) (v * getResources().getDisplayMetrics().density + 0.5f);
    }

    // ============================ 生命周期 ============================

    @Override
    protected void onCreate(@Nullable Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_download);

        Intent in = getIntent();
        filmId = in.getStringExtra(PlayerActivity.EXTRA_FILM_ID);
        if (filmId == null) filmId = "";
        filmName = in.getStringExtra(PlayerActivity.EXTRA_FILM_NAME);
        if (filmName == null) filmName = "";
        proxyBase = in.getStringExtra(PlayerActivity.EXTRA_PROXY_BASE);
        if (proxyBase == null || proxyBase.isEmpty()) {
            String def = JerocinePlayer.defaultProxyBase();
            proxyBase = def == null ? "" : def;
        }
        String sourcesJson = in.getStringExtra(PlayerActivity.EXTRA_SOURCES_JSON);
        if (sourcesJson != null && !sourcesJson.isEmpty()) {
            sources = parseSources(sourcesJson);
        }

        engine = DownloadEngine.get(this, proxyBase);

        bindViews();
        renderTabs();
        buildSourceButtons();
        buildSegmentButtons();
        lvEpisodes.setChoiceMode(ListView.CHOICE_MODE_MULTIPLE);
        lvEpisodes.setOnItemClickListener((p, v, pos, id) -> {
            int total = currentSourceEpisodes().size();
            int segStart = currentSegment * EPISODE_SEG;
            int global = segStart + pos;
            CheckBox cb = v.findViewById(android.R.id.text1) instanceof CheckBox
                    ? (CheckBox) v.findViewById(android.R.id.text1) : null;
            if (selected.contains(global)) {
                selected.remove(global);
                if (cb != null) cb.setChecked(false);
            } else {
                selected.add(global);
                if (cb != null) cb.setChecked(true);
            }
            updateSelectedCount();
        });
        btn(R.id.btn_start_download).setOnClickListener(v -> startDownloads());
        btn(R.id.btn_download_back).setOnClickListener(v -> finish());
    }

    @Override
    protected void onResume() {
        super.onResume();
        refreshTasks();
        engine.setChangeListener(taskId -> runOnUiThread(this::refreshTasks));
    }

    @Override
    protected void onPause() {
        super.onPause();
        engine.setChangeListener(null);
    }

    // ============================ 视图装配 ============================

    private void bindViews() {
        tabEpisodes = findViewById(R.id.tab_episodes);
        tabActive = findViewById(R.id.tab_active);
        tabDone = findViewById(R.id.tab_done);
        llEpisodes = findViewById(R.id.ll_episodes);
        lvEpisodes = findViewById(R.id.lv_episodes);
        lvActive = findViewById(R.id.lv_active);
        lvDone = findViewById(R.id.lv_done);
        hsvSources = findViewById(R.id.hsv_sources);
        llSources = findViewById(R.id.ll_sources);
        hsvSegments = findViewById(R.id.hsv_segments);
        llSegments = findViewById(R.id.ll_segments);
        tvSelectedCount = findViewById(R.id.tv_selected_count);
        tabEpisodes.setOnClickListener(v -> switchTab(0));
        tabActive.setOnClickListener(v -> switchTab(1));
        tabDone.setOnClickListener(v -> switchTab(2));
    }

    private Button btn(int id) {
        return findViewById(id);
    }

    private void switchTab(int idx) {
        llEpisodes.setVisibility(idx == 0 ? View.VISIBLE : View.GONE);
        lvActive.setVisibility(idx == 1 ? View.VISIBLE : View.GONE);
        lvDone.setVisibility(idx == 2 ? View.VISIBLE : View.GONE);
        renderTabs();
        if (idx == 1) renderActiveList();
        if (idx == 2) renderDoneList();
    }

    private void renderTabs() {
        styleTab(tabEpisodes, llEpisodes.getVisibility() == View.VISIBLE);
        styleTab(tabActive, lvActive.getVisibility() == View.VISIBLE);
        styleTab(tabDone, lvDone.getVisibility() == View.VISIBLE);
    }

    private void styleTab(TextView tv, boolean active) {
        tv.setTextColor(active ? (int) 0xFF00141AL : (int) 0xFFB3FFFFFFL);
        GradientDrawable g = new GradientDrawable();
        g.setColor(active ? (int) 0xFF4AD1E5L : (int) 0xFF1E1E28L);
        g.setCornerRadius(dp(10));
        tv.setBackground(g);
    }

    // ============================ 选集数据 ============================

    private List<PlayerSession.SourceData> parseSources(String json) {
        List<PlayerSession.SourceData> out = new ArrayList<>();
        try {
            JSONArray arr = new JSONArray(json);
            for (int i = 0; i < arr.length(); i++) {
                JSONObject src = arr.getJSONObject(i);
                PlayerSession.SourceData sd = new PlayerSession.SourceData();
                sd.id = src.optString("id", "");
                sd.name = src.optString("name", "源 " + (i + 1));
                sd.adFilterOk = src.isNull("adFilterOk") ? null : src.optBoolean("adFilterOk", true);
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
                out.add(sd);
            }
        } catch (Exception ignore) {
        }
        return out;
    }

    private List<String> currentSourceEpisodes() {
        if (sources.isEmpty()) return new ArrayList<>();
        return sources.get(currentSource).urls;
    }

    private void buildSourceButtons() {
        if (sources.size() < 2) {
            hsvSources.setVisibility(View.GONE);
            return;
        }
        hsvSources.setVisibility(View.VISIBLE);
        llSources.removeAllViews();
        for (int i = 0; i < sources.size(); i++) {
            final int idx = i;
            Button b = new Button(this);
            b.setText(sources.get(i).name);
            b.setTextSize(13);
            b.setAllCaps(false);
            b.setPadding(dp(14), dp(8), dp(14), dp(8));
            LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            lp.setMargins(0, dp(4), dp(8), dp(4));
            b.setLayoutParams(lp);
            b.setOnClickListener(v -> {
                currentSource = idx;
                selected.clear();
                buildSourceButtons();
                buildSegmentButtons();
                updateSelectedCount();
            });
            styleChip(b, idx == currentSource);
            llSources.addView(b);
        }
    }

    private void buildSegmentButtons() {
        int total = currentSourceEpisodes().size();
        if (total <= EPISODE_SEG) {
            hsvSegments.setVisibility(View.GONE);
            currentSegment = 0;
            renderEpisodeList(0);
            return;
        }
        hsvSegments.setVisibility(View.VISIBLE);
        llSegments.removeAllViews();
        int segCount = (total + EPISODE_SEG - 1) / EPISODE_SEG;
        for (int i = 0; i < segCount; i++) {
            final int seg = i;
            int s = i * EPISODE_SEG + 1;
            int e = Math.min((i + 1) * EPISODE_SEG, total);
            Button b = new Button(this);
            b.setText("第 " + s + "-" + e + " 集");
            b.setTextSize(13);
            b.setAllCaps(false);
            b.setPadding(dp(14), dp(8), dp(14), dp(8));
            LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            lp.setMargins(0, dp(4), dp(8), dp(4));
            b.setLayoutParams(lp);
            b.setOnClickListener(v -> {
                currentSegment = seg;
                buildSegmentButtons();
                renderEpisodeList(seg);
            });
            styleChip(b, seg == currentSegment);
            llSegments.addView(b);
        }
    }

    private void styleChip(Button b, boolean active) {
        b.setTextColor(active ? (int) 0xFF00141AL : (int) 0xFFE6FFFFFFL);
        GradientDrawable g = new GradientDrawable();
        g.setColor(active ? (int) 0xFF4AD1E5L : (int) 0xFF1E1E28L);
        g.setCornerRadius(dp(10));
        g.setStroke(dp(1), 0x22FFFFFF);
        b.setBackground(g);
    }

    private void renderEpisodeList(int seg) {
        List<String> eps = currentSourceEpisodes();
        int start = seg * EPISODE_SEG;
        int end = Math.min(start + EPISODE_SEG, eps.size());
        final int segStart = start;
        List<EpisodeRow> rows = new ArrayList<>();
        for (int i = start; i < end; i++) {
            String title = eps.get(i);
            rows.add(new EpisodeRow(i, title));
        }
        lvEpisodes.setAdapter(new BaseAdapter() {
            @Override
            public int getCount() {
                return rows.size();
            }

            @Override
            public Object getItem(int position) {
                return rows.get(position);
            }

            @Override
            public long getItemId(int position) {
                return rows.get(position).index;
            }

            @Override
            public View getView(int position, View convertView, ViewGroup parent) {
                EpisodeRow row = rows.get(position);
                CheckBox cb;
                if (convertView instanceof CheckBox) {
                    cb = (CheckBox) convertView;
                } else {
                    cb = new CheckBox(DownloadActivity.this);
                    cb.setTextSize(14);
                    cb.setTextColor((int) 0xFFE6FFFFFFL);
                    cb.setPadding(dp(16), dp(12), dp(16), dp(12));
                    cb.setFocusable(true);
                    cb.setButtonDrawable(null);
                    cb.setCompoundDrawablesWithIntrinsicBounds(
                            R.drawable.jc_checkbox_off, 0, 0, 0);
                    cb.setOnClickListener(v -> {
                        CheckBox c = (CheckBox) v;
                        int g = segStart + (Integer) c.getTag();
                        if (c.isChecked()) {
                            selected.add(g);
                            c.setCompoundDrawablesWithIntrinsicBounds(
                                    R.drawable.jc_checkbox_on, 0, 0, 0);
                        } else {
                            selected.remove(g);
                            c.setCompoundDrawablesWithIntrinsicBounds(
                                    R.drawable.jc_checkbox_off, 0, 0, 0);
                        }
                        updateSelectedCount();
                    });
                }
                cb.setTag(position);
                cb.setText(row.title);
                cb.setChecked(selected.contains(segStart + position));
                cb.setCompoundDrawablesWithIntrinsicBounds(
                        selected.contains(segStart + position) ? R.drawable.jc_checkbox_on : R.drawable.jc_checkbox_off,
                        0, 0, 0);
                return cb;
            }
        });
        updateSelectedCount();
    }

    private static final class EpisodeRow {
        final int index;
        final String title;

        EpisodeRow(int index, String title) {
            this.index = index;
            this.title = title;
        }
    }

    private void updateSelectedCount() {
        int n = selected.size();
        String label = filmName.isEmpty() ? "下载管理" : filmName;
        tvSelectedCount.setText("已选 " + n + " 集");
        ((TextView) findViewById(R.id.tv_download_title)).setText(label);
    }

    // ============================ 开始下载 ============================

    private void startDownloads() {
        if (selected.isEmpty()) {
            toast("请先勾选要下载的集");
            return;
        }
        if (sources.isEmpty()) {
            toast("无片源数据, 请从播放器进入下载");
            return;
        }
        PlayerSession.SourceData src = sources.get(currentSource);
        List<String> urls = src.urls;
        List<String> titles = src.titles;
        List<Integer> eps = new ArrayList<>(selected);
        eps.sort(Integer::compareTo);
        for (int ep : eps) {
            if (ep < 0 || ep >= urls.size()) continue;
            DownloadTask t = new DownloadTask();
            t.filmId = filmId;
            t.filmTitle = filmName;
            t.sourceKey = src.id;
            t.sourceName = src.name;
            t.episode = ep;
            t.episodeTitle = (titles != null && ep < titles.size()) ? titles.get(ep) : "";
            t.srcUrl = urls.get(ep);
            t.id = DownloadTask.idFor(filmId, src.id, ep);
            t.cacheDir = engine.episodeCacheDir(t).getAbsolutePath();
            t.createdAt = System.currentTimeMillis();
            t.updatedAt = t.createdAt;
            engine.enqueue(t);
        }
        selected.clear();
        renderEpisodeList(currentSegment);
        toast("已加入 " + eps.size() + " 个下载任务");
        switchTab(1);
    }

    // ============================ 下载中 / 已完成 ============================

    private void refreshTasks() {
        activeTasks.clear();
        doneTasks.clear();
        inProgress.clear();
        for (DownloadTask t : engine.repository().listAll()) {
            if (t.state == DownloadTask.STATE_COMPLETED) {
                doneTasks.add(t);
            } else if (t.state == DownloadTask.STATE_QUEUED
                    || t.state == DownloadTask.STATE_DOWNLOADING) {
                activeTasks.add(t);
                inProgress.add(t.id);
            } else {
                activeTasks.add(t);
            }
        }
        renderActiveList();
        renderDoneList();
    }

    private void renderActiveList() {
        lvActive.setAdapter(new ActiveAdapter());
    }

    private void renderDoneList() {
        lvDone.setAdapter(new DoneAdapter());
    }

    private final class ActiveAdapter extends BaseAdapter {
        @Override
        public int getCount() {
            return activeTasks.size();
        }

        @Override
        public Object getItem(int position) {
            return activeTasks.get(position);
        }

        @Override
        public long getItemId(int position) {
            return position;
        }

        @Override
        public View getView(int position, View convertView, ViewGroup parent) {
            DownloadTask t = activeTasks.get(position);
            if (convertView == null) {
                convertView = makeTaskRow();
            }
            LinearLayout row = (LinearLayout) convertView;
            TextView name = (TextView) row.getChildAt(0);
            ProgressBar bar = (ProgressBar) row.getChildAt(1);
            TextView status = (TextView) row.getChildAt(2);
            Button action = (Button) row.getChildAt(3);
            name.setText(label(t));
            boolean downloading = inProgress.contains(t.id);
            int pct = (t.totalBytes > 0) ? (int) (t.progressBytes * 100 / t.totalBytes) : 0;
            bar.setProgress(downloading ? pct : 0);
            bar.setVisibility(downloading ? View.VISIBLE : View.GONE);
            status.setText(statusText(t));
            action.setText(actionText(t));
            action.setTag(t);
            action.setOnClickListener(v -> {
                DownloadTask tt = (DownloadTask) v.getTag();
                if (tt.state == DownloadTask.STATE_PAUSED || tt.state == DownloadTask.STATE_FAILED) {
                    engine.resume(tt.id);
                } else if (tt.state == DownloadTask.STATE_DOWNLOADING || tt.state == DownloadTask.STATE_QUEUED) {
                    engine.pause(tt.id);
                } else if (tt.state == DownloadTask.STATE_QUEUED) {
                    engine.pause(tt.id);
                } else if (tt.state == DownloadTask.STATE_PAUSED) {
                    engine.resume(tt.id);
                }
                refreshTasks();
            });
            return convertView;
        }
    }

    private final class DoneAdapter extends BaseAdapter {
        @Override
        public int getCount() {
            return doneTasks.size();
        }

        @Override
        public Object getItem(int position) {
            return doneTasks.get(position);
        }

        @Override
        public long getItemId(int position) {
            return position;
        }

        @Override
        public View getView(int position, View convertView, ViewGroup parent) {
            DownloadTask t = doneTasks.get(position);
            if (convertView == null) {
                convertView = makeTaskRow();
            }
            LinearLayout row = (LinearLayout) convertView;
            TextView name = (TextView) row.getChildAt(0);
            ProgressBar bar = (ProgressBar) row.getChildAt(1);
            TextView status = (TextView) row.getChildAt(2);
            Button action = (Button) row.getChildAt(3);
            name.setText(label(t));
            bar.setVisibility(View.GONE);
            status.setText(exportedText(t));
            action.setText("播放");
            action.setTag(t);
            action.setOnClickListener(v -> playOffline((DownloadTask) v.getTag()));
            return convertView;
        }
    }

    private View makeTaskRow() {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(16), dp(10), dp(16), dp(10));

        TextView name = new TextView(this);
        name.setTextSize(14);
        name.setTextColor((int) 0xFFE6FFFFFFL);
        name.setMaxLines(1);
        row.addView(name, new LinearLayout.LayoutParams(0,
                ViewGroup.LayoutParams.WRAP_CONTENT, 1f));

        ProgressBar bar = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal);
        bar.setMax(100);
        bar.setProgressTintList(android.content.res.ColorStateList.valueOf((int) 0xFF4AD1E5L));
        row.addView(bar, new LinearLayout.LayoutParams(dp(90), dp(6)));

        TextView status = new TextView(this);
        status.setTextSize(12);
        status.setTextColor((int) 0xFF80FFFFFFL);
        status.setGravity(Gravity.CENTER);
        row.addView(status, new LinearLayout.LayoutParams(dp(74), ViewGroup.LayoutParams.WRAP_CONTENT));

        Button action = new Button(this);
        action.setTextSize(13);
        action.setAllCaps(false);
        action.setMinWidth(dp(64));
        action.setPadding(dp(10), dp(4), dp(10), dp(4));
        styleChip(action, true);
        row.addView(action, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        return row;
    }

    private String label(DownloadTask t) {
        String s = t.filmTitle == null || t.filmTitle.isEmpty() ? "未知影片" : t.filmTitle;
        return s + " E" + (t.episode + 1)
                + (t.episodeTitle != null && !t.episodeTitle.isEmpty() ? " " + t.episodeTitle : "");
    }

    private String statusText(DownloadTask t) {
        switch (t.state) {
            case DownloadTask.STATE_QUEUED:
                return "排队中";
            case DownloadTask.STATE_DOWNLOADING:
                return pct(t);
            case DownloadTask.STATE_PAUSED:
                return "已暂停";
            case DownloadTask.STATE_FAILED:
                return "失败";
            default:
                return "";
        }
    }

    private String pct(DownloadTask t) {
        if (t.totalBytes <= 0) return "下载中";
        return String.format(Locale.US, "%d%%", t.progressBytes * 100 / t.totalBytes);
    }

    private String actionText(DownloadTask t) {
        switch (t.state) {
            case DownloadTask.STATE_PAUSED:
            case DownloadTask.STATE_FAILED:
                return "继续";
            case DownloadTask.STATE_QUEUED:
            case DownloadTask.STATE_DOWNLOADING:
                return "暂停";
            default:
                return "删除";
        }
    }

    private String exportedText(DownloadTask t) {
        long mb = t.totalBytes / 1024 / 1024;
        return t.exportedPath != null && !t.exportedPath.isEmpty()
                ? "已导出 · " + mb + "MB"
                : mb + "MB";
    }

    /** 离线播放: 本地过滤后清单 + 下载缓存(PlayerActivity 支持 EXTRA_CACHE_DIR 切换缓存实例). */
    private void playOffline(DownloadTask t) {
        File playlist = new File(t.cacheDir, "playlist.m3u8");
        if (!playlist.exists()) {
            toast("缓存缺失, 请重新下载该集");
            return;
        }
        Intent i = new Intent(this, PlayerActivity.class);
        i.putExtra(PlayerActivity.EXTRA_URL, "file://" + playlist.getAbsolutePath());
        i.putExtra(PlayerActivity.EXTRA_CACHE_DIR, engine.cacheRoot().getAbsolutePath());
        i.putExtra(PlayerActivity.EXTRA_TITLE, label(t));
        startActivity(i);
    }

    private void toast(String msg) {
        android.widget.Toast.makeText(this, msg, android.widget.Toast.LENGTH_SHORT).show();
    }
}
