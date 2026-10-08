package com.jerocine.player;

import android.view.KeyEvent;
import android.view.View;

import androidx.media3.ui.PlayerView;

/**
 * 遥控 / 键盘按键分发.
 *
 * 三态键控:
 *  A. 控制面板隐藏(默认): ←→=快进快退, ↑=上一集, ↓=下一集, OK=播放/暂停, MENU=唤出操作栏, BACK=双击退出
 *  B. 面板可见且焦点在进度条: ←→=快进快退, OK=播放/暂停, MENU/BACK=收起面板
 *  C. 面板可见且焦点在按钮: 方向键=焦点切换, OK=触发(super 处理), MENU/BACK=收起面板
 */
public class PlayerKeyEventHelper {

    private static final long EPISODE_CONFIRM_MS = 2000L;
    private static final long BACK_CONFIRM_MS = 2000L;

    private final PlayerSession session;
    private long lastEpisodeKeyAt = 0L;
    private int lastEpisodeKeyCode = 0;
    private long lastBackAt = 0L;

    public PlayerKeyEventHelper(PlayerSession session) {
        this.session = session;
    }

/**
 * 进度键长按步进 — 按 {@link KeyEvent#getRepeatCount()} 分档。
 *
 * <p><b>不能用 eventTime - downTime</b>: repeat 事件携带**相同的 downTime**、递增的 eventTime,
 * 于是每个 repeat 的"按住时长"都 ≥5s → 每次 repeat 都叠加一次 180s 步幅, 而 seekRelative
 * 每次都重读当前位置累加 → 5次/秒的repeat 速率下就是 900s/s, 按住 3 秒直接跳到片尾并触发
 * 自动下一集。用 repeat 次数分档则步长只增不减, 符合"越按越快"的手感。
 *
 * <p>快退 = 快进的 50%。封顶 {@link #MAX_SEEK_STEP_MS}: 再大就谈不上"步进"了。
 */
private long seekStepFor(KeyEvent ev, boolean forward) {
    int repeats = ev.getRepeatCount();
    long fwd;
    if (repeats <= 0) fwd = 10_000L;         // 首次按下
    else if (repeats < 4) fwd = 15_000L;
    else if (repeats < 12) fwd = 30_000L;
    else fwd = MAX_SEEK_STEP_MS;
    return forward ? fwd : fwd / 2;
}

/** 单次步进上限(快进; 快退为其一半)。 */
private static final long MAX_SEEK_STEP_MS = 60_000L;

public boolean dispatchKeyEvent(KeyEvent event) {
    int code = event.getKeyCode();
    // BACK 必须由本类全权处理: DOWN 走逻辑, UP 一并吞掉。
    // 否则 ACTION_UP 落到 super → Activity.onKeyUp → AOSP 对 BACK 调 onBackPressed()
    // → PlayerActivity.onBackPressed 又调一次 handleBack() → 同一次物理按键被计两次,
    // "再按一次返回退出"退化成单击即退出, 二次确认形同虚设。
    if (code == KeyEvent.KEYCODE_BACK) {
        if (event.getAction() == KeyEvent.ACTION_DOWN) return handleBack();
        return true;
    }
    if (event.getAction() != KeyEvent.ACTION_DOWN) {
        return session.host().dispatchToSuper(event);
    }
    final PlayerView pv = session.host().playerView();
    boolean controllerVisible = pv != null && pv.isControllerFullyVisible();
    boolean onSeekbar = isFocusOnSeekbar();

    switch (code) {
        case KeyEvent.KEYCODE_MENU:
        case KeyEvent.KEYCODE_INFO: {
            // MENU = 唤出/收起"顶部标题栏 + 底部操作栏"(用户要求: 不再弹播放控制弹窗).
            // 原弹窗里的每一项(倍速/选集/换源/广告过滤/跳过/退出)底栏都已有按钮, 弹窗纯属多一层.
            if (pv == null) return true;
            if (controllerVisible) {
                pv.hideController();
            } else {
                pv.showController();
            }
            return true;
        }
            case KeyEvent.KEYCODE_MEDIA_NEXT:
            case KeyEvent.KEYCODE_CHANNEL_UP:
                if (session.player != null && session.player.hasNextMediaItem()) {
                    session.markEpisodeSwitching();
                    session.player.seekToNextMediaItem();
                    session.host().showCenterToast("下一集", 600);
                }
                return true;
            case KeyEvent.KEYCODE_MEDIA_PREVIOUS:
            case KeyEvent.KEYCODE_CHANNEL_DOWN:
                if (session.player != null && session.player.hasPreviousMediaItem()) {
                    session.markEpisodeSwitching();
                    session.player.seekToPreviousMediaItem();
                    session.host().showCenterToast("上一集", 600);
                }
                return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:
            case KeyEvent.KEYCODE_SPACE:
                togglePlayPause();
                return true;
        }

        if (!controllerVisible) {
            switch (code) {
                case KeyEvent.KEYCODE_DPAD_LEFT:
                case KeyEvent.KEYCODE_MEDIA_REWIND:
                    seekRelative(-seekStepFor(event, false));
                    return true;
                case KeyEvent.KEYCODE_DPAD_RIGHT:
                case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD:
                    seekRelative(seekStepFor(event, true));
                    return true;
                case KeyEvent.KEYCODE_DPAD_UP:
                    return handleEpisodeKey(true, code);
                case KeyEvent.KEYCODE_DPAD_DOWN:
                    return handleEpisodeKey(false, code);
                case KeyEvent.KEYCODE_DPAD_CENTER:
                case KeyEvent.KEYCODE_ENTER:
                    togglePlayPause();
                    return true;
                default:
                    return session.host().dispatchToSuper(event);
            }
        }

        switch (code) {
            case KeyEvent.KEYCODE_DPAD_LEFT:
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                if (onSeekbar) {
                    seekRelative(code == KeyEvent.KEYCODE_DPAD_LEFT
                            ? -seekStepFor(event, false)
                            : seekStepFor(event, true));
                    return true;
                }
                return session.host().dispatchToSuper(event);
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER:
                if (onSeekbar) {
                    togglePlayPause();
                    return true;
                }
                return session.host().dispatchToSuper(event);
            default:
                return session.host().dispatchToSuper(event);
        }
    }

