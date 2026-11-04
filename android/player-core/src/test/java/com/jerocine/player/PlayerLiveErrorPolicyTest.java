package com.jerocine.player;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import androidx.media3.common.PlaybackException;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylistTracker;
import androidx.media3.exoplayer.source.BehindLiveWindowException;

import org.junit.Test;

/**
 * Q②/PB3(player-fix 2026-11-05) — 直播 behind-live/reset 类报错的兜底判定
 * 与防循环短窗限次闸门(纯逻辑, JVM 单测)。
 */
public class PlayerLiveErrorPolicyTest {

    // ============================ isBehindLiveError ============================

    @Test
    public void behindLiveErrorCode_matches() {
        assertTrue(PlayerLiveErrorPolicy.isBehindLiveError(
                PlaybackException.ERROR_CODE_BEHIND_LIVE_WINDOW, null));
    }

    @Test
    public void playlistResetInCauseChain_matches() {
        HlsPlaylistTracker.PlaylistResetException cause =
                new HlsPlaylistTracker.PlaylistResetException(null);
        RuntimeException wrapped = new RuntimeException("ExoPlaybackException-like", cause);
        assertTrue(PlayerLiveErrorPolicy.isBehindLiveError(
                PlaybackException.ERROR_CODE_IO_UNSPECIFIED, wrapped));
    }

    @Test
    public void behindLiveWindowException_matches() {
        assertTrue(PlayerLiveErrorPolicy.isBehindLiveError(
                PlaybackException.ERROR_CODE_IO_UNSPECIFIED,
                new RuntimeException("wrap", new BehindLiveWindowException())));
    }

    @Test
    public void deepCauseChain_stillMatched() {
        Throwable deep = new RuntimeException("a",
                new RuntimeException("b",
                        new RuntimeException("c",
                                new HlsPlaylistTracker.PlaylistResetException(null))));
        assertTrue(PlayerLiveErrorPolicy.isBehindLiveError(
                PlaybackException.ERROR_CODE_IO_UNSPECIFIED, deep));
    }

    @Test
    public void playlistStuck_excludedByDesign() {
        // 规格 Q② 只列 PlaylistReset/BehindLive: PlaylistStuck 是"快照冻结"类失败,
        // seek+prepare 治不了(Q① 免缓存后本不该再出现), 兜底反而掩盖真问题 —— 刻意不收。
        assertFalse(PlayerLiveErrorPolicy.isBehindLiveError(
                PlaybackException.ERROR_CODE_IO_UNSPECIFIED,
                new HlsPlaylistTracker.PlaylistStuckException(null)));
    }

    @Test
    public void ordinaryError_notMatched() {
        assertFalse(PlayerLiveErrorPolicy.isBehindLiveError(
                PlaybackException.ERROR_CODE_IO_NETWORK_CONNECTION_FAILED,
                new java.io.IOException("timeout")));
        assertFalse(PlayerLiveErrorPolicy.isBehindLiveError(
                PlaybackException.ERROR_CODE_IO_NETWORK_CONNECTION_FAILED, null));
    }

    // ============================ RecoveryGate ============================

    private static PlayerLiveErrorPolicy.RecoveryGate newGate() {
        return new PlayerLiveErrorPolicy.RecoveryGate(
                PlayerLiveErrorPolicy.MAX_RECOVERS_PER_WINDOW,
                PlayerLiveErrorPolicy.RECOVERY_WINDOW_MS);
    }

    @Test
    public void gate_firstAttemptAllowed() {
        assertTrue(newGate().tryAcquire(1_000L));
    }

    @Test
    public void gate_secondWithinWindow_blocked() {
        PlayerLiveErrorPolicy.RecoveryGate gate = newGate();
        assertTrue(gate.tryAcquire(1_000L));
        assertFalse(gate.tryAcquire(1_000L + PlayerLiveErrorPolicy.RECOVERY_WINDOW_MS - 1));
    }

    @Test
    public void gate_afterWindowReopens() {
        PlayerLiveErrorPolicy.RecoveryGate gate = newGate();
        assertTrue(gate.tryAcquire(1_000L));
        // 恰到窗口边界即视为新窗: 允许重新兜底(防死循环与保可用之间的界线)
        assertTrue(gate.tryAcquire(1_000L + PlayerLiveErrorPolicy.RECOVERY_WINDOW_MS));
    }

    @Test
    public void gate_maxAttempts2_allowsTwoThenBlocks() {
        PlayerLiveErrorPolicy.RecoveryGate gate =
                new PlayerLiveErrorPolicy.RecoveryGate(2, PlayerLiveErrorPolicy.RECOVERY_WINDOW_MS);
        assertTrue(gate.tryAcquire(0L));
        assertTrue(gate.tryAcquire(1_000L));
        assertFalse(gate.tryAcquire(2_000L));
    }
}
