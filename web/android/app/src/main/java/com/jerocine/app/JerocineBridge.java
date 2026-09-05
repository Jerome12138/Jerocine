package com.jerocine.app;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.ConnectivityManager;
import android.net.NetworkCapabilities;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.Locale;
import java.util.TimeZone;

/**
 * JerocineBridge v2 — 注入到 WebView 的全能 JS Bridge.
 *
 * 设计原则: 通过通用 invoke(method, jsonArgs) → resultJson 暴露能力,
 * 未来 web 加新功能不用更新 APK, 只要 native 已经注册对应 method handler.
 *
 * web 端调用 (推荐):
 *   const result = JSON.parse(window.JerocineNative.invoke('getDeviceInfo', ''))
 *
 * 兼容旧调用 (老 PlayView 还用):
 *   window.JerocinePlayer.playPlaylist(jsonString)
 *
 * 反向 (native → web) 用 PlayerEvents 类的 sendEvent.
 */
public class JerocineBridge {

    private static final String TAG = "JerocineBridge";
    static final String EVENT_DISPATCHER_JS = "window.__JerocineEvents";

    /** native 通用 prefs (跨 origin 持久化) */
    private static final String STORAGE_PREFS = "jerocine_storage";

    private final Activity activity;

    public JerocineBridge(Activity activity) {
        this.activity = activity;
    }

    // ========================== 通用 invoke (v2 核心) ==========================

    /**
     * 通用调用入口. method 命名见类注释.
     * 同步返回 (WebView JS 线程调用), 故必须避免长任务; 长任务返 ok 再异步事件回调.
     */
    @JavascriptInterface
    public String invoke(String method, String jsonArgs) {
        try {
            JSONObject args = (jsonArgs == null || jsonArgs.isEmpty())
                    ? new JSONObject()
                    : new JSONObject(jsonArgs);
            switch (method) {
                case "getDeviceInfo":      return handleGetDeviceInfo();
                case "getPlatform":        return jsonOk("platform", "android-tv");
                case "storageGet":         return handleStorageGet(args);
                case "storageSet":         return handleStorageSet(args);
                case "storageRemove":      return handleStorageRemove(args);
                case "toast":              return handleToast(args);
                case "openExternal":       return handleOpenExternal(args);
                case "exitApp":            runOnUi(() -> activity.finishAffinity()); return jsonOk();
                case "keepScreenOn":       return handleKeepScreenOn(args);
                case "getNetworkStatus":   return handleGetNetworkStatus();
                case "playVideo":          return handlePlayVideo(args);
                case "playPlaylist":       return handlePlayPlaylist(args);
                case "stopPlayer":         return handleStopPlayer();
                case "setPlayerSpeed":     return handleSetPlayerSpeed(args);
                case "checkUpdate":        return handleCheckUpdate();
                case "openServerSettings": return handleOpenServerSettings();
                case "setAuthToken":       return handleSetAuthToken(args);
                case "clearAuthToken":     return handleClearAuthToken();
                case "echoTest":           return handleEchoTest(args);
                default:                   return jsonErr("unknown method: " + method);
            }
        } catch (Exception e) {
            Log.e(TAG, "invoke err method=" + method + " err=" + e.getMessage());
            return jsonErr(e.getMessage());
        }
    }

    // ========================== 兼容旧 API (保留, 内部 delegate 到 v2) ==========================

    @JavascriptInterface
    public void playVideo(String url, String title) {
        try {
            JSONObject args = new JSONObject().put("url", url).put("title", title == null ? "" : title);
            handlePlayVideo(args);
        } catch (Exception ignore) {}
    }

    @JavascriptInterface
    public void playPlaylist(String configJson) {
        try {
            handlePlayPlaylist(new JSONObject(configJson == null ? "{}" : configJson));
        } catch (Exception ignore) {}
    }

    @JavascriptInterface
    public String getPlatform() { return "android-tv"; }

