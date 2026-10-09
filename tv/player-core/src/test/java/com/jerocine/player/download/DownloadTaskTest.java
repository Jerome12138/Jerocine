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

    // ============================ 过滤阶段状态(2026-10-10) ============================

    @Test
    public void filteringPhaseTransitions() {
        // 过滤结束: 成功/原始流兜底 → QUEUED; 用户暂停 → PAUSED; 失败 → FAILED
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_FILTERING, DownloadTask.STATE_QUEUED));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_FILTERING, DownloadTask.STATE_PAUSED));
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_FILTERING, DownloadTask.STATE_FAILED));
        // 不能跳过 QUEUED 直接下载/完成/导出
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_FILTERING, DownloadTask.STATE_DOWNLOADING));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_FILTERING, DownloadTask.STATE_COMPLETED));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_FILTERING, DownloadTask.STATE_EXPORTING));
        // 过滤是入队起点, 不接受从其它状态"回退"进过滤
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_QUEUED, DownloadTask.STATE_FILTERING));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_PAUSED, DownloadTask.STATE_FILTERING));
        assertFalse(DownloadTask.canTransition(DownloadTask.STATE_FAILED, DownloadTask.STATE_FILTERING));
        // 暂停的过滤任务恢复后照常走 PAUSED → QUEUED → DOWNLOADING
        assertTrue(DownloadTask.canTransition(DownloadTask.STATE_PAUSED, DownloadTask.STATE_QUEUED));
    }

    @Test
    public void rawBadgeAppendsOnlyWhenFallback() {
        assertEquals("排队中", DownloadTask.withRawBadge(false, "排队中"));
        assertEquals("排队中 · " + DownloadTask.RAW_FALLBACK_BADGE,
                DownloadTask.withRawBadge(true, "排队中"));
        assertEquals(DownloadTask.RAW_FALLBACK_BADGE, DownloadTask.withRawBadge(true, ""));
        assertNull(DownloadTask.withRawBadge(true, null));
    }

    // ============================ 片源标识 / 分片进度(2026-10-10) ============================

    @Test
    public void sourceTagPrefersNameFallsBackToKey() {
        assertEquals("量子", DownloadTask.sourceTag("量子", "src_lz:lzm3u8"));
        // sourceName 空 → 退 sourceKey
        assertEquals("src_lz:lzm3u8", DownloadTask.sourceTag("", "src_lz:lzm3u8"));
        assertEquals("src_lz:lzm3u8", DownloadTask.sourceTag("   ", "src_lz:lzm3u8"));
        assertEquals("量子", DownloadTask.sourceTag(" 量子 ", null));
        // 都空 → 空串(调用方不追加 " · src" 尾巴)
        assertEquals("", DownloadTask.sourceTag(null, null));
        assertEquals("", DownloadTask.sourceTag(null, ""));
    }

    @Test
    public void segmentProgressFraction() {
        DownloadEngine.SegmentProgress sp =
                new DownloadEngine.SegmentProgress(213, 96, 123456789L);
        assertEquals(213, sp.totalSegments);
        assertEquals(96, sp.cachedSegments);
        assertEquals(123456789L, sp.cachedBytes);
        assertEquals(96f / 213f, sp.fraction(), 1e-6f);
        // 无分片表(总片数 0) → -1, 调用方退回字节逻辑
        assertEquals(-1f, new DownloadEngine.SegmentProgress(0, 0, 0).fraction(), 1e-6f);
    }

    // ============================ 导出文件名 ============================

    @Test
    public void exportFileNameCleansIllegalChars() {
        assertEquals("心动的信号第九季_E80.ts",
                DownloadTask.exportFileName("心动的信号第九季", 79, null));
        assertEquals("a_b_c___E1.ts", DownloadTask.exportFileName("a/b\\c:?", 0, ""));
    }

    @Test
    public void exportFileNameFallsBackToEpisodeWhenTitleBlank() {
        assertEquals("E5.ts", DownloadTask.exportFileName("   ", 4, null));
        assertEquals("E5.ts", DownloadTask.exportFileName(null, 4, ""));
    }

    @Test
    public void exportFileNameCleansParentheses() {
        // 括号必须清洗: exportedTargetExists 会把 "(" 之后当旧版提示文案剥掉,
        // 保留括号会让含括号的片名在 API<29 上每次导出都重复生成大文件。
        assertEquals("Rick and Morty _2020__E1.ts",
                DownloadTask.exportFileName("Rick and Morty (2020)", 0, null));
        assertEquals("a_b__E1.ts", DownloadTask.exportFileName("a(b)", 0, null));
        assertEquals("a_b_c_E1.ts", DownloadTask.exportFileName("a[b]c", 0, null));
    }

    @Test
    public void exportFileNameAppendsEpisodeTitle() {
        // 2026-10-08: 文件名加该集名称; 集名同样要清洗非法字符(含括号)
        assertEquals("心动的信号第九季_E1_第1集 初遇.ts",
                DownloadTask.exportFileName("心动的信号第九季", 0, "第1集 初遇"));
        assertEquals("心动的信号第九季_E1.ts",
                DownloadTask.exportFileName("心动的信号第九季", 0, "   "));
        assertEquals("心动的信号第九季_E1.ts",
                DownloadTask.exportFileName("心动的信号第九季", 0, null));
        assertEquals("E1_上集_清洗_.ts",
                DownloadTask.exportFileName(null, 0, "上集(清洗)"));
        // 源返回的集标题常自带"片名 · "前缀: 剥掉避免文件名片名重复
        assertEquals("心动的信号第九季_E1_20260731先导片上.ts",
                DownloadTask.exportFileName("心动的信号第九季", 0, "心动的信号第九季 · 20260731先导片上"));
        // 集名不以片名开头时原样保留
        assertEquals("心动的信号第九季_E2_加更上.ts",
                DownloadTask.exportFileName("心动的信号第九季", 1, "加更上"));
    }

    // ============================ 路径片段净化(防路径穿越) ============================

    // PlayerActivity 已 exported=true(为接 ACTION_VIEW), 任意 App 可注入 film_id。
    // 未净化时 "../../databases/x" 拼进路径 + remove() 的递归删除 = 删掉私有目录树任意路径。
    @Test
    public void safeSegmentStripsPathSeparators() {
        assertEquals(".._.._databases_x", DownloadTask.safeSegment("../../databases/x"));
        assertEquals(".._.._databases_x", DownloadTask.safeSegment("..\\..\\databases\\x"));
        assertFalse("净化结果绝不能含路径分隔符",
                DownloadTask.safeSegment("../x").contains("/"));
        assertFalse(DownloadTask.safeSegment("..\\x").contains("\\"));
    }

    @Test
    public void safeSegmentKeepsNormalIds() {
        // 正常形态不该被破坏: 影片 id 常含数字/点/横线
        assertEquals("3.10", DownloadTask.safeSegment("3.10"));
        assertEquals("film-2", DownloadTask.safeSegment("film-2"));
        assertEquals("abc123", DownloadTask.safeSegment("abc123"));
    }

    @Test
    public void safeSegmentRejectsPureDotNames() {
        // 纯点/点点的变体: 清洗后仍可能是 "." 或 ".." 或 "...", 仍具上跳语义
        assertEquals("_", DownloadTask.safeSegment("."));
        assertEquals("_", DownloadTask.safeSegment(".."));
        assertEquals("_", DownloadTask.safeSegment("..."));
        // 空白: 非空所以不是 null 分支, 两个空格各自替换成一个下划线
        assertEquals("__", DownloadTask.safeSegment("  "));
        assertEquals("_", DownloadTask.safeSegment(""));
        assertEquals("_", DownloadTask.safeSegment(null));
    }

    @Test
    public void safeSegmentHandlesNullAndEmpty() {
        assertEquals("_", DownloadTask.safeSegment(null));
        assertEquals("_", DownloadTask.safeSegment(""));
    }

    @Test
    public void idForKeepsRawFilmIdForPersistenceCompat() {
        // id 是持久化主键, 不能被 safeSegment 改动 —— 否则旧库里的行再也查不到
        assertEquals("../../x:src:3", DownloadTask.idFor("../../x", "src", 3));
        assertEquals(":src:0", DownloadTask.idFor(null, "src", 0));
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
