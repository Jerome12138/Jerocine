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
}
