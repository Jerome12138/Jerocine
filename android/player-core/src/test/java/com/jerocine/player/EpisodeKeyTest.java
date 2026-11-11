package com.jerocine.player;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
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
    public void parse_主次部分复合() {
        // P0-1c：主部分为**复合单段**（中 + 紧随的 下 ⇒ 中下）
        EpisodeKey k = EpisodeKey.parse("20260818中（下）");
        assertEquals("中下", k.part);
        assertEquals("20260818", k.date);
        EpisodeKey k2 = EpisodeKey.parse("第3期中");
        assertEquals("中", k2.part);
        // 前缀相容：中 ⊂ 中下 ⇒ 相容；中上 ≠ 中下 ⇒ 不相容（区分仍在）
        assertTrue(EpisodeKey.partCompatible(k2, k));
        assertFalse(EpisodeKey.partCompatible(
                EpisodeKey.parse("第3期中（上）"), EpisodeKey.parse("第3期中（下）")));
        assertTrue(EpisodeKey.partCompatible(
                EpisodeKey.parse("第3期中（上）"), EpisodeKey.parse("第3期中（上）")));
        assertTrue(EpisodeKey.partCompatible(
                EpisodeKey.parse("第3期中"), EpisodeKey.parse("第3期中（上）")));
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

    /** v2 追加（2026-11-09，三端同规防呆）：日期样式期号不得残留「期」 */
    @Test
    public void dateStylePeriod_noResidualQi() {
        EpisodeKey k = EpisodeKey.parse("第20261005期心动的旅行上");
        assertEquals("心动的旅行", k.series);
        assertEquals("20261005", k.date);
        assertEquals("上", k.part);
        assertEquals(0, k.num);
        assertEquals("中上", EpisodeKey.parse("第3期中（上）").part);
    }

    /** P0-1a（2026-11-10，三端同规防呆）：变体序号 —— 变体词后紧跟的数字必须复合为身份 */
    @Test
    public void variantOrdinal_复合身份() {
        assertEquals(2, EpisodeKey.parse("特辑2").variantNum);
        assertEquals("特辑", EpisodeKey.parse("特辑2").variant);
        assertEquals(2, EpisodeKey.parse("20260128特辑2").variantNum);
        assertEquals("20260128", EpisodeKey.parse("20260128特辑2").date);
        assertEquals(5, EpisodeKey.parse("第1期特辑5上").variantNum);
        assertEquals(2, EpisodeKey.parse("特辑（2）").variantNum);
        assertEquals(12, EpisodeKey.parse("特辑12").variantNum);
        // 序号不紧跟（隔着「版」）→ 不取
        assertEquals(0, EpisodeKey.parse("第1期加更版1").variantNum);
        // 纯日期/长数字 → 不取（避免吞日期）
        assertEquals(0, EpisodeKey.parse("特辑2026").variantNum);
        assertEquals(0, EpisodeKey.parse("20260128特辑").variantNum);
        // 无序号变体 + 栏目名
        assertEquals(0, EpisodeKey.parse("夏日清凉特辑").variantNum);
        assertEquals("夏日清凉", EpisodeKey.parse("夏日清凉特辑").series);
    }

    @Test
    public void variantOrdinal_sameEpisode与epid() {
        assertFalse(EpisodeKey.sameEpisode(EpisodeKey.parse("特辑2"), EpisodeKey.parse("特辑3")));
        assertFalse(EpisodeKey.sameEpisode(EpisodeKey.parse("特辑2"), EpisodeKey.parse("特辑")));
        // 两侧都无期号/日期 ⇒ 不判同（交 L3a 组内唯一兜底）
        assertFalse(EpisodeKey.sameEpisode(EpisodeKey.parse("特辑2"), EpisodeKey.parse("特辑2")));
        assertTrue(EpisodeKey.sameEpisode(EpisodeKey.parse("第1期特辑2"), EpisodeKey.parse("第1期特辑2")));
        assertFalse(EpisodeKey.sameEpisode(EpisodeKey.parse("第1期特辑2"), EpisodeKey.parse("第1期特辑3")));
        // epid: v 段含序号（特辑 vs 特辑2 不得撞缓存键）
        assertEquals("s=-|v=特辑#2|p=-|n=-|d=-", EpisodeKey.parse("特辑2").epid());
        assertEquals("s=-|v=特辑#2|p=-|n=-|d=20260128", EpisodeKey.parse("20260128特辑2").epid());
        assertEquals("s=-|v=特辑|p=-|n=-|d=-", EpisodeKey.parse("特辑").epid());
        assertEquals("s=夏日清凉|v=特辑|p=-|n=-|d=-", EpisodeKey.parse("夏日清凉特辑").epid());
    }

    /**
     * P0-3c（2026-11-10）：续播定位 —— parseEpid / indexByEpid。
     * 与 web `parseEpid`/`indexByEpid`、Go 侧同口径；黄金用例与 TS `episodeKey.spec.ts` 同一组。
     */
    @Test
    public void parseEpid_解析与拒识() {
        assertArrayEquals(new String[]{"", "", "中上", "3", ""},
                EpisodeKey.parseEpid("s=-|v=-|p=中上|n=3|d=-"));
        assertArrayEquals(new String[]{"心动的周末", "特辑#2", "", "", "20260930"},
                EpisodeKey.parseEpid("s=心动的周末|v=特辑#2|p=-|n=-|d=20260930"));
        assertNull(EpisodeKey.parseEpid(""));
        assertNull(EpisodeKey.parseEpid("   "));
        assertNull(EpisodeKey.parseEpid("s=-|v=-"));
        assertNull(EpisodeKey.parseEpid("s=-|v=-|p=-|n=-|zz=-"));
    }

    @Test
    public void indexByEpid_两档定位与回退() {
        // ① 强等：同源集序变化（重排/插集）也能定位回原来那一集
        String ep = EpisodeKey.parse("20260803上").epid();
        assertEquals(2, EpisodeKey.indexByEpid(ep,
                java.util.Arrays.asList("第1集", "第2集", "20260803上", "20260803中")));
        assertEquals(0, EpisodeKey.indexByEpid(ep,
                java.util.Arrays.asList("20260803上", "第1集", "第9集", "第2集", "20260803中")));
        // ② 弱等（相容）：s/v 相等；n/p/d 单边通配、双有须过各自规则；候选唯一才采纳
        java.util.List<String> titles = java.util.Arrays.asList("20260803上", "20260803中", "20260804下");
        // 记录时的期号可能是桥接/锚点算出的值（本地重算不出）⇒ 单边缺 ⇒ 通配
        assertEquals(0, EpisodeKey.indexByEpid("s=-|v=-|p=上|n=1|d=20260803", titles));
        // p 段为空 ⇒ 同日期两条候选 ⇒ 不唯一 ⇒ 放弃（宁缺勿错，交回退）
        assertEquals(-1, EpisodeKey.indexByEpid("s=-|v=-|p=-|n=1|d=20260803", titles));
        // ⚠️ 期号双有不等 ⇒ 拒绝（不得整体忽略 n —— 否则「第3期」会被「第2期」顶上，配错集）
        assertEquals(-1, EpisodeKey.indexByEpid("s=-|v=-|p=上|n=3|d=-",
                java.util.Arrays.asList("第2期上", "第4期上")));
        assertEquals(0, EpisodeKey.indexByEpid("s=-|v=-|p=上|n=2|d=-",
                java.util.Arrays.asList("第2期上")));
        // 日期单边缺失 ⇒ 通配 ⇒ 仍可命中（桥接只补 n，d 一侧可能没有）
        assertEquals(0, EpisodeKey.indexByEpid("s=-|v=-|p=上|n=1|d=-",
                java.util.Arrays.asList("20260803第1期上")));
        // 日期双有不等 ⇒ 硬证据 ⇒ 拒绝
        assertEquals(-1, EpisodeKey.indexByEpid("s=-|v=-|p=上|n=-|d=20260810",
                java.util.Arrays.asList("20260803上")));
        // 对不上 / 空列表 / 坏串 ⇒ -1
        assertEquals(-1, EpisodeKey.indexByEpid("s=-|v=-|p=下|n=-|d=20261231",
                java.util.Arrays.asList("20260803上")));
        assertEquals(-1, EpisodeKey.indexByEpid("s=-|v=-|p=上|n=-|d=20260803",
                java.util.Collections.<String>emptyList()));
        assertEquals(-1, EpisodeKey.indexByEpid("坏串", java.util.Arrays.asList("20260803上")));
        assertEquals(-1, EpisodeKey.indexByEpid("", java.util.Arrays.asList("20260803上")));
        // F6（2026-11-11 评审）：**裸「第N集」不得参与弱等** —— s/v/p/d 全空、只剩 n ⇒ 必须拒绝
        //（否则任意单集源里的第 N 条都会被它"命中"，与"epid 只描述触发的集"相悖 ✗）
        assertEquals(-1, EpisodeKey.indexByEpid("s=-|v=-|p=-|n=1|d=-",
                java.util.Arrays.asList("20260803第1期")));
    }
}
