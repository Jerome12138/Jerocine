package com.jerocine.player.ui;

import android.content.Context;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.StateListDrawable;
import android.util.TypedValue;

import androidx.core.content.ContextCompat;

import com.jerocine.player.R;

/**
 * 编程式 UI 构建的共享工具 —— dp 换算 / 资源取色取尺寸 / 玻璃风格背景的**唯一实现**（单包双壳共用，
 * 组件化二期 A3 下沉）。此前 WebShellActivity 与 JcSettingsDrawer 各存一份逐行相同的
 * dp()/col()/buildDrawerButtonBg()，BrandSplashView / DownloadActivity 又各有一份 dp() ——
 * 公式漂移风险集中在壳层；现在公式只在本类存在一份，壳层方法仅剩一行转引。
 *
 * 设计令牌铁律（CLAUDE.md）：颜色/尺寸唯一真源是本模块 res/values/colors.xml + res/values/dimens.xml，
 * 本类只经资源 id 解析，不写任何十六进制。
 */
public final class JcUi {

    private JcUi() {
    }

    /**
     * dp → px（TypedValue 语义：v*density 后截断）。
     * 与原 WebShellActivity.dp / JcSettingsDrawer.dp / BrandSplashView.dp 逐行等值 —— 值不变。
     */
    public static int dp(Context ctx, int v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v,
                ctx.getResources().getDisplayMetrics());
    }

    /**
     * dp → px（+0.5f 四舍五入）。
     * ⚠️ 与 {@link #dp} 是**两套刻意保留的语义**：原 DownloadActivity.dp(float) 用四舍五入，
     * 其余副本用 TypedValue 截断 —— 在分数密度(如 TV 常见的 1.5/tvdpi 1.33125)下两者可差 1px
     * （例：density 1.5 时 dp(1) → 截断 1px / 四舍五入 2px）。合并时按"值不变"铁律各保原语义。
     */
    public static int dpRound(Context ctx, float v) {
        return (int) (v * ctx.getResources().getDisplayMetrics().density + 0.5f);
    }

    /** 从资源取尺寸(px) —— 尺寸唯一真源是 res/values/dimens.xml（原 JcSettingsDrawer.dimenPx 逐行等值）。 */
    public static int dimenPx(Context ctx, int dimenResId) {
        return (int) ctx.getResources().getDimension(dimenResId);
    }

    /**
     * 从资源取色 —— 编程式 View 构建的**唯一取色入口**（原 WebShellActivity.col /
     * JcSettingsDrawer.col 逐行等值）。
     */
    public static int col(Context ctx, int colorResId) {
        return ContextCompat.getColor(ctx, colorResId);
    }

    /**
     * 抽屉行/按钮背景: 聚焦=青色描边+浅青底, 按下=深青, 常态=玻璃面。
     * 原 WebShellActivity.buildDrawerButtonBg ≡ JcSettingsDrawer.buildDrawerButtonBg（逐行相同）
     * 合并为这一份（组件化二期 A3"两份逐行合并"）。
     */
    public static Drawable glassButtonBg(Context ctx) {
        GradientDrawable focused = new GradientDrawable();
        focused.setColor(col(ctx, R.color.jc_accent_dim));
        focused.setCornerRadius(dp(ctx, 14));
        focused.setStroke(dp(ctx, 2), col(ctx, R.color.jc_accent));

        GradientDrawable pressed = new GradientDrawable();
        pressed.setColor(col(ctx, R.color.jc_accent_press));
        pressed.setCornerRadius(dp(ctx, 14));

        GradientDrawable normal = new GradientDrawable();
        normal.setColor(col(ctx, R.color.jc_glass_soft));
        normal.setCornerRadius(dp(ctx, 14));
        normal.setStroke(dp(ctx, 1), col(ctx, R.color.jc_stroke));

        StateListDrawable sld = new StateListDrawable();
        sld.addState(new int[]{android.R.attr.state_focused}, focused);
        sld.addState(new int[]{android.R.attr.state_pressed}, pressed);
        sld.addState(android.util.StateSet.WILD_CARD, normal);
        return sld;
    }

    /**
     * 玻璃浮层/卡片背景: 半透明深色面 + 1px 细描边 + 大圆角(供抽屉/无网络页等编程式浮层复用)。
     * 原 WebShellActivity.buildGlassPanelBg(radiusDp) 逐行等值。
     */
    public static Drawable glassPanelBg(Context ctx, int radiusDp) {
        GradientDrawable g = new GradientDrawable();
        g.setColor(col(ctx, R.color.jc_glass));
        g.setCornerRadius(dp(ctx, radiusDp));
        g.setStroke(dp(ctx, 1), col(ctx, R.color.jc_stroke));
        return g;
    }
}
