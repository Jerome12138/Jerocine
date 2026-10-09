package com.jerocine.player;

import android.content.Context;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.CompoundButton;
import android.widget.LinearLayout;
import android.widget.Switch;
import android.widget.TextView;

import androidx.media3.common.PlaybackParameters;
import androidx.media3.ui.PlayerView;

import org.json.JSONObject;

/**
 * 播放器各类弹窗 — 倍速 / 选集 / 切源 / 跳过设置 / 播放控制菜单.
 *
 * 依赖 {@link PlayerSession}(状态)与 Host(上下文/提示/退出/视图访问),
 * 与其它 helper 无互相引用.
 */
public class PlayerDialogHelper {

    private static final float[] SPEEDS = {0.5f, 1.0f, 1.25f, 1.5f, 2.0f, 3.0f};
    private static final String[] SPEED_LABELS = {"0.5×", "1.0×", "1.25×", "1.5×", "2.0×", "3.0×"};

    private final PlayerSession session;

    private int speedIndex = 1;
    private boolean ctlBtnsBound = false;

    public PlayerDialogHelper(PlayerSession session) {
        this.session = session;
    }

    private Context ctx() {
        return session.host().context();
    }

    // ============================ 倍速 ============================

    void showSpeedDialog() {
        com.jerocine.player.ui.JcDialog.list(ctx())
                .title("播放速度")
                .items(SPEED_LABELS)
                .checkable(true)
                .checkedIndex(speedIndex)
                .onItemClick((d, i) -> {
                    speedIndex = i;
                    if (session.player != null) {
                        session.player.setPlaybackParameters(new PlaybackParameters(SPEEDS[i]));
                    }
                    session.host().renderSpeedText(SPEED_LABELS[i]);
                    session.host().renderSpeedDot(Math.abs(SPEEDS[i] - 1f) > 0.001f);
                    session.host().showCenterToast("速度 " + SPEED_LABELS[i], 800);
                    d.dismiss();
                })
                .show();
    }

    // ============================ 控制面板按钮 ============================

    /**
     * 绑定进度条下方自定义按钮(面板每次显示时调, 幂等).
     * 已无单独播放/暂停键 — 进度条获焦时按确认键即播暂(见 PlayerKeyEventHelper).
     */
    void bindControlButtons() {
        if (ctlBtnsBound) return;
        PlayerView pv = session.host().playerView();
        if (pv == null) return;
        Button prev = pv.findViewById(R.id.btn_prev);
        Button next = pv.findViewById(R.id.btn_next);
        Button speed = pv.findViewById(R.id.btn_speed);
        Button episodes = pv.findViewById(R.id.btn_episodes);
        Button source = pv.findViewById(R.id.btn_source);
        Button skip = pv.findViewById(R.id.btn_skip);
        Button close = pv.findViewById(R.id.btn_close);
        Button adFilter = pv.findViewById(R.id.btn_ad_filter);
        Button localToggle = pv.findViewById(R.id.btn_local_toggle);
        if (close == null) return;

        boolean multi = session.sourceList.size() >= 2;
        boolean playlist = session.player != null && session.player.getMediaItemCount() > 1;
        if (prev != null) {
            prev.setVisibility(playlist ? View.VISIBLE : View.GONE);
            prev.setOnClickListener(b -> {
                if (session.player != null) session.player.seekToPreviousMediaItem();
            });
        }
        if (next != null) {
            next.setVisibility(playlist ? View.VISIBLE : View.GONE);
            next.setOnClickListener(b -> {
                if (session.player != null) session.player.seekToNextMediaItem();
            });
        }
        if (speed != null) speed.setOnClickListener(b -> showSpeedDialog());
        if (episodes != null) episodes.setOnClickListener(b -> showEpisodeDialog());
        if (source != null) {
            source.setVisibility(multi ? View.VISIBLE : View.GONE);
            source.setOnClickListener(b -> showSourceDialog());
        }
        if (skip != null) skip.setOnClickListener(b -> showSkipSettingsDialog());
        if (adFilter != null) adFilter.setOnClickListener(b -> session.host().toggleAdFilter());
        if (localToggle != null) localToggle.setOnClickListener(b -> session.host().toggleLocalOnline());
        close.setOnClickListener(b -> session.host().finishPlayer());
        ctlBtnsBound = true;
    }
    // ============================ 切源 ============================

