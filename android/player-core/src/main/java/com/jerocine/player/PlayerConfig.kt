package com.jerocine.player

import java.io.File

/**
 * 播放器配置 — 壳层传入
 */
data class PlayerConfig(
    val sourcesJson: String,       // 多源 JSON
    val currentSourceId: String, // 当前源 ID
    val startIndex: Int,         // 起始集索引
    val resumeMs: Long,          // 续播位置
    val skipIntroMs: Long,       // 跳片头
    val skipOutroMs: Long,       // 跳片尾
    val filmId: String,          // 影片 ID
    val filmName: String,        // 影片名
    val proxyBase: String,       // 代理 base
    val cacheDir: File,          // 缓存目录（壳层传，各自独立）
)
