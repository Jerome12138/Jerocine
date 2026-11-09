package com.jerocine.player;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.TreeSet;

/**
 * 跨源集对齐（切源用）—— 只做「落点决策」，解析复用 {@link EpisodeKey}（三端同规 v2）。
 * 与 web 侧 {@code web/src/utils/episodeAlign.ts} 同语义（黄金用例同一组）。
 *
 * <p>背景：切源（含自动换源/error 轮换）此前只做「保同下标」——剧/动漫各源同构时等价，
 * 但**综艺各源分集不一致时会静默切到错集**。
 */
public final class EpisodeAlign {

    public enum Tier {
        /** 身份串全等 */
        EXACT,
        /** 同期号 */
        SAME_PERIOD,
        /** 同日期（两侧都没期号） */
        SAME_DATE,
        /** 同期序（跨命名体系：一侧用日期、一侧用期号） */
        BY_ORDINAL,
        /** 同栏目组内唯一（L3a，与 web bySeries / 服务端 pickSeriesUnique 同口径） */
        BY_SERIES,
        /** 按位置（原有兜底：第 i 个 ↔ 第 i 个） */
        POSITION,
        /** 完全对不上 */
        FAILED
    }

    /** 精确档：可据此**自动**换源；BY_ORDINAL/POSITION 是启发式兜底 ⇒ 只提示、不自动切。 */
    public static boolean canAutoSwitch(Tier t) {
        return t == Tier.EXACT || t == Tier.SAME_PERIOD || t == Tier.SAME_DATE
                || t == Tier.BY_SERIES;
    }

    public static final class Result {
        /** 目标源里的下标；-1 = 完全对不上 */
        public final int index;
        public final Tier tier;

        Result(int index, Tier tier) {
            this.index = index;
            this.tier = tier;
        }

        @Override
        public String toString() {
            return "Align{index=" + index + ", tier=" + tier + '}';
        }
    }

    private EpisodeAlign() {
    }

    /**
     * 身份相容（不含期号/日期）：栏目 + 变体 + 主次部分；**日期双有且不等 → 否决**（硬证据）。
     * 第 ④ 档（跨命名体系）靠它 —— 不能用 {@code sameEpisode}（那要求有共同期号/日期）。
     */
    static boolean identityCompatible(EpisodeKey a, EpisodeKey b) {
        if (!a.variant.equals(b.variant) || !a.series.equals(b.series)) return false;
        if (!EpisodeKey.partCompatible(a, b)) return false;
        if (!a.date.isEmpty() && !b.date.isEmpty() && !a.date.equals(b.date)) return false;
        return true;
    }

    /** 「期槽」：期号优先、无期号用日期（无则空串 = 不参与期序） */
    private static String periodKey(EpisodeKey k) {
        if (k.num > 0) return "n" + k.num;
        return k.date.isEmpty() ? "" : "d" + k.date;
    }

    /** 期序：同 (栏目, 变体) 分组内，按「期槽」去重排序后的序号（1 起；无期槽 = 0） */
    private static int[] periodOrdinals(List<EpisodeKey> list) {
        int[] out = new int[list.size()];
        Map<String, List<Integer>> groups = new LinkedHashMap<>();
        for (int i = 0; i < list.size(); i++) {
            String g = list.get(i).series + '\u0001' + list.get(i).variant;
            groups.computeIfAbsent(g, x -> new ArrayList<>()).add(i);
        }
        for (List<Integer> idxs : groups.values()) {
            TreeSet<String> keys = new TreeSet<>((a, b) -> {
                boolean na = a.charAt(0) == 'n', nb = b.charAt(0) == 'n';
                if (na && nb) {
                    return Integer.compare(Integer.parseInt(a.substring(1)), Integer.parseInt(b.substring(1)));
                }
                return a.compareTo(b);
            });
            for (Integer i : idxs) {
                String k = periodKey(list.get(i));
                if (!k.isEmpty()) keys.add(k);
            }
            Map<String, Integer> rank = new LinkedHashMap<>();
            int n = 1;
            for (String k : keys) rank.put(k, n++);
            for (Integer i : idxs) {
                Integer r = rank.get(periodKey(list.get(i)));
                out[i] = r == null ? 0 : r;
            }
        }
        return out;
    }

