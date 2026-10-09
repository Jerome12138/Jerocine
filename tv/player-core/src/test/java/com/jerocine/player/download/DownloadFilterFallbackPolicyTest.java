package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;

import com.jerocine.player.M3u8FilterClient.FailureCause;
import com.jerocine.player.download.DownloadFilterFallbackPolicy.Action;

import org.junit.Test;

/**
 * 下载侧广告过滤降级策略 — 用户拍板(2026-10-10, 二次确认):
 * 数据获取类重试 3 轮后与其他原因一样原始流兜底; 仅取消中止; 未知原因防御性 FAIL。
 */
public class DownloadFilterFallbackPolicyTest {

    @Test
    public void network_retriesUpToThreeRounds_thenRaw() {
        assertEquals(Action.RETRY, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 1));
        assertEquals(Action.RETRY, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 2));
        // 第 3 轮失败 = 重试耗尽 → 不再 RETRY, 与其他原因一样原始流兜底(用户拍板, 不再 FAIL)
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 3));
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.NETWORK, 99));
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
    public void badResponse_retriesUpToThreeRounds_thenRaw() {
        // 读失败(连接中断/流异常)是瞬时故障: 2026-10-10 三轮起并入重试组
        // (360 片源实测: 一次瞬时 BAD_RESPONSE 就原始流兜底, 用户被迫"删了重下")
        assertEquals(Action.RETRY, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.BAD_RESPONSE, 1));
        assertEquals(Action.RETRY, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.BAD_RESPONSE, 2));
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.BAD_RESPONSE, 3));
    }

    @Test
    public void tooLarge_useRaw() {
        // 响应超 4MB 上限是确定性失败, 重试同样会超 → 直接原始流
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.TOO_LARGE, 1));
        assertEquals(Action.USE_RAW, DownloadFilterFallbackPolicy.onFilterFailure(FailureCause.TOO_LARGE, 5));
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
