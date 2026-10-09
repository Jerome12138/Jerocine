package com.jerocine.player.download;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;

import java.util.ArrayList;
import java.util.List;

/**
 * 下载任务 SQLite 存储 — {@link DownloadTaskStore} 的落地实现.
 *
 * <p>选型(见 harness docs/下载器详细设计-2026-10-02.md §2.0): 批量几百集 + 过滤后清单大字段 +
 * 崩溃恢复 + 下载线程/UI 双线程并发 → 框架自带 SQLiteOpenHelper 零新依赖, 单表够用, 不引 Room。
 *
 * <p>与 Media3 {@code DefaultDownloadIndex} 的分工: 本表只存**业务元数据**(片名/集名/过滤后清单/
 * 导出路径/错误), 下载引擎自身的 uri/字节进度由 Media3 持久化; 两者通过
 * {@code DownloadRequest.data = task.id} 关联。
 */
public class DownloadRepository extends SQLiteOpenHelper implements DownloadTaskStore {

    private static final String DB_NAME = "jerocine_downloads.db";
    /** v2(2026-10-10): 加 rawFallback 列 — 原始流兜底任务常驻标记(用户要求过滤结果可见)。 */
    private static final int DB_VERSION = 2;
    private static final String TABLE = "download_task";

    /**
     * <b>列表查询的列投影 — 刻意排除 {@code filteredPlaylist}。</b>
     *
     * <p>该字段是整份过滤后清单, 单集约 77KB(见 {@code M3u8FilterClient} 的量级说明),
     * 几百集就是 20MB+ String。而下载管理页 {@code refreshTasks()} 走 {@link #listAll()},
     * 被 {@code onResume} 和每次状态变化回调触发; media3 的进度通知间隔是 5000ms,
     * 集级并行 3 → 每 5 秒至少 3 次全表扫描 + 全表反序列化, 且都在主线程 —— 会 ANR/OOM。
     *
     * <p>清单只在真正需要时单取: 导出({@link #getFilteredPlaylist})与离线播放。
     */
    private static final String[] LIST_COLUMNS = {
            "id", "filmId", "filmTitle", "sourceKey", "sourceName", "episode",
            "episodeTitle", "srcUrl", "state", "progressBytes", "totalBytes", "error",
            "rawFallback", "cacheDir", "exportedPath", "createdAt", "updatedAt",
    };

    public DownloadRepository(Context context) {
        super(context.getApplicationContext(), DB_NAME, null, DB_VERSION);
    }

    @Override
    public void onCreate(SQLiteDatabase db) {
        db.execSQL("CREATE TABLE " + TABLE + " ("
                + "id TEXT PRIMARY KEY,"
                + "filmId TEXT NOT NULL,"
                + "filmTitle TEXT NOT NULL,"
                + "sourceKey TEXT NOT NULL,"
                + "sourceName TEXT NOT NULL DEFAULT '',"
                + "episode INTEGER NOT NULL,"
                + "episodeTitle TEXT NOT NULL DEFAULT '',"
                + "srcUrl TEXT NOT NULL,"
                + "filteredPlaylist TEXT NOT NULL DEFAULT '',"
                + "state INTEGER NOT NULL,"
                + "progressBytes INTEGER NOT NULL DEFAULT 0,"
                + "totalBytes INTEGER NOT NULL DEFAULT 0,"
                + "error TEXT NOT NULL DEFAULT '',"
                + "rawFallback INTEGER NOT NULL DEFAULT 0,"
                + "cacheDir TEXT NOT NULL DEFAULT '',"
                + "exportedPath TEXT NOT NULL DEFAULT '',"
                + "createdAt INTEGER NOT NULL,"
                + "updatedAt INTEGER NOT NULL)");
        db.execSQL("CREATE INDEX idx_download_film ON " + TABLE + "(filmId)");
    }

