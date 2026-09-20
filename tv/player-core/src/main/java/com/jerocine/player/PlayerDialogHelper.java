package com.jerocine.player;

import android.app.AlertDialog;
import android.widget.Button;
import android.widget.CompoundButton;
import android.widget.LinearLayout;
import android.widget.Switch;
import android.widget.TextView;
import android.view.View;
import android.view.ViewGroup;

import androidx.media3.common.PlaybackParameters;

import java.util.ArrayList;

/**
 * 各种 Dialog 助手 — 倍速/选集/切源/跳过设置/播放控制菜单。
 */
public class PlayerDialogHelper {

    private static final float[] SPEEDS = {0.5f, 1.0f, 1.25f, 1.5f, 2.0f, 3.0f};
    private static final String[] SPEED_LABELS = {"0.5×", "1.0×", "1.25×", "1.5×", "2.0×", "3.0×"};

    /** 每个选集分段(tab)的集数 — 与 web 选集分段一致 */
    private static final int EPISODE_SEG = 30;

    private final PlayerActivity activity;
    private final PlayerSkipHelper skipHelper;

    int speedIndex = 1;
    private boolean ctlBtnsBound = false;

    public PlayerDialogHelper(PlayerActivity activity, PlayerSkipHelper skipHelper) {
        this.activity = activity;
        this.skipHelper = skipHelper;
    }

    void showSpeedDialog() {
        new AlertDialog.Builder(activity, R.style.JcPlayerDialog)
                .setTitle("播放速度")
                .setSingleChoiceItems(SPEED_LABELS, speedIndex, (d, i) -> {
                    speedIndex = i;
                    if (activity.player != null) {
                        activity.player.setPlaybackParameters(new PlaybackParameters(SPEEDS[i]));
                    }
                    activity.speedText.setText(SPEED_LABELS[i]);
                    activity.showCenterToast("速度 " + SPEED_LABELS[i], 800);
                    d.dismiss();
                })
                .show();
    }

    /**
     * 绑定进度条下方自定义控件按钮(控件面板每次显示时调, 幂等)。
     */
    void bindControlButtons() {
        if (ctlBtnsBound) return;
        Button prev = activity.playerView.findViewById(R.id.btn_prev);
        Button next = activity.playerView.findViewById(R.id.btn_next);
        Button speed = activity.playerView.findViewById(R.id.btn_speed);
        Button episodes = activity.playerView.findViewById(R.id.btn_episodes);
        Button source = activity.playerView.findViewById(R.id.btn_source);
        Button skip = activity.playerView.findViewById(R.id.btn_skip);
        Button close = activity.playerView.findViewById(R.id.btn_close);
        if (close == null) return;

        boolean multi = activity.sourceList.size() >= 2;
        boolean playlist = activity.player != null && activity.player.getMediaItemCount() > 1;
        if (prev != null) {
            prev.setVisibility(playlist ? View.VISIBLE : View.GONE);
            prev.setOnClickListener(b -> { if (activity.player != null) activity.player.seekToPreviousMediaItem(); });
        }
        if (next != null) {
            next.setVisibility(playlist ? View.VISIBLE : View.GONE);
            next.setOnClickListener(b -> { if (activity.player != null) activity.player.seekToNextMediaItem(); });
        }
        if (speed != null) speed.setOnClickListener(b -> showSpeedDialog());
        if (episodes != null) episodes.setOnClickListener(b -> showEpisodeDialog());
        if (source != null) {
            source.setVisibility(multi ? View.VISIBLE : View.GONE);
            source.setOnClickListener(b -> showSourceDialog());
        }
        if (skip != null) skip.setOnClickListener(b -> showSkipSettingsDialog());
        close.setOnClickListener(b -> activity.finish());
        ctlBtnsBound = true;
    }

    /**
     * 菜单键: 播放控制总入口
     */
    void showPlayMenu() {
        boolean multiSrc = activity.sourceList.size() >= 2;
        String srcLabel = multiSrc
                ? "切换源 (" + activity.sourceList.get(activity.currentSourceIndex).name + ")"
                : null;
        String adLabel = "广告过滤: " + (activity.adFilterHelper.adFilterOn ? "开" : "关");
        String[] items = multiSrc
                ? new String[]{"倍速 " + SPEED_LABELS[speedIndex], "选集", srcLabel, adLabel, "跳过设置", "关闭播放"}
                : new String[]{"倍速 " + SPEED_LABELS[speedIndex], "选集", adLabel, "跳过设置", "关闭播放"};
        new AlertDialog.Builder(activity, R.style.JcPlayerDialog)
                .setTitle("播放控制")
                .setItems(items, (d, w) -> {
                    if (multiSrc) {
                        switch (w) {
                            case 0: showSpeedDialog(); break;
                            case 1: showEpisodeDialog(); break;
                            case 2: showSourceDialog(); break;
                            case 3: activity.adFilterHelper.toggleAdFilter(); break;
                            case 4: showSkipSettingsDialog(); break;
                            case 5: activity.finish(); break;
                        }
                    } else {
                        switch (w) {
                            case 0: showSpeedDialog(); break;
                            case 1: showEpisodeDialog(); break;
                            case 2: activity.adFilterHelper.toggleAdFilter(); break;
                            case 3: showSkipSettingsDialog(); break;
                            case 4: activity.finish(); break;
                        }
                    }
                })
                .show();
    }

