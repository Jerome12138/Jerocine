package com.jerocine.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;

import androidx.core.content.FileProvider;

import org.json.JSONObject;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.concurrent.TimeUnit;

import okhttp3.Call;
import okhttp3.Callback;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;

/**
 * UpdateChecker - APK 自升级.
 *
 * 流程:
 *   1) check(userTriggered) 异步 GET <server>/api/version/latest
 *      Authorization: Bearer <jwt>  (jwt 由 JerocineBridge.setAuthToken 写到 SharedPreferences,
 *      未登录 = 不带 header → 后端按 stable 返)
 *   2) 比较 remoteCode vs BuildConfig.VERSION_CODE
 *   3) remote <= 本地: userTriggered 提示"已是最新版", 否则静默
 *   4) remote > 本地: 弹 AlertDialog
 *        - force=true: 不能取消, 必须升 (取消按钮隐藏)
 *        - 否则: "暂不更新" 记 ignored_version 不再打扰直到下个版本
 *   5) "立即更新" → OkHttp 下载到 cache/updates/ → FileProvider 拿 content:// → ACTION_VIEW 启动安装器
 *
 * 触发时机:
 *   - MainActivity onCreate 后 postDelayed 5s 自动调 check(false)
 *   - 前端"检查更新"按钮 → JerocineBridge.checkUpdate() → check(true)
 */
public class UpdateChecker {

    private static final String PREFS = "jerocine_update";
    private static final String KEY_IGNORED_VERSION = "ignored_version_code";
    private static final String KEY_AUTH_TOKEN = "auth_token";
    private static final String VERSION_PATH = "/api/version/latest";

    private final Activity activity;
    private final OkHttpClient client;

