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
    /**
     * **主部分（复合单段）**：上/中/下；若紧跟（最多隔一个左括号）还有 上/中/下 ⇒ 复合为「中上」「中下」「上中」等。
     * P0-1c（2026-11-10）：由 `part` + `sub` 两字段合并为**一个复合字段** —— 少一个字段、少一处三端同步面，
     * 且让 `epid` 串自足（`p=中上`）。比较规则见 {@link #partCompatible}（**前缀相容**）。
     */
    public final String part;
    /** 有意义变体，字典序 `|` 连接 */
    public final String variant;
    /**
     * **变体序号**（无则 0）：变体词后**紧跟**的数字 —— 「特辑2」⇒ (variant=特辑, variantNum=2)。
     * 必须与变体复合为身份：否则「特辑1..5」坍缩成同一 variant，L3a 组内不唯一 ⇒ 整组配不上
     * （桃花坞6 特辑 4~5 集丢弹幕的根因；P0-1a，2026-11-10 三端同规）。纯日期/长数字不取。
     */
    public final int variantNum;
    /** 栏目/系列名（正片为空；连续片段提取，绝不跨段拼字） */
    public final String series;

    private EpisodeKey(String date, int num, int eNum, String part, String variant, int variantNum, String series) {
        this.date = date;
        this.num = num;
        this.eNum = eNum;
        this.part = part;
        this.variant = variant;
        this.variantNum = variantNum;
        this.series = series;
    }

    /** 解析集名（与 Go/TS v2 语义一致） */
    public static EpisodeKey parse(String title) {
        String s = title == null ? "" : title.trim();
        if (s.isEmpty()) return new EpisodeKey("", 0, 0, "", "", 0, "");
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
        String part = parsePart(s);
        String variant = "";
        java.util.regex.Matcher vm = java.util.regex.Pattern.compile("加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外").matcher(s);
        java.util.TreeSet<String> vs = new java.util.TreeSet<>();
        while (vm.find()) vs.add(vm.group());
        if (!vs.isEmpty()) variant = String.join("|", vs);
        // 变体序号（P0-1a）: 变体词后**紧跟**的数字（最多隔一个左括号/空白）。
        //   特辑2 → 2 ／ 加更版1 → 0（隔着「版」）／ 特辑2026 → 0（避免吞日期）
        //   注: `(?:[^0-9]|$)` 消费一个后继字符做等价断言 —— 与 Go RE2（不支持 lookahead）同写法
        int variantNum = 0;
        java.util.regex.Matcher vnm = java.util.regex.Pattern
                .compile("(?:加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外)[\\s（(【\\[《「『]*(\\d{1,3})(?:[^0-9]|$)")
                .matcher(s);
        if (vnm.find()) variantNum = Integer.parseInt(vnm.group(1));
        return new EpisodeKey(date, num, eNum, part, variant, variantNum, seriesOf(s));
    }

    /**
     * 解析**主部分（复合）**：最先出现的 上/中/下；若紧跟（最多隔一个左括号）还有 ⇒ 拼成复合值「中上」。
     * 次部分只在紧邻时才算，避免把摘要正文当成分支标记。
     */
    private static String parsePart(String s) {
        int[] cps = s.codePoints().toArray();
        for (int i = 0; i < cps.length; i++) {
            int c = cps[i];
            if (c == '上' || c == '中' || c == '下') {
                StringBuilder out = new StringBuilder(new String(Character.toChars(c)));
                int j = i + 1;
                if (j < cps.length && isLeftBracket(cps[j])) j++;
                if (j < cps.length && (cps[j] == '上' || cps[j] == '中' || cps[j] == '下')) {
                    out.append(Character.toChars(cps[j]));
                }
                return out.toString();
            }
        }
        return "";
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

    /**
     * 主部分是否指向同一支 —— **前缀相容**（P0-1c 2026-11-10，替代旧「part 严格 + sub 通配」两段式）：
     * 一方为空 ⇒ 通配（App 常只有「中」而库里是「中（上）/（下）」）；短的是长的前缀 ⇒ 相容（`中` ⊂ `中上`）；
     * 互不为前缀 ⇒ 不相容（`中上` ≠ `中下`，**区分仍在**）。与旧实现逐例等价。
     */
    public static boolean partCompatible(EpisodeKey a, EpisodeKey b) {
        if (a.part.isEmpty() || b.part.isEmpty()) return true;
        return a.part.startsWith(b.part) || b.part.startsWith(a.part);
    }

    /** SameEpisode′（日期双有不等 → 否决, 硬证据）；P0-1a: 变体序号纳入身份 */
    public static boolean sameEpisode(EpisodeKey a, EpisodeKey b) {
        if (!a.variant.equals(b.variant) || a.variantNum != b.variantNum || !a.series.equals(b.series))
            return false;
        if (!a.date.isEmpty() && !b.date.isEmpty()) {
            if (!a.date.equals(b.date)) return false;
            return partCompatible(a, b);
        }
        if (a.num > 0 && b.num > 0 && a.num == b.num) return partCompatible(a, b);
        if (a.eNum > 0 && b.eNum > 0 && a.eNum == b.eNum) return true;
        return false;
    }

    /**
     * P0-1c（2026-11-10）：**身份串相容**（对齐 EXACT 档用）。与"epid 串全等"只差一处 —— `p` 段按
     * **前缀相容**比较，其余 `s` / `v`（含变体序号）/ `n` / `d` 四段仍要求**全等**。
     * 逐例等价于合并前的「`epid` 全等 && `sameEpisode`」。
     */
    public static boolean epidCompatible(EpisodeKey a, EpisodeKey b) {
        if (!a.series.equals(b.series) || !a.variant.equals(b.variant) || a.variantNum != b.variantNum) return false;
        if (a.num != b.num || !a.date.equals(b.date)) return false;
        return partCompatible(a, b);
    }

    /** 综艺/衍生形态判定（epid 只给触发的集; 裸「第N集/期」与 E圆号不触发） */
    public boolean triggered() {
        return !date.isEmpty() || !part.isEmpty() || !variant.isEmpty() || !series.isEmpty();
    }

    /**
     * P0-3c（2026-11-10）：把 epid 串（`s=..|v=..|p=..|n=..|d=..`，5 段，缺值 `-`）拆成 5 段。
     * 返回顺序 `[s, v, p, n, d]`；空串 / 段数不对 / 含未知键 ⇒ null（当作"无 epid"）。
     * 与 web `parseEpid`、Go 侧同规。
     */
    public static String[] parseEpid(String epid) {
        String raw = epid == null ? "" : epid.trim();
        if (raw.isEmpty()) return null;
        String[] seg = raw.split("\\|", -1);
        if (seg.length != 5) return null;
        String[] out = new String[]{"", "", "", "", ""};
        for (String kv : seg) {
            int i = kv.indexOf('=');
            if (i <= 0) return null;
            String key = kv.substring(0, i);
            String raw3 = kv.substring(i + 1);
            String val = "-".equals(raw3) ? "" : raw3;
            switch (key) {
                case "s": out[0] = val; break;
                case "v": out[1] = val; break;
                case "p": out[2] = val; break;
                case "n": out[3] = val; break;
                case "d": out[4] = val; break;
                default: return null;
            }
        }
        return out;
    }

    /**
     * P0-3c：在**目标源集名列表**里按 epid 定位集下标；定位不到返回 -1
     * （调用方回退 {@code episodeIndex} —— 方案 §3.1.9 的两级回退）。与 web {@code indexByEpid} 同口径：
     * <ol>
     *   <li>① <b>严格</b>：{@link #epid()} 与存储 epid <b>全等</b> —— 同源集序变化（重排/插集/补集）时下标会失效，
     *       epid 仍能对上（这正是本方法的价值）；</li>
     *   <li>② <b>宽松</b>：<b>忽略 {@code n} 段</b> —— 记录时的期号可能是 A2 桥接/锚点算出的值，本地重算不出来；
     *       此时要求 {@code s} / {@code v}（含变体序号）/ {@code d} 相等、{@code p} 段前缀相容，
     *       且<b>候选唯一</b>才采纳（多个 ⇒ -1，宁缺勿错）。</li>
     * </ol>
     */
    public static int indexByEpid(String epid, java.util.List<String> titles) {
        String[] want = parseEpid(epid);
        if (want == null || titles == null || titles.isEmpty()) return -1;
        for (int i = 0; i < titles.size(); i++) {
            String t = titles.get(i) == null ? "" : titles.get(i);
            if (parse(t).epid().equals(epid)) return i;
        }
        int only = -1;
        for (int i = 0; i < titles.size(); i++) {
            EpisodeKey k = parse(titles.get(i) == null ? "" : titles.get(i));
            String kv = (!k.variant.isEmpty() && k.variantNum > 0)
                    ? k.variant + "#" + k.variantNum : k.variant;
            if (!k.series.equals(want[0]) || !kv.equals(want[1]) || !k.date.equals(want[4])) continue;
            if (!want[2].isEmpty() && !k.part.isEmpty()
                    && !(k.part.startsWith(want[2]) || want[2].startsWith(k.part))) continue;
            if (only >= 0) return -1;
            only = i;
        }
        return only;
    }

    /** 5 段身份串: s|v|p|n|d, 缺值填 `-`（`v` 段含变体序号 `v=特辑#2`；触发的集必有） */
    public String epid() {
        String v = variant.isEmpty() ? "-" : (variantNum > 0 ? variant + "#" + variantNum : variant);
        return "s=" + (series.isEmpty() ? "-" : series)
                + "|v=" + v
                + "|p=" + (part.isEmpty() ? "-" : part)
                + "|n=" + (num > 0 ? String.valueOf(num) : "-")
                + "|d=" + (date.isEmpty() ? "-" : date);
    }

    /** 返回期号被替换后的键（桥接归一用）; n<=0 返回原键。 */
    public EpisodeKey withNum(int n) {
        return n <= 0 ? this : new EpisodeKey(date, n, eNum, part, variant, variantNum, series);
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
