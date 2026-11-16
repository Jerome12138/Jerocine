package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

import org.junit.Test;

/**
 * {@link EpisodeAlign} 黄金用例 —— 与 web 侧 episodeAlign.spec.ts、Go 侧同一组语义。
 * 覆盖 5 档降级 + 兜底档不自动换源的判定。
 */
public class EpisodeAlignTest {

    private static List<String> of(String... a) {
        return new ArrayList<>(Arrays.asList(a));
    }

    @Test
    public void exact_sameIndexDrama() {
        List<String> eps = of("第1集", "第2集", "第3集", "第4集", "第5集");
        EpisodeAlign.Result r = EpisodeAlign.align(4, eps, new ArrayList<>(eps));
        assertEquals(4, r.index);
        assertEquals(EpisodeAlign.Tier.EXACT, r.tier);
    }

    /** 序号错位但期号相同 ⇒ 仍能对上（综艺核心场景） */
    @Test
    public void exact_periodSameIndexShifted() {
        EpisodeAlign.Result r = EpisodeAlign.align(2, of("第1期上", "第1期中", "第1期下"),
                of("第2期上", "第1期下", "第1期上"));
        assertEquals(1, r.index);
        assertEquals(EpisodeAlign.Tier.EXACT, r.tier);
    }

    /** 源 A 尾部多出「纯享」等非正片集，也不影响对齐 */
    @Test
    public void exact_extraVariantEpisodes() {
        EpisodeAlign.Result r = EpisodeAlign.align(2, of("第1期上", "第1期上纯享", "第1期下"),
                of("第1期下", "第1期上"));
        assertEquals(0, r.index);
        assertEquals(EpisodeAlign.Tier.EXACT, r.tier);
    }

    @Test
    public void exact_spaceInsensitive() {
        EpisodeAlign.Result r = EpisodeAlign.align(2, of("第1集", "第2集", "第 3 集"),
                of("第1集", "第2集", "第3集"));
        assertEquals(2, r.index);
        assertEquals(EpisodeAlign.Tier.EXACT, r.tier);
    }

    /** 一侧日期、一侧期号 ⇒ 第 ④ 档（启发式，不自动换源） */
    @Test
    public void byOrdinal_crossNamingSystems() {
        EpisodeAlign.Result r = EpisodeAlign.align(1, of("20260803上", "20260804下"),
                of("第1期上", "第2期下"));
        assertEquals(1, r.index);
        assertEquals(EpisodeAlign.Tier.BY_ORDINAL, r.tier);
        assertFalse(EpisodeAlign.canAutoSwitch(r.tier));
    }

    /** 完全对不上 ⇒ 位置兜底（原有行为），且不自动换源 */
    @Test
    public void position_fallbackNotAutoSwitchable() {
        EpisodeAlign.Result r = EpisodeAlign.align(1, of("先导片上", "先导片下"),
                of("第1集", "第2集", "第3集"));
        assertEquals(1, r.index);
        assertEquals(EpisodeAlign.Tier.POSITION, r.tier);
        assertFalse(EpisodeAlign.canAutoSwitch(r.tier));
    }

    /** 目标源更短 ⇒ 位置兜底夹到最后一集 */
    @Test
    public void position_clampedWhenTargetShorter() {
        List<String> cur = new ArrayList<>();
        for (int i = 1; i <= 10; i++) cur.add("第" + i + "集");
        EpisodeAlign.Result r = EpisodeAlign.align(9, cur, of("第1期"));
        assertEquals(0, r.index);
        assertEquals(EpisodeAlign.Tier.POSITION, r.tier);
    }

    /** 栏目名不同不误配（正片 ↔ 心动的周末） */
    @Test
    public void position_seriesMismatchNotMatched() {
        EpisodeAlign.Result r = EpisodeAlign.align(0, of("正片第1期上"), of("心动的周末第1期上"));
        assertEquals(EpisodeAlign.Tier.POSITION, r.tier);
        assertFalse(EpisodeAlign.canAutoSwitch(r.tier));
    }

    /** 同一期内 上/中/下 共享期序（心动的周末） */
    @Test
    public void byOrdinal_sameEpisodeSharedOrdinal() {
        List<String> cur = of("20261012心动的周末上", "20261013心动的周末下");
        List<String> tgt = of("心动的周末第1期上", "心动的周末第1期下");
        assertEquals(0, EpisodeAlign.align(0, cur, tgt).index);
        assertEquals(1, EpisodeAlign.align(1, cur, tgt).index);
    }

    /** 主/次部分前缀相容：库里「中（上）/（下）」、源里只有「中」 */
    @Test
    public void prefixCompatible_midSub() {
        EpisodeAlign.Result r = EpisodeAlign.align(0, of("第3期中"),
                of("第3期中（上）", "第3期中（下）"));
        assertEquals(0, r.index);
        assertTrue(EpisodeAlign.canAutoSwitch(r.tier));
    }

    /** ① 档回归(dd8ea00)：epid 串不含 sub ⇒ 「中（上）/（下）」不得被判为同一集。
     *  与 web episodeAlign.spec.ts 的①档回归用例同语义(评审 D1: Java 侧此前缺防呆用例)。 */
    @Test
    public void exact_sameEpisodeGuard_midUpDown() {
        // 「中（上）」↔ [中（上）, 中（下）]: 精确命中第 0 条, 不得错落到（下)
        EpisodeAlign.Result r = EpisodeAlign.align(0, of("第3期中（上）"),
                of("第3期中（上）", "第3期中（下）"));
        assertEquals(0, r.index);
        assertEquals(EpisodeAlign.Tier.EXACT, r.tier);
        // 只有「中」: 不得错配成「中（下）」(至多按前缀相容落到（上), 绝不 exact 到（下))
        EpisodeAlign.Result r2 = EpisodeAlign.align(0, of("第3期中"),
                of("第3期中（上）", "第3期中（下）"));
        assertFalse(r2.tier == EpisodeAlign.Tier.EXACT && r2.index != 0);
    }

