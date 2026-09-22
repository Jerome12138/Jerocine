package com.jerocine.player;

import java.io.File;

/**
 * 播放器配置 — 壳层传入.
 *
 * 字段与 Intent extras 一一对应(见 {@link PlayerActivity#EXTRA_SOURCES_JSON} 等),
 * {@link JerocinePlayer#start} 负责搬运.
 */
public class PlayerConfig {
    /** 多源 JSON: [{"id","name","episodes":[{"url","title"}]}] */
    public String sourcesJson;
    /** 初始选中的源 id(找不到则用第一源) */
    public String currentSourceId;
    public int startIndex;
    public long resumeMs;
    public long skipIntroMs;
    public long skipOutroMs;
    /** 片尾是否自动连播下一集 */
    public boolean autoNext = true;
    public String filmId;
    public String filmName;
    /** 代理 base(到 /api), 供端侧广告过滤; 为空时回退 {@link JerocinePlayer#defaultProxyBase()} */
    public String proxyBase;
    /** 视频缓存目录; 为空时用壳的默认缓存目录 */
    public File cacheDir;

    public PlayerConfig() {}

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
        this(sourcesJson, currentSourceId, startIndex, resumeMs, skipIntroMs, skipOutroMs,
                true, filmId, filmName, proxyBase, cacheDir);
    }

    public PlayerConfig(
        String sourcesJson,
        String currentSourceId,
        int startIndex,
        long resumeMs,
        long skipIntroMs,
        long skipOutroMs,
        boolean autoNext,
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
        this.autoNext = autoNext;
        this.filmId = filmId;
        this.filmName = filmName;
        this.proxyBase = proxyBase;
        this.cacheDir = cacheDir;
    }
}
