package com.jerocine.player.download;

import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.BaseAdapter;
import android.widget.Button;
import android.widget.HorizontalScrollView;
import android.widget.LinearLayout;
import android.widget.ListView;
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
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
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

    private View tabEpisodes, tabActive, tabDone;
    private TextView tvTabEpisodes, tvTabActive, tvTabDone;
    private View underlineEpisodes, underlineActive, underlineDone;
    private View llEpisodes;
    private ListView lvEpisodes, lvActive, lvDone;
    private HorizontalScrollView hsvSources, hsvSegments;
    private LinearLayout llSources, llSegments;
    private TextView tvTotalCount, tvBannerTitle, tvBannerSpeed;
    private TextView tvActiveSummary, tvDoneSummary;
    private View llSelectAll, ivSelectAll, llBanner;
    private View tabActiveContainer, tabDoneContainer;
    private Button btnBannerAction;
    private View tvEmptyActive, tvEmptyDone;

    /** 当前选中的 tab(0选集/1下载中/2已完成)。*/
    private int currentTab = 0;

    private final Set<Integer> selected = new HashSet<>();
    private final Set<String> inProgress = new HashSet<>();

    private final List<DownloadTask> activeTasks = new ArrayList<>();
    private final List<DownloadTask> doneTasks = new ArrayList<>();

    /** 选集行状态查询: 任务id → 任务(含已完成/暂停/失败), adapter 里不再逐条查库。 */
    private final Map<String, DownloadTask> taskMap = new HashMap<>();
    /** 选集列表: 适配器复用(同 active/done —— 每次重建会丢焦点/滚动位置, TV 上是硬伤)。 */
    private final List<EpisodeRow> episodeRows = new ArrayList<>();
    private EpisodeAdapter episodeAdapter;
    /** 速度估算: 上次采样(活动任务总进度字节, 时间ms)。 */
    private long lastBytes = -1;
    private long lastTs = 0;

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
        // 问题1: 壳层主题(如 web 壳 AppTheme)自带 ActionBar 时, 页顶会挂一条系统亮色标题栏
        // 显示应用名"Jerocine影视" —— 下载页有自己的标题栏(返回+标题), 这里统一隐藏
        // (两壳通用, 不依赖 manifest 主题是否 NoActionBar)。
        androidx.appcompat.app.ActionBar ab = getSupportActionBar();
        if (ab != null) ab.hide();
        // 同因: 亮色主题的 windowBackground 会在转场时白闪, 统一压成深色底。
        getWindow().setBackgroundDrawable(new android.graphics.drawable.ColorDrawable(
                getColor(R.color.jc_bg)));
        // 问题1: 标题 = "下载管理 · 影片名", 只设一次; 勾选数由底部主按钮文案表达。
        TextView title = findViewById(R.id.tv_download_title);
        title.setText(filmName.isEmpty() ? getString(R.string.download_title)
                : getString(R.string.download_title) + " · " + filmName);
        renderTabs();
        buildSourceButtons();
        buildSegmentButtons();
        renderEpisodeList(0);
        btn(R.id.btn_start_download).setOnClickListener(v -> startDownloads());
        btn(R.id.btn_download_back).setOnClickListener(v -> finish());
        llSelectAll.setOnClickListener(v -> toggleSelectAll());
        btnBannerAction.setOnClickListener(v -> onBannerAction());
        // 【TV 遥控器】进入页面必须有人持有焦点, 否则 D-pad 没有起点
        // (真机 uiautomator 实测入场 focused=0, 遥控器按键完全无响应)。
        // post 到队列: 等 ListView 完成首次渲染后再落焦, 避免被布局覆盖。
        currentTabView().post(() -> {
            if (getCurrentFocus() == null) currentTabView().requestFocus();
        });
    }

    /** 当前 tab 的可聚焦页签 View(初始焦点用)。 */
    private View currentTabView() {
        if (currentTab == 1) return tabActive;
        if (currentTab == 2) return tabDone;
        return tabEpisodes;
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
        tvTabEpisodes = findViewById(R.id.tv_tab_episodes);
        tvTabActive = findViewById(R.id.tv_tab_active);
        tvTabDone = findViewById(R.id.tv_tab_done);
        underlineEpisodes = findViewById(R.id.underline_episodes);
        underlineActive = findViewById(R.id.underline_active);
        underlineDone = findViewById(R.id.underline_done);
        llEpisodes = findViewById(R.id.ll_episodes);
        lvEpisodes = findViewById(R.id.lv_episodes);
        lvActive = findViewById(R.id.lv_active);
        lvDone = findViewById(R.id.lv_done);
        // 【TV 遥控器】AbsListView 构造器会强制 focusableInTouchMode=true 且焦点搜索
        // 会命中容器本身 —— 真机实测焦点停在 ListView 上, 行永远拿不到焦点, OK 无效。
        // FOCUS_AFTER_DESCENDANTS + setItemsCanFocus(true) = 焦点直达行, 容器只在
        // 行全部不可焦时才接盘(标准 TV 组合)。
        android.widget.ListView[] lists = {lvEpisodes, lvActive, lvDone};
        for (android.widget.ListView lv : lists) {
            lv.setDescendantFocusability(android.view.ViewGroup.FOCUS_AFTER_DESCENDANTS);
            lv.setItemsCanFocus(true);
            lv.setFocusable(false);
        }
        hsvSources = findViewById(R.id.hsv_sources);
        llSources = findViewById(R.id.ll_sources);
        hsvSegments = findViewById(R.id.hsv_segments);
        llSegments = findViewById(R.id.ll_segments);
        tvTotalCount = findViewById(R.id.tv_total_count);
        tvActiveSummary = findViewById(R.id.tv_active_summary);
        tvDoneSummary = findViewById(R.id.tv_done_summary);
        tabActiveContainer = findViewById(R.id.tab_active_container);
        tabDoneContainer = findViewById(R.id.tab_done_container);
        llSelectAll = findViewById(R.id.ll_select_all);
        ivSelectAll = findViewById(R.id.iv_select_all);
        llBanner = findViewById(R.id.ll_banner);
        tvBannerTitle = findViewById(R.id.tv_banner_title);
        tvBannerSpeed = findViewById(R.id.tv_banner_speed);
        btnBannerAction = findViewById(R.id.btn_banner_action);
        // 空态视图: setEmptyView 交给 ListView 管理可见性(列表空时自动显示); 切 tab 时手动同步。
        tvEmptyActive = findViewById(R.id.tv_empty_active);
        tvEmptyDone = findViewById(R.id.tv_empty_done);
        lvActive.setEmptyView(tvEmptyActive);
        lvDone.setEmptyView(tvEmptyDone);
        tabEpisodes.setOnClickListener(v -> switchTab(0));
        tabActive.setOnClickListener(v -> switchTab(1));
        tabDone.setOnClickListener(v -> switchTab(2));
    }

    /** 按钮按id 取控件(返回键是 ImageButton, 走 View 父类型即可)。 */
    private View btn(int id) {
        return findViewById(id);
    }

    private void switchTab(int idx) {
        currentTab = idx;
        // 只切三个 tab 容器的可见性: ListView.setEmptyView 会在数据变化时接管空态 TextView
        // 的可见性(强行 VISIBLE), 手动同步它反而造成"空态叠在选集页上"(2026-10-08 用户实锤);
        // 容器化后容器 GONE 即整棵子树隐藏, 空态只在自家容器内出现。
        llEpisodes.setVisibility(idx == 0 ? View.VISIBLE : View.GONE);
        tabActiveContainer.setVisibility(idx == 1 ? View.VISIBLE : View.GONE);
        tabDoneContainer.setVisibility(idx == 2 ? View.VISIBLE : View.GONE);
        renderTabs();
        if (idx == 1) renderActiveList();
        if (idx == 2) renderDoneList();
    }

    private void renderTabs() {
        // ⚠ 必须用 currentTab 判断: 容器化改造后 ListView 永远是 VISIBLE(隐藏的是容器),
        // 旧实现看 lvActive/lvDone 可见性 → 下载中/已完成两个 tab 恒高亮(用户实锤
        // "同时选中多个 tab")。
        styleTab(0, currentTab == 0);
        styleTab(1, currentTab == 1);
        styleTab(2, currentTab == 2);
    }

    /**
     * Tab 状态(文字 Tab + 下划线版式): 选中 = 文字变青(jc_download_tab_text 的
     * state_selected) + 下划线; 焦点态(暗青底+青描边)由 jc_download_tab_bg 接管。
     * 注意不能用背景表达选中 —— 焦点也用背景(描边), 两个维度会打架。
     */
    private void styleTab(int idx, boolean active) {
        TextView tv = idx == 0 ? tvTabEpisodes : idx == 1 ? tvTabActive : tvTabDone;
        View underline = idx == 0 ? underlineEpisodes : idx == 1 ? underlineActive : underlineDone;
        tv.setSelected(active);
        // 二轮1: 下划线用 GONE 会让文字的垂直位置在选中前后上下跳动 ——
        // 改 INVISIBLE: 占位不变, 只是看不见。
        underline.setVisibility(active ? View.VISIBLE : View.INVISIBLE);
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
            Button b = createChip(sources.get(i).name);
            b.setOnClickListener(v -> {
                currentSource = idx;
                selected.clear();
                buildSourceButtons();
                buildSegmentButtons();
                renderEpisodeList(currentSegment);
            });
            // 源 chip: 选中态= jc_download_chip_bg 的 selected 分支(半透明青底+青描边), 文字恒白。
            b.setSelected(idx == currentSource);
            llSources.addView(b);
        }
    }

    /**
     * chip 统一构造 — 源按钮与分段按钮共用。
     *
     * <p>必须显式铺样式: {@code new Button(this)} 走的是 Material 默认主题(亮色实底+大圆角),
     * 在深色玻璃页面上非常突兀; 且默认背景不含 state_focused 分支 → D-pad 看不到焦点。
     */
    private Button createChip(String label) {
        Button b = new Button(this, null, 0);// null, 0 = 不套默认 Material 样式
        b.setText(label);
        b.setTextSize(14);
        b.setAllCaps(false);
        b.setPadding(dp(14), dp(8), dp(14), dp(8));
        // ⚠ 2026-10-08 修"胶囊文字不是白色": setTextColor(int) 把传进去的值当**原始 ARGB**,
        // 而这里传的是 R.color 资源 ID(0x7f06xxxx) —— 被当成半透明深色渲染。
        // 颜色 selector 必须经 ColorStateList 解析。
        b.setTextColor(androidx.appcompat.content.res.AppCompatResources
                .getColorStateList(this, R.color.jc_download_chip_text));
        b.setBackgroundResource(R.drawable.jc_download_chip_bg);
        b.setFocusable(true);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        lp.setMargins(0, dp(4), dp(8), dp(4));
        b.setLayoutParams(lp);
        return b;
    }

    /**
     * 选集分段胶囊行(2026-10-08 二轮: 用户拍板"选集范围按胶囊标签形式", 从弹窗改回 chips),
     * 同时刷新信息行"共 N 集"。<=30 集时不显示分段行(不分段)。
     */
    private void buildSegmentButtons() {
        int total = currentSourceEpisodes().size();
        tvTotalCount.setText(getString(R.string.download_total_fmt, total));
        if (total <= EPISODE_SEG) {
            hsvSegments.setVisibility(View.GONE);
            currentSegment = 0;
            return;
        }
        hsvSegments.setVisibility(View.VISIBLE);
        llSegments.removeAllViews();
        int segCount = (total + EPISODE_SEG - 1) / EPISODE_SEG;
        for (int i = 0; i < segCount; i++) {
            final int seg = i;
            Button b = createChip(String.format(Locale.US, "第 %d-%d 集",
                    i * EPISODE_SEG + 1, Math.min((i + 1) * EPISODE_SEG, total)));
            b.setOnClickListener(v -> {
                currentSegment = seg;
                selected.clear();
                buildSegmentButtons();
                renderEpisodeList(seg);
            });
            b.setSelected(seg == currentSegment);
            llSegments.addView(b);
        }
    }

    /** 选集行对应的任务 id(无源时空串 → taskMap 查不到 → 视为无任务)。 */
    private String episodeTaskId(int ep) {
        if (sources.isEmpty()) return "";
        return DownloadTask.idFor(filmId, sources.get(currentSource).id, ep);
    }

    /** 该集是否"不可再勾"(已完成/导出中/下载中/排队 —— 任务已在跑或已落盘)。 */
    private boolean episodeLocked(DownloadTask t) {
        return t != null && (t.state == DownloadTask.STATE_COMPLETED
                || t.state == DownloadTask.STATE_EXPORTING
                || t.state == DownloadTask.STATE_DOWNLOADING
                || t.state == DownloadTask.STATE_QUEUED);
    }

    /** 全选 = 勾上当前分段所有可选的集(锁定集跳过); 本段已全勾时再点 = 清空本段勾选。 */
    private void toggleSelectAll() {
        if (sources.isEmpty()) return;
        List<String> urls = currentSourceEpisodes();
        int start = currentSegment * EPISODE_SEG;
        int end = Math.min(start + EPISODE_SEG, urls.size());
        boolean allSelected = end > start;
        for (int i = start; i < end; i++) {
            if (!episodeLocked(taskMap.get(episodeTaskId(i))) && !selected.contains(i)) {
                allSelected = false;
                break;
            }
        }
        for (int i = start; i < end; i++) {
            if (episodeLocked(taskMap.get(episodeTaskId(i)))) continue;
            if (allSelected) selected.remove(i);
            else selected.add(i);
        }
        updateSelectAllIcon();
        updateSelectedCount();
        notifyEpisodeAdapter();
    }

    /** 全选圆圈: 本段所有可选集都在勾选集中 → 青色对勾, 否则灰圈。 */
    private void updateSelectAllIcon() {
        boolean all = false;
        if (!sources.isEmpty()) {
            List<String> urls = currentSourceEpisodes();
            int start = currentSegment * EPISODE_SEG;
            int end = Math.min(start + EPISODE_SEG, urls.size());
            all = end > start;
            for (int i = start; i < end; i++) {
                if (!episodeLocked(taskMap.get(episodeTaskId(i))) && !selected.contains(i)) {
                    all = false;
                    break;
                }
            }
        }
        ivSelectAll.setBackgroundResource(all ? R.drawable.jc_checkbox_on
                : R.drawable.jc_checkbox_off);
    }

    /** 当前源的集名列表（与 urls 同下标；缺集名时为空串）。 */
    private List<String> currentSourceTitles() {
        if (sources.isEmpty()) return new ArrayList<>();
        return sources.get(currentSource).titles;
    }

    private void renderEpisodeList(int seg) {
        List<String> urls = currentSourceEpisodes();
        List<String> titles = currentSourceTitles();
        int start = seg * EPISODE_SEG;
        int end = Math.min(start + EPISODE_SEG, urls.size());
        episodeRows.clear();
        for (int i = start; i < end; i++) {
            String url = urls.get(i);
            // 行主标题必须是**集名**(综艺是期名, 剧集是"第N集"); 缺集名回退"第 N 集",
            // 绝不把原始 URL 当标题展示。
            String name = (i < titles.size() && titles.get(i) != null && !titles.get(i).isEmpty())
                    ? titles.get(i) : String.format(Locale.US,
                    getString(R.string.download_episode_name), i + 1);
            episodeRows.add(new EpisodeRow(i, name, url,
                    taskMap.get(episodeTaskId(i))));
        }
        notifyEpisodeAdapter();
        updateSelectAllIcon();
        updateSelectedCount();
    }

    /** 适配器复用: 每次 notifyDataSetChanged 只重画不重建, 保住焦点与滚动位置(TV 硬伤)。 */
    private void notifyEpisodeAdapter() {
        if (episodeAdapter == null) {
            episodeAdapter = new EpisodeAdapter();
            lvEpisodes.setAdapter(episodeAdapter);
        } else {
            episodeAdapter.notifyDataSetChanged();
        }
    }

    /** 选集列表适配器 — 数据源 episodeRows, 行视图 makeEpisodeRow(Holder 复用)。 */
    private final class EpisodeAdapter extends BaseAdapter {
        @Override
        public int getCount() {
            return episodeRows.size();
        }

        @Override
        public Object getItem(int position) {
            return episodeRows.get(position);
        }

        @Override
        public long getItemId(int position) {
            return episodeRows.get(position).index;
        }

        @Override
        public View getView(int position, View convertView, ViewGroup parent) {
            View row = convertView;
            Holder h;
            if (row == null) {
                row = makeEpisodeRow();
            }
            h = (Holder) row.getTag();
            h.row = episodeRows.get(position);
            bindEpisodeRow(h);
            return row;
        }
    }

    /** 行内状态: 任务优先(已完成✓/下载中环), 无任务时表达勾选态。 */
    private void bindEpisodeRow(Holder h) {
        EpisodeRow row = h.row;
        DownloadTask t = row.task;
        boolean done = t != null && (t.state == DownloadTask.STATE_COMPLETED
                || t.state == DownloadTask.STATE_EXPORTING);
        boolean busy = t != null && (t.state == DownloadTask.STATE_DOWNLOADING
                || t.state == DownloadTask.STATE_QUEUED);
        boolean checked = selected.contains(row.index);

        h.num.setText(String.valueOf(row.index + 1));
        h.title.setText(row.title);
        // 二轮10: 行内小字显示大小 —— 下载中=已下/总, 已完成/暂停=总量; 无任务不显示。
        // ⚠ HLS 下载的 totalBytes 落库前一直是 -1(见 DownloadEngine.onDownloadChanged),
        // 必须用 progressBytes 兑底, 否则显示空白/0KB。
        boolean hasTotal = t != null && t.totalBytes > 0;
        long shown = t == null ? 0 : displaySize(t);
        String size;
        if (hasTotal && t.state == DownloadTask.STATE_DOWNLOADING && t.progressBytes > 0) {
            size = sizeText(t.progressBytes) + "/" + sizeText(t.totalBytes);
        } else if (shown > 0) {
            size = sizeText(shown);
        } else {
            size = "";
        }
        h.size.setText(size);
        h.size.setVisibility(size.isEmpty() ? View.GONE : View.VISIBLE);
        // 已完成/下载中的行整体弱化(参考版式同款), 只有可勾选的行保持全对比度。
        h.root.setAlpha(busy || done ? 0.45f : 1f);
        if (busy) {
            // 下载中/排队中: 圆环接管(有总量画进度弧, 没有则转圈)。
            h.ring.setVisibility(View.GONE);
            h.ringProg.setVisibility(View.VISIBLE);
            if (hasTotal) {
                h.ringProg.setProgress((float) ((double) t.progressBytes / t.totalBytes));
            } else {
                h.ringProg.setSpinning();
            }
        } else {
            h.ringProg.setVisibility(View.GONE);
            h.ring.setVisibility(View.VISIBLE);
            // 已完成 → 青底对勾(复用 jc_checkbox_on, 与"全选"同一套状态语言);
            // 暂停/失败/未选 → 灰圈; 已选 → 青底对勾。
            h.ring.setBackgroundResource(done || checked
                    ? R.drawable.jc_checkbox_on : R.drawable.jc_checkbox_off);
        }
    }

    /**
     * 选集行(2026-10-08 重设计): [大集号] [集名 + 体积] [状态圈]。
     *
     * <p>交互: 行点击 = 勾选/取消(锁定集 toast 说明); 行**长按** = 复制链接 —— 参考版式的
     * 右侧留给状态圈, 复制链接挪到长按, pad 触屏长按与 TV 遥控器"长按 OK"都能触发。
     *
     * <p>为什么不用 CheckBox 当整行(旧实现): ListView CHOICE_MODE_MULTIPLE 会经
     * mCheckStates 与 CheckBox 自身 onClick 双通道切换 —— 勾上就取消不掉(问题4, pad 实测),
     * 且整行 Checkable 时行内放不了别的可点控件。行是普通 LinearLayout, 勾选状态只用
     * 圈标图 + selected 集合表达, 单一事实来源。
     */
    private View makeEpisodeRow() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.HORIZONTAL);
        root.setGravity(Gravity.CENTER_VERTICAL);
        root.setPadding(dp(16), dp(12), dp(16), dp(12));
        root.setFocusable(true);
        root.setClickable(true);
        root.setLongClickable(true);
        root.setBackgroundResource(R.drawable.jc_download_row_bg);

        TextView num = new TextView(this);
        // 二轮4: 集号是辅助信息, 14sp 退到集名之下, 不再抢视觉(旧 22sp 过大)。
        num.setTextSize(14);
        num.setTextColor(getColor(R.color.jc_text_tertiary));
        num.setMinWidth(dp(32));
        root.addView(num);

        LinearLayout mid = new LinearLayout(this);
        mid.setOrientation(LinearLayout.VERTICAL);
        TextView title = new TextView(this);
        title.setTextSize(15);
        title.setTextColor(getColor(R.color.jc_text));
        title.setMaxLines(1);
        title.setEllipsize(android.text.TextUtils.TruncateAt.END);
        mid.addView(title);
        TextView size = new TextView(this);
        size.setTextSize(11);
        size.setTextColor(getColor(R.color.jc_text_tertiary));
        size.setMaxLines(1);
        LinearLayout.LayoutParams sizeLp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        sizeLp.setMargins(0, dp(2), 0, 0);
        mid.addView(size, sizeLp);
        LinearLayout.LayoutParams midLp = new LinearLayout.LayoutParams(0,
                ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
        midLp.setMargins(dp(12), 0, dp(12), 0);
        root.addView(mid, midLp);

        // 二轮9: 右侧状态标 = 圆环进度(下载中画青色进度弧/排队转圈), 不再是 indeterminate loading。
        RingProgressView ringProg = new RingProgressView(this);
        root.addView(ringProg, new LinearLayout.LayoutParams(dp(22), dp(22)));

        View ring = new View(this);
        ring.setBackgroundResource(R.drawable.jc_checkbox_off);
        root.addView(ring, new LinearLayout.LayoutParams(dp(20), dp(20)));

        Holder h = new Holder();
        h.root = root;
        h.num = num;
        h.title = title;
        h.size = size;
        h.ringProg = ringProg;
        h.ring = ring;
        root.setTag(h);
        root.setOnClickListener(v -> {
            EpisodeRow r = ((Holder) v.getTag()).row;
            if (r == null) return;
            DownloadTask tk = r.task;
            if (tk != null && (tk.state == DownloadTask.STATE_COMPLETED
                    || tk.state == DownloadTask.STATE_EXPORTING)) {
                toast(getString(R.string.download_already_done));
                return;
            }
            if (tk != null && (tk.state == DownloadTask.STATE_DOWNLOADING
                    || tk.state == DownloadTask.STATE_QUEUED)) {
                toast(getString(R.string.download_already_active));
                return;
            }
            if (selected.contains(r.index)) {
                selected.remove(r.index);
            } else {
                selected.add(r.index);
            }
            updateSelectAllIcon();
            updateSelectedCount();
            notifyEpisodeAdapter();
        });
        root.setOnLongClickListener(v -> {
            EpisodeRow r = ((Holder) v.getTag()).row;
            if (r == null || r.url == null) return false;
            ClipboardManager cm =
                    (ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
            if (cm != null) {
                cm.setPrimaryClip(ClipData.newPlainText("url", r.url));
                toast(getString(R.string.download_copied));
                return true;
            }
            return false;
        });
        return root;
    }

    private static final class Holder {
        LinearLayout root;
        TextView num, title, size;
        RingProgressView ringProg;
        View ring;
        EpisodeRow row;
    }

    private static final class EpisodeRow {
        final int index;
        final String title;
        final String url;
        /** 该集既有任务(已完成/下载中/暂停/失败), 无任务为 null — 决定行状态与可勾选性。 */
        final DownloadTask task;

        EpisodeRow(int index, String title, String url, DownloadTask task) {
            this.index = index;
            this.title = title;
            this.url = url;
            this.task = task;
        }
    }

    /** 底部主按钮: 无勾选时禁用态(jc_download_btn_primary 的 disabled 分支), 有勾选带数量。 */
    private void updateSelectedCount() {
        Button btn = findViewById(R.id.btn_start_download);
        btn.setText(selected.isEmpty() ? getString(R.string.download_start)
                : getString(R.string.download_btn_start_fmt, selected.size()));
        btn.setEnabled(!selected.isEmpty());
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
        int failed = 0;
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
            try {
                // episodeCacheDir 会校验路径(防穿越); 非法 filmId 应只跳过这一集,
                // 绝不能让异常抛到主线程把**整批**下载一起带崩。
                t.cacheDir = engine.episodeCacheDir(t).getAbsolutePath();
            } catch (IllegalArgumentException ex) {
                failed++;
                android.util.Log.w("DownloadActivity",
                        "非法 filmId, 跳过该集: " + filmId, ex);
                continue;
            }
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
        if (failed > 0) {
            toast(failed + " 集因影片ID 含非法字符被跳过");
        } else if (added > 0) {
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
        taskMap.clear();
        int downloading = 0, queued = 0, paused = 0;
        long downloadingBytes = 0;
        for (DownloadTask t : engine.repository().listAll()) {
            taskMap.put(t.id, t);
            switch (t.state) {
                case DownloadTask.STATE_COMPLETED:
                case DownloadTask.STATE_EXPORTING:
                    doneTasks.add(t); // 导出中仍属"已完成"(状态由 EXPORTING 防并发, 完成回 COMPLETED)
                    break;
                case DownloadTask.STATE_DOWNLOADING:
                    activeTasks.add(t);
                    inProgress.add(t.id);
                    downloading++;
                    downloadingBytes += t.progressBytes;
                    break;
                case DownloadTask.STATE_QUEUED:
                    activeTasks.add(t);
                    inProgress.add(t.id);
                    queued++;
                    break;
                case DownloadTask.STATE_PAUSED:
                    activeTasks.add(t);
                    paused++;
                    break;
                default:
                    activeTasks.add(t);
            }
        }
        updateBanner(downloading, queued, paused, downloadingBytes);
        // 三轮1: 应用内已下载总大小(已完成任务之和) — 下载中/已完成两个 tab 各显示一份。
        long doneBytes = 0;
        for (DownloadTask t : doneTasks) doneBytes += displaySize(t);
        String summary = doneTasks.isEmpty() ? ""
                : getString(R.string.download_summary_fmt, doneTasks.size(), sizeText(doneBytes));
        tvActiveSummary.setText(summary);
        tvDoneSummary.setText(summary);
        tvActiveSummary.setVisibility(doneTasks.isEmpty() ? View.GONE : View.VISIBLE);
        tvDoneSummary.setVisibility(doneTasks.isEmpty() ? View.GONE : View.VISIBLE);
        renderActiveList();
        renderDoneList();
        // 任务状态变化会影响选集行的状态圈(✓/环/灰圈)与全选圆圈。
        // ⚠ 必须重建 episodeRows( renderEpisodeList 内部再 notify): episodeRows 里持有
        // 删除/完成前的旧 task 引用, 只 notifyDataSetChanged 会让选集页停留在陈旧状态
        // (2026-10-08 实测: 删除 E1 后选集行仍显示"✓ 已完成", 点行 toast"该集已下载完成")。
        renderEpisodeList(currentSegment);
    }

    /**
     * 下载横幅(选集 tab 顶部): 有活动任务(下载中/排队/暂停)才显示。
     * 速度 = 活动任务总进度字节的时间差分(粗估, 够横幅展示用);
     * 按钮 = 有在跑的就"暂停全部", 全暂停了就"全部继续"。
     */
    private void updateBanner(int downloading, int queued, int paused, long downloadingBytes) {
        int total = downloading + queued + paused;
        if (total == 0) {
            llBanner.setVisibility(View.GONE);
            lastBytes = -1;
            return;
        }
        llBanner.setVisibility(View.VISIBLE);
        long now = SystemClock.elapsedRealtime();
        if (downloading > 0 && lastBytes >= 0 && now > lastTs) {
            double kbps = (downloadingBytes - lastBytes) / (double) (now - lastTs);
            if (kbps >= 0) {
                tvBannerSpeed.setText(String.format(Locale.US, "%.1f MB/s", kbps / 1024.0));
            }
        } else if (downloading == 0) {
            tvBannerSpeed.setText("");
        }
        lastBytes = downloadingBytes;
        lastTs = now;

        tvBannerTitle.setText((downloading + queued) > 0
                ? getString(R.string.download_banner_downloading, downloading + queued, total)
                : getString(R.string.download_banner_paused, paused, total));
        boolean canPause = (downloading + queued) > 0;
        btnBannerAction.setText(canPause ? R.string.download_pause_all
                : R.string.download_resume_all);
        btnBannerAction.setTag(canPause);
    }

    private void onBannerAction() {
        boolean pause = Boolean.TRUE.equals(btnBannerAction.getTag());
        int n = 0;
        for (DownloadTask t : activeTasks) {
            if (pause && (t.state == DownloadTask.STATE_DOWNLOADING
                    || t.state == DownloadTask.STATE_QUEUED)) {
                engine.pause(t.id);
                n++;
            } else if (!pause && t.state == DownloadTask.STATE_PAUSED) {
                engine.resume(t.id);
                n++;
            }
        }
        if (n > 0) {
            toast(getString(pause ? R.string.download_paused_n : R.string.download_resumed_n, n));
        } else {
            toast(getString(R.string.download_none_operable));
        }
        refreshTasks();
    }

        // 适配器实例复用: refreshTasks 每 5 秒被 media3 进度通知触发一次。
    // 每次 setAdapter(新对象) 会清空 ListView 的 RecyclePool 并重建全部子 view,
    // **焦点与滚动位置一起丢失** —— TV 遥控器上表现为"正在浏览下载列表, 5 秒后焦点跳回顶部"。
    // 改为复用同一个适配器 + notifyDataSetChanged, 只重画不重建。
    private ActiveAdapter activeAdapter;
    private DoneAdapter doneAdapter;

    private void renderActiveList() {
        if (activeAdapter == null) {
            activeAdapter = new ActiveAdapter();
            lvActive.setAdapter(activeAdapter);
        } else {
            activeAdapter.notifyDataSetChanged();
        }
    }

    private void renderDoneList() {
        if (doneAdapter == null) {
            doneAdapter = new DoneAdapter();
            lvDone.setAdapter(doneAdapter);
        } else {
            doneAdapter.notifyDataSetChanged();
        }
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
            LinearLayout left = (LinearLayout) row.getChildAt(0);
            TextView name = (TextView) left.getChildAt(0);
            TextView info = (TextView) left.getChildAt(1);
            RingProgressView ring = (RingProgressView) row.getChildAt(1);
            Button action = (Button) row.getChildAt(2);
            Button del = (Button) row.getChildAt(3);
            name.setText(label(t));
            // 二轮9/10: 右侧圆环进度 + 信息行"百分比 · 已下/总量"。
            boolean hasTotal = t.totalBytes > 0;
            float frac = hasTotal ? (float) ((double) t.progressBytes / t.totalBytes) : -1f;
            String infoStr;
            switch (t.state) {
                case DownloadTask.STATE_DOWNLOADING:
                    infoStr = progressInfo(t);
                    if (frac >= 0f) {
                        ring.setProgress(frac);
                    } else {
                        ring.setSpinning();
                    }
                    ring.setVisibility(View.VISIBLE);
                    break;
                case DownloadTask.STATE_QUEUED:
                    infoStr = displaySize(t) > 0 ? "排队中 · " + sizeText(displaySize(t)) : "排队中";
                    ring.setSpinning();
                    ring.setVisibility(View.VISIBLE);
                    break;
                case DownloadTask.STATE_PAUSED:
                    infoStr = pausedFailedInfo(t);
                    ring.setProgress(frac >= 0f ? frac : 0f); // 暂停: 冻结的进度弧
                    ring.setVisibility(View.VISIBLE);
                    break;
                case DownloadTask.STATE_FAILED:
                    infoStr = pausedFailedInfo(t);
                    ring.setProgress(frac >= 0f ? frac : 0f);
                    ring.setVisibility(View.VISIBLE);
                    break;
                default:
                    infoStr = "";
                    ring.setVisibility(View.GONE);
            }
            info.setText(infoStr);
            // 失败态用危险红标出, 一眼看出哪条挂了; 其余保持弱化灰。
            info.setTextColor(t.state == DownloadTask.STATE_FAILED
                    ? getColor(R.color.jc_danger) : getColor(R.color.jc_text_secondary));
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

    /** 已完成行: 名 + 状态 + [导出][播放][删除] 三按钮. */
    private View makeDoneRow() {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(16), dp(12), dp(12), dp(12));
        row.setFocusable(true);
        row.setBackgroundResource(R.drawable.jc_download_row_bg);

        TextView name = new TextView(this);
        name.setTextSize(14);
        name.setTextColor(getColor(R.color.jc_text));
        name.setMaxLines(1);
        name.setEllipsize(android.text.TextUtils.TruncateAt.END);
        row.addView(name, new LinearLayout.LayoutParams(0,
                ViewGroup.LayoutParams.WRAP_CONTENT, 1f));

        TextView status = new TextView(this);
        status.setTextSize(12);
        status.setTextColor(getColor(R.color.jc_text_secondary));
        status.setGravity(Gravity.CENTER);
        row.addView(status, new LinearLayout.LayoutParams(dp(74), ViewGroup.LayoutParams.WRAP_CONTENT));

        Button exportBtn = smallAction("");
        row.addView(exportBtn, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        Button playBtn = smallAction("");
        row.addView(playBtn, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        // 删除: 下载缓存区用 NoOpCacheEvictor(主动下载永不清), 若没有删除入口用户永远
        // 无法释放空间 —— remove() 早已实现却没有 UI 能到, 等于死代码。
        Button deleteBtn = smallAction("");
        row.addView(deleteBtn, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        return row;
    }

    /** 行内小操作按钮(暂停/继续/删除/导出/播放): 统一走 chip 样式。
     *
     * <p>问题2/5: 旧实现 {@code new Button(this)} 继承壳层 Material 主题 ——
     * web 壳 AppTheme 是 Light.DarkActionBar, 按钮渲染成亮灰实底深色字,
     * 与整页深色玻璃风格完全脱节(2026-10-08 pad 截图实锤: "暂停/删除"白底突兀)。
     * 统一用 jc_download_chip_bg selector + jc_download_chip_text, 焦点态也由 selector 接管。
     */
    private Button smallAction(String text) {
        Button b = new Button(this, null, 0); // null,0 = 不套 Material 默认样式
        b.setText(text);
        b.setTextSize(13);
        b.setAllCaps(false);
        b.setMinWidth(0);
        b.setMinHeight(0);
        b.setPadding(dp(12), dp(5), dp(12), dp(5));
        // 同 createChip: setTextColor(int) 传资源 ID 是把 ID 当 ARGB, 必须 ColorStateList
        b.setTextColor(androidx.appcompat.content.res.AppCompatResources
                .getColorStateList(this, R.color.jc_download_chip_text));
        b.setBackgroundResource(R.drawable.jc_download_chip_bg);
        b.setFocusable(true);
        return b;
    }

    /** 删除任务(二次确认): 会连带删除该集的下载缓存分片与已导出的文件引用. */
    private void confirmRemove(DownloadTask t) {
        if (t == null) return;
        String msg = "删除「" + label(t) + "」?\n\n将同时删除该集的下载缓存分片, 之后需要重新下载。"
                + (t.exportedPath != null && !t.exportedPath.isEmpty()
                ? "\n已导出的文件不会被删除。" : "");
        new androidx.appcompat.app.AlertDialog.Builder(this, R.style.JcPlayerDialog)
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

    /** 下载中行: [名+信息] [圆环进度] [暂停/继续] [删除]. */
    private View makeTaskRow() {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(16), dp(12), dp(12), dp(12));
        // 行做成玻璃卡片(焦点/按压态由 jc_download_row_bg selector 接管),
        // TV 上方向键也能看出焦点在哪一行。
        row.setFocusable(true);
        row.setBackgroundResource(R.drawable.jc_download_row_bg);

        LinearLayout left = new LinearLayout(this);
        left.setOrientation(LinearLayout.VERTICAL);
        TextView name = new TextView(this);
        name.setTextSize(15);
        name.setTextColor(getColor(R.color.jc_text));
        name.setMaxLines(1);
        name.setEllipsize(android.text.TextUtils.TruncateAt.END);
        left.addView(name);
        TextView info = new TextView(this);
        info.setTextSize(12);
        info.setTextColor(getColor(R.color.jc_text_secondary));
        info.setMaxLines(1);
        LinearLayout.LayoutParams infoLp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        infoLp.setMargins(0, dp(2), 0, 0);
        left.addView(info, infoLp);
        LinearLayout.LayoutParams leftLp = new LinearLayout.LayoutParams(0,
                ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
        leftLp.setMargins(0, 0, dp(12), 0);
        row.addView(left, leftLp);

        // 二轮9: 圆环进度条(青色进度弧), 替代横向进度条 + loading。
        RingProgressView ring = new RingProgressView(this);
        row.addView(ring, new LinearLayout.LayoutParams(dp(26), dp(26)));

        Button action = smallAction("");
        LinearLayout.LayoutParams actionLp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        actionLp.setMargins(dp(10), 0, 0, 0);
        row.addView(action, actionLp);

        // 删除: 队列里的任务也能删(不然用户只能等它跑完/失败才能清缓存)
        Button del = smallAction("");
        row.addView(del, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        return row;
    }

    /** 行标题: "片名 E3 集名"。二轮11: 片方集名常自带片名("片名 第3集"/"片名·期名"), 去重防重复。 */
    private String label(DownloadTask t) {
        String film = (t.filmTitle == null || t.filmTitle.isEmpty()) ? "未知影片" : t.filmTitle;
        String ep = t.episodeTitle == null ? "" : t.episodeTitle.trim();
        if (!ep.isEmpty() && !film.equals("未知影片")) {
            if (ep.equals(film)) {
                ep = "";
            } else if (ep.startsWith(film)) {
                ep = trimSeparators(ep.substring(film.length()));
            } else if (ep.contains(film)) {
                ep = trimSeparators(ep.replace(film, " "));
            }
        }
        if (ep.isEmpty()) {
            ep = String.format(Locale.US, getString(R.string.download_episode_name), t.episode + 1);
        }
        return film + " E" + (t.episode + 1) + " " + ep;
    }

    /** 去掉片名前缀后残留的分隔符(空格 - · : ： — _ 全角空格)。 */
    private static String trimSeparators(String s) {
        if (s == null) return "";
        int i = 0;
        while (i < s.length()) {
            char c = s.charAt(i);
            if (c == ' ' || c == '-' || c == '·' || c == ':' || c == '：' || c == '—'
                    || c == '_' || c == '　') {
                i++;
            } else {
                break;
            }
        }
        return s.substring(i).trim();
    }

    /** 下载中行的信息文案: "45% · 12.3MB/45.6MB"; HLS 总量未知时退"下载中 · 已下 12.3MB"。 */
    private static String progressInfo(DownloadTask t) {
        boolean hasTotal = t.totalBytes > 0;
        if (!hasTotal) {
            return t.progressBytes > 0 ? "下载中 · " + sizeText(t.progressBytes) : "下载中";
        }
        String sz = sizeText(t.progressBytes) + "/" + sizeText(t.totalBytes);
        return (int) (t.progressBytes * 100 / t.totalBytes) + "% · " + sz;
    }

    /** 暂停/失败行信息: 总量已知用 x/y, 否则退已下字节(0 时只有状态词)。 */
    private static String pausedFailedInfo(DownloadTask t) {
        String label = t.state == DownloadTask.STATE_PAUSED ? "已暂停" : "失败";
        if (t.totalBytes > 0) {
            return label + " · " + sizeText(t.progressBytes) + "/" + sizeText(t.totalBytes);
        }
        return t.progressBytes > 0 ? label + " · " + sizeText(t.progressBytes) : label;
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
        long bytes = displaySize(t);
        if (t.exportedPath != null && !t.exportedPath.isEmpty()) {
            return "已导出 · " + sizeText(bytes);
        }
        return sizeText(bytes);
    }

    /**
     * 行显示体积: totalBytes 优先, 无效(≤0, HLS 完成前恒 -1)时退 progressBytes。
     * 完成态两者都已由 DownloadEngine 落真实值, 这里只是旧数据兑底。
     */
    private static long displaySize(DownloadTask t) {
        if (t == null) return 0;
        if (t.totalBytes > 0) return t.totalBytes;
        return Math.max(0L, t.progressBytes);
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
