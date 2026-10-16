package com.jerocine.player;

import androidx.annotation.IdRes;

/**
 * 底部按钮行焦点决策(纯逻辑, 2026-10-11 用户要求).
 *
 * <p>此前从进度条按 ↓ 恒送焦点到「更多」; 用户要求改为: 有"上次选中"记忆(仍可见)
 * 则恢复记忆按钮, 无记忆(或记忆按钮已隐藏)则给行内第一个可见按钮。
 *
 * <p>行序 = exo_player_control_view.xml 底部操作行从左到右(btn_prev…btn_close)。
 */
final class PlayerBottomRowFocus {

    /** 底部按钮行从左到右的 id 序(= "第一个按钮"的判定顺序)。 */
    static final int[] ROW_IDS = {
            R.id.btn_prev, R.id.btn_next, R.id.btn_episodes, R.id.btn_source,
            R.id.btn_speed, R.id.btn_skip, R.id.btn_ad_filter,
            R.id.btn_local_toggle, R.id.btn_more, R.id.btn_close
    };

    /** id→可见性谓词(隔离 View#isShown, 便于纯 JVM 单测)。 */
    interface Visibility {
        boolean isShown(@IdRes int id);
    }

    private PlayerBottomRowFocus() {}

    /**
     * 选出应获焦的按钮 id。
     *
     * @param rememberedId 上次选中记忆(0 = 无记忆)
     * @param visibility   可见性谓词
     * @return 目标按钮 id; 行内无可见按钮时返回 0(调用方回落默认焦点搜索)
     */
    static int pick(int rememberedId, Visibility visibility) {
        if (rememberedId != 0 && visibility.isShown(rememberedId)) return rememberedId;
        for (int id : ROW_IDS) {
            if (visibility.isShown(id)) return id;
        }
        return 0;
    }
}
