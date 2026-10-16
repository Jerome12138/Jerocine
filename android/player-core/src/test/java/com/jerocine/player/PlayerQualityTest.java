package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;

import org.junit.Test;

/**
 * 画质命名纯逻辑测试.
 */
public class PlayerQualityTest {

    @Test
    public void labelsByActualSize() {
        assertEquals("4K", PlayerQuality.resolutionLabel(3840, 2160));
        assertEquals("2K", PlayerQuality.resolutionLabel(2560, 1440));
        assertEquals("1080P", PlayerQuality.resolutionLabel(1920, 1080));
        assertEquals("720P", PlayerQuality.resolutionLabel(1280, 720));
        assertEquals("480P", PlayerQuality.resolutionLabel(854, 480));
        assertEquals("360P", PlayerQuality.resolutionLabel(640, 360));
    }

    @Test
    public void heightAloneIsEnoughForHighLabels() {
        assertEquals("1080P", PlayerQuality.resolutionLabel(100, 1080));
        assertEquals("4K", PlayerQuality.resolutionLabel(100, 2160));
    }

    @Test
    public void invalidSizeYieldsNull() {
        assertNull(PlayerQuality.resolutionLabel(0, 0));
        assertNull(PlayerQuality.resolutionLabel(-1, 720));
        assertNull(PlayerQuality.resolutionLabel(1280, 0));
    }

    @Test
    public void fallsBackToHeightPForLowRes() {
        String label = PlayerQuality.resolutionLabel(320, 240);
        assertNotNull(label);
        assertEquals("240P", label);
    }
}