    /** 切换源(仅多源模式) — 保留当前集数 + 播放进度. */
    void showSourceDialog() {
        if (session.sourceList.size() < 2) {
            session.host().showCenterToast("仅一个源, 无需切换", 1500);
            return;
        }
        final int curEp = session.player != null ? session.player.getCurrentMediaItemIndex() : 0;
        java.util.List<com.jerocine.player.ui.JcDialogItem> rows = new java.util.ArrayList<>();
        for (int i = 0; i < session.sourceList.size(); i++) {
            PlayerSession.SourceData s = session.sourceList.get(i);
            // 当前集在该源有已下载副本 → 右侧灰色标签(当前源看本地副本表, 别源看 otherSource 表);
            // tag 传 null = 不显示不占位(JcDialog 统一行自管)
            rows.add(new com.jerocine.player.ui.JcDialogItem(s.name + " (" + s.urls.size() + " 集)",
                    session.isEpisodeDownloadedOnSource(curEp, i)
                            ? PlayerModes.downloadedSourceTag() : null));
        }
        final long curPos = session.player != null ? session.player.getCurrentPosition() : 0L;
        com.jerocine.player.ui.JcDialog.list(ctx())
                .title("切换播放源")
                .items(rows)
                .checkable(true)
                .checkedIndex(session.currentSourceIndex)
                .onItemClick((d, w) -> {
                    if (w == session.currentSourceIndex) {
                        d.dismiss();
                        return;
                    }
                    PlaybackTarget target = PlaybackTarget.switchSource(
                            w, curEp, curPos, session.sourceList.get(w).urls.size());
                    session.currentSourceIndex = target.sourceIndex;
                    session.loadSourceIntoPlayer(
                            target.sourceIndex, target.episodeIndex, target.positionMs);
                    session.host().showCenterToast("已切到「" + session.sourceList.get(w).name
                            + "」 (从 " + (target.positionMs / 1000) + "s 续播)", 2000);
                    d.dismiss();
                })
                .show();
    }

    // ============================ 选集 ============================

    /** 选集: >30 集时先按 30 集一档分段选择, 再选具体集(D-pad 友好). 分段计算见 {@link com.jerocine.player.ui.EpisodeSegments}. */
    void showEpisodeDialog() {
        if (session.playlistTitles == null || session.playlistTitles.size() < 2) {
            session.host().showCenterToast("仅一集, 无需选择", 1500);
            return;
        }
        int total = session.playlistTitles.size();
        int cur = session.player != null ? session.player.getCurrentMediaItemIndex() : 0;
        if (total <= com.jerocine.player.ui.EpisodeSegments.PER_SEG) {
            showEpisodeSegment(0, total, cur);
            return;
        }
        String[] segs = com.jerocine.player.ui.EpisodeSegments.titles(total);
        com.jerocine.player.ui.JcDialog.list(ctx())
                .title("选集 (共 " + total + " 集)")
                .items(segs)
                .checkable(true)
                .checkedIndex(com.jerocine.player.ui.EpisodeSegments.checkedSegment(cur))
                .onItemClick((d, w) -> {
                    d.dismiss();
                    int start = com.jerocine.player.ui.EpisodeSegments.startOf(w);
                    showEpisodeSegment(start,
                            com.jerocine.player.ui.EpisodeSegments.endOf(w, total), cur);
                })
                .show();
    }

    /** 展示 [start,end) 区间内的集供选择, 当前集在区间内则高亮; 已下载的集带右侧灰色标签. */
    private void showEpisodeSegment(int start, int end, int cur) {
        java.util.List<com.jerocine.player.ui.JcDialogItem> rows = new java.util.ArrayList<>();
        for (int i = start; i < end; i++) {
            // 当前集已下载 → 右侧灰色标签; tag 传 null = 不显示不占位
            rows.add(new com.jerocine.player.ui.JcDialogItem(session.playlistTitles.get(i),
                    session.isLocalEpisode(i) ? PlayerModes.downloadedEpisodeTag() : null));
        }
        int checked = (cur >= start && cur < end) ? cur - start : 0;
        com.jerocine.player.ui.JcDialog.list(ctx())
                .title("选集 " + (start + 1) + "-" + end)
                .items(rows)
                .checkable(true)
                .checkedIndex(checked)
                .onItemClick((d, w) -> {
                    PlaybackTarget target = PlaybackTarget.selectEpisode(
                            session.currentSourceIndex, start + w, session.playlistTitles.size());
                    if (session.player != null) {
                        session.markEpisodeSwitching();
                        session.player.seekTo(target.episodeIndex, target.positionMs);
                    }
                    d.dismiss();
                })
                .show();
    }

    // ============================ 跳过片头/片尾 ============================

    /** 跳过参数变化 → 通知壳层回写账号(跨设备记忆). 关闭时记 0/0(=不跳). */
    private void emitSkipChanged() {
        try {
            JSONObject p = new JSONObject();
            p.put("filmId", session.host().filmId());
            p.put("intro", session.skipEnabled ? session.skipIntroMs / 1000 : 0);
            p.put("outro", session.skipEnabled ? session.skipOutroMs / 1000 : 0);
            session.host().emitEvent("skipSettingChanged", p);
        } catch (Exception ignore) {
        }
    }

