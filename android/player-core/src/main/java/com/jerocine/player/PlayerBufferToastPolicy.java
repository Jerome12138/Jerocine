package com.jerocine.player;

import androidx.media3.common.Player;

/**
 * 缓冲进度提示策略(纯逻辑, 2026-10-11 用户要求; 2026-10-12 加过滤窗口抑制):
 * STATE_BUFFERING 期间中央 toast 常驻并刷新缓冲百分比, 离开缓冲态即收起。
 *
 * <p><b>例外 —— 起播/切集的「广告过滤窗口」</b>: 此时播放器卡在"等过滤后的清单"上,
 * 媒体分片一个都还没开始加载, {@code getBufferedPercentage()} 恒为 0。该窗口里显示
 * "缓冲中 0%"有两个坏处:
 * <ol>
 *   <li>它是噪音 —— 0% 不会动, 用户以为卡死了(实际是过滤在跑);</li>
 *   <li>它与 {@code filter_loading_text} 的「正在准备视频 · 广告过滤中」语义打架 ——
 *       同一件事(等过滤)被两个提示各说各话。</li>
 * </ol>
 * 因此由调用方判定该窗口并把 {@code filtering=true} 传进来, 本类让位:
 * 不启动(IGNORE), 已显示则收起(STOP)。
 */
final class PlayerBufferToastPolicy {

    /** UI 侧动作: START=起显并开刷新; KEEP=已在显, 继续刷新; STOP=收起; IGNORE=无事可做. */
    enum Action { START, KEEP, STOP, IGNORE }

    private PlayerBufferToastPolicy() {}

    /**
     * @param playbackState 本次播放状态回调的 state
     * @param toastShowing  缓冲进度 toast 当前是否在显示
     * @param filtering     是否处于起播/切集的广告过滤窗口(见类注释; 真缓冲才开始算)
     */
    static Action onState(int playbackState, boolean toastShowing, boolean filtering) {
        if (playbackState == Player.STATE_BUFFERING && !filtering) {
            return toastShowing ? Action.KEEP : Action.START;
        }
        return toastShowing ? Action.STOP : Action.IGNORE;
    }
}
