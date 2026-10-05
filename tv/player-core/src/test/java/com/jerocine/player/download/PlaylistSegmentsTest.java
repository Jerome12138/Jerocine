package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.List;

/** PlaylistSegments 纯逻辑单测 — 分片解析(绝对/相对 URL)与分钟范围选择. */
public class PlaylistSegmentsTest {

    private static final String SAMPLE =
            "#EXTM3U\n"
                    + "#EXT-X-VERSION:3\n"
                    + "#EXT-X-TARGETDURATION:10\n"
                    + "#EXTINF:10.000,\n"
                    + "seg0.ts\n"
                    + "#EXTINF:9.500,\n"
                    + "seg1.ts\n"
                    + "#EXTINF:10.000,\n"
                    + "https://cdn.example.com/path/seg2.ts\n"
                    + "#EXT-X-ENDLIST\n";

    @Test
    public void parse_absoluteAndRelativeUrls() {
        List<PlaylistSegments.Segment> segs =
                PlaylistSegments.parse(SAMPLE, "https://cdn.example.com/play/playlist.m3u8");
        assertEquals(3, segs.size());
        // 相对路径按 base 的目录拼接
        assertEquals("https://cdn.example.com/play/seg0.ts", segs.get(0).url);
        assertEquals(10.0, segs.get(0).durationSeconds, 0.001);
        assertEquals("https://cdn.example.com/play/seg1.ts", segs.get(1).url);
        assertEquals(9.5, segs.get(1).durationSeconds, 0.001);
        // 绝对 URL 原样保留
        assertEquals("https://cdn.example.com/path/seg2.ts", segs.get(2).url);
        assertEquals(10.0, segs.get(2).durationSeconds, 0.001);
    }

    @Test
    public void parse_nullOrEmptyReturnsEmpty() {
        assertTrue(PlaylistSegments.parse(null, "https://x/").isEmpty());
        assertTrue(PlaylistSegments.parse("", "https://x/").isEmpty());
    }

    @Test
    public void parse_commentAndTagLinesSkipped() {
        String pl = "#EXTM3U\n#EXT-X-DISCONTINUITY\n#EXTINF:8.0,\nsegA.ts\n#COMMENT\nsegB.ts\n";
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(pl, "https://x/");
        assertEquals(2, segs.size());
        assertEquals(8.0, segs.get(0).durationSeconds, 0.001);
        // 无 EXTINF 前缀的分片时长按 0 计
        assertEquals(0.0, segs.get(1).durationSeconds, 0.001);
    }

    @Test
    public void parse_byterangeSkipped() {
        String pl = "#EXTINF:10.0,\nbase.ts\n#EXT-X-BYTERANGE:524288@0\n#EXTINF:10.0,\nnext.ts\n";
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(pl, "https://x/");
        assertEquals(2, segs.size());
        assertEquals("https://x/base.ts", segs.get(0).url);
        assertEquals("https://x/next.ts", segs.get(1).url);
    }

    @Test
    public void rangeForMinutes_coversExactMinutes() {
        // 总时长 29.5s: 3 分钟 → 全覆盖; 0.2 分钟(12s) → 2 片(10+9.5 超 12s)
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(SAMPLE, "https://x/");
        assertEquals(3, PlaylistSegments.rangeForMinutes(segs, 3.0));
        assertEquals(2, PlaylistSegments.rangeForMinutes(segs, 0.2));
        assertEquals(1, PlaylistSegments.rangeForMinutes(segs, 0.1));
    }

    @Test
    public void rangeForMinutes_emptyOrZeroReturnsZero() {
        assertEquals(0, PlaylistSegments.rangeForMinutes(null, 5.0));
        assertEquals(0, PlaylistSegments.rangeForMinutes(new java.util.ArrayList<>(), 5.0));
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(SAMPLE, "https://x/");
        assertEquals(0, PlaylistSegments.rangeForMinutes(segs, 0));
    }
}
