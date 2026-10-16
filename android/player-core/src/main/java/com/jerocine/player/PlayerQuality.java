package com.jerocine.player;

/**
 * 画质命名 — 纯逻辑, 可单测.
 *
 * 按视频"实际宽高"给出展示画质, 与源站标称无关(用于一眼看出虚标源).
 */
public final class PlayerQuality {

    private PlayerQuality() {}

    /** 未拿到有效宽高时返回 null(调用方应隐藏角标). */
    public static String resolutionLabel(int width, int height) {
        if (width <= 0 || height <= 0) return null;
        if (width >= 3800 || height >= 2100) return "4K";
        if (width >= 2500 || height >= 1400) return "2K";
        if (width >= 1800 || height >= 950) return "1080P";
        if (width >= 1200 || height >= 680) return "720P";
        if (width >= 800 || height >= 460) return "480P";
        return height + "P";
    }
}
