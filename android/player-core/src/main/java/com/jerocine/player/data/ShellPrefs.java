package com.jerocine.player.data;

/**
 * 单包双壳共用的"壳层 prefs"契约 —— **唯一声明处**。
 *
 * 合并后 web 壳与原生壳运行在同一个 APK、读写**同一份** SharedPreferences，
 * 键名一旦漂移就会出现"切壳后设置/登录态错配"这类静默 bug（不会报错、只是行为怪）。
 * 所以文件名与键名只在此定义一次，其余位置一律引用这里。
 *
 * 下沉说明：合并前本契约在 :tvshell（Kotlin object），web 壳需要 import tv 壳的包；
 * 下沉到 core 后两个壳平等依赖（细则 §5.1：共性件下沉 player-core）。
 */
public final class ShellPrefs {

    /** SharedPreferences 文件名（两壳共用） */
    public static final String NAME = "jerocine_prefs";

    /**
     * 壳模式（auto / web / native）。
     *
     * <p><b>值域固定为小写</b> {@code "auto"} / {@code "web"} / {@code "native"}（缺省 auto）。
     * 写入端只有两处：设置抽屉的壳模式三态循环、web 壳 WebView 不可恢复时的强制降级；
     * 读取端为 {@code DeviceProbe.readUserMode} 与两壳的当前值展示。读取端对大小写不敏感
     * （兼容历史版本写入的枚举大写名），但**新写入一律小写** —— 2026-10-12 曾因此出现
     * "用户选了 Web/原生却被静默忽略"的真机回归。</p>
     */
    public static final String KEY_SHELL_MODE = "jc-shell-mode";

    /** 服务器地址（web 壳写入；原生壳经 ServiceLocator ⇄ TokenStore 收敛到同一份） */
    public static final String KEY_SERVER_URL = "server_url";

    private ShellPrefs() {}
}
