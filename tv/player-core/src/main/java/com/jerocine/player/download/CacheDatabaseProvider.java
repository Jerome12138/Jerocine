package com.jerocine.player.download;

import android.content.Context;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;

import androidx.media3.database.DatabaseProvider;

/**
 * 下载缓存专用数据库 — 独立于播放器缓存(media3.db).
 *
 * <p>为什么独立: 播放器 {@code PlayerActivity.sCache} 已用 StandaloneDatabaseProvider 打开
 * 默认 DB 文件; 下载缓存若复用同一个 DB 文件, 两个 SQLiteOpenHelper 实例写同一路径会引入
 * 锁竞争风险。这里把下载缓存(SimpleCache 元数据 + DefaultDownloadIndex)隔离到
 * {@code jerocine_downloads_cache.db}, 与播放缓存完全解耦。
 */
final class CacheDatabaseProvider implements DatabaseProvider {

    private static final String DB_NAME = "jerocine_downloads_cache.db";

    private final SQLiteOpenHelper helper;

    CacheDatabaseProvider(Context context) {
        helper = new SQLiteOpenHelper(context.getApplicationContext(), DB_NAME, null, 1) {
            @Override
            public void onCreate(SQLiteDatabase db) {
                // 表由 media3(SimpleCache 元数据 / DefaultDownloadIndex)自行建, 无需预建
            }

            @Override
            public void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion) {
            }
        };
    }

    @Override
    public SQLiteDatabase getReadableDatabase() {
        return helper.getReadableDatabase();
    }

    @Override
    public SQLiteDatabase getWritableDatabase() {
        return helper.getWritableDatabase();
    }
}