    @JavascriptInterface
    public void setAuthToken(String token) {
        try { handleSetAuthToken(new JSONObject().put("token", token == null ? "" : token)); } catch (Exception ignore) {}
    }

    @JavascriptInterface
    public void clearAuthToken() { handleClearAuthToken(); }

    @JavascriptInterface
    public void checkUpdate() { handleCheckUpdate(); }

    @JavascriptInterface
    public void openServerSettings() { handleOpenServerSettings(); }

    // ========================== Method handlers ==========================

    private String handleGetDeviceInfo() throws Exception {
        JSONObject d = new JSONObject();
        d.put("platform", "android");
        d.put("brand", Build.BRAND);
        d.put("manufacturer", Build.MANUFACTURER);
        d.put("model", Build.MODEL);
        d.put("device", Build.DEVICE);
        d.put("sdk", Build.VERSION.SDK_INT);
        d.put("release", Build.VERSION.RELEASE);
        d.put("abi", Build.SUPPORTED_ABIS != null && Build.SUPPORTED_ABIS.length > 0 ? Build.SUPPORTED_ABIS[0] : "");
        d.put("locale", Locale.getDefault().toString());
        d.put("timezone", TimeZone.getDefault().getID());
        d.put("appVersionCode", BuildConfig.VERSION_CODE);
        d.put("appVersionName", BuildConfig.VERSION_NAME);
        return jsonOk("device", d);
    }

    private String handleStorageGet(JSONObject args) throws Exception {
        String key = args.optString("key", "");
        if (key.isEmpty()) return jsonErr("missing key");
        SharedPreferences sp = activity.getSharedPreferences(STORAGE_PREFS, Context.MODE_PRIVATE);
        String v = sp.getString(key, null);
        JSONObject r = new JSONObject().put("ok", true).put("value", v == null ? JSONObject.NULL : v);
        return r.toString();
    }

    private String handleStorageSet(JSONObject args) throws Exception {
        String key = args.optString("key", "");
        String value = args.optString("value", "");
        if (key.isEmpty()) return jsonErr("missing key");
        activity.getSharedPreferences(STORAGE_PREFS, Context.MODE_PRIVATE)
                .edit().putString(key, value).apply();
        return jsonOk();
    }

    private String handleStorageRemove(JSONObject args) throws Exception {
        String key = args.optString("key", "");
        if (key.isEmpty()) return jsonErr("missing key");
        activity.getSharedPreferences(STORAGE_PREFS, Context.MODE_PRIVATE)
                .edit().remove(key).apply();
        return jsonOk();
    }

    private String handleToast(JSONObject args) {
        String msg = args.optString("message", "");
        int dur = args.optInt("duration", 0) >= 1 ? Toast.LENGTH_LONG : Toast.LENGTH_SHORT;
        if (msg.isEmpty()) return jsonErr("missing message");
        final CharSequence m = msg;
        final int d = dur;
        runOnUi(() -> GlassToast.show(activity, m, d));
        return jsonOk();
    }

    private String handleOpenExternal(JSONObject args) {
        String url = args.optString("url", "");
        if (url.isEmpty()) return jsonErr("missing url");
        // 安全: 仅允许 http/https, 拒绝 intent: / file: / content: / 自定义 scheme,
        // 防 JS 桥被滥用打开任意 Intent(开放重定向 / 启动敏感组件).
        Uri u = Uri.parse(url);
        String scheme = u.getScheme();
        if (scheme == null
                || (!scheme.equalsIgnoreCase("http") && !scheme.equalsIgnoreCase("https"))) {
            return jsonErr("invalid scheme (only http/https allowed)");
        }
        try {
            Intent it = new Intent(Intent.ACTION_VIEW, u);
            it.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            // 显式清组件/选择器, 避免被构造成定向到特定组件
            it.setComponent(null);
            it.setSelector(null);
            activity.startActivity(it);
            return jsonOk();
        } catch (Exception e) {
            return jsonErr(e.getMessage());
        }
    }

