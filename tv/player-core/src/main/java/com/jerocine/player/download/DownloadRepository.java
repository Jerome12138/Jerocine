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
    private static final int DB_VERSION = 1;
    private static final String TABLE = "download_task";

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
                + "cacheDir TEXT NOT NULL DEFAULT '',"
                + "exportedPath TEXT NOT NULL DEFAULT '',"
                + "createdAt INTEGER NOT NULL,"
                + "updatedAt INTEGER NOT NULL)");
        db.execSQL("CREATE INDEX idx_download_film ON " + TABLE + "(filmId)");
    }

    @Override
    public void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion) {
        // v1 初版; 未来加列走 ALTER, 不 drop(下载进度不能丢)
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

    @Override
    public List<DownloadTask> listByFilm(String filmId) {
        return query("filmId=?", new String[]{filmId}, "episode ASC");
    }

    @Override
    public List<DownloadTask> listAll() {
        return query(null, null, "createdAt ASC");
    }

    @Override
    public boolean delete(String id) {
        return getWritableDatabase().delete(TABLE, "id=?", new String[]{id}) > 0;
    }

    /**
     * 启动恢复: 上次进程被杀时仍在 DOWNLOADING 的任务标记为 PAUSED(等用户续传).
     * Media3 DefaultDownloadIndex 自身也持久, 但业务表的状态必须同步, 否则 UI 会显示卡死的"下载中".
     */
    public void markInterruptedAsPaused() {
        ContentValues v = new ContentValues();
        v.put("state", DownloadTask.STATE_PAUSED);
        v.put("updatedAt", System.currentTimeMillis());
        getWritableDatabase().update(TABLE, v,
                "state=?", new String[]{String.valueOf(DownloadTask.STATE_DOWNLOADING)});
    }

    private List<DownloadTask> query(String where, String[] args, String orderBy) {
        List<DownloadTask> out = new ArrayList<>();
        try (Cursor c = getReadableDatabase().query(
                TABLE, null, where, args, null, null, orderBy)) {
            while (c.moveToNext()) out.add(fromCursor(c));
        }
        return out;
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
        v.put("cacheDir", t.cacheDir == null ? "" : t.cacheDir);
        v.put("exportedPath", t.exportedPath == null ? "" : t.exportedPath);
        v.put("createdAt", t.createdAt);
        v.put("updatedAt", t.updatedAt);
        return v;
    }

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
        t.filteredPlaylist = c.getString(c.getColumnIndexOrThrow("filteredPlaylist"));
        t.state = c.getInt(c.getColumnIndexOrThrow("state"));
        t.progressBytes = c.getLong(c.getColumnIndexOrThrow("progressBytes"));
        t.totalBytes = c.getLong(c.getColumnIndexOrThrow("totalBytes"));
        t.error = c.getString(c.getColumnIndexOrThrow("error"));
        t.cacheDir = c.getString(c.getColumnIndexOrThrow("cacheDir"));
        t.exportedPath = c.getString(c.getColumnIndexOrThrow("exportedPath"));
        t.createdAt = c.getLong(c.getColumnIndexOrThrow("createdAt"));
        t.updatedAt = c.getLong(c.getColumnIndexOrThrow("updatedAt"));
        return t;
    }
}