    /**
     * 切换源 dialog (仅多源模式) — 保留当前集数 + 当前播放时间, 不打断观看。
     */
    void showSourceDialog() {
        if (activity.sourceList.size() < 2) {
            activity.showCenterToast("仅一个源, 无需切换", 1500);
            return;
        }
        String[] names = new String[activity.sourceList.size()];
        for (int i = 0; i < activity.sourceList.size(); i++) {
            PlayerActivity.SourceData s = activity.sourceList.get(i);
            names[i] = s.name + " (" + s.urls.size() + " 集)";
        }
        final int curEp = activity.player != null ? activity.player.getCurrentMediaItemIndex() : 0;
        final long curPos = activity.player != null ? activity.player.getCurrentPosition() : 0;
        new AlertDialog.Builder(activity, R.style.JcPlayerDialog)
                .setTitle("切换播放源")
                .setSingleChoiceItems(names, activity.currentSourceIndex, (d, w) -> {
                    if (w == activity.currentSourceIndex) { d.dismiss(); return; }
                    activity.currentSourceIndex = w;
                    activity.loadSourceIntoPlayer(w, curEp, curPos);
                    activity.showCenterToast("已切到「" + activity.sourceList.get(w).name + "」 (从 " + (curPos / 1000) + "s 续播)", 2000);
                    d.dismiss();
                })
                .show();
    }

    /**
     * 选集 dialog: >30 集时先按 30 集一档分段选择, 再选具体集(D-pad 友好)。
     */
    void showEpisodeDialog() {
        if (activity.playlistTitles == null || activity.playlistTitles.size() < 2) {
            activity.showCenterToast("仅一集, 无需选择", 1500);
            return;
        }
        int total = activity.playlistTitles.size();
        int cur = activity.player != null ? activity.player.getCurrentMediaItemIndex() : 0;
        if (total <= EPISODE_SEG) {
            showEpisodeSegment(0, total, cur);
            return;
        }
        int segCount = (total + EPISODE_SEG - 1) / EPISODE_SEG;
        String[] segs = new String[segCount];
        for (int i = 0; i < segCount; i++) {
            int s = i * EPISODE_SEG + 1;
            int e = Math.min((i + 1) * EPISODE_SEG, total);
            segs[i] = "第 " + s + "-" + e + " 集";
        }
        new AlertDialog.Builder(activity, R.style.JcPlayerDialog)
                .setTitle("选集 (共 " + total + " 集)")
                .setSingleChoiceItems(segs, cur / EPISODE_SEG, (d, w) -> {
                    d.dismiss();
                    int start = w * EPISODE_SEG;
                    showEpisodeSegment(start, Math.min(start + EPISODE_SEG, total), cur);
                })
                .show();
    }

    private void showEpisodeSegment(int start, int end, int cur) {
        String[] arr = new String[end - start];
        for (int i = start; i < end; i++) arr[i - start] = activity.playlistTitles.get(i);
        int checked = (cur >= start && cur < end) ? cur - start : -1;
        new AlertDialog.Builder(activity, R.style.JcPlayerDialog)
                .setTitle("选集 " + (start + 1) + "-" + end)
                .setSingleChoiceItems(arr, checked, (d, w) -> {
                    if (activity.player != null) activity.player.seekTo(start + w, 0);
                    d.dismiss();
                })
                .show();
    }

    /**
     * 跳过参数变化 → 通知 web 回写账号(跨设备记忆)。关闭时记 0/0(=不跳)。
     */
    private void emitSkipChanged() {
        try {
            org.json.JSONObject p = new org.json.JSONObject();
            p.put("filmId", activity.getIntent().getStringExtra(PlayerActivity.EXTRA_FILM_ID));
            p.put("intro", skipHelper.skipEnabled ? skipHelper.skipIntroMs / 1000 : 0);
            p.put("outro", skipHelper.skipEnabled ? skipHelper.skipOutroMs / 1000 : 0);
            PlayerActivity.emit("skipSettingChanged", p);
        } catch (Exception ignore) {}
    }