    @Override
    public void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion) {
        // 只加列不 drop(下载进度不能丢)
        if (oldVersion < 2) {
            db.execSQL("ALTER TABLE " + TABLE
                    + " ADD COLUMN rawFallback INTEGER NOT NULL DEFAULT 0");
        }
    }

    @Override
    public boolean insertIgnore(DownloadTask t) {
        SQLiteDatabase db = getWritableDatabase();
        ContentValues v = toValues(t);
        long row = db.insertWithOnConflict(TABLE, null, v, SQLiteDatabase.CONFLICT_IGNORE);
        return row != -1;
    }

    @Override
    public boolean update(DownloadTask t) {
        SQLiteDatabase db = getWritableDatabase();
        return db.update(TABLE, toValues(t), "id=?", new String[]{t.id}) > 0;
    }

    @Override
    public DownloadTask get(String id) {
        SQLiteDatabase db = getReadableDatabase();
        try (Cursor c = db.query(TABLE, null, "id=?", new String[]{id},
                null, null, null, "1")) {
            return c.moveToFirst() ? fromCursor(c) : null;
        }
    }

    /**
     * 单取某个任务的过滤后清单全文 — 仅在导出/离线播放时调用。
     *
     * <p>列表查询刻意不带这一列(见 {@link #LIST_COLUMNS}), 所以需要时按 id 单独取,
     * 避免把几十 MB 的清单常驻内存并反复在主线程反序列化。
     *
     * @return 清单文本; 任务不存在或字段为空时返回 null
     */
    public String getFilteredPlaylist(String id) {
        if (id == null || id.isEmpty()) return null;
        SQLiteDatabase db = getReadableDatabase();
        try (Cursor c = db.query(TABLE, new String[]{"filteredPlaylist"},
                "id=?", new String[]{id}, null, null, null, "1")) {
            if (!c.moveToFirst()) return null;
            String s = c.getString(0);
            return (s == null || s.isEmpty()) ? null : s;
        }
    }

    @Override
    public List<DownloadTask> listByFilm(String filmId) {
        return query(LIST_COLUMNS, "filmId=?", new String[]{filmId}, "episode ASC");
    }

    @Override
    public List<DownloadTask> listAll() {
        return query(LIST_COLUMNS, null, null, "createdAt ASC");
    }

    /**
     * 只更新进度/状态列 — 供 media3 高频进度回调使用(见 DownloadEngine.onDownloadChanged)。
     *
     * <p><b>为什么要单独开这个方法</b>: {@link #update} 走 {@link #toValues} 写<b>全部列</b>,
     * 包括 {@code filteredPlaylist} 与 {@code exportedPath}。而进度回调每 5 秒就来一次,
     * 且拿到的对象是"回调触发时刻"的快照 —— 若期间导出流程正在写 {@code EXPORTING}/
     * {@code exportedPath}, 全行覆盖会把它们<b>写回旧值</b>:
     * <ul>
     *   <li>导出完成后 UI 显示不出"已导出"(exportedPath 被清空);</li>
     *   <li>EXPORTING 被覆盖回 COMPLETED → DownloadTask 状态机的防并发被绕过,
     *       重复点"导出"会整集重拼(数百 MB 重写)。</li>
     * </ul>
     * 局部更新从根上避免这两种覆盖。
     *
     * @param state  要写入的状态; {@link #EXPORTING_SENTINEL} 表示"不改state"
     */
    public void updateProgressOnly(String id, int state, long progressBytes, long totalBytes,
                                   String error, long updatedAt) {
        if (id == null) return;
        ContentValues v = new ContentValues();
        if (state != EXPORTING_SENTINEL) v.put("state", state);
        v.put("progressBytes", progressBytes);
        v.put("totalBytes", totalBytes);
        v.put("updatedAt", updatedAt);
        if (error != null) v.put("error", error);
        // 条件更新: 正在导出/已导出的任务, 其 state 与 exportedPath 不受下载进度回写影响。
        // 否则下载回调会把 EXPORTING 覆盖回 COMPLETED, 击穿防并发。
        //
        // ⚠️ 2026-10-08 修 bug: 原实现 writingCompleted 时 WHERE 写成 "state=EXPORTING",
        // 导致普通下载 DOWNLOADING→COMPLETED 的落库**永远静默失败**(当前 state 是
        // DOWNLOADING 不等于 EXPORTING) → 任务永远显示"下载中 100%", 永不进已完成 tab
        // (pad 实测实锤)。COMPLETED 是终态且不会被 EXPORTING 回写之外的路径产生,
        // 这里必须无条件放行 —— 注释里说的"自愈出口"本来就是要求穿透 EXPORTING。
        int exporting = DownloadTask.STATE_EXPORTING;
        boolean writingCompleted = state == DownloadTask.STATE_COMPLETED;
        getWritableDatabase().update(TABLE, v,
                writingCompleted ? "id=?" : "id=? AND state<>?",
                writingCompleted ? new String[]{id}
                        : new String[]{id, String.valueOf(exporting)});
    }

    /**
 * 条件更新 state — 用于"用户意图写状态"的场景(如暂停), 只在当前状态符合预期时才改。
 *
 * <p>比 {@link #update} 全行覆盖安全: 暂停是 UI 动作, 可能与进度回调/导出并发。若对方
 * 已把状态推进到别的阶段(如开始导出), 这里的 {@code allowedFrom} 判据会让更新落空,
 * 而全行覆盖会把它倒退回去。
 *
 * @param allowedFrom 允许的当前状态集合; 任一命中才更新, 否则不动
 * @return 是否真的改了
 */
    public boolean updateStateIf(String id, int newState, int... allowedFrom) {
        if (id == null || allowedFrom == null || allowedFrom.length == 0) return false;
        StringBuilder where = new StringBuilder("id=? AND state IN (");
        String[] args = new String[allowedFrom.length + 1];
        args[0] = id;
        for (int i = 0; i < allowedFrom.length; i++) {
            if (i > 0) where.append(',');
            where.append('?');
            args[i + 1] = String.valueOf(allowedFrom[i]);
        }
        where.append(')');
        ContentValues v = new ContentValues();
        v.put("state", newState);
        v.put("updatedAt", System.currentTimeMillis());
        return getWritableDatabase().update(TABLE, v, where.toString(), args) > 0;
    }

    /**
     * 进度轮询专用: 仅 DOWNLOADING 态才写字节进度。
     *
     * <p>条件 WHERE 是必须的: 轮询线程读过任务后、写库前用户可能按了暂停(业务表已 PAUSED),
     * 无条件写会把状态冲回 DOWNLOADING(暂停"看起来失效")。
     */
    public void updateProgressIfDownloading(String id, long progressBytes, long totalBytes,
                                            long updatedAt) {
        if (id == null) return;
        ContentValues v = new ContentValues();
        v.put("progressBytes", progressBytes);
        v.put("totalBytes", totalBytes);
        v.put("updatedAt", updatedAt);
        getWritableDatabase().update(TABLE, v, "id=? AND state=?",
                new String[]{id, String.valueOf(DownloadTask.STATE_DOWNLOADING)});
    }

    /** updateProgressOnly 的 state 参数哨兵: 表示"本次不写 state"(NULL 不能用, 会清列)。 */
    public static final int EXPORTING_SENTINEL = -1;

    /**
     * 过滤阶段的过程文案(如"广告过滤中 · 重试 2/3")写进 error 列 — 仅 FILTERING 态生效。
     *
     * <p>复用 error 列承载临时过程文案(任务终态前会被清空/覆盖), 避免为几秒一次的
     * 过程刷新加专列; WHERE 限定 state=FILTERING, 过滤已结束(成功/失败)时静默不写,
     * 不会盖掉真实失败原因或入队后的清空。
     */
    public boolean updateFilterNote(String id, String note) {
        if (id == null) return false;
        ContentValues v = new ContentValues();
        v.put("error", note == null ? "" : note);
        v.put("updatedAt", System.currentTimeMillis());
        return getWritableDatabase().update(TABLE, v, "id=? AND state=?",
                new String[]{id, String.valueOf(DownloadTask.STATE_FILTERING)}) > 0;
    }

    @Override
    public boolean delete(String id) {
        return getWritableDatabase().delete(TABLE, "id=?", new String[]{id}) > 0;
    }

    /**
     * 启动恢复: 上次进程被杀时仍在 DOWNLOADING 的任务标记为 PAUSED(等用户续传);
     * 导出中(EXPORTING)被杀的 → 回滚 COMPLETED(EXPORTING 是临时态, 残留 export.tmp 由下次导出覆盖).
     * Media3 DefaultDownloadIndex 自身也持久, 但业务表的状态必须同步, 否则 UI 会显示卡死的"下载中"/"导出中".
     */
    public void markInterruptedAsPaused() {
        SQLiteDatabase db = getWritableDatabase();
        long now = System.currentTimeMillis();
        // 下载中被杀 → PAUSED(等续传)
        ContentValues v = new ContentValues();
        v.put("state", DownloadTask.STATE_PAUSED);
        v.put("updatedAt", now);
        db.update(TABLE, v, "state=?", new String[]{String.valueOf(DownloadTask.STATE_DOWNLOADING)});
        // 导出中被杀 → 回 COMPLETED(EXPORTING→EXPORTING 是非法迁移, 不回滚会永久卡死"正在导出中")
        ContentValues v2 = new ContentValues();
        v2.put("state", DownloadTask.STATE_COMPLETED);
        v2.put("updatedAt", now);
        db.update(TABLE, v2, "state=?", new String[]{String.valueOf(DownloadTask.STATE_EXPORTING)});
    }

    private List<DownloadTask> query(String[] columns, String where, String[] args, String orderBy) {
        List<DownloadTask> out = new ArrayList<>();
        try (Cursor c = getReadableDatabase().query(
                TABLE, columns, where, args, null, null, orderBy)) {
            while (c.moveToNext()) out.add(fromCursor(c));
        }
        return out;
    }

    /**
     * 读游标 → 任务对象。
     *
     * <p>{@code filteredPlaylist} 用 {@code getColumnIndex} 而非 {@code getColumnIndexOrThrow}:
     * 列表投影刻意不含该列(见 {@link #LIST_COLUMNS}), 缺列时留空而不是抛异常。
     * 真要用清单的地方请显式调 {@link #getFilteredPlaylist(String)}。
     */
    private static DownloadTask fromCursor(Cursor c) {
        DownloadTask t = new DownloadTask();
        t.id = c.getString(c.getColumnIndexOrThrow("id"));
        t.filmId = c.getString(c.getColumnIndexOrThrow("filmId"));
        t.filmTitle = c.getString(c.getColumnIndexOrThrow("filmTitle"));
        t.sourceKey = c.getString(c.getColumnIndexOrThrow("sourceKey"));
        t.sourceName = c.getString(c.getColumnIndexOrThrow("sourceName"));
        t.episode = c.getInt(c.getColumnIndexOrThrow("episode"));
        t.episodeTitle = c.getString(c.getColumnIndexOrThrow("episodeTitle"));
        t.srcUrl = c.getString(c.getColumnIndexOrThrow("srcUrl"));
        int plIdx = c.getColumnIndex("filteredPlaylist");
        t.filteredPlaylist = plIdx >= 0 ? c.getString(plIdx) : "";
        t.state = c.getInt(c.getColumnIndexOrThrow("state"));
        t.progressBytes = c.getLong(c.getColumnIndexOrThrow("progressBytes"));
        t.totalBytes = c.getLong(c.getColumnIndexOrThrow("totalBytes"));
        t.error = c.getString(c.getColumnIndexOrThrow("error"));
        t.rawFallback = c.getInt(c.getColumnIndexOrThrow("rawFallback")) != 0;
        t.cacheDir = c.getString(c.getColumnIndexOrThrow("cacheDir"));
        t.exportedPath = c.getString(c.getColumnIndexOrThrow("exportedPath"));
        t.createdAt = c.getLong(c.getColumnIndexOrThrow("createdAt"));
        t.updatedAt = c.getLong(c.getColumnIndexOrThrow("updatedAt"));
        return t;
    }

    private static ContentValues toValues(DownloadTask t) {
        ContentValues v = new ContentValues();
        v.put("id", t.id);
        v.put("filmId", t.filmId);
        v.put("filmTitle", t.filmTitle == null ? "" : t.filmTitle);
        v.put("sourceKey", t.sourceKey == null ? "" : t.sourceKey);
        v.put("sourceName", t.sourceName == null ? "" : t.sourceName);
        v.put("episode", t.episode);
        v.put("episodeTitle", t.episodeTitle == null ? "" : t.episodeTitle);
        v.put("srcUrl", t.srcUrl == null ? "" : t.srcUrl);
        v.put("filteredPlaylist", t.filteredPlaylist == null ? "" : t.filteredPlaylist);
        v.put("state", t.state);
        v.put("progressBytes", t.progressBytes);
        v.put("totalBytes", t.totalBytes);
        v.put("error", t.error == null ? "" : t.error);
        v.put("rawFallback", t.rawFallback ? 1 : 0);
        v.put("cacheDir", t.cacheDir == null ? "" : t.cacheDir);
        v.put("exportedPath", t.exportedPath == null ? "" : t.exportedPath);
        v.put("createdAt", t.createdAt);
        v.put("updatedAt", t.updatedAt);
        return v;
    }
}
