package com.jerocine.player;

/**
 * 广告过滤角标状态 — 纯逻辑(文案 + 色调), 可单测.
 *
 * 文案与色调逐字对齐前端 web 播放器({@code web/src/views/public/PlayView.vue} 的
 * {@code .jc-player-adtag} 角标): 圆点绿=正常/有结果, 灰=未启用/无需过滤/失败, 蓝=过滤中.
 *
 * 六态(与 web 同名同文案; 2026-10-28 proxy 态随服务端过滤下线移除):
 *   off         过滤未开启        灰
 *   unsupported 该源无需过滤      灰
 *   filtering   广告过滤中        蓝   (端侧抓清单+本地过滤在途, 结果未出; 2026-10-19 起,
 *                                     此前端侧过滤窗口误显"该源无需过滤")
 *   clean       未检出广告        绿
 *   filtered    已过滤 N 段广告   绿
 *   failed      过滤失败          灰   (端侧过滤链路异常, 已回退原始源;
 *                                     2026-10-19 起 web 同场景同文案, 不再是 Android 特有)
 */
public final class AdFilterStatus {

    /** 状态点色调 — 由壳层映射到 drawable/颜色, 纯逻辑层不依赖 Android 资源. */
    public enum Tone { GREEN, GRAY, BLUE }

    public static final String KIND_OFF = "off";
    public static final String KIND_UNSUPPORTED = "unsupported";
    public static final String KIND_FILTERING = "filtering";
    public static final String KIND_CLEAN = "clean";
    public static final String KIND_FILTERED = "filtered";
    public static final String KIND_FAILED = "failed";

    public final String kind;
    public final Tone tone;
    public final String text;
    /** 本集剔除的分段数(仅 filtered 态非 0, 供中央提示复用). */
    public final int count;

    private AdFilterStatus(String kind, Tone tone, String text, int count) {
        this.kind = kind;
        this.tone = tone;
        this.text = text;
        this.count = count;
    }

    /**
     * 按当前播放态求角标状态.
     *
     * @param adFilterOn     广告过滤总开关(关 → 常驻显示"过滤未开启", 与 web 一致)
     * @param hls            当前播的是 HLS 清单(非 HLS 源无从过滤)
     * @param filtering      端侧过滤在途(还没有任何结果; 起播/切集窗口应显"广告过滤中")
     * @param filterFailed   端侧过滤链路失败(已回退原始源)
     * @param filterAttempted 端侧过滤逻辑跑过(用于区分"跑完没检出"和"根本没触发")
     * @param filteredCount  本集剔除的分段数
     */
    public static AdFilterStatus of(
            boolean adFilterOn,
            boolean hls,
            boolean filtering,
            boolean filterFailed,
            boolean filterAttempted,
            int filteredCount
    ) {
        if (!adFilterOn) {
            return new AdFilterStatus(KIND_OFF, Tone.GRAY, "过滤未开启", 0);
        }
        if (!hls) {
            return new AdFilterStatus(KIND_UNSUPPORTED, Tone.GRAY, "该源无需过滤", 0);
        }
        // 端侧过滤在途: 起播/切集的第一帧就是真实状态(此前这段窗口误显"该源无需过滤")。
        // 只在"还没有任何结果"时成立 —— 防御调用方误传: 已有结果(clean/filtered/failed)必须压过它。
        if (filtering && !filterAttempted && !filterFailed && filteredCount == 0) {
            return new AdFilterStatus(KIND_FILTERING, Tone.BLUE, "广告过滤中", 0);
        }
        if (filterFailed) {
            return new AdFilterStatus(KIND_FAILED, Tone.GRAY, "过滤失败", 0);
        }
        if (filteredCount > 0) {
            return new AdFilterStatus(
                    KIND_FILTERED, Tone.GREEN, "已过滤 " + filteredCount + " 段广告", filteredCount);
        }
        if (filterAttempted) {
            return new AdFilterStatus(KIND_CLEAN, Tone.GREEN, "未检出广告", 0);
        }
        return new AdFilterStatus(KIND_UNSUPPORTED, Tone.GRAY, "该源无需过滤", 0);
    }
}
