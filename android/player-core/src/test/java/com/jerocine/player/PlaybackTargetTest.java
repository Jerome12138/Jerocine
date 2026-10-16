package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

/**
 * 切源 / 选掉落点的钳位规则测试.
 */
public class PlaybackTargetTest {

    @Test
    public void sourceSwitchClampsEpisodeAndPreservesPosition() {
        PlaybackTarget t = PlaybackTarget.switchSource(1, 5, 45_000L, 3);
        assertEquals(1, t.getSourceIndex());
        assertEquals(2, t.getEpisodeIndex());
        assertEquals(45_000L, t.getPositionMs());
    }

    @Test
    public void sourceSwitchKeepsEpisodeWhenTargetHasEnoughEpisodes() {
        PlaybackTarget t = PlaybackTarget.switchSource(0, 2, 10_000L, 20);
        assertEquals(0, t.getSourceIndex());
        assertEquals(2, t.getEpisodeIndex());
        assertEquals(10_000L, t.getPositionMs());
    }

    @Test
    public void sourceSwitchWithEmptyTargetSourceClampsToZeroAndFloorsPosition() {
        PlaybackTarget t = PlaybackTarget.switchSource(3, 7, -100L, 0);
        assertEquals(3, t.getSourceIndex());
        assertEquals(0, t.getEpisodeIndex());
        assertEquals(0L, t.getPositionMs());
    }

    @Test
    public void episodeSelectionStartsAtZero() {
        PlaybackTarget t = PlaybackTarget.selectEpisode(2, 4, 10);
        assertEquals(2, t.getSourceIndex());
        assertEquals(4, t.getEpisodeIndex());
        assertEquals(0L, t.getPositionMs());
    }

    @Test
    public void episodeSelectionClampsOutOfRangeIndex() {
        assertEquals(9, PlaybackTarget.selectEpisode(0, 99, 10).getEpisodeIndex());
        assertEquals(0, PlaybackTarget.selectEpisode(0, -3, 10).getEpisodeIndex());
        assertEquals(0, PlaybackTarget.selectEpisode(0, 5, 0).getEpisodeIndex());
    }

    @Test
    public void equalsAndHashCodeCompareValues() {
        assertEquals(PlaybackTarget.switchSource(1, 2, 3_000L, 10),
                PlaybackTarget.switchSource(1, 2, 3_000L, 10));
        assertEquals(PlaybackTarget.switchSource(1, 2, 3_000L, 10).hashCode(),
                PlaybackTarget.switchSource(1, 2, 3_000L, 10).hashCode());
    }
}