    @Test
    public void failed_onBadInput() {
        assertEquals(-1, EpisodeAlign.align(9, of("第1集", "第2集"), of("第1期")).index);
        assertEquals(EpisodeAlign.Tier.FAILED, EpisodeAlign.align(9, of("第1集", "第2集"), of("第1期")).tier);
        assertEquals(-1, EpisodeAlign.align(0, of("第1集"), Collections.emptyList()).index);
    }

    @Test
    public void canAutoSwitch_onlyExactTiers() {
        assertTrue(EpisodeAlign.canAutoSwitch(EpisodeAlign.Tier.EXACT));
        assertTrue(EpisodeAlign.canAutoSwitch(EpisodeAlign.Tier.SAME_PERIOD));
        assertTrue(EpisodeAlign.canAutoSwitch(EpisodeAlign.Tier.SAME_DATE));
        assertFalse(EpisodeAlign.canAutoSwitch(EpisodeAlign.Tier.BY_ORDINAL));
        assertFalse(EpisodeAlign.canAutoSwitch(EpisodeAlign.Tier.POSITION));
        assertFalse(EpisodeAlign.canAutoSwitch(EpisodeAlign.Tier.FAILED));
    }

    /** L3a：目标侧无期号无日期 + 组内按 (series,part) 唯一 ⇒ 命中且可自动换源 */
    @Test
    public void bySeries_unique() {
        EpisodeAlign.Result r = EpisodeAlign.align(0, of("20261005心动的旅行上"),
                of("心动的旅行上：草原团建！", "心动的旅行中：…", "心动的旅行下：…"));
        assertEquals(0, r.index);
        assertEquals(EpisodeAlign.Tier.BY_SERIES, r.tier);
        assertTrue(EpisodeAlign.canAutoSwitch(r.tier));
    }

    /** L3a：纯栏目名（无限号无主次）+ 组内唯一 ⇒ 命中 */
    @Test
    public void bySeries_pureLabel() {
        EpisodeAlign.Result r = EpisodeAlign.align(0, of("20260507线上集结篇"),
                of("线上集结篇：新老坞民烧脑身份推理战", "线下集结篇：新老坞民爆笑破冰"));
        assertEquals(0, r.index);
        assertEquals(EpisodeAlign.Tier.BY_SERIES, r.tier);
    }

    /** L3a：组内不唯一 ⇒ 落位置兜底且不自动换源 */
    @Test
    public void bySeries_ambiguousFallsBack() {
        EpisodeAlign.Result r = EpisodeAlign.align(0, of("20261005心动的旅行"),
                of("心动的旅行上：…", "心动的旅行中：…"));
        assertEquals(EpisodeAlign.Tier.POSITION, r.tier);
        assertFalse(EpisodeAlign.canAutoSwitch(r.tier));
    }

    /** L3a：正片（series/variant 皆空）不适用 */
    @Test
    public void bySeries_notForMainFeature() {
        assertFalse(EpisodeAlign.align(0, of("20260803上"), of("第1期上")).tier
                == EpisodeAlign.Tier.BY_SERIES);
    }

    /**
     * P0-1a（2026-11-10）：**变体序号进入 L3a 组键** —— 桃花坞6 特辑组真实形态。
     * 改前：特辑1..5 的 variant 全是「特辑」⇒ 组内 6 条 ⇒ 永不唯一 ⇒ 整组丢弹幕。
     * 改后：variantNum 复合身份 ⇒ 各自唯一 ⇒ 命中并允许自动换源。
     */
    @Test
    public void bySeries_variantOrdinalNoCollapse() {
        List<String> tgt = of("特辑1", "特辑2", "特辑3", "特辑4", "特辑5", "夏日清凉特辑");
        assertEquals(0, EpisodeAlign.align(0, of("20260809特辑1"), tgt).index);
        assertEquals(2, EpisodeAlign.align(0, of("20260809特辑3"), tgt).index);
        assertEquals(4, EpisodeAlign.align(0, of("20260809特辑5"), tgt).index);
        assertEquals(EpisodeAlign.Tier.BY_SERIES, EpisodeAlign.align(0, of("20260809特辑3"), tgt).tier);
        assertTrue(EpisodeAlign.canAutoSwitch(EpisodeAlign.align(0, of("20260809特辑3"), tgt).tier));
        // 无序号「特辑」身份 ≠ 任何「特辑N」⇒ 不配、不自动换源（宁缺勿错）
        EpisodeAlign.Result r = EpisodeAlign.align(0, of("20260809特辑"), tgt);
        assertEquals(EpisodeAlign.Tier.POSITION, r.tier);
        assertFalse(EpisodeAlign.canAutoSwitch(r.tier));
        // 夏日清凉特辑：序号同为 0 但 series 不同 ⇒ 自成一组、唯一 ⇒ 命中
        assertEquals(5, EpisodeAlign.align(0, of("20260809夏日清凉特辑"), tgt).index);
    }
}