    public UpdateChecker(Activity activity) {
        this.activity = activity;
        this.client = new OkHttpClient.Builder()
                .connectTimeout(8, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .build();
    }

    /** 由 JerocineBridge.setAuthToken / clearAuthToken 静态调用 */
    public static void saveAuthToken(Context ctx, String token) {
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                .edit()
                .putString(KEY_AUTH_TOKEN, token == null ? "" : token)
                .apply();
    }

    /**
     * 检查更新.
     * @param userTriggered true=用户在 UI 点的, 即使无更新也提示 + 忽略 ignored_version
     */
    public void check(boolean userTriggered) {
        String serverUrl = activity.getSharedPreferences(MainActivity.PREFS, Context.MODE_PRIVATE)
                .getString(MainActivity.KEY_SERVER_URL, null);
        if (serverUrl == null || serverUrl.isEmpty()) {
            if (userTriggered) toast("尚未设置服务器地址");
            return;
        }
        String versionUrl = serverUrl.replaceAll("/$", "") + VERSION_PATH;
        String token = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                .getString(KEY_AUTH_TOKEN, "");

        Request.Builder b = new Request.Builder().url(versionUrl);
        if (token != null && !token.isEmpty()) {
            b.header("Authorization", "Bearer " + token);
        }
        client.newCall(b.build()).enqueue(new Callback() {
            @Override
            public void onFailure(Call call, IOException e) {
                if (userTriggered) toast("检查更新失败: " + e.getMessage());
            }

            @Override
            public void onResponse(Call call, Response response) throws IOException {
                try {
                    if (!response.isSuccessful() || response.body() == null) {
                        if (userTriggered) toast("检查更新失败: HTTP " + response.code());
                        return;
                    }
                    JSONObject root = new JSONObject(response.body().string());
                    JSONObject data = root.optJSONObject("data");
                    if (data == null) {
                        if (userTriggered) toast("响应无版本数据");
                        return;
                    }
                    int remoteCode = data.optInt("versionCode", 0);
                    String remoteName = data.optString("versionName", "");
                    String apkUrl = data.optString("apkUrl", "");
                    String changelog = data.optString("changelog", "");
                    boolean force = data.optBoolean("force", false);
                    String channel = data.optString("channel", "stable");

                    int currentCode = BuildConfig.VERSION_CODE;
                    if (remoteCode <= currentCode) {
                        if (userTriggered) {
                            toast("已是最新版 v" + BuildConfig.VERSION_NAME);
                        }
                        return;
                    }

                    // 自动检查时, 用户点过"暂不更新"的版本号不再打扰
                    if (!userTriggered) {
                        int ignored = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                                .getInt(KEY_IGNORED_VERSION, 0);
                        if (ignored == remoteCode) return;
                    }

                    activity.runOnUiThread(() ->
                            showUpdateDialog(remoteCode, remoteName, apkUrl, changelog, force, channel));
                } catch (Exception e) {
                    if (userTriggered) toast("解析响应异常: " + e.getMessage());
                } finally {
                    try { response.close(); } catch (Exception ignore) {}
                }
            }
        });
    }

    private void showUpdateDialog(int remoteCode, String remoteName, String apkUrl,
                                  String changelog, boolean force, String channel) {
        if (apkUrl == null || apkUrl.isEmpty()) {
            toast("远程版本未配置 apkUrl, 暂不能更新");
            return;
        }
        String title = "发现新版本 v" + remoteName
                + ("beta".equalsIgnoreCase(channel) ? " (beta)" : "");
        String msg = (changelog == null || changelog.isEmpty())
                ? "建议尽快更新到最新版本以获得最佳体验"
                : changelog;
        if (force) msg = "[强制更新] " + msg;

        AlertDialog.Builder b = new AlertDialog.Builder(activity, R.style.GfPlayerDialog)
                .setTitle(title)
                .setMessage(msg)
                .setCancelable(!force)
                .setPositiveButton("立即更新", (d, w) -> downloadAndInstall(apkUrl, remoteName));
        if (!force) {
            b.setNegativeButton("暂不更新", (d, w) ->
                    activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                            .edit()
                            .putInt(KEY_IGNORED_VERSION, remoteCode)
                            .apply());
        }
        b.show();
    }

    private void downloadAndInstall(String apkUrl, String versionName) {
        File targetDir = new File(activity.getCacheDir(), "updates");
        if (!targetDir.exists()) targetDir.mkdirs();
        // 清旧 apk 避免占用空间
        File[] olds = targetDir.listFiles();
        if (olds != null) for (File f : olds) f.delete();
        File dst = new File(targetDir, "Jerocine-" + versionName + ".apk");
        toast("开始下载更新, 请稍候...");
        client.newCall(new Request.Builder().url(apkUrl).build())
                .enqueue(new Callback() {
                    @Override
                    public void onFailure(Call call, IOException e) {
                        toast("下载失败: " + e.getMessage());
                    }

                    @Override
                    public void onResponse(Call call, Response response) throws IOException {
                        try {
                            if (!response.isSuccessful() || response.body() == null) {
                                toast("下载失败: HTTP " + response.code());
                                return;
                            }
                            try (InputStream in = response.body().byteStream();
                                 FileOutputStream out = new FileOutputStream(dst)) {
                                byte[] buf = new byte[8192];
                                int n;
                                while ((n = in.read(buf)) > 0) {
                                    out.write(buf, 0, n);
                                }
                            }
                            // 安全: 安装前校验下载的 APK 签名证书与当前已装应用一致,
                            // 阻止被劫持/篡改的 apkUrl 装入异签名 APK(防 RCE).
                            if (!verifySameSigner(dst)) {
                                dst.delete();
                                toast("更新包签名校验失败, 已拒绝安装");
                                return;
                            }
                            activity.runOnUiThread(() -> installApk(dst));
                        } finally {
                            try { response.close(); } catch (Exception ignore) {}
                        }
                    }
                });
    }

    /**
     * 校验下载的 APK 与当前已安装应用是同一签名证书.
     * 防御: apkUrl 被中间人/配置劫持 → 下到异签名(攻击者)APK 被静默安装(RCE).
     * 同签名才允许装(同 Android 系统对 in-place 升级的要求, 也确保来源可信).
     */
    @SuppressWarnings("deprecation")
    private boolean verifySameSigner(File apk) {
        try {
            android.content.pm.PackageManager pm = activity.getPackageManager();
            String pkg = activity.getPackageName();

            byte[] installedSig;
            byte[] downloadedSig;

            if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.P) {
                // API 28+: 用 SigningInfo
                android.content.pm.PackageInfo cur = pm.getPackageInfo(
                        pkg, android.content.pm.PackageManager.GET_SIGNING_CERTIFICATES);
                android.content.pm.PackageInfo dl = pm.getPackageArchiveInfo(
                        apk.getAbsolutePath(), android.content.pm.PackageManager.GET_SIGNING_CERTIFICATES);
                if (cur == null || dl == null || cur.signingInfo == null || dl.signingInfo == null) {
                    return false;
                }
                installedSig = firstSig(cur.signingInfo.getApkContentsSigners());
                downloadedSig = firstSig(dl.signingInfo.getApkContentsSigners());
            } else {
                android.content.pm.PackageInfo cur = pm.getPackageInfo(
                        pkg, android.content.pm.PackageManager.GET_SIGNATURES);
                android.content.pm.PackageInfo dl = pm.getPackageArchiveInfo(
                        apk.getAbsolutePath(), android.content.pm.PackageManager.GET_SIGNATURES);
                if (cur == null || dl == null) return false;
                installedSig = firstSig(cur.signatures);
                downloadedSig = firstSig(dl.signatures);
            }
            if (installedSig == null || downloadedSig == null) return false;
            return java.security.MessageDigest.isEqual(installedSig, downloadedSig);
        } catch (Exception e) {
            // 任何异常一律视为校验失败, 拒绝安装
            return false;
        }
    }

    private byte[] firstSig(android.content.pm.Signature[] sigs) {
        if (sigs == null || sigs.length == 0) return null;
        try {
            return java.security.MessageDigest.getInstance("SHA-256")
                    .digest(sigs[0].toByteArray());
        } catch (Exception e) {
            return null;
        }
    }

    private void installApk(File apk) {
        try {
            Uri uri = FileProvider.getUriForFile(
                    activity,
                    activity.getPackageName() + ".fileprovider",
                    apk);
            Intent it = new Intent(Intent.ACTION_VIEW);
            it.setDataAndType(uri, "application/vnd.android.package-archive");
            it.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_ACTIVITY_NEW_TASK);
            activity.startActivity(it);
        } catch (Exception e) {
            toast("启动安装失败: " + e.getMessage());
        }
    }

    private void toast(String msg) {
        new Handler(Looper.getMainLooper())
                .post(() -> GlassToast.show(activity, msg));
    }
}
