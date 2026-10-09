package com.jerocine.player.download;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;

import org.junit.Test;

/**
 * 原始流兜底清单绝对化 — 相对分片/URI 属性按来源 URL 绝对化,
 * 语义对齐 media3 的 RFC 3986 解析(下载缓存 key 与播放期一致的前提)。
 */
public class HlsPlaylistAbsolutizerTest {

    private static final String BASE =
            "https://cdn.example.com/hls/2000k/hls/mixed.m3u8";

    @Test
    public void relativeSegments_resolvedAgainstChildUrl() {
        String in = "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:10.0,\nseg001.ts\n#EXTINF:10.0,\nseg002.ts\n";
        String out = HlsPlaylistAbsolutizer.absolutize(in, BASE);
        assertFalse(out.contains("\nseg001.ts"));
        assertEquals("https://cdn.example.com/hls/2000k/hls/seg001.ts",
                lineContaining(out, "seg001.ts"));
        assertEquals("https://cdn.example.com/hls/2000k/hls/seg002.ts",
                lineContaining(out, "seg002.ts"));
    }

    @Test
    public void rootRelativeAndDotDot_resolved() {
        String in = "#EXTM3U\n/ads/a.ts\n../key.bin\n";
        String out = HlsPlaylistAbsolutizer.absolutize(in, BASE);
        // / 开头 → 挂主机根; ../ → 回一级(BASE 目录 = /hls/2000k/hls/)
        assertEquals("https://cdn.example.com/ads/a.ts", lineContaining(out, "a.ts"));
        assertEquals("https://cdn.example.com/hls/2000k/key.bin", lineContaining(out, "key.bin"));
    }

    @Test
    public void absoluteUrls_untouched() {
        String in = "#EXTM3U\nhttps://other.example.net/x/seg.ts\nhttp://legacy.example.org/y.ts\n";
        String out = HlsPlaylistAbsolutizer.absolutize(in, BASE);
        assertEquals("https://other.example.net/x/seg.ts", lineContaining(out, "seg.ts"));
        assertEquals("http://legacy.example.org/y.ts", lineContaining(out, "y.ts"));
    }

    @Test
    public void tagUriAttributes_rewritten() {
        String in = "#EXTM3U\n"
                + "#EXT-X-KEY:METHOD=AES-128,URI=\"keys/k1\",IV=0x1234\n"
                + "#EXT-X-MAP:URI=\"init.mp4\"\n"
                + "#EXTINF:10.0,\nseg.ts\n";
        String out = HlsPlaylistAbsolutizer.absolutize(in, BASE);
        assertEquals(
                "#EXT-X-KEY:METHOD=AES-128,URI=\"https://cdn.example.com/hls/2000k/hls/keys/k1\",IV=0x1234",
                lineContaining(out, "EXT-X-KEY"));
        assertEquals(
                "#EXT-X-MAP:URI=\"https://cdn.example.com/hls/2000k/hls/init.mp4\"",
                lineContaining(out, "EXT-X-MAP"));
        // 非属性行不能被 URI= 误伤; IV=0x1234 原样
        assertFalse(out.replace("IV=0x1234", "").contains("0x1234"));
    }

    @Test
    public void commentsAndBlankLines_preserved() {
        String in = "#EXTM3U\n\n# 由服务端生成\n#EXTINF:6.006,\nseg.ts\n";
        String out = HlsPlaylistAbsolutizer.absolutize(in, BASE);
        assertEquals("# 由服务端生成", lineContaining(out, "由服务端生成"));
        assertEquals("#EXTINF:6.006,", lineContaining(out, "EXTINF"));
        // 空行保留
        assertTrue(out.contains("\n\n"));
    }

    @Test
    public void queryStringsAndEncodedChars_kept() {
        String in = "#EXTM3U\nseg.ts?token=abc%2Fdef&sign=$1$x\n";
        String out = HlsPlaylistAbsolutizer.absolutize(in, BASE);
        assertEquals("https://cdn.example.com/hls/2000k/hls/seg.ts?token=abc%2Fdef&sign=$1$x",
                lineContaining(out, "token="));
    }

    @Test
    public void idempotent_forAbsoluteInput() {
        String in = "#EXTM3U\nhttps://cdn.example.com/hls/2000k/hls/seg.ts\n";
        String once = HlsPlaylistAbsolutizer.absolutize(in, BASE);
        assertEquals(once, HlsPlaylistAbsolutizer.absolutize(once, BASE));
    }

    @Test
    public void degenerateInputs_returnedAsIs() {
        assertEquals(null, HlsPlaylistAbsolutizer.absolutize(null, BASE));
        assertEquals("", HlsPlaylistAbsolutizer.absolutize("", BASE));
        assertEquals("#EXTM3U\nx.ts", HlsPlaylistAbsolutizer.absolutize("#EXTM3U\nx.ts", null));
    }

    private static String lineContaining(String text, String needle) {
        for (String line : text.split("\n")) {
            if (line.contains(needle)) return line;
        }
        return "";
    }

    private static void assertTrue(boolean b) {
        org.junit.Assert.assertTrue(b);
    }
}
