package com.jerocine.app;

/**
 * 设置抽屉的**纯逻辑**(不碰 View / Context) —— 单独成类是为了能在本机跑 JVM 单测
 * (见 app/src/test/java/com/jerocine/app/SettingsDrawerLogicTest.java)。
 *
 * 抽屉本身的构建是编程式 View 组装(MainActivity.buildSettingsDrawer), 只能靠编译 + 真机验收;
 * 但下面这几处"取值/拼接"逻辑是最容易出错的边界(显示模式三态循环、深链 URL 拼接),
 * 所以明确拆出来覆盖。
 */
final class SettingsDrawerLogic {

    /**
     * 显示模式取值: 下发/读回的都是 WebView localStorage 的 **jc-native-mode**
     * (前端 useViewMode.NATIVE_MODE_KEY) —— 只有抽屉会写这个键; 缺省(=null) 表示
     * "没选过", 壳内按默认强制 TV。
     *
     * 注意**不是** jc-mode: 那个键被壳内 detectMode() 刻意忽略(历史残留值会让 TV 页面
     * 按桌面布局横向溢出), 前端为此另开了来源唯一的壳专用键。
     */
    static final String MODE_TV = "tv";
    static final String MODE_DESKTOP = "desktop";

    private SettingsDrawerLogic() {}

    /**
     * 显示模式三态循环: TV → 桌面 → 自动 → TV …（原三个按钮合并成一行, 见抽屉重设计 §4.2）
     * @param current 当前值; null/空 表示"自动检测"
     * @return 下一个值({@link #MODE_TV} / {@link #MODE_DESKTOP} / null=自动)
     */
    static String nextDisplayMode(String current) {
        if (MODE_TV.equals(current)) return MODE_DESKTOP;
        if (MODE_DESKTOP.equals(current)) return null; // 自动
        return MODE_TV;
    }

    /**
     * window.__jcSetMode 的入参: 只有 tv / desktop 是有效值, 其余(null=自动 / 空 / 非法)
     * 一律下发 "auto" ⇒ 前端清除覆盖、回到壳内默认的强制 TV。
     * (前端 normalizeNativeMode 也是同一套归一化, 两边一致才能保证"自动"确实生效。)
     */
    static String jsModeArg(String mode) {
        if (MODE_TV.equals(mode) || MODE_DESKTOP.equals(mode)) return mode;
        return "auto";
    }

    /** 显示模式行右侧的状态文案 */
    static String displayModeLabel(String current) {
        if (MODE_TV.equals(current)) return "TV";
        if (MODE_DESKTOP.equals(current)) return "桌面";
        return "自动";
    }

    /** 循环切换后给用户的 toast 文案 */
    static String displayModeToast(String next) {
        if (MODE_TV.equals(next)) return "已设为 TV 模式";
        if (MODE_DESKTOP.equals(next)) return "已设为桌面模式";
        return "已清除模式 · 自动检测";
    }

    /**
     * SPA 设置页深链: 原生抽屉里的"账号设置 / 跳过片头片尾"都要跳到 web 的对应分组。
     * 末尾斜杠必须归一化(服务器地址允许用户填成 http://ip/ 或 http://ip), 否则会拼出
     * "//settings?group=..." 这种被个别源站当异常路径的 URL。
     *
     * @param baseUrl 服务器地址(可带尾斜杠)
     * @param group   分组 id, 如 account / play; 空则不带 query
     */
    static String spaSettingsUrl(String baseUrl, String group) {
        String base = baseUrl == null ? "" : baseUrl.trim();
        while (base.endsWith("/")) base = base.substring(0, base.length() - 1);
        String url = base + "/settings";
        if (group != null && !group.isEmpty()) url += "?group=" + group;
        return url;
    }
}
