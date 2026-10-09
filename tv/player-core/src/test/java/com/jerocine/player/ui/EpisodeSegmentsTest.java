package com.jerocine.player.ui;

import org.junit.Test;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertThrows;

/** 选集分段纯逻辑边界: 30/31/60/61 集。 */
public class EpisodeSegmentsTest {

    @Test
    public void count_boundaries() {
        assertEquals(0, EpisodeSegments.count(0));
        assertEquals(0, EpisodeSegments.count(-5));
        assertEquals(1, EpisodeSegments.count(1));
        assertEquals(1, EpisodeSegments.count(30));
        assertEquals(2, EpisodeSegments.count(31));
        assertEquals(2, EpisodeSegments.count(60));
        assertEquals(3, EpisodeSegments.count(61));
    }

    @Test
    public void titles_labels_and_length() {
        assertEquals(0, EpisodeSegments.titles(0).length);
        String[] one = EpisodeSegments.titles(30);
        assertEquals(1, one.length);
        assertEquals("第 1-30 集", one[0]);

        String[] two = EpisodeSegments.titles(31);
        assertEquals(2, two.length);
        assertEquals("第 1-30 集", two[0]);
        assertEquals("第 31-31 集", two[1]);
    }

    @Test
    public void startEnd_clamped_by_total() {
        assertEquals(0, EpisodeSegments.startOf(0));
        assertEquals(30, EpisodeSegments.startOf(1));
        assertEquals(60, EpisodeSegments.startOf(2));

        assertEquals(30, EpisodeSegments.endOf(0, 61));
        assertEquals(60, EpisodeSegments.endOf(1, 61));
        assertEquals(61, EpisodeSegments.endOf(2, 61));
        // 末段未满 30 集时钳到 total
        assertEquals(45, EpisodeSegments.endOf(1, 45));
    }

    @Test
    public void checkedSegment_mapping() {
        assertEquals(0, EpisodeSegments.checkedSegment(0));
        assertEquals(0, EpisodeSegments.checkedSegment(29));
        assertEquals(1, EpisodeSegments.checkedSegment(30));
        assertEquals(2, EpisodeSegments.checkedSegment(60));
    }
}
