package com.jerocine.app;

import android.content.Context;
import android.graphics.drawable.GradientDrawable;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.animation.AlphaAnimation;
import android.view.animation.Animation;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.TextView;

/**
 * 原生品牌首屏(方案 §7) —— 替代原来"黑底 + 裸转圈"的启动动画。
 *
 * 形态: 站点根背景色(@color/jc_bg #0B0B0F) + 居中品牌 logo + 品牌名 + 底部呼吸 loading(三点)。
 * 系统 splash 与页面根背景也用同一色(styles.xml / activity_main) ⇒ 三段无跳变。
 *
 * 三条硬约束(评审 C9/C10 校正后确认):
 *  1. **不展示白屏**: 现状本就不是白屏(styles.xml 已显式黑底), 这里只是把它品牌化, 不改性能;
 *  2. **不拦截遥控器按键**: 本视图 setFocusable(false) / setClickable(false) ⇒ 不吃 D-pad,
 *     按键直接落到 WebView(否则用户会觉得首屏"假死");
 *  3. **必须有超时兜底**: MainActivity 不能覆盖 WebViewClient(会破坏 Capacitor 注入),
 *     拿不到 onReceivedError —— 所以只能由调用方给一个 15s 超时强制淡出, 露出 WebView 内容/错误。
 *
 * 淡出由 {@link #fadeOutAndRemove(Runnable)} 驱动(幂等), 200ms 后把自身从父容器移除。
 */
public class BrandSplashView extends FrameLayout {

    /** 淡出时长(方案 §7.3) */
    static final long FADE_OUT_MS = 200L;

    private static final int JC_BG = 0xFF0B0B0F;      // 与站点根背景一致
    private static final int JC_TEXT = 0x99FFFFFF;    // 品牌名: 白 60%
    private static final int JC_ACCENT = 0xFF4AD1E5;  // 呼吸点: 强调青

    private boolean hidden = false;

    public BrandSplashView(Context context) {
        super(context);
        build();
    }

    private int dp(int v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v,
                getResources().getDisplayMetrics());
    }

    private void build() {
        setBackgroundColor(JC_BG);
        // 不拦截任何输入: 不可聚焦 ⇒ 不参与焦点/D-pad 分发; 不可点 ⇒ 不吃触摸
        setFocusable(false);
        setFocusableInTouchMode(false);
        setClickable(false);

        LinearLayout col = new LinearLayout(getContext());
        col.setOrientation(LinearLayout.VERTICAL);
        col.setGravity(Gravity.CENTER_HORIZONTAL);
        FrameLayout.LayoutParams colLp = new FrameLayout.LayoutParams(
                LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, Gravity.CENTER);
        addView(col, colLp);

        // 品牌 logo(复用 adaptive icon 的前景图, 全版本可用的位图)
        // 不做 try/catch: R.drawable.* 是编译期常量 —— 资源缺失在编译期就会失败,
        // 运行期不可能走到"缺图"分支(留个假兜底反而让人以为这里能容错)。
        ImageView logo = new ImageView(getContext());
        int logoSize = dp(96);
        logo.setImageResource(R.drawable.ic_launcher_foreground_gradient03);
        col.addView(logo, new LinearLayout.LayoutParams(logoSize, logoSize));

        // 品牌名
        TextView name = new TextView(getContext());
        name.setText("Jerocine 影视");
        name.setTextColor(JC_TEXT);
        name.setTextSize(TypedValue.COMPLEX_UNIT_SP, 18);
        name.setLetterSpacing(0.08f);
        name.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams nameLp = new LinearLayout.LayoutParams(
                LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
        nameLp.topMargin = dp(18);
        col.addView(name, nameLp);

        // 呼吸 loading: 三点错相位明暗循环(比裸转圈更"品牌化", 也不抢注意力)
        LinearLayout dots = new LinearLayout(getContext());
        dots.setOrientation(LinearLayout.HORIZONTAL);
        dots.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams dotsLp = new LinearLayout.LayoutParams(
                LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
        dotsLp.topMargin = dp(22);
        col.addView(dots, dotsLp);

        int dotSize = dp(6);
        for (int i = 0; i < 3; i++) {
            View dot = new View(getContext());
            GradientDrawable bg = new GradientDrawable();
            bg.setShape(GradientDrawable.OVAL);
            bg.setColor(JC_ACCENT);
            dot.setBackground(bg);
            LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(dotSize, dotSize);
            lp.leftMargin = dp(4);
            lp.rightMargin = dp(4);
            dots.addView(dot, lp);

            AlphaAnimation anim = new AlphaAnimation(0.22f, 1f);
            anim.setDuration(700);
            anim.setStartOffset(i * 180L);
            anim.setRepeatCount(Animation.INFINITE);
            anim.setRepeatMode(Animation.REVERSE);
            dot.startAnimation(anim);
        }
    }

    /**
     * 淡出并移除(幂等)。onEnd 可为 null —— 在动画结束后回调(通常用于把焦点交回 WebView)。
     * 幂等靠 hidden 位保证: 超时兜底与加载完成可能同时触发, 第二次直接返回。
     */
    public void fadeOutAndRemove(final Runnable onEnd) {
        if (hidden) return;
        hidden = true;
        // 先停掉子视图动画, 避免淡出期间还在跑无限动画
        dismissChildAnimations(this);
        animate()
                .alpha(0f)
                .setDuration(FADE_OUT_MS)
                .withEndAction(new Runnable() {
                    @Override
                    public void run() {
                        View parent = (View) getParent();
                        if (parent instanceof FrameLayout) {
                            ((FrameLayout) parent).removeView(BrandSplashView.this);
                        }
                        if (onEnd != null) onEnd.run();
                    }
                })
                .start();
    }

    private static void dismissChildAnimations(View v) {
        if (!(v instanceof android.view.ViewGroup)) return;
        android.view.ViewGroup g = (android.view.ViewGroup) v;
        for (int i = 0; i < g.getChildCount(); i++) {
            View child = g.getChildAt(i);
            child.clearAnimation();
            dismissChildAnimations(child);
        }
    }

    /** 便捷: 直接淡出(无回调) */
    public void fadeOutAndRemove() {
        fadeOutAndRemove(null);
    }
}
