package com.jerocine.player;

/**
 * 切源 / 选集的落点 — 纯逻辑, 可单测.
 *
 * 统一"切到哪一源、哪一集、从第几毫秒起播"的钳位规则, 避免各处各写一遍 coerceIn.
 * 切集从 0 起播; 切源保留当前进度(仅在目标源集数不足时钳到最后一集).
 */
public final class PlaybackTarget {

    public final int sourceIndex;
    public final int episodeIndex;
    public final long positionMs;

    private PlaybackTarget(int sourceIndex, int episodeIndex, long positionMs) {
        this.sourceIndex = sourceIndex;
        this.episodeIndex = episodeIndex;
        this.positionMs = positionMs;
    }

    public int getSourceIndex() { return sourceIndex; }

    public int getEpisodeIndex() { return episodeIndex; }

    public long getPositionMs() { return positionMs; }

    /** 切换片源: 保留当前集号与播放进度, 目标源集数不足时钳到最后一集. */
    public static PlaybackTarget switchSource(
            int targetSourceIndex,
            int currentEpisode,
            long currentPositionMs,
            int targetEpisodeCount
    ) {
        return new PlaybackTarget(
                targetSourceIndex,
                clamp(currentEpisode, targetEpisodeCount),
                Math.max(0L, currentPositionMs)
        );
    }

    /** 选集: 从该集开头起播. */
    public static PlaybackTarget selectEpisode(int sourceIndex, int episodeIndex, int episodeCount) {
        return new PlaybackTarget(sourceIndex, clamp(episodeIndex, episodeCount), 0L);
    }

    private static int clamp(int episodeIndex, int episodeCount) {
        int max = Math.max(0, episodeCount - 1);
        return Math.max(0, Math.min(episodeIndex, max));
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof PlaybackTarget)) return false;
        PlaybackTarget other = (PlaybackTarget) o;
        return sourceIndex == other.sourceIndex
                && episodeIndex == other.episodeIndex
                && positionMs == other.positionMs;
    }

    @Override
    public int hashCode() {
        int result = sourceIndex;
        result = 31 * result + episodeIndex;
        result = 31 * result + (int) (positionMs ^ (positionMs >>> 32));
        return result;
    }

    @Override
    public String toString() {
        return "PlaybackTarget{sourceIndex=" + sourceIndex
                + ", episodeIndex=" + episodeIndex
                + ", positionMs=" + positionMs + '}';
    }
}
