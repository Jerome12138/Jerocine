package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

import java.util.Arrays;
import java.util.Collections;
import java.util.List;

/**
 * 源测速择优(buffer-perf 规格 D⑧)纯逻辑测试。
 *
 * <p>规则:
 * <ul>
 *   <li>空表 / 全失败 → -1(不动);</li>
 *   <li>成功者中耗时最小者胜;</li>
 *   <li>当前源成功时, 最优须<b>严格</b> {@code < 0.8×current} 才返回, 恰 0.8×current 不切
 *       (抗单次采样噪声, margin 比较需要当前源也参与探测);</li>
 *   <li>当前源失败且存在成功者 → 最快成功者;</li>
 *   <li>最优 == 当前 → -1。</li>
 * </ul>
 */
public class SourceSpeedRankerTest {

    private static SourceSpeedRanker.Result ok(int idx, long ms) {
        return new SourceSpeedRanker.Result(idx, true, ms);
    }

    private static SourceSpeedRanker.Result fail(int idx) {
        return new SourceSpeedRanker.Result(idx, false, 0L);
    }

    @Test
    public void emptyList_returnsNone() {
        assertEquals(-1, SourceSpeedRanker.pickBest(Collections.<SourceSpeedRanker.Result>emptyList(), 0));
    }

    @Test
    public void allFailed_returnsNone() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(fail(0), fail(1), fail(2));
        assertEquals(-1, SourceSpeedRanker.pickBest(rs, 0));
    }

    @Test
    public void fastestSuccessWins() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(0, 1000), ok(1, 500), ok(2, 700));
        assertEquals(1, SourceSpeedRanker.pickBest(rs, 0));
    }

    /** 恰 0.8×current 不切(D⑧ 严格小于)。 */
    @Test
    public void exactlyEightyPercentOfCurrent_notSwitched() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(0, 1000), ok(1, 800));
        assertEquals(-1, SourceSpeedRanker.pickBest(rs, 0));
    }

    /** 边界内侧: 801 ≥ 800 → 不切。 */
    @Test
    public void justOverEightyPercentOfCurrent_notSwitched() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(0, 1000), ok(1, 801));
        assertEquals(-1, SourceSpeedRanker.pickBest(rs, 0));
    }

    /** 严格快超过 20% 才切: 799 < 800。 */
    @Test
    public void strictlyFasterThanEightyPercent_switched() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(0, 1000), ok(1, 799));
        assertEquals(1, SourceSpeedRanker.pickBest(rs, 0));
    }

    @Test
    public void currentFailed_fastestSuccessWins() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(fail(0), ok(1, 900), ok(2, 300));
        assertEquals(2, SourceSpeedRanker.pickBest(rs, 0));
    }

    @Test
    public void bestIsCurrent_returnsNone() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(0, 300), ok(1, 900));
        assertEquals(-1, SourceSpeedRanker.pickBest(rs, 0));
    }

    /** 防御: 当前源无结果按失败口径处理(全源探测含当前源, 正常不发生)。 */
    @Test
    public void currentMissing_treatedAsFailed() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(1, 900), ok(2, 400));
        assertEquals(2, SourceSpeedRanker.pickBest(rs, 0));
    }

    /** 并列最快取序号靠前者(确定性, 不依赖实现内部遍历方式)。 */
    @Test
    public void tie_firstIndexWins_deterministic() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(0, 1000), ok(1, 500), ok(2, 500));
        assertEquals(1, SourceSpeedRanker.pickBest(rs, 0));
    }

    /** 当前源成功但最优快超 20% 的前提是与**当前源耗时**比, 与名次无关。 */
    @Test
    public void marginIsAgainstCurrentElapsed_notRanking() {
        // current=2 (900ms), 最快=0 (750ms): 750 ≥ 0.8×900=720 → 不切
        List<SourceSpeedRanker.Result> rs = Arrays.asList(ok(0, 750), ok(1, 2000), ok(2, 900));
        assertEquals(-1, SourceSpeedRanker.pickBest(rs, 2));
        // current=2 (900ms), 最快=1 (600ms): 600 < 720 → 切到 1
        List<SourceSpeedRanker.Result> rs2 = Arrays.asList(ok(0, 750), ok(1, 600), ok(2, 900));
        assertEquals(1, SourceSpeedRanker.pickBest(rs2, 2));
    }

    /** 全失败且当前也失败 → -1(无成功者可切)。 */
    @Test
    public void currentFailedAndNoSuccess_returnsNone() {
        List<SourceSpeedRanker.Result> rs = Arrays.asList(fail(0), fail(1));
        assertEquals(-1, SourceSpeedRanker.pickBest(rs, 0));
    }
}
