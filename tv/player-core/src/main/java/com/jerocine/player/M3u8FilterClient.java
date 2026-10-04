package com.jerocine.player;

import java.net.URLEncoder;
import java.util.concurrent.TimeUnit;

import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;

/**
 * 端侧广告过滤客户端 — 同步 POST 原始 m3u8 到 /v1/m3u8/filter 剔广告.
 *
 * <p>播放器({@link PlayerAdFilterHelper})与下载器(download 包)共用:
 * 下载任务入队前同样需要"过滤后清单"作为下载列表, 同一份客户端避免两处重复实现。
 *
 * <p>超时上限说明: 媒体表实际可达 900+ 段, 实测单张清单上行 ~25KB、过滤后下行 ~77KB,
 * 服务端纯过滤耗时 3s+, 弱网设备上行更慢 → callTimeout 12s, 并重试一次(切集/入队时网络争用偶发失败)。
 * 8s 会把"其实能过滤"误判成"过滤失败"。
 */
public final class M3u8FilterClient {

    private static final OkHttpClient CLIENT = new OkHttpClient.Builder()
            .connectTimeout(8, TimeUnit.SECONDS)
            .readTimeout(10, TimeUnit.SECONDS)
            .writeTimeout(10, TimeUnit.SECONDS)
            .callTimeout(12, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .build();

    private M3u8FilterClient() {}

    /** 过滤结果: data=过滤后字节; filteredCount=本层被剔除的广告段数(master 恒 0). */
    public static final class Result {
        public final byte[] data;
        public final int filteredCount;

        Result(byte[] data, int filteredCount) {
            this.data = data;
            this.filteredCount = filteredCount;
        }
    }

    /**
     * 同步 POST 原始 m3u8 到 /v1/m3u8/filter, 返回过滤后结果.
     *
     * @param proxyBase 服务端 API base(如 https://jerocine.art/api); 空/异常 → null
     * @param srcUrl    源站 m3u8 原始 URL(用作 ?src= 参数, 服务端据此判断源站可达性)
     * @param raw       原始清单字节(application/vnd.apple.mpegurl)
     * @return 过滤后结果; 网络失败/非 2xx/无 body(重试一次后仍失败) → null
     */
    public static Result filter(String proxyBase, String srcUrl, byte[] raw) {
        if (proxyBase == null || proxyBase.isEmpty() || raw == null || raw.length == 0) return null;
        final String url;
        try {
            String base = proxyBase.endsWith("/")
                    ? proxyBase.substring(0, proxyBase.length() - 1)
                    : proxyBase;
            url = base + "/v1/m3u8/filter?src=" + URLEncoder.encode(srcUrl, "UTF-8");
        } catch (Exception e) {
            return null;
        }
        for (int attempt = 0; attempt < 2; attempt++) {
            try {
                Request req = new Request.Builder().url(url)
                        .post(RequestBody.create(
                                MediaType.parse("application/vnd.apple.mpegurl"), raw))
                        .build();
                try (Response resp = CLIENT.newCall(req).execute()) {
                    if (!resp.isSuccessful() || resp.body() == null) {
                        if (attempt == 1) return null;
                    } else {
                        byte[] out = resp.body().bytes();
                        int cnt = 0;
                        String n = resp.header("X-Ad-Filtered");
                        if (n != null) {
                            try {
                                cnt = Integer.parseInt(n);
                            } catch (NumberFormatException ignore) {
                            }
                        }
                        return new Result(out, cnt);
                    }
                }
            } catch (Exception e) {
                if (attempt == 1) return null;
            }
            try {
                Thread.sleep(250);
            } catch (InterruptedException ie) {
                Thread.currentThread().interrupt();
                return null;
            }
        }
        return null;
    }
}
