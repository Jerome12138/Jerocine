package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
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
    public void parse_byterangeKeptInListButExportBlocked() {
        // 原用例把 BYTERANGE 放在 base.ts 和一个**全新**的 next.ts 之间, 并断言 next.ts 被当完整分片收入 ——
        // 那等于声明"next.ts 是 base.ts 的第524288 字节起的片段", 与 HLS 规范不符(规范形态是
        // URL 在前、BYTERANGE 修饰它自己), 断言固化的是错误行为。
        // 现在: 分片仍照常收入(不改变解析面), 但 inspect() 会拦住导出 → 不会静默产出缺字节的文件。
        String pl = "#EXTM3U\n#EXTINF:10.0,\nbase.ts\n#EXT-X-BYTERANGE:524288@0\n"
                + "#EXTINF:10.0,\nnext.ts\n#EXT-X-ENDLIST\n";
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(pl, "https://x/");
        assertEquals(2, segs.size());
        assertNotNull("BYTERANGE 清单必须被拦下, 不能产出缺字节的导出文件",
                PlaylistSegments.inspect(pl).exportBlockReason());
    }

    // ---- 能力检测(inspect): 片源不固定时, 导出前必须能判断"能不能导" ----

    @Test
    public void inspect_plainTsPlaylistIsExportable() {
        assertNull(PlaylistSegments.inspect(SAMPLE).exportBlockReason());
    }

    @Test
    public void inspect_aes128BlocksExport() {
        // KEY 只被当普通标签跳过 → 分片 URL 照常收入 → 缓存里是密文, 拼出来打不开
        String pl = "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n"
                + "#EXTINF:10.0,\nseg0.ts\n#EXT-X-ENDLIST\n";
        PlaylistSegments.Capabilities c = PlaylistSegments.inspect(pl);
        assertEquals("AES-128", c.encryptionMethod);
        assertNotNull(c.exportBlockReason());
        assertTrue(c.exportBlockReason().contains("加密"));
    }

    @Test
    public void inspect_methodNoneIsNotBlocked() {
        String pl = "#EXTM3U\n#EXT-X-KEY:METHOD=NONE\n#EXTINF:10.0,\nseg0.ts\n#EXT-X-ENDLIST\n";
        assertNull(PlaylistSegments.inspect(pl).exportBlockReason());
    }

    // HLS 允许属性值带引号(RFC 8216§4.3.4.2)。不剥引号会让 equalsIgnoreCase("NONE")
    // 失效 → 明文流被误判为加密而拦下, 且用户看到的加密方式是带引号的怪字符串。
    @Test
    public void inspect_quotedAttributeValuesUnwrapped() {
        String pl = "#EXTM3U\n#EXT-X-KEY:METHOD=\"NONE\"\n#EXTINF:10.0,\nseg0.ts\n#EXT-X-ENDLIST\n";
        PlaylistSegments.Capabilities c = PlaylistSegments.inspect(pl);
        assertEquals("NONE", c.encryptionMethod);
        assertNull("带引号的 METHOD=NONE 必须放行", c.exportBlockReason());
    }

    @Test
    public void inspect_quotedAes128StillDetected() {
        String pl = "#EXTM3U\n#EXT-X-KEY:METHOD=\"AES-128\",URI=\"k.bin\"\n"
                + "#EXTINF:10.0,\nseg0.ts\n#EXT-X-ENDLIST\n";
        PlaylistSegments.Capabilities c = PlaylistSegments.inspect(pl);
        assertEquals("AES-128", c.encryptionMethod);
        assertNotNull(c.exportBlockReason());
    }

    // KEY 标签的属性顺序不固定: URI 在前、METHOD 在后也要能取到
    @Test
    public void inspect_keyAttributesInAnyOrder() {
        String pl = "#EXTM3U\n#EXT-X-KEY:URI=\"key.bin\",METHOD=AES-128\n"
                + "#EXTINF:10.0,\nseg0.ts\n#EXT-X-ENDLIST\n";
        assertEquals("AES-128", PlaylistSegments.inspect(pl).encryptionMethod);
    }

    @Test
    public void inspect_fmp4InitBlocksExport() {
        // EXT-X-MAP 的 init 分片(ftyp+moov)不在分片列表里, 且 .m4s 不是 MPEG-TS
        String pl = "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n"
                + "#EXTINF:10.0,\nseg0.m4s\n#EXT-X-ENDLIST\n";
        PlaylistSegments.Capabilities c = PlaylistSegments.inspect(pl);
        assertEquals("init.mp4", c.initUri);
        assertNotNull(c.exportBlockReason());
    }

    @Test
    public void inspect_byterangeBlocksExport() {
        String pl = "#EXTM3U\n#EXTINF:10.0,\nbase.ts\n#EXT-X-BYTERANGE:52128@0\n#EXT-X-ENDLIST\n";
        assertTrue(PlaylistSegments.inspect(pl).hasByteRange);
        assertNotNull(PlaylistSegments.inspect(pl).exportBlockReason());
    }

    @Test
    public void inspect_livePlaylistWithoutEndlistBlocked() {
        // 无 ENDLIST = 直播流, 导出得到的是被截断的录像
        String pl = "#EXTM3U\n#EXTINF:10.0,\nseg0.ts\n#EXTINF:10.0,\nseg1.ts\n";
        PlaylistSegments.Capabilities c = PlaylistSegments.inspect(pl);
        assertTrue(!c.hasEndList);
        assertNotNull(c.exportBlockReason());
    }

    @Test
    public void inspect_htmlErrorPageBlocked() {
        // 服务端 200 返回 HTML 错误页: 以前每行都会变"分片 URL", 现在明确判定不是清单
        String html = "<!DOCTYPE html>\n<html><head><title>404</title></head>\n<body>Not Found</body></html>";
        PlaylistSegments.Capabilities c = PlaylistSegments.inspect(html);
        assertTrue(!c.looksLikePlaylist);
        assertNotNull(c.exportBlockReason());
    }

    @Test
    public void inspect_nullOrEmptyBlocked() {
        assertNotNull(PlaylistSegments.inspect(null).exportBlockReason());
        assertNotNull(PlaylistSegments.inspect("").exportBlockReason());
    }

    // ---- BOM: trim() 不处理 U+FEFF, 会把首行 #EXTM3U 当分片 URL ----

    @Test
    public void parse_bomStrippedFromFirstLine() {
        String pl = "\uFEFF#EXTM3U\n#EXTINF:10.0,\nseg0.ts\n#EXT-X-ENDLIST\n";
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(pl, "https://x/");
        // 修复前: 首行因 BOM 不匹配任何标签也不以 # 开头 → 被当分片收进来, size=2
        assertEquals(1, segs.size());
        assertEquals("https://x/seg0.ts", segs.get(0).url);
        assertNull(PlaylistSegments.inspect(pl).exportBlockReason());
    }

    // ---- resolveUrl: base 落在主机根(无尾斜杠)时的畸形 URL ----

    @Test
    public void resolve_baseOnHostRootWithoutTrailingSlash() {
        // 修复前: lastIndexOf('/') 命中 "https://" 里的斜杠 → dir="https://" → "https://seg.ts"
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(
                "#EXTINF:5,\nseg.ts\n", "https://cdn.example.com");
        assertEquals("https://cdn.example.com/seg.ts", segs.get(0).url);
    }

    // ---- 时长未知时, rangeForMinutes 不能放大成全集 ----

    @Test
    public void rangeForMinutes_allZeroDurationsCapped() {
        // 无 EXTINF → 时长全 0 → 修复前 return segments.size(), "缓冲 N 分钟"变缓冲整集。
        // 构造远超封顶值的分片数(cap * 3), 这样常量将来调大到该量级时用例会明确失败,
        // 而不会因为"分片数 <= cap"变成静默的假通过。
        int n = PlaylistSegments.MINUTES_FALLBACK_CAP * 3;
        StringBuilder pl = new StringBuilder("#EXTM3U\n");
        for (int i = 0; i < n; i++) pl.append("seg").append(i).append(".ts\n");
        pl.append("#EXT-X-ENDLIST\n");
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(pl.toString(), "https://x/");
        assertEquals(n, segs.size());
        int end = PlaylistSegments.rangeForMinutes(segs, 5.0);
        assertTrue("时长未知时不应返回全集(实际 " + end + "/" + n + ")",
                end < segs.size());
        assertEquals("封顶值应恰好是 MINUTES_FALLBACK_CAP",
                PlaylistSegments.MINUTES_FALLBACK_CAP, end);
        assertTrue(end > 0);
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

    // ---- resolveUrl 对齐 media3 Uri.resolve(RFC 3986) 的回归用例 ----

    @Test
    public void resolve_parentDirReferenceNormalized() {
        // 与 Uri.resolve("https://cdn/play/video/playlist.m3u8", "../hd/seg.ts") 一致
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(
                "#EXTINF:5,\n../hd/seg.ts\n", "https://cdn/play/video/playlist.m3u8");
        assertEquals("https://cdn/play/hd/seg.ts", segs.get(0).url);
    }

    @Test
    public void resolve_rootRelativePathUsesAuthority() {
        // 以 / 开头 → 替换 base 的 scheme://authority, 与 Uri.resolve 一致
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(
                "#EXTINF:5,\n/video/seg.ts\n", "https://cdn.example.com/play/playlist.m3u8");
        assertEquals("https://cdn.example.com/video/seg.ts", segs.get(0).url);
    }

    @Test
    public void resolve_queryAndFragmentPreserved() {
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(
                "#EXTINF:5,\nseg.ts?token=abc&x=1#frag\n", "https://cdn.example.com/play/playlist.m3u8");
        assertEquals("https://cdn.example.com/play/seg.ts?token=abc&x=1#frag", segs.get(0).url);
        // base 自身带 query 时, 相对拼接只取 base 的目录部分
        List<PlaylistSegments.Segment> baseWithQuery = PlaylistSegments.parse(
                "#EXTINF:5,\nseg.ts\n", "https://cdn.example.com/play/playlist.m3u8?token=9");
        assertEquals("https://cdn.example.com/play/seg.ts", baseWithQuery.get(0).url);
    }

    @Test
    public void resolve_dotSegmentsNormalized() {
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(
                "#EXTINF:5,\n./seg.ts\n#EXTINF:5,\na/../b/seg.ts\n", "https://cdn.example.com/play/playlist.m3u8");
        assertEquals("https://cdn.example.com/play/seg.ts", segs.get(0).url);
        assertEquals("https://cdn.example.com/play/b/seg.ts", segs.get(1).url);
    }

    @Test
    public void resolve_baseOnHostRoot() {
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(
                "#EXTINF:5,\nseg.ts\n", "https://cdn.example.com/");
        assertEquals("https://cdn.example.com/seg.ts", segs.get(0).url);
    }

    @Test
    public void resolve_absoluteUrlUnchanged() {
        List<PlaylistSegments.Segment> segs = PlaylistSegments.parse(
                "#EXTINF:5,\nhttps://other.cdn/x/seg.ts\n", "https://cdn.example.com/play/playlist.m3u8");
        assertEquals("https://other.cdn/x/seg.ts", segs.get(0).url);
    }
}
