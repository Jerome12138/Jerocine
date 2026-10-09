package com.jerocine.player.ui;

/**
 * 选集"30 集一档"分段计算(纯逻辑, JVM 可测)。
 * 从 PlayerDialogHelper 提出 — 与 Web 端选集分段保持一致。
 */
public final class EpisodeSegments {

    /** 每个选集分段的集数 — 与 Web 端选集分段一致 */
    public static final int PER_SEG = 30;

    private EpisodeSegments() {
    }

    /** 分段数: 30→1, 31→2, 61→3。total ≤ 0 视为无集, 返回 0。 */
    public static int count(int total) {
        if (total <= 0) return 0;
        return (total + PER_SEG - 1) / PER_SEG;
    }

    /** 各分段标题(展示用 1-based), 如 ["第 1-30 集", "第 31-60 集"]。total ≤ 0 返回空数组。 */
    public static String[] titles(int total) {
        int n = count(total);
        String[] segs = new String[n];
        for (int i = 0; i < n; i++) {
            segs[i] = "第 " + (startOf(i) + 1) + "-" + endOf(i, total) + " 集";
        }
        return segs;
    }

    /** 分段起始集下标(0-based): seg0→0, seg1→30。 */
    public static int startOf(int segIndex) {
        return segIndex * PER_SEG;
    }

    /** 分段结束集下标(开区间, 不含): 受 total 钳制, 末段取 total。 */
    public static int endOf(int segIndex, int total) {
        return Math.min((segIndex + 1) * PER_SEG, total);
    }

    /** 当前集(0-based)所在分段下标。 */
    public static int checkedSegment(int episodeIndex) {
        return episodeIndex / PER_SEG;
    }
}
