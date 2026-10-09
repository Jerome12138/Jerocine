package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * 中转线路(relay)相关纯逻辑测试 — 覆盖本次"中转配置开关"新增的选路/校验语义.
 *
 * 不变量:
 *   ① isRelay: forceRawIdx(强制原始) > relayOn(用户开关) > forceRelayIdx(单集自愈);
 *   ② canSwitchNetworkMode: 广告过滤开 + 源可代理 + 代理地址非空 + idx 有效 + m3u8 形态
 *      + 非已代理地址, 六项全满足才允许手动切中转;
 *   ③ mediaUriFor: relayOn=true 出 proxyMedia=1 全量中转; forceRaw 兜底优先; sourcePreferProxy
 *      粘性偏好出 proxyMedia=0 服务端代理; 服务端抓不到该源时任何代理包装都回退原始.
 */
public class PlayerSessionRelayTest {

    private static final String RAW = "https://cdn.example.com/video/01.m3u8";
    private static final String BASE = "https://example.com/api";

    /** 占位 Host + 记录 renderNetworkMode 调用(供 updateNetworkModeUi 转发验证). */
    private static final class RecordingHost implements PlayerSession.Host {
        boolean lastRenderNetworkMode = false;
        boolean renderNetworkModeCalled = false;

        @Override public android.content.Context context() { return null; }
        @Override public void showCenterToast(String msg, long durationMs) { }
        @Override public void showCenterIcon(int drawableRes, long durationMs) { }
        @Override public void showCenterIconPersistent(int drawableRes) { }
        @Override public void hideGestureToast() { }
        @Override public void finishPlayer() { }
        @Override public void updateTitleForCurrent() { }
        @Override public void emitEvent(String name, org.json.JSONObject payload) { }
        @Override public String filmId() { return ""; }
        @Override public void toggleAdFilter() { }
        @Override public void renderAdFilterSwitch(boolean on) { }
        @Override public void toggleNetworkMode() { }
        @Override public void toggleLocalOnline() { }
        @Override public void renderNetworkMode(boolean relay) {
            renderNetworkModeCalled = true;
            lastRenderNetworkMode = relay;
        }
        @Override public androidx.media3.ui.PlayerView playerView() { return null; }
        @Override public void renderAdFilterBadge(AdFilterStatus status) { }
        @Override public void renderSpeedText(String label) { }
        @Override public void renderSpeedDot(boolean on) { }
        @Override public void renderSkipDot(boolean on) { }
        @Override public boolean dispatchToSuper(android.view.KeyEvent event) { return false; }
    }

    private static PlayerSession newSession(RecordingHost host) {
        PlayerSession s = new PlayerSession(host);
        s.setRawUrls(java.util.Collections.singletonList(RAW));
        s.proxyBase = BASE;
        return s;
    }

    // ==================== isRelay: forceRawIdx > relayOn > forceRelayIdx ====================

    @Test
    public void relayOffByDefault() {
        PlayerSession s = newSession(new RecordingHost());
        assertFalse(s.isRelay(0));
    }

    @Test
    public void relayOnTurnsRelayForAllItems() {
        PlayerSession s = newSession(new RecordingHost());
        s.relayOn = true;
        assertTrue(s.isRelay(0));
    }

    @Test
    public void singleEpisodeSelfHealRelay() {
        PlayerSession s = newSession(new RecordingHost());
        s.setRawUrls(java.util.Arrays.asList(RAW, "https://cdn.example.com/video/02.m3u8"));
        s.forceRelayIdx.add(0);
        assertTrue(s.isRelay(0));
        // 只影响自愈标记的那一集, 其它集仍是直连
        assertFalse(s.isRelay(1));
    }

    @Test
    public void forceRawWinsOverRelayOnAndSelfHeal() {
        PlayerSession s = newSession(new RecordingHost());
        s.relayOn = true;
        s.forceRelayIdx.add(0);
        s.forceRawIdx.add(0);
        // 回退直连(代理失败兜底)优先于用户开关与单集自愈
        assertFalse(s.isRelay(0));
    }

    // ==================== canSwitchNetworkMode: 六项判据 ====================

    @Test
    public void switchAllowedWhenAllConditionsMet() {
        PlayerSession s = newSession(new RecordingHost());
        assertTrue(s.canSwitchNetworkMode(0));
    }

    @Test
    public void switchRejectedWhenAdFilterOff() {
        PlayerSession s = newSession(new RecordingHost());
        s.adFilterOn = false;
        assertFalse(s.canSwitchNetworkMode(0));
    }

    @Test
    public void switchRejectedWhenSourceNotProxyUsable() {
        PlayerSession s = newSession(new RecordingHost());
        s.sourceProxyUsable = false;
        assertFalse(s.canSwitchNetworkMode(0));
    }