    private String handleKeepScreenOn(JSONObject args) {
        boolean on = args.optBoolean("on", false);
        runOnUi(() -> {
            if (on) activity.getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            else activity.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        });
        return jsonOk();
    }

    private String handleGetNetworkStatus() throws Exception {
        ConnectivityManager cm = (ConnectivityManager) activity.getSystemService(Context.CONNECTIVITY_SERVICE);
        String type = "none";
        boolean connected = false;
        if (cm != null) {
            android.net.Network nw = cm.getActiveNetwork();
            if (nw != null) {
                NetworkCapabilities caps = cm.getNetworkCapabilities(nw);
                if (caps != null) {
                    connected = caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET);
                    if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) type = "wifi";
                    else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) type = "cellular";
                    else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) type = "ethernet";
                    else type = "other";
                }
            }
        }
        JSONObject r = new JSONObject().put("ok", true).put("connected", connected).put("type", type);
        return r.toString();
    }

    private String handlePlayVideo(JSONObject args) throws Exception {
        String url = args.optString("url", "");
        String title = args.optString("title", "");
        if (url.isEmpty()) return jsonErr("missing url");
        Intent it = new Intent(activity, PlayerActivity.class);
        it.putExtra(PlayerActivity.EXTRA_URL, url);
        it.putExtra(PlayerActivity.EXTRA_TITLE, title);
        runOnUi(() -> activity.startActivity(it));
        return jsonOk();
    }

    private String handlePlayPlaylist(JSONObject cfg) throws Exception {
        // 多源模式 (v3): cfg.sources = [{id, name, episodes:[{url,title}]}], cfg.currentSourceId
        JSONArray sources = cfg.optJSONArray("sources");
        Intent it = new Intent(activity, PlayerActivity.class);
        if (sources != null && sources.length() > 0) {
            // sanity: 至少一源有 episodes
            boolean any = false;
            for (int i = 0; i < sources.length(); i++) {
                JSONArray eps = sources.getJSONObject(i).optJSONArray("episodes");
                if (eps != null && eps.length() > 0) { any = true; break; }
            }
            if (!any) return jsonErr("all sources empty");
            it.putExtra(PlayerActivity.EXTRA_SOURCES_JSON, sources.toString());
            it.putExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID, cfg.optString("currentSourceId", ""));
        } else {
            // 单源模式 (v2 兼容): cfg.episodes
            JSONArray eps = cfg.optJSONArray("episodes");
            if (eps == null || eps.length() == 0) return jsonErr("missing episodes/sources");
            ArrayList<String> urls = new ArrayList<>(eps.length());
            ArrayList<String> titles = new ArrayList<>(eps.length());
            for (int i = 0; i < eps.length(); i++) {
                JSONObject ep = eps.getJSONObject(i);
                String u = ep.optString("url", "");
                if (u.isEmpty()) continue;
                urls.add(u);
                titles.add(ep.optString("title", ""));
            }
            if (urls.isEmpty()) return jsonErr("no valid url in playlist");
            it.putStringArrayListExtra(PlayerActivity.EXTRA_PLAYLIST_URLS, urls);
            it.putStringArrayListExtra(PlayerActivity.EXTRA_PLAYLIST_TITLES, titles);
        }
        it.putExtra(PlayerActivity.EXTRA_START_INDEX, cfg.optInt("startIndex", 0));
        it.putExtra(PlayerActivity.EXTRA_RESUME_MS, (long) (cfg.optDouble("resumeAtSec", 0) * 1000));
        it.putExtra(PlayerActivity.EXTRA_SKIP_INTRO_MS, (long) cfg.optInt("skipIntroSec", 0) * 1000L);
        it.putExtra(PlayerActivity.EXTRA_SKIP_OUTRO_MS, (long) cfg.optInt("skipOutroSec", 0) * 1000L);
        it.putExtra(PlayerActivity.EXTRA_AUTO_NEXT, cfg.optBoolean("autoNext", true));
        it.putExtra(PlayerActivity.EXTRA_FILM_ID, cfg.optString("filmId", ""));
        it.putExtra(PlayerActivity.EXTRA_FILM_NAME, cfg.optString("filmName", ""));
        // 方案B: 代理 base, 原生据此对原始 m3u8 每集自行包装代理过滤
        it.putExtra(PlayerActivity.EXTRA_PROXY_BASE, cfg.optString("proxyBase", ""));
        runOnUi(() -> activity.startActivity(it));
        return jsonOk();
    }

    private String handleStopPlayer() {
        PlayerActivity.stopRunningInstance();
        return jsonOk();
    }

    private String handleSetPlayerSpeed(JSONObject args) throws Exception {
        float speed = (float) args.optDouble("speed", 1.0);
        PlayerActivity.setSpeedOnRunningInstance(speed);
        return jsonOk();
    }

    private String handleCheckUpdate() {
        if (activity instanceof MainActivity) {
            runOnUi(((MainActivity) activity)::checkUpdateExposed);
        }
        return jsonOk();
    }

    private String handleOpenServerSettings() {
        if (activity instanceof MainActivity) {
            runOnUi(((MainActivity) activity)::promptServerUrlExposed);
        }
        return jsonOk();
    }

    private String handleSetAuthToken(JSONObject args) throws Exception {
        String token = args.optString("token", "");
        UpdateChecker.saveAuthToken(activity, token);
        return jsonOk();
    }

    private String handleClearAuthToken() {
        UpdateChecker.saveAuthToken(activity, "");
        return jsonOk();
    }

    /**
     * 桥接双向健康检查 — web 调 invoke('echoTest', {nonce, t}), native 200ms 后用
     * sendEvent('echoReply', {nonce, t, replyAt}) 回吐. web 侧记录是否收到 reply →
     * 一次调用验证两个方向都通.
     */
    private String handleEchoTest(JSONObject args) throws Exception {
        final String nonce = args.optString("nonce", "");
        final long t = args.optLong("t", 0L);
        if (activity instanceof MainActivity) {
            final MainActivity ma = (MainActivity) activity;
            // 200ms 后回吐, 避免与同步 invoke 返回竞争
            new Handler(Looper.getMainLooper()).postDelayed(() -> {
                try {
                    JSONObject p = new JSONObject()
                            .put("nonce", nonce)
                            .put("t", t)
                            .put("replyAt", System.currentTimeMillis());
                    sendEvent(ma.webViewRef, "echoReply", p);
                } catch (Exception ignore) {}
            }, 200);
        }
        return jsonOk("ackAt", System.currentTimeMillis());
    }

    // ========================== Helpers ==========================

    private void runOnUi(Runnable r) {
        new Handler(Looper.getMainLooper()).post(r);
    }

    private static String jsonOk() {
        return "{\"ok\":true}";
    }

    private static String jsonOk(String key, Object value) {
        try {
            return new JSONObject().put("ok", true).put(key, value).toString();
        } catch (Exception e) {
            return "{\"ok\":true}";
        }
    }

    private static String jsonErr(String msg) {
        try {
            return new JSONObject().put("ok", false).put("error", msg == null ? "" : msg).toString();
        } catch (Exception e) {
            return "{\"ok\":false}";
        }
    }

    // ========================== 反向事件 (native → web) ==========================

    /** native 内部调用. eventName 例: 'keyMenu' / 'playerEnded' / 'appResume'. payload 可为 null. */
    public static void sendEvent(WebView webView, String eventName, JSONObject payload) {
        if (webView == null || eventName == null) return;
        final String payloadStr = payload == null ? "null" : payload.toString();
        final String js = EVENT_DISPATCHER_JS + " && " + EVENT_DISPATCHER_JS
                + "(" + JSONObject.quote(eventName) + ", " + payloadStr + ");";
        webView.post(() -> webView.evaluateJavascript(js, null));
    }
}
