package com.jerocine.app;

/**
 * 设置抽屉的**纯逻辑**(不碰 View / Context) —— 单独成类是为了能在本机跑 JVM 单测
 * (见 app/src/test/java/com/jerocine/app/SettingsDrawerLogicTest.java)。
 *
 * 抽屉本身的构建是编程式 View 组装(MainActivity.buildSettingsDrawer), 只能靠编译 + 真机验收;
 * 但下面这几处"取值/拼接"逻辑是最容易出错的边界(显示模式三态循环、账号快照解析),
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

    // ==================== 账号行(2026-10-10 抽屉改版) ====================
    // 数据源 = web 端 window.__jcAuth.user() 返回的 JSON 字符串 {"loggedIn":true,"name":"昵称"}。
    // 之前"跳 SPA /settings"的入口已随该页删除而移除。

    /** 解析后的账号快照(.loggedIn / .name; 未登录时 name 为空串) */
    static final class AccountSnapshot {
        final boolean loggedIn;
        final String name;

        AccountSnapshot(boolean loggedIn, String name) {
            this.loggedIn = loggedIn;
            this.name = name;
        }
    }

    static final AccountSnapshot ACCOUNT_LOGGED_OUT = new AccountSnapshot(false, "");

    /**
     * 解析 window.__jcAuth.user() 的返回。
     *
     * **手写解析而不是 org.json** —— unit 测试的 android.jar 里 org.json 是 stub(一调就抛),
     * 引它等于让这段逻辑测不了。报文是我们自己的钩子产出的固定形状, 手写足够:
     * 取 "loggedIn" 后的 true/false, 再取 "name" 后的 JSON 字符串(处理常见转义)。
     *
     * @param json 形如 {"loggedIn":true,"name":"x"}; null/"null"/空/异常报文一律归一成未登录
     */
    static AccountSnapshot parseAccountSnapshot(String json) {
        if (json == null) return ACCOUNT_LOGGED_OUT;
        String s = json.trim();
        if (s.isEmpty() || "null".equals(s)) return ACCOUNT_LOGGED_OUT;
        boolean loggedIn = containsField(s, "loggedIn") && extractBoolean(s, "loggedIn");
        String name = extractString(s, "name");
        return new AccountSnapshot(loggedIn, name);
    }

    /** 账号行主标题: 已登录显示昵称, 未登录固定"未登录" */
    static String accountTitle(AccountSnapshot snapshot) {
        if (snapshot == null || !snapshot.loggedIn || snapshot.name == null
                || snapshot.name.isEmpty()) {
            return "未登录";
        }
        return snapshot.name;
    }

    /** 账号行副标题 */
    static String accountSubtitle(AccountSnapshot snapshot) {
        if (snapshot == null || !snapshot.loggedIn) {
            return "登录后收藏 / 历史 / 跳过设置多端同步";
        }
        return "登录中 · 点右侧按钮退出";
    }

    /** 账号行右侧动作文案(也是动作语义: 登录 / 退出) */
    static String accountActionText(AccountSnapshot snapshot) {
        if (snapshot != null && snapshot.loggedIn) return "退出";
        return "登录";
    }

    // ---------- 解析小工具(仅面向我们自己钩子产出的 JSON 形状) ----------

    /** 找 "key": 并返回其值片段的起点(跳过冒号与空白); 找不到返回 -1 */
    private static int valueStart(String s, String key) {
        String needle = "\"" + key + "\"";
        int k = s.indexOf(needle);
        if (k < 0) return -1;
        int i = s.indexOf(':', k + needle.length());
        if (i < 0) return -1;
        i++;
        while (i < s.length() && Character.isWhitespace(s.charAt(i))) i++;
        return i < s.length() ? i : -1;
    }

    private static boolean containsField(String s, String key) {
        return valueStart(s, key) >= 0;
    }

    private static boolean extractBoolean(String s, String key) {
        int i = valueStart(s, key);
        return s.startsWith("true", i);
    }

    /** 解析 "key":"value" 的字符串值, 处理 \\\" \\\\ \\n 等常见转义; 非字符串值返回空串 */
    private static String extractString(String s, String key) {
        int i = valueStart(s, key);
        if (i < 0 || s.charAt(i) != '"') return "";
        StringBuilder sb = new StringBuilder();
        for (int j = i + 1; j < s.length(); j++) {
            char c = s.charAt(j);
            if (c == '\\') {
                if (++j >= s.length()) break;
                char e = s.charAt(j);
                switch (e) {
                    case '"' -> sb.append('"');
                    case '\\' -> sb.append('\\');
                    case '/' -> sb.append('/');
                    case 'n' -> sb.append('\n');
                    case 't' -> sb.append('\t');
                    case 'r' -> sb.append('\r');
                    case 'u' -> {
                        if (j + 4 < s.length()) {
                            try {
                                sb.append((char) Integer.parseInt(s.substring(j + 1, j + 5), 16));
                                j += 4;
                            } catch (NumberFormatException ignored) {
                                // 非法 \\u 转义: 原样丢弃(报文是我们自己产出的, 不该出现)
                            }
                        }
                    }
                    default -> sb.append(e);
                }
            } else if (c == '"') {
                break;
            } else {
                sb.append(c);
            }
        }
        return sb.toString();
    }
}