    @Test
    public void switchRejectedWhenProxyBaseMissing() {
        PlayerSession s = newSession(new RecordingHost());
        s.proxyBase = null;
        assertFalse(s.canSwitchNetworkMode(0));
        s.proxyBase = "";
        assertFalse(s.canSwitchNetworkMode(0));
    }

    @Test
    public void switchRejectedWhenIndexOutOfBounds() {
        PlayerSession s = newSession(new RecordingHost());
        assertFalse(s.canSwitchNetworkMode(-1));
        assertFalse(s.canSwitchNetworkMode(1));
    }

    @Test
    public void switchRejectedWhenNotM3u8() {
        PlayerSession s = newSession(new RecordingHost());
        s.setRawUrls(java.util.Collections.singletonList("https://cdn.example.com/video/01.mp4"));
        assertFalse(s.canSwitchNetworkMode(0));
    }

    @Test
    public void switchRejectedWhenAlreadyProxiedManifest() {
        PlayerSession s = newSession(new RecordingHost());
        s.setRawUrls(java.util.Collections.singletonList(BASE + "/v1/m3u8/proxy?src=x&proxyMedia=0"));
        assertFalse(s.canSwitchNetworkMode(0));
    }

    @Test
    public void switchAllowedForM3u8WithQuery() {
        PlayerSession s = newSession(new RecordingHost());
        s.setRawUrls(java.util.Collections.singletonList("https://cdn.example.com/video/01.m3u8?token=abc"));
        assertTrue(s.canSwitchNetworkMode(0));
    }

    // ==================== mediaUriFor 联动 ====================

    @Test
    public void relayOnProducesFullProxyMediaUrl() {
        PlayerSession s = newSession(new RecordingHost());
        s.relayOn = true;
        String playable = s.mediaUriFor(0, RAW);
        assertTrue(playable.startsWith(BASE + "/v1/m3u8/proxy?src="));
        assertTrue(playable.endsWith("&filterAds=1&proxyMedia=1"));
    }

    @Test
    public void relayOffKeepsRawUrlForClientSideFilter() {
        PlayerSession s = newSession(new RecordingHost());
        assertEquals(RAW, s.mediaUriFor(0, RAW));
    }

    @Test
    public void forceRawBeatsRelayInMediaUri() {
        PlayerSession s = newSession(new RecordingHost());
        s.relayOn = true;
        s.forceRawIdx.add(0);
        // 回退直连后不再包代理(哪怕中转开关开着)
        assertEquals(RAW, s.mediaUriFor(0, RAW));
    }

    @Test
    public void sourcePreferProxyProducesServerProxyWithoutRelay() {
        PlayerSession s = newSession(new RecordingHost());
        s.sourcePreferProxy = true;
        String playable = s.mediaUriFor(0, RAW);
        // 端侧过滤坏过的粘性偏好 → 服务端清单代理, 但分片仍直连(proxyMedia=0)
        assertTrue(playable.startsWith(BASE + "/v1/m3u8/proxy?src="));
        assertTrue(playable.endsWith("&filterAds=1&proxyMedia=0"));
    }

    @Test
    public void forceProxyIdxAlsoProducesServerProxy() {
        PlayerSession s = newSession(new RecordingHost());
        s.forceProxyIdx.add(0);
        assertTrue(s.mediaUriFor(0, RAW).endsWith("&filterAds=1&proxyMedia=0"));
    }

    @Test
    public void unproxyableSourceStaysRawEvenWithRelayOn() {
        PlayerSession s = newSession(new RecordingHost());
        s.relayOn = true;
        s.sourceProxyUsable = false;
        // 服务端抓不到该源 → 中转包装无意义, 只能设备抓清单端侧过滤
        assertEquals(RAW, s.mediaUriFor(0, RAW));
    }

    @Test
    public void missingProxyBaseStaysRawEvenWithRelayOn() {
        PlayerSession s = newSession(new RecordingHost());
        s.relayOn = true;
        s.proxyBase = "";
        assertEquals(RAW, s.mediaUriFor(0, RAW));
    }

    // ==================== updateNetworkModeUi 转发 ====================

    @Test
    public void updateNetworkModeUiForwardsRelayStateToHost() {
        RecordingHost host = new RecordingHost();
        PlayerSession s = newSession(host);

        s.relayOn = true;
        s.updateNetworkModeUi();
        assertTrue(host.renderNetworkModeCalled);
        assertTrue(host.lastRenderNetworkMode);

        s.relayOn = false;
        s.updateNetworkModeUi();
        assertFalse(host.lastRenderNetworkMode);
    }
}
