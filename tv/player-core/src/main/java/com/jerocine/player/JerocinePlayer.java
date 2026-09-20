package com.jerocine.player;

import android.content.Context;
import android.content.Intent;

import java.io.File;

/**
 * 播放器入口 — 壳层调用
 */
public class JerocinePlayer {

    private static PlayerActivity.PlayerEventCallback callback;

    public static void start(Context context, PlayerConfig config) {
        Intent intent = new Intent(context, PlayerActivity.class);
        intent.putExtra(PlayerActivity.EXTRA_SOURCES_JSON, config.sourcesJson);
        intent.putExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID, config.currentSourceId);
        intent.putExtra(PlayerActivity.EXTRA_START_INDEX, config.startIndex);
        intent.putExtra(PlayerActivity.EXTRA_RESUME_MS, config.resumeMs);
        intent.putExtra(PlayerActivity.EXTRA_SKIP_INTRO_MS, config.skipIntroMs);
        intent.putExtra(PlayerActivity.EXTRA_SKIP_OUTRO_MS, config.skipOutroMs);
        intent.putExtra(PlayerActivity.EXTRA_FILM_ID, config.filmId);
        intent.putExtra(PlayerActivity.EXTRA_FILM_NAME, config.filmName);
        intent.putExtra(PlayerActivity.EXTRA_PROXY_BASE, config.proxyBase);
        intent.putExtra(PlayerActivity.EXTRA_CACHE_DIR, config.cacheDir.getAbsolutePath());
        context.startActivity(intent);
    }

    public static void setCallback(PlayerActivity.PlayerEventCallback cb) {
        callback = cb;
        PlayerActivity.sCallback = cb;
    }

    public static void stopCurrent() {
        PlayerActivity.stopRunningInstance();
    }

    public static void setSpeed(float speed) {
        PlayerActivity.setSpeedOnRunningInstance(speed);
    }
}
