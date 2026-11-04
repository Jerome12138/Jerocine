package com.jerocine.player;

import static org.junit.Assert.assertSame;

import androidx.media3.common.C;
import androidx.media3.datasource.DataSource;
import androidx.media3.datasource.DataSpec;
import androidx.media3.datasource.TransferListener;

import org.junit.Test;

import java.io.IOException;
import java.util.Collections;
import java.util.Map;

/**
 * Q①/PB2(player-fix 2026-11-05) — HLS 数据源按类型拆分:
 * `C.DATA_TYPE_MANIFEST`(master/媒体清单)绕过 SimpleCache 直连回源;
 * 其余类型(分片/初始化段)维持原 CacheDataSource 链路。
 */
public class HlsManifestSplitDataSourceFactoryTest {

    private static DataSource stub() {
        return new DataSource() {
            @Override
            public void addTransferListener(TransferListener transferListener) {
            }

            @Override
            public long open(DataSpec dataSpec) throws IOException {
                return 0;
            }

            @Override
            public int read(byte[] buffer, int offset, int length) throws IOException {
                return 0;
            }

            @Override
            public android.net.Uri getUri() {
                return null;
            }

            @Override
            public Map<String, java.util.List<String>> getResponseHeaders() {
                return Collections.emptyMap();
            }

            @Override
            public void close() throws IOException {
            }
        };
    }

    @Test
    public void manifestType_goesToNoCacheFactory() {
        DataSource manifestDs = stub();
        DataSource mediaDs = stub();
        HlsManifestSplitDataSourceFactory f = new HlsManifestSplitDataSourceFactory(
                () -> manifestDs, () -> mediaDs);
        assertSame(manifestDs, f.createDataSource(C.DATA_TYPE_MANIFEST));
    }

    @Test
    public void mediaTypes_goToCacheFactory() {
        DataSource manifestDs = stub();
        DataSource mediaDs = stub();
        HlsManifestSplitDataSourceFactory f = new HlsManifestSplitDataSourceFactory(
                () -> manifestDs, () -> mediaDs);
        assertSame(mediaDs, f.createDataSource(C.DATA_TYPE_MEDIA));
        assertSame(mediaDs, f.createDataSource(C.DATA_TYPE_MEDIA_INITIALIZATION));
        assertSame(mediaDs, f.createDataSource(C.DATA_TYPE_CUSTOM_BASE));
    }
}
