package com.jerocine.player;

import androidx.media3.common.C;
import androidx.media3.datasource.DataSource;
import androidx.media3.exoplayer.hls.HlsDataSourceFactory;

/**
 * Q①/PB2(player-fix 2026-11-05) — HLS 数据源按 media3 数据类型拆分。
 *
 * <p>{@link C#DATA_TYPE_MANIFEST}(master/媒体清单, 每 targetDuration 轮询一次)绕过
 * SimpleCache 直连回源: 清单(媒体表 length=UNSET)被写进播放缓存后, 直播滑窗每轮都从磁盘
 * 回放同一静态快照 → DefaultHlsPlaylistTracker 判 stuck → PlaylistStuckException(排查
 * 报告①, 2026-11-05)。VOD 的缓存收益在分片(占体积), 清单缓存收益本来就小。
 * 其余类型(分片/初始化段)维持原 CacheDataSource 链路不变。
 *
 * <p>清单工厂用 {@code DefaultDataSource}(OkHttp 上游): {@code file://} 本地下载集清单
 * 由 DefaultDataSource 委给 FileDataSource(不进缓存, 行为与预期一致), http(s) 走 OkHttp。
 */
public final class HlsManifestSplitDataSourceFactory implements HlsDataSourceFactory {

    private final DataSource.Factory manifestFactory;
    private final DataSource.Factory mediaFactory;

    public HlsManifestSplitDataSourceFactory(DataSource.Factory manifestFactory,
                                             DataSource.Factory mediaFactory) {
        this.manifestFactory = manifestFactory;
        this.mediaFactory = mediaFactory;
    }

    @Override
    public DataSource createDataSource(int dataType) {
        return dataType == C.DATA_TYPE_MANIFEST
                ? manifestFactory.createDataSource()
                : mediaFactory.createDataSource();
    }
}
