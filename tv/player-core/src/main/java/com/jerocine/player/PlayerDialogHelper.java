package com.jerocine.player;

import android.app.AlertDialog;
import android.content.Context;
import android.view.View;
import android.view.ViewGroup;
import android.widget.ArrayAdapter;
import android.widget.Button;
import android.widget.CompoundButton;
import android.widget.LinearLayout;
import android.widget.ListAdapter;
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

    /** 每个选集分段的集数 — 与 Web 端选集分段一致 */
    private static final int EPISODE_SEG = 30;

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
        AlertDialog dialog = new AlertDialog.Builder(ctx(), R.style.JcPlayerDialog)
                .setTitle("播放速度")
                .setSingleChoiceItems(SPEED_LABELS, speedIndex, (d, i) -> {
                    speedIndex = i;
                    if (session.player != null) {
                        session.player.setPlaybackParameters(new PlaybackParameters(SPEEDS[i]));
                    }
                    session.host().renderSpeedText(SPEED_LABELS[i]);
                    session.host().renderSpeedDot(Math.abs(SPEEDS[i] - 1f) > 0.001f);
                    session.host().showCenterToast("速度 " + SPEED_LABELS[i], 800);
                    d.dismiss();
                })
                .create();
        showDialog(dialog);
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
        String[] names = new String[session.sourceList.size()];
        boolean[] downloaded = new boolean[session.sourceList.size()];
        final int curEp = session.player != null ? session.player.getCurrentMediaItemIndex() : 0;
        for (int i = 0; i < session.sourceList.size(); i++) {
            PlayerSession.SourceData s = session.sourceList.get(i);
            names[i] = s.name + " (" + s.urls.size() + " 集)";
            // 当前集在该源有已下载副本 → 右侧灰色标签(当前源看本地副本表, 别源看 otherSource 表)
            downloaded[i] = session.isEpisodeDownloadedOnSource(curEp, i);
        }
        final long curPos = session.player != null ? session.player.getCurrentPosition() : 0L;
        AlertDialog dialog = new AlertDialog.Builder(ctx(), R.style.JcPlayerDialog)
                .setTitle("切换播放源")
                .setSingleChoiceItems(
                        downloadedRowAdapter(names, downloaded, PlayerModes.downloadedSourceTag()),
                        session.currentSourceIndex, (d, w) -> {
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
                .create();
        showDialog(dialog);
    }

    // ============================ 选集 ============================

    /** 选集: >30 集时先按 30 集一档分段选择, 再选具体集(D-pad 友好). */
    void showEpisodeDialog() {
        if (session.playlistTitles == null || session.playlistTitles.size() < 2) {
            session.host().showCenterToast("仅一集, 无需选择", 1500);
            return;
        }
        int total = session.playlistTitles.size();
        int cur = session.player != null ? session.player.getCurrentMediaItemIndex() : 0;
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
        AlertDialog dialog = new AlertDialog.Builder(ctx(), R.style.JcPlayerDialog)
                .setTitle("选集 (共 " + total + " 集)")
                .setSingleChoiceItems(segs, cur / EPISODE_SEG, (d, w) -> {
                    d.dismiss();
                    int start = w * EPISODE_SEG;
                    showEpisodeSegment(start, Math.min(start + EPISODE_SEG, total), cur);
                })
                .create();
        showDialog(dialog);
    }

    /** 展示 [start,end) 区间内的集供选择, 当前集在区间内则高亮; 已下载的集带右侧灰色标签. */
    private void showEpisodeSegment(int start, int end, int cur) {
        String[] arr = new String[end - start];
        boolean[] downloaded = new boolean[end - start];
        for (int i = start; i < end; i++) {
            arr[i - start] = session.playlistTitles.get(i);
            downloaded[i - start] = session.isLocalEpisode(i);
        }
        int checked = (cur >= start && cur < end) ? cur - start : 0;
        AlertDialog dialog = new AlertDialog.Builder(ctx(), R.style.JcPlayerDialog)
                .setTitle("选集 " + (start + 1) + "-" + end)
                .setSingleChoiceItems(
                        downloadedRowAdapter(arr, downloaded, PlayerModes.downloadedEpisodeTag()),
                        checked, (d, w) -> {
                    PlaybackTarget target = PlaybackTarget.selectEpisode(
                            session.currentSourceIndex, start + w, session.playlistTitles.size());
                    if (session.player != null) {
                        session.markEpisodeSwitching();
                        session.player.seekTo(target.episodeIndex, target.positionMs);
                    }
                    d.dismiss();
                })
                .create();
        showDialog(dialog);
    }

    // ============================ 跳过片头/片尾 ============================

    /**
     * 单选列表适配器: 标题左(带单选圆点) + 右侧灰色小标签(见 {@link PlayerModes} 的
     * downloadedSourceTag/downloadedEpisodeTag), 无标记的行标签隐藏、占位不塌。
     * 行布局 jc_dialog_row_downloaded; 字号/颜色由弹窗主题(JcPlayerDialog)统一。
     */
    private ListAdapter downloadedRowAdapter(String[] names, boolean[] tags, String tagText) {
        final Context c = ctx();
        return new ArrayAdapter<CharSequence>(
                c, R.layout.jc_dialog_row_downloaded, R.id.jc_row_text, names) {
            @Override
            public View getView(int position, View convertView, ViewGroup parent) {
                View row = super.getView(position, convertView, parent);
                TextView tag = row.findViewById(R.id.jc_row_tag);
                if (tags != null && position < tags.length && tags[position]) {
                    tag.setText(tagText);
                    tag.setVisibility(View.VISIBLE);
                } else {
                    tag.setVisibility(View.GONE);
                }
                return row;
            }
        };
    }

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
        ll.setPadding(pad * 3, pad, pad * 3, pad);

        // 弹窗内文字统一缩小一档(需求: 换源/换集/跳过弹窗的文字与控件缩放一个档位)
        final String skipLabel = "启用跳过 (开后片头" + PlayerSkipHelper.DEFAULT_SKIP_INTRO_MS / 1000
                + "s 片尾" + PlayerSkipHelper.DEFAULT_SKIP_OUTRO_MS / 1000 + "s)";
        Switch sw = new Switch(ctx());
        sw.setText(skipLabel);
        sw.setChecked(session.skipEnabled);
        sw.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 13);
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
        introValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 14);
        introValue.setPadding(0, pad, 0, 0);
        ll.addView(introValue);
        ll.addView(introRow);

        outroValue.setText("片尾跳过: " + session.skipOutroMs / 1000 + " 秒");
        outroValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 14);
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
        AlertDialog dialog = new AlertDialog.Builder(ctx(), R.style.JcPlayerDialog)
                .setTitle("跳过片头 / 片尾")
                .setView(ll)
                .create();
        showDialog(dialog);
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
            b.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 13);
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

    /**
     * 统一弹窗展示: 内容超高时限制为屏幕高度的 80% (选集/切源等大列表弹窗),
     * 内容少时保持 wrap_content 不拉伸.
     */
    private void showDialog(AlertDialog dialog) {
        dialog.show();
        android.view.Window win = dialog.getWindow();
        if (win == null) return;
        View decor = win.getDecorView();
        android.util.DisplayMetrics dm = ctx().getResources().getDisplayMetrics();
        int maxH = (int) (dm.heightPixels * 0.8f);
        // 高度上限 80% 屏幕(用户拍板 2026-10-09)。不能用 UNSPECIFIED 量 decor:
        // ListView wrap-content 只实测前几个子项、其余按均值估算 → 30 集的选集弹窗
        // 实际远超 80% 却被判成"没超"。改用 AT_MOST(maxH) 上限测量, 量到上限即钉死。
        decor.measure(
                View.MeasureSpec.makeMeasureSpec(dm.widthPixels, View.MeasureSpec.AT_MOST),
                View.MeasureSpec.makeMeasureSpec(maxH, View.MeasureSpec.AT_MOST));
        if (decor.getMeasuredHeight() >= maxH) {
            android.view.WindowManager.LayoutParams lp = win.getAttributes();
            lp.height = maxH;
            win.setAttributes(lp);
        }
    }
}
