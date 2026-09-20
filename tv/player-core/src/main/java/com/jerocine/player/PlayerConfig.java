package com.jerocine.player;

import java.io.File;

/**
 * 播放器配置 — 壳层传入
 */
public class PlayerConfig {
    public String sourcesJson;
    public String currentSourceId;
    public int startIndex;
    public long resumeMs;
    public long skipIntroMs;
    public long skipOutroMs;
    public String filmId;
    public String filmName;
    public String proxyBase;
    public File cacheDir;

    public PlayerConfig(
        String sourcesJson,
        String currentSourceId,
        int startIndex,
        long resumeMs,
        long skipIntroMs,
        long skipOutroMs,
        String filmId,
        String filmName,
        String proxyBase,
        File cacheDir
    ) {
        this.sourcesJson = sourcesJson;
        this.currentSourceId = currentSourceId;
        this.startIndex = startIndex;
        this.resumeMs = resumeMs;
        this.skipIntroMs = skipIntroMs;
        this.skipOutroMs = skipOutroMs;
        this.filmId = filmId;
        this.filmName = filmName;
        this.proxyBase = proxyBase;
        this.cacheDir = cacheDir;
    }
}