    /**
     * 跳过片头片尾设置 — 总开关 + ±10/±60 stepper, 改动即时生效并回写账号。
     */
    void showSkipSettingsDialog() {
        LinearLayout ll = new LinearLayout(activity);
        ll.setOrientation(LinearLayout.VERTICAL);
        int pad = (int) (16 * activity.getResources().getDisplayMetrics().density);
        ll.setPadding(pad * 3, pad, pad * 3, pad);

        Switch sw = new Switch(activity);
        sw.setText("启用跳过 (开后片头" + PlayerSkipHelper.DEFAULT_SKIP_INTRO_MS / 1000 + "s 片尾" + PlayerSkipHelper.DEFAULT_SKIP_OUTRO_MS / 1000 + "s)");
        sw.setChecked(skipHelper.skipEnabled);
        sw.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 16);
        sw.setTextColor(0xFFFFFFFF);
        ll.addView(sw);

        final TextView introValue = new TextView(activity);
        final TextView outroValue = new TextView(activity);
        introValue.setTextColor(0xFFFFFFFF);
        outroValue.setTextColor(0xFFFFFFFF);
        final LinearLayout introRow = makeStepperRow(pad, -60, -10, 10, 60, (delta) -> {
            skipHelper.skipIntroMs = Math.max(0, skipHelper.skipIntroMs + delta * 1000L);
            introValue.setText("片头跳过: " + skipHelper.skipIntroMs / 1000 + " 秒");
            emitSkipChanged();
        });
        final LinearLayout outroRow = makeStepperRow(pad, -60, -10, 10, 60, (delta) -> {
            skipHelper.skipOutroMs = Math.max(0, skipHelper.skipOutroMs + delta * 1000L);
            outroValue.setText("片尾跳过: " + skipHelper.skipOutroMs / 1000 + " 秒");
            emitSkipChanged();
        });

        introValue.setText("片头跳过: " + skipHelper.skipIntroMs / 1000 + " 秒");
        introValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 18);
        introValue.setPadding(0, pad, 0, 0);
        ll.addView(introValue);
        ll.addView(introRow);

        outroValue.setText("片尾跳过: " + skipHelper.skipOutroMs / 1000 + " 秒");
        outroValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 18);
        outroValue.setPadding(0, pad, 0, 0);
        ll.addView(outroValue);
        ll.addView(outroRow);

        Runnable applyEnabled = () -> {
            boolean on = skipHelper.skipEnabled;
            introValue.setAlpha(on ? 1f : 0.4f);
            outroValue.setAlpha(on ? 1f : 0.4f);
            setRowEnabled(introRow, on);
            setRowEnabled(outroRow, on);
        };
        applyEnabled.run();
        sw.setOnCheckedChangeListener((CompoundButton b, boolean isOn) -> {
            skipHelper.skipEnabled = isOn;
            applyEnabled.run();
            activity.showCenterToast(isOn ? "跳过已开启" : "跳过已关闭", 1000);
            emitSkipChanged();
        });

        new AlertDialog.Builder(activity, R.style.JcPlayerDialog)
                .setTitle("跳过片头 / 片尾")
                .setView(ll)
                .show();
    }

    private void setRowEnabled(LinearLayout row, boolean on) {
        for (int i = 0; i < row.getChildCount(); i++) {
            row.getChildAt(i).setEnabled(on);
        }
    }

    interface IntConsumer { void accept(int v); }

    /**
     * 一行 4 个 step 按钮: [-60s][-10s][+10s][+60s], D-pad 可聚焦。
     */
    private LinearLayout makeStepperRow(int pad, int s1, int s2, int s3, int s4, IntConsumer onStep) {
        LinearLayout row = new LinearLayout(activity);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setPadding(0, pad / 2, 0, 0);
        int[] steps = {s1, s2, s3, s4};
        for (int step : steps) {
            final int s = step;
            Button b = new Button(activity);
            b.setText((s > 0 ? "+" : "") + s + "s");
            b.setAllCaps(false);
            b.setFocusable(true);
            b.setBackgroundResource(R.drawable.jc_step_btn_bg);
            b.setTextColor(activity.getResources().getColorStateList(R.color.jc_step_btn_text));
            b.setOnFocusChangeListener((v, hasFocus) ->
                    v.animate().scaleX(hasFocus ? 1.12f : 1f).scaleY(hasFocus ? 1.12f : 1f)
                            .setDuration(120).start());
            LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(0,
                    ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
            int gap = (int) (4 * activity.getResources().getDisplayMetrics().density);
            lp.setMargins(gap, 0, gap, 0);
            b.setLayoutParams(lp);
            b.setOnClickListener(v -> onStep.accept(s));
            row.addView(b);
        }
        return row;
    }
}
