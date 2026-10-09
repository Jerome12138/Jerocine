package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;

import com.jerocine.player.M3u8FilterClient.FailureCause;
import com.jerocine.player.download.DownloadFilterFallbackPolicy.Action;

import org.junit.Test;

/**
 * 下载侧广告过滤降级策略 — 用户拍板(2026-10-10):
 * 数据获取类重试 3 轮(耗尽 FAIL); 本地逻辑/服务端拒绝/响应异常直接原始流兜底; 取消中止。
 */
public class DownloadFilterFallbackPolicyTest {

    @Test
    public void network_retriesUpToThreeRounds_thenFail() {
        assertEquals(Action.RETRY, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 1));
        assertEquals(Action.RETRY, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 2));
        // 第 3 轮失败已是最后一轮 → 不再 RETRY
        assertEquals(Action.FAIL, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 3));
        assertEquals(Action.FAIL, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 99));
    }

    @Test
    public void noFilter_useRaw() {
        // proxyBase 未配置 / 参数不合法 — 过滤服务不可用, 原始流是唯一可得产物
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NO_FILTER, 1));
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NO_FILTER, 5));
    }

    @Test
    public void rejected_useRaw() {
        // 4xx 重试无意义(典型: 后端无 /v1/m3u8/filter 端点) → 原始流
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.REJECTED, 1));
    }

    @Test
    public void badResponse_useRaw() {
        // 响应超限/读失败 — 重试同样会超限 → 原始流
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.BAD_RESPONSE, 1));
    }

    @Test
    public void cancelled_abort() {
        // 取消: 既不重试也不降级原始流
        assertEquals(Action.ABORT, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.CANCELLED, 1));
    }

    @Test
    public void nullCause_failsSafe() {
        // 防御: 未知/缺失原因不降级原始流(广告防护优先)
        assertEquals(Action.FAIL, DownloadFilterFallbackPolicy.onFilterFailure(null, 1));
    }
}
