package com.jerocine.player.download;

import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.os.Build;
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
        // Android 13+ 通知是运行时权限: 不申请则前台服务通知不展示 → 用户看不到下载进度、
        // 也不知道下载在跑。缺权限不阻断下载(下载本身仍能完成), 只是提示一次。
        requestNotificationPermissionIfNeeded();
        PlayerSession.SourceData src = sources.get(currentSource);
        List<String> urls = src.urls;
        List<String> titles = src.titles;
        List<Integer> eps = new ArrayList<>(selected);
        eps.sort(Integer::compareTo);
        int added = 0;
        int skipped = 0;
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
            // 已存在的任务跳过(engine 内部 insertIgnore 幂等兜底; 这里先统计, 提示才准确)
            if (engine.repository().get(t.id) != null) {
                skipped++;
                continue;
            }
            engine.enqueue(t);
            added++;
        }
        selected.clear();
        renderEpisodeList(currentSegment);
        if (added > 0) {
            toast(added + " 个任务已加入" + (skipped > 0 ? " · " + skipped + " 个已存在" : ""));
        } else if (skipped > 0) {
            toast("所选集数均已存在下载任务");
        }
        switchTab(1);
    }

    // ============================ 下载中 / 已完成 ============================

    private void refreshTasks() {
        activeTasks.clear();
        doneTasks.clear();
        inProgress.clear();
        for (DownloadTask t : engine.repository().listAll()) {
            if (t.state == DownloadTask.STATE_COMPLETED || t.state == DownloadTask.STATE_EXPORTING) {
                doneTasks.add(t); // 导出中仍属"已完成"(状态由 EXPORTING 防并发, 完成回 COMPLETED)
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
            Button del = (Button) row.getChildAt(4);
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
                } else { // QUEUED / DOWNLOADING → 暂停
                    engine.pause(tt.id);
                }
                refreshTasks();
            });
            del.setText("删除");
            del.setTag(t);
            del.setOnClickListener(v -> confirmRemove((DownloadTask) v.getTag()));
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
                convertView = makeDoneRow();
            }
            LinearLayout row = (LinearLayout) convertView;
            TextView name = (TextView) row.getChildAt(0);
            TextView status = (TextView) row.getChildAt(1);
            Button exportBtn = (Button) row.getChildAt(2);
            Button playBtn = (Button) row.getChildAt(3);
            Button deleteBtn = (Button) row.getChildAt(4);
            name.setText(label(t));
            status.setText(exportedText(t));
            boolean exporting = t.state == DownloadTask.STATE_EXPORTING;
            exportBtn.setText(exporting ? "导出中" : "导出");
            exportBtn.setEnabled(!exporting);
            exportBtn.setTag(t);
            exportBtn.setOnClickListener(v -> exportTs((DownloadTask) v.getTag()));
            playBtn.setText("播放");
            playBtn.setTag(t);
            playBtn.setOnClickListener(v -> playOffline((DownloadTask) v.getTag()));
            deleteBtn.setText("删除");
            deleteBtn.setTag(t);
            deleteBtn.setOnClickListener(v -> confirmRemove((DownloadTask) v.getTag()));
            return convertView;
        }
    }

    /** 已完成行: 名 + 状态 + [导出][播放] 双按钮. */
    private View makeDoneRow() {
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

        TextView status = new TextView(this);
        status.setTextSize(12);
        status.setTextColor((int) 0xFF80FFFFFFL);
        status.setGravity(Gravity.CENTER);
        row.addView(status, new LinearLayout.LayoutParams(dp(74), ViewGroup.LayoutParams.WRAP_CONTENT));

        Button exportBtn = new Button(this);
        exportBtn.setTextSize(13);
        exportBtn.setAllCaps(false);
        exportBtn.setMinWidth(dp(64));
        exportBtn.setPadding(dp(10), dp(4), dp(10), dp(4));
        styleChip(exportBtn, false);
        row.addView(exportBtn, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        Button playBtn = new Button(this);
        playBtn.setTextSize(13);
        playBtn.setAllCaps(false);
        playBtn.setMinWidth(dp(64));
        playBtn.setPadding(dp(10), dp(4), dp(10), dp(4));
        styleChip(playBtn, true);
        row.addView(playBtn, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        // 删除: 下载缓存区用 NoOpCacheEvictor(主动下载永不清), 若没有删除入口用户永远
        // 无法释放空间 —— remove() 早已实现却没有 UI 能到, 等于死代码。
        Button deleteBtn = new Button(this);
        deleteBtn.setTextSize(13);
        deleteBtn.setAllCaps(false);
        deleteBtn.setMinWidth(dp(64));
        deleteBtn.setPadding(dp(10), dp(4), dp(10), dp(4));
        styleChip(deleteBtn, false);
        row.addView(deleteBtn, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        return row;
    }

    /** 删除任务(二次确认): 会连带删除该集的下载缓存分片与已导出的文件引用. */
    private void confirmRemove(DownloadTask t) {
        if (t == null) return;
        String msg = "删除「" + label(t) + "」?\n\n将同时删除该集的下载缓存分片, 之后需要重新下载。"
                + (t.exportedPath != null && !t.exportedPath.isEmpty()
                ? "\n已导出的文件不会被删除。" : "");
        new androidx.appcompat.app.AlertDialog.Builder(this)
                .setTitle("删除下载任务")
                .setMessage(msg)
                .setPositiveButton("删除", (d, w) -> {
                    try {
                        engine.remove(t.id);
                        toast("已删除");
                    } catch (IllegalArgumentException e) {
                        // 路径校验失败(filmId 非法): 任务已从库与 media3 移除, 仅缓存目录未清
                        toast("任务已删除, 但缓存清理失败: " + e.getMessage());
                    } catch (Exception e) {
                        toast("删除失败: " + (e.getMessage() == null ? "未知错误" : e.getMessage()));
                    }
                    refreshTasks();
                })
                .setNegativeButton("取消", null)
                .show();
    }

    /** 导出 .ts: 下载缓存分片按序拼接 → 系统下载目录(MediaStore Downloads). */
    private void exportTs(DownloadTask t) {
        TsExporter exporter = new TsExporter(this, engine.cache());
        exporter.export(engine.repository(), t, new TsExporter.Callback() {
            @Override
            public void onExported(String path) {
                toast("已导出: " + path);
                refreshTasks();
            }

            @Override
            public void onError(String message) {
                toast(message);
                refreshTasks();
            }
        });
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

        // 删除: ��下的任务也能删(不然用户只能等它跑完/失败才能清缓存)
        Button del = new Button(this);
        del.setTextSize(13);
        del.setAllCaps(false);
        del.setMinWidth(dp(64));
        del.setPadding(dp(10), dp(4), dp(10), dp(4));
        styleChip(del, false);
        row.addView(del, new LinearLayout.LayoutParams(
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
        long bytes = t.totalBytes;
        if (t.exportedPath != null && !t.exportedPath.isEmpty()) {
            return "已导出 · " + sizeText(bytes);
        }
        return sizeText(bytes);
    }

    private static String sizeText(long bytes) {
        if (bytes >= 1024 * 1024) {
            return String.format(Locale.US, "%.1fMB", bytes / 1024f / 1024f);
        }
        if (bytes >= 1024) {
            return String.format(Locale.US, "%dKB", bytes / 1024);
        }
        return "0KB";
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

    /** 运行时权限申请码(仅用于日志/回调区分)。 */
    private static final int REQ_POST_NOTIFICATIONS = 4101;

    /**
     * 首次点「开始下载」时申请通知权限(Android 13+ 是运行时权限, Manifest 声明不够)。
     *
     * <p>不阻断下载: 用户拒绝后下载照样能完成, 只是前台服务通知不展示 → 看不到进度、
     * 不知道后台在跑。所以只提示一次, 不反复弹。
     */
    private void requestNotificationPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return; // 33 以下无需申请
        String perm = android.Manifest.permission.POST_NOTIFICATIONS;
        if (checkSelfPermission(perm) == PackageManager.PERMISSION_GRANTED) return;
        if (!notificationPermissionAsked) {
            notificationPermissionAsked = true;
            try {
                requestPermissions(new String[]{perm}, REQ_POST_NOTIFICATIONS);
            } catch (Exception ignore) {
                // 某些壳 Activity 可能不支持运行时申请; 下载本身不依赖通知
            }
            return;
        }
        // 已问过一次还被拒: 不再弹系统框, 只留一句说明(避免每次点下载都被系统框打断)
        if (checkSelfPermission(perm) != PackageManager.PERMISSION_GRANTED) {
            toast("未授予通知权限, 下载进度将不会显示在通知栏");
        }
    }

    /** 是否已弹过通知权限申请(避免重复打扰)。 */
    private boolean notificationPermissionAsked = false;
}
