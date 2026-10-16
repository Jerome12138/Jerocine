package com.jerocine.player.data;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.os.Build;
import android.util.Log;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.Arrays;
import java.util.Date;
import java.util.Locale;

/**
 * 全局未捕获异常落盘（2026-10-15 补关键异常记录缺口）。
 *
 * <p>背景：坚果投影(Android 6)两起 NCDFE 崩溃，用户只能手动 adb logcat 取证——
 * 装在远端/用户手里的设备崩了就丢了现场。本类在进程启动时挂
 * {@link Thread#setDefaultUncaughtExceptionHandler}：崩溃瞬间把报告写进
 * {@code filesDir/crashes/}（环形保留 {@value #KEEP_COUNT} 份），随后
 * <b>交回系统原 handler</b>（系统崩溃对话框/杀进程流程完全不变）。</p>
 *
 * <p>报告可在「设置 → 设备诊断」里看到摘要（两壳共用 {@code JcDeviceDiagnostics.show()}），
 * 且诊断文本本就"可一键复制发给维护者"——崩溃摘要随诊断文本一起出去，远程取证闭环。</p>
 *
 * <p>设计约束：</p>
 * <ul>
 *   <li>写文件/读版本号全程 try-catch —— <b>handler 里再抛异常会取代真实崩溃现场</b>；</li>
 *   <li>只用 API 21+ 可用接口（本工程 minSdk 21 无 desugaring，禁 java.util.function）；</li>
 *   <li>纯逻辑（{@link #formatReport} / {@link #pruneSelection} / {@link #summaryFrom}）与
 *       文件 IO（{@link #writeReport}）拆开，JVM 单测不碰 Android 类。</li>
 * </ul>
 */
public final class CrashLog {

    /** 崩溃报告目录名（filesDir 下）。 */
    static final String DIR_NAME = "crashes";
    /** 环形保留份数 —— 只留最近的，避免久置设备攒一堆旧报告。 */
    static final int KEEP_COUNT = 5;
    private static final String TAG = "JcCrashLog";

    private CrashLog() {}

    /** 进程启动时调用（幂等：重复 install 不叠加 handler）。 */
    public static void install(Context context) {
        Thread.UncaughtExceptionHandler previous = Thread.getDefaultUncaughtExceptionHandler();
        if (previous instanceof CrashHandler) {
            return; // 已装过（App 重建等场景），别链两层
        }
        final Context app = context.getApplicationContext();
        Thread.setDefaultUncaughtExceptionHandler(new CrashHandler(app, previous));
    }

    private static final class CrashHandler implements Thread.UncaughtExceptionHandler {
        private final Context app;
        private final Thread.UncaughtExceptionHandler previous;

        CrashHandler(Context app, Thread.UncaughtExceptionHandler previous) {
            this.app = app;
            this.previous = previous;
        }

        @Override
        public void uncaughtException(Thread t, Throwable e) {
            try {
                File dir = new File(app.getFilesDir(), DIR_NAME);
                writeReport(dir, System.currentTimeMillis(), t == null ? "?" : t.getName(),
                        deviceLine(), appLine(app), Log.getStackTraceString(e));
            } catch (Throwable ignored) {
                // 崩溃 handler 里绝不能再抛 —— 记不了盘就让系统流程照常走
            }
            if (previous != null) {
                previous.uncaughtException(t, e);
            }
        }
    }

    // ---- 报告格式（纯逻辑，单测钉住） ----

    /** 生成报告文本。deviceLine / appLine 为描述性行，允许为 null（缺信息好过崩溃）。 */
    public static String formatReport(long timeMs, String threadName,
                                      String deviceLine, String appLine, String stack) {
        StringBuilder sb = new StringBuilder();
        sb.append("Jerocine 崩溃报告\n");
        sb.append("时间: ").append(formatTime(timeMs)).append('\n');
        sb.append("线程: ").append(threadName == null ? "?" : threadName).append('\n');
        if (deviceLine != null) {
            sb.append("设备: ").append(deviceLine).append('\n');
        }
        if (appLine != null) {
            sb.append("应用: ").append(appLine).append('\n');
        }
        sb.append("---- 栈 ----\n");
        sb.append(stack == null || stack.trim().isEmpty() ? "(无栈)" : stack.trim());
        return sb.toString();
    }