    /**
     * 统一返回处理: 控制面板可见先收起, 否则双击确认退出播放.
     * 遥控器 BACK (dispatchKeyEvent) 与安卓系统手势返回 (PlayerActivity.onBackPressed) 共用,
     * 保证两条路径行为一致.
     */
    public boolean handleBack() {
        // 本地模式: 无剧集上下文, 返回直接退出(回文件管理器/上一页), 不做双击确认
        if (session.localPlayback) {
            session.host().finishPlayer();
            return true;
        }
        final PlayerView pv = session.host().playerView();
        boolean controllerVisible = pv != null && pv.isControllerFullyVisible();
        if (controllerVisible) {
            pv.hideController();
            lastBackAt = 0L;
            return true;
        }
        long now = System.currentTimeMillis();
        if (now - lastBackAt < BACK_CONFIRM_MS) {
            lastBackAt = 0L;
            session.host().finishPlayer();
        } else {
            lastBackAt = now;
            session.host().showCenterToast("再按一次返回退出播放", 1800);
        }
        return true;
    }

    private boolean isFocusOnSeekbar() {
        PlayerView pv = session.host().playerView();
        if (pv == null) return false;
        View focus = pv.findFocus();
        if (focus == null) return false;
        return focus.getId() == androidx.media3.ui.R.id.exo_progress;
    }

    /** 切集 + 防误触: 1 下提示, 2s 内再按 1 下才生效. prev=true 上一集, false 下一集. */
    private boolean handleEpisodeKey(boolean prev, int code) {
        if (session.player == null) return true;
        boolean has = prev ? session.player.hasPreviousMediaItem() : session.player.hasNextMediaItem();
        if (!has) {
            session.host().showCenterToast(prev ? "已是第一集" : "已是最后一集", 800);
            return true;
        }
        long now = System.currentTimeMillis();
        if (lastEpisodeKeyCode == code && now - lastEpisodeKeyAt < EPISODE_CONFIRM_MS) {
            lastEpisodeKeyAt = 0;
            lastEpisodeKeyCode = 0;
            session.markEpisodeSwitching();
            if (prev) session.player.seekToPreviousMediaItem();
            else session.player.seekToNextMediaItem();
            session.host().showCenterToast(prev ? "上一集" : "下一集", 600);
        } else {
            lastEpisodeKeyAt = now;
            lastEpisodeKeyCode = code;
            session.host().showCenterToast(prev ? "再按 ▲ 切上一集" : "再按 ▼ 切下一集", 1500);
        }
        return true;
    }

    private void seekRelative(long deltaMs) {
        if (session.player == null) return;
        long target = Math.max(0, session.player.getCurrentPosition() + deltaMs);
        long duration = session.player.getDuration();
        if (duration > 0) target = Math.min(target, duration);
        session.player.seekTo(target);
        String dir = deltaMs > 0 ? "▶ +" : "◀ ";
        session.host().showCenterToast(dir + (deltaMs / 1000) + "s", 800);
    }

    private void togglePlayPause() {
        if (session.player == null) return;
        if (session.player.getPlayWhenReady()) {
            session.player.pause();
        } else {
            session.player.play();
        }
    }
}
