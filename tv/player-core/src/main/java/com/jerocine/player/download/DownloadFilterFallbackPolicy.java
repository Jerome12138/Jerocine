package com.jerocine.player.download;

import com.jerocine.player.M3u8FilterClient;

/**
 * 下载侧广告过滤降级策略 — 纯逻辑, JVM 可测(用户拍板 2026-10-10, 二次确认同日)。
 *
 * <p>此前过滤失败一律任务 FAIL(不静默下原始流, 防广告进产物); 按用户要求改为按失败原因分级,
 * <b>除取消外全部原始流兜底</b>:
 * <ul>
 *   <li>{@code NETWORK}(数据获取类: 网络异常/服务端 5xx) → 先重试最多 {@link #NETWORK_RETRY_ROUNDS}
 *       轮(每轮内 M3u8FilterClient 自带一次快速重试), 耗尽仍失败 → <b>原始流兜底</b>
 *       (用户拍板: 网络耗尽不再 FAIL);</li>
 *   <li>{@code NO_FILTER}(无过滤服务: proxyBase 未配置/参数不合法)、{@code REJECTED}(服务端 4xx,
 *       典型如后端无此端点)、{@code BAD_RESPONSE}(响应超限/读失败) → <b>直接下载原始流</b>
 *       (重试无意义或过滤本就不可用, 原始流是唯一可得产物);</li>
 *   <li>{@code CANCELLED}(任务被取消) → 中止, 既不重试也不降级。</li>
 * </ul>
 *
 * <p><b>取舍说明</b>: 原始流兜底的产物含潜在广告段, 离线播放不经过滤 —— 用户明确接受该取舍
 * ("本地逻辑失败/无广告/其他原因直接下载原始流")。原始流落盘前必须经
 * {@link HlsPlaylistAbsolutizer} 绝对化(服务端 FilterText 的绝对化不在场)。
 */
public final class DownloadFilterFallbackPolicy {

    /** NETWORK 失败的重试轮数上限(每轮内 M3u8FilterClient 自带 1 次快速重试)。 */
    public static final int NETWORK_RETRY_ROUNDS = 3;

    /** 单次失败的处置。 */
    public enum Action {
        /** 再试一轮(仅 NETWORK 且未达上限)。 */
        RETRY,
        /** 用原始清单继续(记录原因后按无过滤产物处理)。 */
        USE_RAW,
        /** 任务失败(仅防御: 未知/缺失原因时不降级)。 */
        FAIL,
        /** 中止(取消), 不降级不重试。 */
        ABORT
    }

    private DownloadFilterFallbackPolicy() {
    }

    /**
     * @param cause   本次过滤失败原因(来自 M3u8FilterClient.filterDetailed)
     * @param attempt 本次失败的轮次(1 = 第一次失败; NETWORK 达到 {@link #NETWORK_RETRY_ROUNDS} 后不再 RETRY)
     */
    public static Action onFilterFailure(M3u8FilterClient.FailureCause cause, int attempt) {
        if (cause == null) return Action.FAIL;
        switch (cause) {
            case CANCELLED:
                return Action.ABORT;
            case NETWORK:
                return attempt < NETWORK_RETRY_ROUNDS ? Action.RETRY : Action.USE_RAW;
            case NO_FILTER:
            case REJECTED:
            case BAD_RESPONSE:
                return Action.USE_RAW;
            default:
                return Action.FAIL;
        }
    }
}
