package com.jerocine.player;

/**
 * 集名身份键（与 server/internal/danmu/episode_key.go v2、web/src/utils/episodeKey.ts 同规,
 * 2026-11-07；黄金用例与 Go/TS 测试同一组 —— 三端同规, 任一侧改动必须同组通过）。
 *
 * <p>用途（方案-epid §5）: 切源对齐 —— SameEpisode′（日期双有不等→否决）+ 同键按序配对;
 * 期号经他源桥接归一（bridgeNum）。
 *
 * <p>触发口径: epid 只给「综艺/衍生形态」的集（日期/上中下/变体/栏目任一）;
 * 裸「第N集/期/话/回」与 E 圆号形态不触发（剧的常态, 位置兜底即可）。
 */
public final class EpisodeKey {
    /** YYYYMMDD（三格式归一后） */
    public final String date;
    /** 第N期/集/话/回/季/部（无则 0） */
    public final int num;
    /** E01/EP01（无则 0） */
    public final int eNum;
    /** 主部分 上/中/下 */
    public final String part;
    /** 次部分（紧邻主部分） */
    public final String sub;
    /** 有意义变体，字典序 `|` 连接 */
    public final String variant;
    /** 栏目/系列名（正片为空；连续片段提取，绝不跨段拼字） */
    public final String series;

    private EpisodeKey(String date, int num, int eNum, String part, String sub, String variant, String series) {
        this.date = date;
        this.num = num;
        this.eNum = eNum;
        this.part = part;
        this.sub = sub;
        this.variant = variant;
        this.series = series;
    }

    /** 解析集名（与 Go/TS v2 语义一致） */
    public static EpisodeKey parse(String title) {
        String s = title == null ? "" : title.trim();
        if (s.isEmpty()) return new EpisodeKey("", 0, 0, "", "", "", "");
        // 日期格式归一(先于一切提取): 破折/点分 → 8 位
        s = s.replaceAll("((?:19|20)\\d{2})[-.](\\d{2})[-.](\\d{2})", "$1$2$3");
        s = s.replaceAll("会员|未公开|未播|直拍|完整版|未删减", "");
        String date = "";
        java.util.regex.Matcher dm = java.util.regex.Pattern.compile("(?:19|20)\\d{6}").matcher(s);
        if (dm.find()) date = dm.group();
        int num = 0;
        java.util.regex.Matcher nm = java.util.regex.Pattern.compile("第\\s*(\\d{1,4})\\s*[期集话回季部]").matcher(s);
        if (nm.find()) num = Integer.parseInt(nm.group(1));
        int eNum = 0;
        java.util.regex.Matcher em = java.util.regex.Pattern.compile("(?:^|[^A-Za-z])(?:E|EP)\\s*0*(\\d{1,3})(?:\\D|$)", java.util.regex.Pattern.CASE_INSENSITIVE).matcher(s);
        if (em.find()) eNum = Integer.parseInt(em.group(1));
        String[] ps = partAndSub(s);
        String variant = "";
        java.util.regex.Matcher vm = java.util.regex.Pattern.compile("加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外").matcher(s);
        java.util.TreeSet<String> vs = new java.util.TreeSet<>();
        while (vm.find()) vs.add(vm.group());
        if (!vs.isEmpty()) variant = String.join("|", vs);
        return new EpisodeKey(date, num, eNum, ps[0], ps[1], variant, seriesOf(s));
    }

    /** 主/次部分: 最先出现的 上/中/下 + 紧邻（最多隔一个左括号）的第二个 */
    private static String[] partAndSub(String s) {
        int[] cps = s.codePoints().toArray();
        for (int i = 0; i < cps.length; i++) {
            int c = cps[i];
            if (c == '上' || c == '中' || c == '下') {
                String part = new String(Character.toChars(c));
                int j = i + 1;
                if (j < cps.length && isLeftBracket(cps[j])) j++;
                String sub = j < cps.length && (cps[j] == '上' || cps[j] == '中' || cps[j] == '下')
                        ? new String(Character.toChars(cps[j])) : "";
                return new String[]{part, sub};
            }
        }
        return new String[]{"", ""};
    }

    private static boolean isLeftBracket(int c) {
        return c == '（' || c == '(' || c == '【' || c == '[' || c == '《' || c == '「' || c == '『';
    }

