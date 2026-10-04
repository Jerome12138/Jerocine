package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.List;

/**
 * 下载任务模型 + 存储语义测试(纯 JVM, 不加载 Android).
 *
 * 覆盖: 幂等键 / 状态机全路径 / 导出文件名清洗 / 存储幂等插入与查询删除。
 */
public class DownloadTaskTest {

    // ============================ 幂等键 ============================

    @Test
    public void idUsesFilmSourceEpisode() {
        assertEquals("149293:src_lz:lzm3u8:79",
                DownloadTask.idFor("149293", "src_lz:lzm3u8", 79));
    }

    @Test
    public void idToleratesNullFilmAndSource() {
        assertEquals("::5", DownloadTask.idFor(null, null, 5));
    }

    // ============================ 状态机 ============================

    @Test
    public void happyPathQueuedDownloadingCompleted() {
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_QUEUED, DownloadTask.STATE_DOWNLOADING));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_DOWNLOADING, DownloadTask.STATE_COMPLETED));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_QUEUED, DownloadTask.STATE_COMPLETED)); // 不能跳级
    }

    @Test
    public void pauseAndResumeCycle() {
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_DOWNLOADING, DownloadTask.STATE_PAUSED));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_QUEUED, DownloadTask.STATE_PAUSED));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_PAUSED, DownloadTask.STATE_QUEUED));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_PAUSED, DownloadTask.STATE_COMPLETED));
    }

    @Test
    public void failureAndRetry() {
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_DOWNLOADING, DownloadTask.STATE_FAILED));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_QUEUED, DownloadTask.STATE_FAILED));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_FAILED, DownloadTask.STATE_QUEUED));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_FAILED, DownloadTask.STATE_DOWNLOADING)); // 重试必须回 QUEUED
    }

    @Test
    public void exportTransition() {
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_COMPLETED, DownloadTask.STATE_EXPORTING));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_EXPORTING, DownloadTask.STATE_COMPLETED));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_DOWNLOADING, DownloadTask.STATE_EXPORTING));
    }

    @Test
    public void invalidTransitionsRejected() {
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_COMPLETED, DownloadTask.STATE_DOWNLOADING));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_EXPORTING, DownloadTask.STATE_FAILED));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_COMPLETED, DownloadTask.STATE_PAUSED));
    }

    @Test
    public void instanceTransitionUsesCurrentState() {
        DownloadTask t = new DownloadTask();
        t.state = DownloadTask.STATE_QUEUED;
        assertTrue(t.canTransitionTo(DownloadTask.STATE_DOWNLOADING));
        assertFalse(t.canTransitionTo(DownloadTask.STATE_COMPLETED));
    }

    // ============================ 导出文件名 ============================

    @Test
    public void exportFileNameCleansIllegalChars() {
        assertEquals("心动的信号第九季_E80.ts",
                DownloadTask.exportFileName("心动的信号第九季", 79));
        assertEquals("a_b_c___E1.ts", DownloadTask.exportFileName("a/b\\c:?", 0));
    }

    @Test
    public void exportFileNameFallsBackToEpisodeWhenTitleBlank() {
        assertEquals("E5.ts", DownloadTask.exportFileName("   ", 4));
        assertEquals("E5.ts", DownloadTask.exportFileName(null, 4));
    }

    // ============================ 存储语义(内存实现) ============================

    /** 测试用内存存储 — 语义与 DownloadRepository 一致(幂等插入/更新/查询/删除). */
    static class InMemoryStore implements DownloadTaskStore {
        final java.util.Map<String, DownloadTask> map = new java.util.HashMap<>();

        @Override
        public boolean insertIgnore(DownloadTask t) {
            if (map.containsKey(t.id)) return false;
            map.put(t.id, t);
            return true;
        }

        @Override
        public boolean update(DownloadTask t) {
            if (!map.containsKey(t.id)) return false;
            map.put(t.id, t);
            return true;
        }

        @Override
        public DownloadTask get(String id) {
            return map.get(id);
        }

        @Override
        public List<DownloadTask> listByFilm(String filmId) {
            java.util.List<DownloadTask> out = new java.util.ArrayList<>();
            for (DownloadTask t : map.values()) {
                if (filmId.equals(t.filmId)) out.add(t);
            }
            out.sort((a, b) -> Integer.compare(a.episode, b.episode));
            return out;
        }

        @Override
        public List<DownloadTask> listAll() {
            return new java.util.ArrayList<>(map.values());
        }

        @Override
        public boolean delete(String id) {
            return map.remove(id) != null;
        }
    }

    private static DownloadTask task(String id, String filmId, int episode) {
        DownloadTask t = new DownloadTask();
        t.id = id;
        t.filmId = filmId;
        t.episode = episode;
        return t;
    }

    @Test
    public void insertIgnoreIsIdempotent() {
        InMemoryStore store = new InMemoryStore();
        DownloadTask a = task("149293:src_lz:lzm3u8:79", "149293", 79);
        assertTrue(store.insertIgnore(a));
        assertFalse(store.insertIgnore(a)); // 同 id 忽略
        assertEquals(1, store.listAll().size());
    }

    @Test
    public void updateOnlyExisting() {
        InMemoryStore store = new InMemoryStore();
        DownloadTask t = task("f:s:1", "f", 1);
        assertFalse(store.update(t)); // 不存在
        store.insertIgnore(t);
        t.state = DownloadTask.STATE_DOWNLOADING;
        assertTrue(store.update(t));
        assertEquals(DownloadTask.STATE_DOWNLOADING, store.get("f:s:1").state);
    }

    @Test
    public void listByFilmSortedByEpisode() {
        InMemoryStore store = new InMemoryStore();
        store.insertIgnore(task("f:s:3", "f", 3));
        store.insertIgnore(task("f:s:1", "f", 1));
        store.insertIgnore(task("g:s:9", "g", 9));
        List<DownloadTask> list = store.listByFilm("f");
        assertEquals(2, list.size());
        assertEquals(1, list.get(0).episode);
        assertEquals(3, list.get(1).episode);
    }

    @Test
    public void deleteRemovesTask() {
        InMemoryStore store = new InMemoryStore();
        store.insertIgnore(task("f:s:1", "f", 1));
        assertTrue(store.delete("f:s:1"));
        assertNull(store.get("f:s:1"));
        assertFalse(store.delete("f:s:1"));
    }
}
