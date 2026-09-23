package com.jerocine.player;

/**
 * 广告过滤角标状态 — 纯逻辑(文案 + 色调), 可单测.
 *
 * 文案与色调逐字对齐前端 web 播放器({@code web/src/views/public/PlayView.vue} 的
 * {@code .jc-player-adtag} 五态): 圆点绿=正常/有结果, 灰=未启用/无需过滤, 蓝=服务端处理中.
 *
 * 五态(与 web 同名同文案):
 *   off         过滤未开启        灰
 *   unsupported 该源无需过滤      灰
 *   proxy       服务端过滤中      蓝   (清单走服务端 /m3u8/proxy, 已过滤但数量未知)
 *   clean       未检出广告        绿
 *   filtered    已过滤 N 段广告   绿
 *
 * Android 特有态(web 无对应场景, 属"必须有差异"):
 *   ineffective 过滤未生效        灰   (壳注入的代理地址缺失, 端侧过滤跑不起来)
 *   failed      过滤失败          灰   (端侧 POST 过滤链路异常, 已回退原始源)
 */
public final class AdFilterStatus {

    /** 状态点色调 — 由壳层映射到 drawable/颜色, 纯逻辑层不依赖 Android 资源. */
    public enum Tone { GREEN, GRAY, BLUE }

    public static final String KIND_OFF = "off";
    public static final String KIND_UNSUPPORTED = "unsupported";
    public static final String KIND_PROXY = "proxy";
    public static final String KIND_CLEAN = "clean";
    public static final String KIND_FILTERED = "filtered";
    public static final String KIND_INEFFECTIVE = "ineffective";
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
     * @param proxyMissing   开关开着但没有代理地址(壳未注入)
     * @param hls            当前播的是 HLS 清单(非 HLS 源无从过滤)
     * @param viaServerProxy 当前清单是服务端已经过滤过的 /m3u8/proxy(端侧不再重复过滤)
     * @param filterFailed   端侧过滤请求失败(已回退原始源)
     * @param filterAttempted 端侧过滤逻辑跑过(用于区分"跑完没检出"和"根本没触发")
     * @param filteredCount  本集剔除的分段数
     */
    public static AdFilterStatus of(
            boolean adFilterOn,
            boolean proxyMissing,
            boolean hls,
            boolean viaServerProxy,
            boolean filterFailed,
            boolean filterAttempted,
            int filteredCount
    ) {
        if (!adFilterOn) {
            return new AdFilterStatus(KIND_OFF, Tone.GRAY, "过滤未开启", 0);
        }
        if (proxyMissing) {
            return new AdFilterStatus(KIND_INEFFECTIVE, Tone.GRAY, "过滤未生效", 0);
        }
        if (!hls) {
            return new AdFilterStatus(KIND_UNSUPPORTED, Tone.GRAY, "该源无需过滤", 0);
        }
        if (filterFailed) {
            return new AdFilterStatus(KIND_FAILED, Tone.GRAY, "过滤失败", 0);
        }
        if (viaServerProxy) {
            return new AdFilterStatus(KIND_PROXY, Tone.BLUE, "服务端过滤中", 0);
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