    private static String formatTime(long timeMs) {
        return new SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.US).format(new Date(timeMs));
    }

    /** 设备描述行（诊断/报告共用口径）。 */
    public static String deviceLine() {
        return Build.MANUFACTURER + " " + Build.MODEL + " (API " + Build.VERSION.SDK_INT + ")";
    }

    /** 应用版本行：包名 versionName(versionCode)；读不到给 "unknown"。 */
    public static String appLine(Context context) {
        try {
            PackageInfo pi = context.getPackageManager()
                    .getPackageInfo(context.getPackageName(), 0);
            return context.getPackageName() + " " + pi.versionName
                    + "(" + pi.versionCode + ")";
        } catch (Throwable t) {
            return context.getPackageName() + " unknown";
        }
    }

    /**
     * 从报告全文提取诊断页用的一行摘要：「时间行 | 栈首行(异常类型)」，超长截断。
     * 纯函数；内容不像报告（读坏了/半截）时返回 null。
     */
    public static String summaryFrom(String content) {
        if (content == null) {
            return null;
        }
        String timeLine = null;
        String exceptionLine = null;
        String[] lines = content.split("\n");
        for (int i = 0; i < lines.length; i++) {
            String line = lines[i].trim();
            if (timeLine == null && line.startsWith("时间: ")) {
                timeLine = line.substring("时间: ".length());
            }
            if (exceptionLine == null && line.startsWith("---- 栈 ----")) {
                for (int j = i + 1; j < lines.length; j++) {
                    String first = lines[j].trim();
                    if (!first.isEmpty() && !".".equals(first)) {
                        exceptionLine = first;
                        break;
                    }
                }
                break;
            }
        }
        if (timeLine == null) {
            return null;
        }
        String summary = timeLine + (exceptionLine == null ? "" : " | " + exceptionLine);
        return summary.length() > 160 ? summary.substring(0, 160) + "…" : summary;
    }

    // ---- 文件 IO（Context 薄封装 + File 层可 JVM 单测） ----

    /**
     * 写一份报告到 dir（crash-<时间戳>.txt），并裁掉超出 {@value #KEEP_COUNT} 份的旧报告。
     * dir 不存在会创建；创建/写失败抛 IOException（由调用方决定兜底）。
     */
    static void writeReport(File dir, long timeMs, String threadName,
                            String deviceLine, String appLine, String stack) throws IOException {
        if (!dir.exists() && !dir.mkdirs() && !dir.isDirectory()) {
            throw new IOException("cannot create " + dir);
        }
        String name = "crash-" + timeMs + ".txt";
        byte[] bytes = formatReport(timeMs, threadName, deviceLine, appLine, stack)
                .getBytes(StandardCharsets.UTF_8);
        FileOutputStream out = new FileOutputStream(new File(dir, name));
        try {
            out.write(bytes);
        } finally {
            out.close();
        }
        prune(dir, KEEP_COUNT);
    }

    /** 环形裁剪：按文件名升序（crash-<时间戳> 定宽，字典序=时间序）保留最新 keep 份。 */
    static void prune(File dir, int keep) {
        String[] names = dir.list();
        if (names == null) {
            return;
        }
        Arrays.sort(names);
        for (String victim : pruneSelection(names, keep)) {
            // 删旧报告失败无所谓 —— 下一份崩溃再试
            new File(dir, victim).delete();
        }
    }

    /**
     * 纯函数：从按升序排好的文件名里挑出要删的（超出 keep 的最老一批）。
     * names 里混入非本类文件（子目录/脏文件）时只考虑 crash-*.txt。
     */
    static String[] pruneSelection(String[] sortedNames, int keep) {
        int crashFiles = 0;
        for (String n : sortedNames) {
            if (isCrashFile(n)) {
                crashFiles++;
            }
        }
        int toDelete = crashFiles - keep;
        if (toDelete <= 0) {
            return new String[0];
        }
        String[] victims = new String[toDelete];
        int idx = 0;
        for (String n : sortedNames) {
            if (idx >= toDelete) {
                break;
            }
            if (isCrashFile(n)) {
                victims[idx++] = n;
            }
        }
        return victims;
    }

    private static boolean isCrashFile(String name) {
        return name.startsWith("crash-") && name.endsWith(".txt");
    }

    /** 诊断页用：最近一份崩溃的摘要；无崩溃/读失败返回 null（绝不抛）。 */
    public static String latestSummary(Context context) {
        try {
            File dir = new File(context.getFilesDir(), DIR_NAME);
            String[] names = dir.list();
            if (names == null || names.length == 0) {
                return null;
            }
            Arrays.sort(names);
            File latest = new File(dir, names[names.length - 1]);
            return summaryFrom(readAll(latest));
        } catch (Throwable t) {
            Log.w(TAG, "latestSummary failed", t);
            return null;
        }
    }

    private static String readAll(File f) throws IOException {
        FileInputStream in = new FileInputStream(f);
        try {
            byte[] buf = new byte[(int) Math.min(f.length(), 64 * 1024)];
            int read = in.read(buf);
            return new String(buf, 0, Math.max(read, 0), StandardCharsets.UTF_8);
        } finally {
            in.close();
        }
    }
}
