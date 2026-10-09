package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * {@link DownloadQueueOrder} 单测 — 队列按集序激活(2026-10-10 用户要求):
 * 全部入队显示"排队中", 按原集序、容量门控逐个激活。
 */
public class DownloadQueueOrderTest {

    private static DownloadTask task(String id, int state, int episode, long createdAt) {
        DownloadTask t = new DownloadTask();
        t.id = id;
        t.state = state;
        t.episode = episode;
        t.createdAt = createdAt;
        return t;
    }

    @Test
    public void picksQueuedInEpisodeOrder() {
        // 故意乱序入列: E3 先过滤完, E0 最后 —— 激活仍必须按集序
        List<DownloadTask> all = new ArrayList<>(Arrays.asList(
                task("f:s:3", DownloadTask.STATE_QUEUED, 3, 900),
                task("f:s:0", DownloadTask.STATE_QUEUED, 0, 999),
                task("f:s:1", DownloadTask.STATE_QUEUED, 1, 100)));
        List<DownloadTask> picks = DownloadQueueOrder.pickActivatable(all, new HashSet<>(), 10);
        assertEquals(3, picks.size());
        assertEquals("f:s:0", picks.get(0).id);
        assertEquals("f:s:1", picks.get(1).id);
        assertEquals("f:s:3", picks.get(2).id);
    }

    @Test
    public void sameEpisodeBreaksTieByCreatedAt() {
        List<DownloadTask> all = new ArrayList<>(Arrays.asList(
                task("b:2", DownloadTask.STATE_QUEUED, 0, 200),
                task("a:1", DownloadTask.STATE_QUEUED, 0, 100)));
        List<DownloadTask> picks = DownloadQueueOrder.pickActivatable(all, new HashSet<>(), 10);
        assertEquals("a:1", picks.get(0).id);
        assertEquals("b:2", picks.get(1).id);
    }

    @Test
    public void skipsTrackedAndNonQueued() {
        List<DownloadTask> all = new ArrayList<>(Arrays.asList(
                task("t1", DownloadTask.STATE_DOWNLOADING, 0, 1),
                task("t2", DownloadTask.STATE_FILTERING, 1, 2),
                task("t3", DownloadTask.STATE_QUEUED, 2, 3),
                task("t4", DownloadTask.STATE_QUEUED, 3, 4),
                task("t5", DownloadTask.STATE_PAUSED, 4, 5),
                task("t6", DownloadTask.STATE_COMPLETED, 5, 6)));
        // t4 已在 media3(tracked) → 只剩 t3 可选
        List<DownloadTask> picks = DownloadQueueOrder.pickActivatable(
                all, new HashSet<>(Collections.singletonList("t4")), 10);
        assertEquals(1, picks.size());
        assertEquals("t3", picks.get(0).id);
    }

    @Test
    public void respectsSlotCapacity() {
        List<DownloadTask> all = new ArrayList<>();
        for (int i = 0; i < 5; i++) {
            all.add(task("id" + i, DownloadTask.STATE_QUEUED, i, i));
        }
        List<DownloadTask> picks = DownloadQueueOrder.pickActivatable(all, new HashSet<>(), 2);
        assertEquals(2, picks.size());
        assertEquals("id0", picks.get(0).id);
        assertEquals("id1", picks.get(1).id);
    }

    @Test
    public void zeroOrNegativeSlotsYieldEmpty() {
        List<DownloadTask> all = new ArrayList<>(Collections.singletonList(
                task("t1", DownloadTask.STATE_QUEUED, 0, 1)));
        assertTrue(DownloadQueueOrder.pickActivatable(all, new HashSet<>(), 0).isEmpty());
        assertTrue(DownloadQueueOrder.pickActivatable(all, new HashSet<>(), -1).isEmpty());
    }

    @Test
    public void emptyInputsAreSafe() {
        assertTrue(DownloadQueueOrder.pickActivatable(new ArrayList<>(), new HashSet<>(), 3).isEmpty());
        assertTrue(DownloadQueueOrder.pickActivatable(new ArrayList<>(), null, 3).isEmpty());
        assertTrue(DownloadQueueOrder.pickActivatable(null, null, 3).isEmpty());
        assertEquals(0, DownloadQueueOrder.countOccupied(null));
        assertEquals(0, DownloadQueueOrder.countOccupied(new ArrayList<>()));
    }

    @Test
    public void countsDownloadingAndFilteringAsOccupied() {
        // 2026-10-10 三轮: 过滤是队列前置步骤, 出队先过滤 → 过滤中也占名额
        List<DownloadTask> all = new ArrayList<>(Arrays.asList(
                task("a", DownloadTask.STATE_DOWNLOADING, 0, 1),
                task("b", DownloadTask.STATE_FILTERING, 1, 2),
                task("c", DownloadTask.STATE_QUEUED, 2, 3),
                task("d", DownloadTask.STATE_PAUSED, 3, 4),
                task("e", DownloadTask.STATE_COMPLETED, 4, 5)));
        assertEquals(2, DownloadQueueOrder.countOccupied(all));
    }

    @Test
    public void tenEpisodesSortNumericallyNotLexicographically() {
        // 集序必须是数值序: media3 内部排队不受集序控制(同批按 id 字符串序 "E10"<"E2"),
        List<DownloadTask> all = new ArrayList<>();
        for (int i : new int[]{10, 2, 1, 11, 3}) {
            all.add(task("f:s:" + i, DownloadTask.STATE_QUEUED, i, i));
        }
        List<DownloadTask> picks = DownloadQueueOrder.pickActivatable(all, new HashSet<>(), 10);
        assertEquals(5, picks.size());
        int[] expected = {1, 2, 3, 10, 11}; // 数值序, 不是 "1,10,11,2,3" 的字符串序
        for (int i = 0; i < picks.size(); i++) {
            assertEquals(expected[i], picks.get(i).episode);
        }
    }
}
