package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * 黄金用例 —— 与 server/internal/danmu/episode_key_test.go、web/src/utils/episodeKey.spec.ts
 * 同一组（三端同规, 任一侧改动必须同组通过）。用例全部取自现网真实集名。
 */
public class EpisodeKeyTest {

    @Test
    public void parse_基础形态() {
        EpisodeKey k = EpisodeKey.parse("20260803(第1期上)");
        assertEquals("20260803", k.date);
        assertEquals(1, k.num);
        assertEquals("上", k.part);
        assertEquals("", k.series);
    }

    @Test
    public void parse_主次部分() {
        EpisodeKey k = EpisodeKey.parse("20260818中（下）");
        assertEquals("中", k.part);
        assertEquals("下", k.sub);
        EpisodeKey k2 = EpisodeKey.parse("第3期中");
        assertEquals("中", k2.part);
        assertEquals("", k2.sub);
    }

    @Test
    public void parse_日期三格式归一() {
        assertEquals("20261010", EpisodeKey.parse("2026-10-10期").date);
        assertEquals("20261010", EpisodeKey.parse("第20261010期").date);
        assertEquals("20220501", EpisodeKey.parse("2022.05.01期").date);
        assertEquals("20261010", EpisodeKey.parse("【qq】 第1期上：出发团变落汤鸡 2026-10-10期").date);
    }

    @Test
    public void parse_栏目名连续片段() {
        // 「先导片」是变体: 变体身份优先, 栏目名为空
        EpisodeKey k = EpisodeKey.parse("第20260731期先导片上");
        assertEquals("先导", k.variant);
        assertEquals("", k.series);
        // 季号跨度删除（第七季 整段）
        assertEquals("王牌对王牌", EpisodeKey.parse("王牌对王牌第七季2022.05.01期-会员版").series);
        // 13 字自由描述 → 拒识
        assertEquals("", EpisodeKey.parse("檀健次带北江小分队回好六了！ 2025-01-04期").series);
        assertEquals("心动的周末", EpisodeKey.parse("20261027心动的周末第3期下").series);
        assertEquals("心动的周末", EpisodeKey.parse("20261012心动的周末上").series);
        assertEquals("", EpisodeKey.parse("【qq】 第1期上：又争又抢，心动了就要打明牌！").series);
    }

    @Test
    public void sameEpisode_键规则() {
        EpisodeKey app = EpisodeKey.parse("20260803(第1期上)");
        assertTrue(EpisodeKey.sameEpisode(app, EpisodeKey.parse("第1期上")));
        assertTrue(EpisodeKey.sameEpisode(app, EpisodeKey.parse("第1期")));      // 一方缺 part 通配
        assertFalse(EpisodeKey.sameEpisode(EpisodeKey.parse("20260806(第1期加更上)"), EpisodeKey.parse("第1期上")));
        assertFalse(EpisodeKey.sameEpisode(EpisodeKey.parse("20260803上"), EpisodeKey.parse("20260803中")));
        assertFalse(EpisodeKey.sameEpisode(
                EpisodeKey.parse("心动的周末第1期上：高甜游戏"), EpisodeKey.parse("第1期上：又争又抢")));
        assertFalse(EpisodeKey.sameEpisode(EpisodeKey.parse("正片"), EpisodeKey.parse("正片")));
    }

    @Test
    public void sameEpisode_日期归一与硬否决() {
        // 心动9 真实场景: App 纯日期 vs 库「期号+破折日期」→ 归一后直等
        assertTrue(EpisodeKey.sameEpisode(
                EpisodeKey.parse("20261010上"),
                EpisodeKey.parse("【qq】 第1期上：出发团变落汤鸡 2026-10-10期")));
        assertTrue(EpisodeKey.sameEpisode(
                EpisodeKey.parse("2022.05.01期"), EpisodeKey.parse("20220501")));
        // 日期双有不等 → 否决(即使期号相等); 一方只有期号不触发否决
        assertFalse(EpisodeKey.sameEpisode(
                EpisodeKey.parse("20260817第3期上"), EpisodeKey.parse("20260810第3期上")));
        assertTrue(EpisodeKey.sameEpisode(
                EpisodeKey.parse("20260817第3期上"), EpisodeKey.parse("第3期上")));
    }

    @Test
    public void epid_五段定长() {
        assertEquals("s=-|v=-|p=上|n=1|d=-", EpisodeKey.parse("正片第1期上").epid());
        assertEquals("s=-|v=-|p=上|n=-|d=20260803", EpisodeKey.parse("20260803上").epid());
        assertEquals("s=-|v=加更|p=上|n=-|d=20260806", EpisodeKey.parse("20260806加更上").epid());
        // 裸序号/E圆号不触发（无 epid 语义, 调用方回退位置兜底）
        assertFalse(EpisodeKey.parse("第3集").triggered());
        assertFalse(EpisodeKey.parse("E01.220705").triggered());
        assertTrue(EpisodeKey.parse("20260803上").triggered());
    }

    @Test
    public void bridgeNum_心动9场景() {
        EpisodeKey[] others = {
                EpisodeKey.parse("20260803第1期上"),
                EpisodeKey.parse("20260803第1期中"),
                EpisodeKey.parse("20260810第2期中"),
                EpisodeKey.parse("20261005第10期上"),
                EpisodeKey.parse("20261012心动的周末第1期上")
        };
        assertEquals(1, EpisodeKey.bridgeNum(EpisodeKey.parse("20260803上"), others));
        assertEquals(10, EpisodeKey.bridgeNum(EpisodeKey.parse("20261005上"), others));
        assertEquals(1, EpisodeKey.bridgeNum(EpisodeKey.parse("20261012心动的周末上"), others));
        // 变体不同不桥接
        assertEquals(0, EpisodeKey.bridgeNum(EpisodeKey.parse("20260731先导片上"),
                new EpisodeKey[]{EpisodeKey.parse("20260731第0期先导片上")}));
        // 候选期号冲突 → 放弃
        assertEquals(0, EpisodeKey.bridgeNum(EpisodeKey.parse("20260803上"),
                new EpisodeKey[]{EpisodeKey.parse("20260803第1期上"), EpisodeKey.parse("20260803第2期上")}));
        // 桥接后跨源同集 epid 串相等（bf#110 ↔ lz#116 用户场景）
        EpisodeKey lz = EpisodeKey.parse("20261005上");
        EpisodeKey bf = EpisodeKey.parse("20261005第10期上");
        assertEquals(bf.epid(), EpisodeKey.epidFor(lz, others));
    }
}
