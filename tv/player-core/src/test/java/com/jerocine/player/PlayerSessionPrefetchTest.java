package com.jerocine.player;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;

import android.content.Context;
import android.view.KeyEvent;

import androidx.media3.ui.PlayerView;

import org.json.JSONObject;
import org.junit.Test;

import java.nio.charset.StandardCharsets;

/**
 * 端侧预取缓存的语义测试(纯逻辑, 不触发任何 Android 调用 —— Host 只做占位实现).
 *
 * 三条不变量:
 *   ① 命中: 写入后能按原始清单 URL 取回同一份字节与剔除段数;
 *   ② 代际: 换片/换源(invalidatePrefetch)后, 在途任务拿旧代际写回的结果必须被丢弃;
 *   ③ 容量: 超过上限按时间淘汰最旧, 最新写入的一定还在(不然预取刚做完就被自己挤掉)。
 */
public class PlayerSessionPrefetchTest {

    private static final byte[] BODY =
            "#EXTM3U\n#EXTINF:6.0,\nseg001.ts\n".getBytes(StandardCharsets.UTF_8);

    /** Host 占位实现: 本测试只碰缓存字段, 不会调到任何一个方法. */
    static PlayerSession newSession() {
        return new PlayerSession(new PlayerSession.Host() {
            @Override public Context context() { return null; }
            @Override public void showCenterToast(String msg, long durationMs) { }
            @Override public void showCenterIcon(int drawableRes, long durationMs) { }
            @Override public void showCenterIconPersistent(int drawableRes) { }
            @Override public void hideGestureToast() { }
            @Override public void finishPlayer() { }
            @Override public void updateTitleForCurrent() { }
            @Override public void emitEvent(String name, JSONObject payload) { }
            @Override public String filmId() { return ""; }
            @Override public void toggleAdFilter() { }
            @Override public void renderAdFilterSwitch(boolean on) { }
            @Override public void toggleNetworkMode() { }
            @Override public void toggleLocalOnline() { }
            @Override public void renderNetworkMode(boolean relay) { }
            @Override public PlayerView playerView() { return null; }
            @Override public void renderAdFilterBadge(AdFilterStatus status) { }
            @Override public void renderSpeedText(String label) { }
            @Override public void renderSpeedDot(boolean on) { }
            @Override public void renderSkipDot(boolean on) { }
            @Override public boolean dispatchToSuper(KeyEvent event) { return false; }
        });
    }

    @Test
    public void hitReturnsSameBytesAndFilteredCount() {
        PlayerSession s = newSession();
        s.putPrefetched("https://cdn.example.com/index.m3u8", BODY, 7, s.prefetchGeneration());

        PlayerSession.Prefetched p = s.takePrefetched("https://cdn.example.com/index.m3u8");

        assertNotNull(p);
        assertArrayEquals(BODY, p.data);
        assertEquals(7, p.filteredCount);
        // 未命中(别的 URL) 必须是 null, 否则会把 A 集的清单喂给 B 集
        assertNull(s.takePrefetched("https://cdn.example.com/other.m3u8"));
        assertNull(s.takePrefetched(null));
    }

    @Test
    public void staleGenerationResultIsDiscarded() {
        PlayerSession s = newSession();
        int genBeforeSwitch = s.prefetchGeneration();

        // 模拟: 预取在途时用户换了片 → invalidate 抬代际; 随后旧任务才写回
        s.invalidatePrefetch();
        s.putPrefetched("https://cdn.example.com/old.m3u8", BODY, 3, genBeforeSwitch);

        assertNull(s.takePrefetched("https://cdn.example.com/old.m3u8"));
        // 新代际写入照常生效
        s.putPrefetched("https://cdn.example.com/new.m3u8", BODY, 1, s.prefetchGeneration());
        assertNotNull(s.takePrefetched("https://cdn.example.com/new.m3u8"));
    }

    @Test
    public void invalidateClearsEverything() {
        PlayerSession s = newSession();
        s.putPrefetched("https://cdn.example.com/a.m3u8", BODY, 1, s.prefetchGeneration());

        s.invalidatePrefetch();

        assertNull(s.takePrefetched("https://cdn.example.com/a.m3u8"));
    }

    @Test
    public void emptyPayloadIsNeverCached() {
        PlayerSession s = newSession();
        s.putPrefetched("https://cdn.example.com/a.m3u8", new byte[0], 1, s.prefetchGeneration());
        s.putPrefetched(null, BODY, 1, s.prefetchGeneration());

        assertNull(s.takePrefetched("https://cdn.example.com/a.m3u8"));
    }

    @Test
    public void cacheKeepsNewestEntryWhenOverCapacity() {
        PlayerSession s = newSession();
        int over = PlayerSession.PREFETCH_MAX_ENTRIES + 1;
        for (int i = 0; i < over; i++) {
            s.putPrefetched("https://cdn.example.com/" + i + ".m3u8", BODY, 1, s.prefetchGeneration());
        }

        // 最新一条一定还在(刚预取完就被自己挤掉 = 白做)
        assertNotNull(s.takePrefetched("https://cdn.example.com/" + (over - 1) + ".m3u8"));
        int alive = 0;
        for (int i = 0; i < over; i++) {
            if (s.takePrefetched("https://cdn.example.com/" + i + ".m3u8") != null) alive++;
        }
        assertEquals(PlayerSession.PREFETCH_MAX_ENTRIES, alive);
    }
}
