package com.jerocine.player.download;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.os.Build;

import androidx.annotation.Nullable;
import androidx.core.app.NotificationCompat;
import androidx.media3.exoplayer.offline.Download;
import androidx.media3.exoplayer.offline.DownloadManager;
import androidx.media3.exoplayer.offline.DownloadService;
import androidx.media3.exoplayer.scheduler.Scheduler;

import com.jerocine.player.JerocinePlayer;
import com.jerocine.player.R;

import java.util.List;
import java.util.Locale;

/**
 * 下载前台服务 — Media3 {@link DownloadService} 子类.
 *
 * <p>两壳 manifest 各自声明(遵循 PlayerActivity 先例); 服务在下载入队时被
 * {@link DownloadEngine#enqueue} 通过 {@link #sendAddDownload} 拉起, 前台通知实时显示任务数与进度.
 */
public class JerocineDownloadService extends DownloadService {

    private static final int FOREGROUND_NOTIFICATION_ID = 1001;
    private static final String CHANNEL_ID = "jerocine_downloads";

    public JerocineDownloadService() {
        super(FOREGROUND_NOTIFICATION_ID, 1000L, CHANNEL_ID,
                R.string.download_channel_name, R.string.download_channel_description);
        // ⚠️ 严禁在构造函数里碰 Context 方法(getString/getResources 等)!
        // 系统经 Class.newInstance() 实例化 Service 时 attachBaseContext() 还没跑,
        // baseContext == null → getString() 直接 NPE → "Unable to create service"
        // → **整个进程崩溃** → 界面退回播放页, 而且下载请求丢失、任务永远卡"排队中"。
        // (2026-10-08 pad 实测: 每次首次点「开始下载」必崩, dropbox 三条同因记录。)
        // 通知渠道创建挪到 onCreate() —— 此时 Context 已就绪。
    }

    @Override
    public void onCreate() {
        super.onCreate();
        createChannel();
    }

    private void createChannel() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                NotificationChannel ch = new NotificationChannel(CHANNEL_ID,
                        getString(R.string.download_channel_name),
                        NotificationManager.IMPORTANCE_LOW);
                ch.setDescription(getString(R.string.download_channel_description));
                ch.setShowBadge(false);
                NotificationManager nm = getSystemService(NotificationManager.class);
                if (nm != null) nm.createNotificationChannel(ch);
            }
        } catch (Exception e) {
            // 渠道创建失败不致命(通知可能不展示), 但绝不能让它把服务/进程带崩
            android.util.Log.w("JerocineDownloadService", "createChannel failed", e);
        }
    }

    @Override
    protected DownloadManager getDownloadManager() {
        DownloadEngine engine = DownloadEngine.existing();
        if (engine != null) return engine.manager();
        // 进程被杀后系统恢复服务: 静态单例已丢 → 重建。
        // 注意这里**不能传空串**: 空 proxyBase 会让广告过滤直接返回 null,
        // 导致重启后所有在途下载必然失败("广告过滤失败")且无法自愈。
        // 兜底取壳层配置的默认代理地址(JerocinePlayer.defaultProxyBase)。
        // 重建会跑 markInterruptedAsPaused(把残留 DOWNLOADING 标 PAUSED), 与服务恢复语义一致。
        return DownloadEngine.get(this, JerocinePlayer.defaultProxyBase()).manager();
    }

    @Override
    protected Scheduler getScheduler() {
        return null; // 不依赖网络/充电条件调度: 用户手动触发, 前台跑
    }

    @Override
    protected Notification getForegroundNotification(
            List<Download> downloads, int notMetRequirements) {
        int active = 0;
        long downloaded = 0;
        long total = 0;
        int done = 0;
        for (Download d : downloads) {
            if (d.state == Download.STATE_DOWNLOADING || d.state == Download.STATE_QUEUED) {
                active++;
                downloaded += d.getBytesDownloaded();
                total += d.contentLength;
            } else if (d.state == Download.STATE_COMPLETED) {
                done++;
            }
        }
        String title;
        if (active > 0) {
            title = active + " 个任务下载中" + (done > 0 ? " · " + done + " 个已完成" : "");
        } else if (done > 0) {
            title = "下载已完成 (" + done + ")";
        } else {
            title = "影视下载";
        }
        String text = active > 0 && total > 0
                ? String.format(Locale.US, "%.1f%%", downloaded * 100f / total)
                : (done > 0 ? "点击查看下载管理" : "暂无任务");
        NotificationCompat.Builder b = new NotificationCompat.Builder(this, CHANNEL_ID)
                .setSmallIcon(R.drawable.ic_download)
                .setContentTitle(title)
                .setContentText(text)
                .setOngoing(active > 0)
                .setOnlyAlertOnce(true)
                .setPriority(NotificationCompat.PRIORITY_LOW);
        // 点通知 → 打开下载管理页(壳 manifest 声明 DownloadActivity)
        try {
            Intent i = new Intent(this, DownloadActivity.class);
            PendingIntent pi = PendingIntent.getActivity(this, 0, i,
                    PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
            b.setContentIntent(pi);
        } catch (Exception ignore) {
        }
        return b.build();
    }
}
