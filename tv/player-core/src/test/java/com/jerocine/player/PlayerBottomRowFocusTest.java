package com.jerocine.player;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.HashSet;
import java.util.Set;

/** {@link PlayerBottomRowFocus} 纯逻辑单测 — 进度条 ↓ 进底部按钮行的落点决策(2026-10-11). */
public class PlayerBottomRowFocusTest {

    private static final int PREV = R.id.btn_prev;
    private static final int NEXT = R.id.btn_next;
    private static final int EPISODES = R.id.btn_episodes;
    private static final int MORE = R.id.btn_more;

    /** 可见集合驱动的谓词替身。 */
    private static PlayerBottomRowFocus.Visibility shown(final int... visibleIds) {
        return id -> {
            for (int v : visibleIds) if (v == id) return true;
            return false;
        };
    }

    @Test
    public void rowIds_coversWholeBottomRow_inLeftToRightOrder() {
        // 行序 = 布局从左到右: 上/下集翻页 → 选集 → 换源 → 倍速 → 跳过 → 过滤 → 切本地 → 更多 → 关闭
        assertArrayEquals(new int[]{
                R.id.btn_prev, R.id.btn_next, R.id.btn_episodes, R.id.btn_source,
                R.id.btn_speed, R.id.btn_skip, R.id.btn_ad_filter,
                R.id.btn_local_toggle, R.id.btn_more, R.id.btn_close
        }, PlayerBottomRowFocus.ROW_IDS);
    }

    @Test
    public void rowIds_noDuplicates() {
        Set<Integer> seen = new HashSet<>();
        for (int id : PlayerBottomRowFocus.ROW_IDS) {
            assertTrue("重复 id: " + id, seen.add(id));
        }
    }

    @Test
    public void noMemory_picksFirstVisibleButton_notMore() {
        // 全行可见: 落第一个(btn_prev) —— 旧实现恒落「更多」, 这是本次修复的核心断言
        int picked = PlayerBottomRowFocus.pick(0, shown(PREV, NEXT, EPISODES, MORE));
        assertEquals(PREV, picked);
    }

    @Test
    public void noMemory_firstHidden_skipsToNextVisible() {
        // 本地模式等场景首几个按钮 GONE: 落第一个"可见"的
        int picked = PlayerBottomRowFocus.pick(0, shown(NEXT, MORE));
        assertEquals(NEXT, picked);
    }

    @Test
    public void memoryVisible_restoresRememberedButton() {
        // 上次选中「更多」: 记忆恢复优先于"第一个"
        int picked = PlayerBottomRowFocus.pick(MORE, shown(PREV, NEXT, EPISODES, MORE));
        assertEquals(MORE, picked);
    }

    @Test
    public void memoryHidden_fallsBackToFirstVisible() {
        // 记忆按钮已隐藏(模式切换 hideOnlineControls): 退回第一个可见
        int picked = PlayerBottomRowFocus.pick(EPISODES, shown(PREV, MORE));
        assertEquals(PREV, picked);
    }

    @Test
    public void nothingVisible_returnsZero() {
        assertEquals(0, PlayerBottomRowFocus.pick(0, shown()));
        assertEquals(0, PlayerBottomRowFocus.pick(MORE, shown()));
    }
}