    /** 跳过片头片尾设置 — 总开关 + ±10/±60 stepper, 改动即时生效并回写账号. */
    void showSkipSettingsDialog() {
        LinearLayout ll = new LinearLayout(ctx());
        ll.setOrientation(LinearLayout.VERTICAL);
        int pad = (int) (16 * ctx().getResources().getDisplayMetrics().density);
        // 左右内边距与标题对齐(同一 dialogPreferredPadding), 不再散写 3 倍 pad
        int padX = com.jerocine.player.ui.JcDialog.contentPaddingX(ctx());
        ll.setPadding(padX, pad, padX, 0);

        // 弹窗内容文字统一 15sp(2026-10-09 用户拍板: 与列表主文字/按钮同一档)
        final String skipLabel = "启用跳过 (开后片头" + PlayerSkipHelper.DEFAULT_SKIP_INTRO_MS / 1000
                + "s 片尾" + PlayerSkipHelper.DEFAULT_SKIP_OUTRO_MS / 1000 + "s)";
        Switch sw = new Switch(ctx());
        sw.setText(skipLabel);
        sw.setChecked(session.skipEnabled);
        sw.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 15);
        sw.setTextColor(0xFFFFFFFF);
        ll.addView(sw);

        final TextView introValue = new TextView(ctx());
        final TextView outroValue = new TextView(ctx());
        introValue.setTextColor(0xFFFFFFFF);
        outroValue.setTextColor(0xFFFFFFFF);
        // 操作即时生效: stepper 直接改 session 的 skip 值, 无需"保存"
        final LinearLayout introRow = makeStepperRow(pad, -60, -10, 10, 60, delta -> {
            session.skipIntroMs = Math.max(0, session.skipIntroMs + delta * 1000L);
            introValue.setText("片头跳过: " + session.skipIntroMs / 1000 + " 秒");
            emitSkipChanged();
        });
        final LinearLayout outroRow = makeStepperRow(pad, -60, -10, 10, 60, delta -> {
            session.skipOutroMs = Math.max(0, session.skipOutroMs + delta * 1000L);
            outroValue.setText("片尾跳过: " + session.skipOutroMs / 1000 + " 秒");
            emitSkipChanged();
        });

        introValue.setText("片头跳过: " + session.skipIntroMs / 1000 + " 秒");
        introValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 15);
        introValue.setPadding(0, pad, 0, 0);
        ll.addView(introValue);
        ll.addView(introRow);

        outroValue.setText("片尾跳过: " + session.skipOutroMs / 1000 + " 秒");
        outroValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 15);
        outroValue.setPadding(0, pad, 0, 0);
        ll.addView(outroValue);
        ll.addView(outroRow);

        // 开关 off 时灰度禁用 stepper
        Runnable applyEnabled = () -> {
            boolean on = session.skipEnabled;
            introValue.setAlpha(on ? 1f : 0.4f);
            outroValue.setAlpha(on ? 1f : 0.4f);
            setRowEnabled(introRow, on);
            setRowEnabled(outroRow, on);
        };
        applyEnabled.run();
        sw.setOnCheckedChangeListener((CompoundButton b, boolean isOn) -> {
            session.skipEnabled = isOn;
            applyEnabled.run();
            session.host().renderSkipDot(isOn);
            session.host().showCenterToast(isOn ? "跳过已开启" : "跳过已关闭", 1000);
            emitSkipChanged();
        });

        // 不放"完成"按钮: 开关/stepper 即时生效, 返回键关闭弹窗即可
        com.jerocine.player.ui.JcDialog.custom(ctx())
                .title("跳过片头 / 片尾")
                .content(ll)
                .show();
    }

    private void setRowEnabled(LinearLayout row, boolean on) {
        for (int i = 0; i < row.getChildCount(); i++) {
            row.getChildAt(i).setEnabled(on);
        }
    }

    interface IntConsumer {
        void accept(int v);
    }

    /** 一行 4 个 step 按钮: [-60s][-10s][+10s][+60s], D-pad 可聚焦. */
    private LinearLayout makeStepperRow(int pad, int s1, int s2, int s3, int s4, IntConsumer onStep) {
        LinearLayout row = new LinearLayout(ctx());
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setPadding(0, pad / 2, 0, 0);
        int[] steps = {s1, s2, s3, s4};
        for (int step : steps) {
            final int s = step;
            Button b = new Button(ctx());
            b.setText((s > 0 ? "+" : "") + s + "s");
            b.setAllCaps(false);
            b.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 15);
            b.setFocusable(true);
            b.setBackgroundResource(R.drawable.jc_step_btn_bg);
            b.setTextColor(ctx().getResources().getColorStateList(R.color.jc_step_btn_text));
            b.setOnFocusChangeListener((v, hasFocus) ->
                    v.animate().scaleX(hasFocus ? 1.12f : 1f).scaleY(hasFocus ? 1.12f : 1f)
                            .setDuration(120).start());
            LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(0,
                    ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
            int gap = (int) (4 * ctx().getResources().getDisplayMetrics().density);
            lp.setMargins(gap, 0, gap, 0);
            b.setLayoutParams(lp);
            b.setOnClickListener(v -> onStep.accept(s));
            row.addView(b);
        }
        return row;
    }

}