    /**
     * 栏目名 v2: 跨度删除 + 最长纯汉字连续段 —— 连续性由构造保证（用户拍板：绝不跨段拼字）。
     * 删除动作以 \u0000 占位为界；只做两类边界截断：尾部 上/中/下、首个「季」处；要求 2~8 字。
     */
    private static String seriesOf(String s) {
        int ci = s.indexOf('：');
        int ci2 = s.indexOf(':');
        if (ci < 0 || (ci2 >= 0 && ci2 < ci)) ci = ci2 >= 0 ? ci2 : ci;
        if (ci >= 0) s = s.substring(0, ci);
        s = s.replaceAll("【[^】]*】|（[^）]*）|\\([^)]*\\)|《[^》]*》|「[^」]*」|『[^』]*』|\\[[^\\]]*\\]", "\u0000");
        s = s.replaceAll("第\\s*\\d{0,8}\\s*[期集话回季部]", "\u0000");
        s = s.replaceAll("(?:19|20)\\d{6}", "\u0000");
        s = s.replaceAll("第\\s*[\\d一二三四五六七八九十零两]+\\s*季", "\u0000");
        s = s.replaceAll("(?i)(?:^|[^A-Za-z])(?:E|EP)\\s*0*\\d{1,3}(?:\\D|$)", "\u0000");
        s = s.replaceAll("加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外", "\u0000");
        s = s.replaceAll("会员|未公开|未播|直拍|完整版|未删减", "\u0000");
        String best = "";
        for (String frag : s.split("[^\u4e00-\u9fff]+")) {
            if (frag.isEmpty()) continue;
            // 尾部 part 标记截断（上/中/下总在栏目名之后; 全 BMP 单字符, char 操作即可）
            while (!frag.isEmpty()) {
                char last = frag.charAt(frag.length() - 1);
                if (last != '上' && last != '中' && last != '下') break;
                frag = frag.substring(0, frag.length() - 1);
            }
            // 「季」边界截断
            int qi = frag.indexOf('季');
            if (qi >= 0) frag = frag.substring(0, qi);
            int len = frag.length();
            if (len < 2 || len > 8) continue;
            if (frag.equals("正片") || frag.equals("国语") || frag.equals("粤语") || frag.equals("原声")
                    || frag.equals("普通话") || frag.equals("双语")) continue;
            if (len > best.length()) best = frag;
        }
        return best;
    }

    /** 主/次部分相容: 一方缺失视为通配（App 常只有「中」而库里是「中（上）/（下）」） */
    public static boolean partCompatible(EpisodeKey a, EpisodeKey b) {
        if (!a.part.isEmpty() && !b.part.isEmpty() && !a.part.equals(b.part)) return false;
        if (!a.sub.isEmpty() && !b.sub.isEmpty() && !a.sub.equals(b.sub)) return false;
        return true;
    }

    /** SameEpisode′（日期双有不等 → 否决, 硬证据） */
    public static boolean sameEpisode(EpisodeKey a, EpisodeKey b) {
        if (!a.variant.equals(b.variant) || !a.series.equals(b.series)) return false;
        if (!a.date.isEmpty() && !b.date.isEmpty()) {
            if (!a.date.equals(b.date)) return false;
            return partCompatible(a, b);
        }
        if (a.num > 0 && b.num > 0 && a.num == b.num) return partCompatible(a, b);
        if (a.eNum > 0 && b.eNum > 0 && a.eNum == b.eNum) return true;
        return false;
    }

    /** 综艺/衍生形态判定（epid 只给触发的集; 裸「第N集/期」与 E圆号不触发） */
    public boolean triggered() {
        return !date.isEmpty() || !part.isEmpty() || !variant.isEmpty() || !series.isEmpty();
    }

    /** 5 段身份串: s|v|p|n|d, 缺值填 `-`（触发的集必有） */
    public String epid() {
        return "s=" + (series.isEmpty() ? "-" : series)
                + "|v=" + (variant.isEmpty() ? "-" : variant)
                + "|p=" + (part.isEmpty() ? "-" : part)
                + "|n=" + (num > 0 ? String.valueOf(num) : "-")
                + "|d=" + (date.isEmpty() ? "-" : date);
    }

    /** 返回期号被替换后的键（桥接归一用）; n<=0 返回原键。 */
    public EpisodeKey withNum(int n) {
        return n <= 0 ? this : new EpisodeKey(date, n, eNum, part, sub, variant, series);
    }

    /** 当前集 epid（触发的集必有; 期号缺失经桥接归一写入 n 段）。未触发返回 null（调用方回退位置兜底）。 */
    public static String epidFor(EpisodeKey cur, EpisodeKey[] others) {
        if (!cur.triggered()) return null;
        EpisodeKey k = cur;
        if (k.num == 0 && !k.date.isEmpty()) {
            k = k.withNum(bridgeNum(k, others));
        }
        return k.epid();
    }

    /**
     * 期号桥接（§5.1 归一）: 在其余源的键里找「期号+日期并存、日期相等、栏目/变体一致、part 相容」
     * 的集读其期号; 多个候选期号不一致 → 放弃（宁缺勿错）。返回 0 = 桥接失败。
     */
    public static int bridgeNum(EpisodeKey cur, EpisodeKey[] others) {
        java.util.Set<Integer> seen = new java.util.HashSet<>();
        if (others != null) {
            for (EpisodeKey k : others) {
                if (k == null || k.num == 0 || k.date.isEmpty() || !k.date.equals(cur.date)) continue;
                if (!k.variant.equals(cur.variant) || !k.series.equals(cur.series)) continue;
                if (!partCompatible(cur, k)) continue;
                seen.add(k.num);
            }
        }
        if (seen.size() == 1) {
            for (Integer n : seen) return n;
        }
        return 0;
    }
}
