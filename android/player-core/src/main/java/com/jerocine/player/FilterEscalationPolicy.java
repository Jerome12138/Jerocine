package com.jerocine.player;

/**
 * 端侧广告过滤失败升级策略 — 纯逻辑, JVM 可测(2026-10-10, 用户实测反馈定稿)。
 *
 * <p>背景: 端侧过滤({@code /v1/m3u8/filter})失败后 {@code PlayerAdFilterHelper.escalateToProxy}
 * 会把失败集升级为服务端代理, 并把整个片源粘性切到代理({@code sourcePreferProxy}) ——
 * 后续所有集跳过端侧过滤、预取加速也被关掉。原实现**一集**瞬时失败(如一次网络抖动)就触发粘性,
 * 弱代理源会出现"一集失败, 后面的集跟着失败"。
 *
 * <p>策略: 单集失败只升级该集({@code forceProxyIdx}); **连续 {@link #STICKY_THRESHOLD} 集**
 * 端侧失败才把整片源粘性切代理 —— 一次抖动由单集升级兜住, 连续失败才认定端侧链路真坏了。
 * 端侧过滤成功(播放解析或预取)即清零连击。
 */
public final class FilterEscalationPolicy {

    /** 连续端侧过滤失败多少集后, 才把整个片源粘性切到服务端代理。 */
    public static final int STICKY_THRESHOLD = 2;

    private FilterEscalationPolicy() {
    }

    /** 当前失败连击数是否足以把整片源粘性切代理。 */
    public static boolean shouldStickSource(int failStreak) {
        return failStreak >= STICKY_THRESHOLD;
    }
}