    /**
     * L3a（客户端等价档，与服务端 pickSeriesUnique、web seriesUniqueIndex 同口径）：
     * 同 {@code (series, variant)} 组内**唯一定位** —— 覆盖"目标侧既无期号也无日期"的衍生栏目
     * （心动的旅行 / 线上集结篇 / 夏日清凉特辑…）。边界：正片（series/variant 皆空）不适用；
     * 主/次相容；期号两侧都有须相等；日期两侧都有须相等；**目标侧该组仅 1 条**才返回，否则 -1。
     */
    static int seriesUniqueIndex(EpisodeKey c, java.util.List<EpisodeKey> tgt) {
        if (c.series.isEmpty() && c.variant.isEmpty()) return -1;
        int only = -1;
        for (int i = 0; i < tgt.size(); i++) {
            EpisodeKey t = tgt.get(i);
            if (!t.series.equals(c.series) || !t.variant.equals(c.variant)) continue;
            if (!EpisodeKey.partCompatible(c, t)) continue;
            if (c.num > 0 && t.num > 0 && c.num != t.num) continue;
            if (!c.date.isEmpty() && !t.date.isEmpty() && !c.date.equals(t.date)) continue;
            if (only >= 0) return -1;
            only = i;
        }
        return only;
    }

    /**
     * 5 档降级对齐。两侧传**完整集名列表**（下标即播放用的集号）；全对不上返回 {@code index=-1}。
     */
    public static Result align(int curIndex, List<String> curTitles, List<String> targetTitles) {
        if (curTitles == null || targetTitles == null || targetTitles.isEmpty()
                || curIndex < 0 || curIndex >= curTitles.size()) {
            return new Result(-1, Tier.FAILED);
        }
        List<EpisodeKey> cur = new ArrayList<>();
        for (String t : curTitles) cur.add(EpisodeKey.parse(t));
        List<EpisodeKey> tgt = new ArrayList<>();
        for (String t : targetTitles) tgt.add(EpisodeKey.parse(t));
        EpisodeKey c = cur.get(curIndex);

        // ① 身份串全等
        String curEpid = c.epid();
        for (int i = 0; i < tgt.size(); i++) {
            // ⚠️ epid 串是 5 段（s|v|p|n|d），不含 sub ⇒ 必须同时过 sameEpisode，
            // 否则「第3期中（上）」与「第3期中（下）」epid 相等会被误判为同一集。
            if (tgt.get(i).epid().equals(curEpid) && EpisodeKey.sameEpisode(c, tgt.get(i))) {
                return new Result(i, Tier.EXACT);
            }
        }
        // ② 同集（SameEpisode′）+ 期号相同 —— 期号优先，此时不看日期
        if (c.num > 0) {
            for (int i = 0; i < tgt.size(); i++) {
                EpisodeKey t = tgt.get(i);
                if (t.num > 0 && t.num == c.num && EpisodeKey.sameEpisode(c, t)) {
                    return new Result(i, Tier.SAME_PERIOD);
                }
            }
        }
        // ③ 同集 + 日期相同（两侧都没期号、都有日期）
        if (c.num == 0 && !c.date.isEmpty()) {
            for (int i = 0; i < tgt.size(); i++) {
                EpisodeKey t = tgt.get(i);
                if (t.num == 0 && t.date.equals(c.date) && EpisodeKey.sameEpisode(c, t)) {
                    return new Result(i, Tier.SAME_DATE);
                }
            }
        }
        // ④ 身份相容 + 期序相同（跨命名体系）
        int ord = periodOrdinals(cur)[curIndex];
        if (ord > 0) {
            int[] tgtOrd = periodOrdinals(tgt);
            for (int i = 0; i < tgt.size(); i++) {
                if (tgtOrd[i] == ord && identityCompatible(c, tgt.get(i))) {
                    return new Result(i, Tier.BY_ORDINAL);
                }
            }
        }
        // ⑤ 同栏目组内唯一（L3a）
        int uniq = seriesUniqueIndex(c, tgt);
        if (uniq >= 0) return new Result(uniq, Tier.BY_SERIES);
        // ⑥ 整体位置（原有兜底）
        return new Result(Math.min(curIndex, tgt.size() - 1), Tier.POSITION);
    }
}
