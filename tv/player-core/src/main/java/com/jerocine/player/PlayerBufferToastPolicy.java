package com.jerocine.player;

import androidx.media3.common.Player;

/**
 * 缓冲进度提示策略(纯逻辑, 2026-10-11 用户要求):
 * STATE_BUFFERING 期间中央 toast 常驻并刷新缓冲百分比, 离开缓冲态即收起。
 */
final class PlayerBufferToastPolicy {

    /** UI 侧动作: START=起显并开刷新; KEEP=已在显, 继续刷新; STOP=收起; IGNORE=无事可做. */
    enum Action { START, KEEP, STOP, IGNORE }

    private PlayerBufferToastPolicy() {}

    /**
     * @param playbackState 本次播放状态回调的 state
     * @param toastShowing  缓冲进度 toast 当前是否在显示
     */
    static Action onState(int playbackState, boolean toastShowing) {
        if (playbackState == Player.STATE_BUFFERING) {
            return toastShowing ? Action.KEEP : Action.START;
        }
        return toastShowing ? Action.STOP : Action.IGNORE;
    }
}
