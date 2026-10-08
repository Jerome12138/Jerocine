package com.jerocine.player.download;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.RectF;
import android.os.SystemClock;
import android.view.View;

import com.jerocine.player.R;

/**
 * 圆环进度(下载行右侧, 2026-10-08 二轮) — 替代 indeterminate loading 圈:
 * <ul>
 *   <li>{@link #setProgress}: 确定态, 画青色进度弧(从顶部顺时针), 已知 totalBytes 的下载用;</li>
 *   <li>{@link #setSpinning}: 不确定态, 旋转弧段(排队中/未知总量时)。</li>
 * </ul>
 * 颜色走 player-core 令牌(jc_accent / jc_stroke_faint), 不引入新资源文件。
 */
final class RingProgressView extends View {

    /** 不确定态的刷新间隔(~30fps 足够, 省电)。 */
    private static final long FRAME_MS = 33;
    private static final long SPIN_PERIOD_MS = 1200L;

    private final Paint bgPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint arcPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF rect = new RectF();

    private boolean spinning;
    /** 0..1; 确定态进度。 */
    private float progress = 0f;
    private boolean attached;

    RingProgressView(Context c) {
        super(c);
        float stroke = c.getResources().getDisplayMetrics().density * 2.5f;
        bgPaint.setStyle(Paint.Style.STROKE);
        bgPaint.setStrokeWidth(stroke);
        bgPaint.setColor(c.getColor(R.color.jc_stroke_faint));
        arcPaint.setStyle(Paint.Style.STROKE);
        arcPaint.setStrokeWidth(stroke);
        arcPaint.setColor(c.getColor(R.color.jc_accent));
        arcPaint.setStrokeCap(Paint.Cap.ROUND);
    }

    /** 确定态(0..1); 退出不确定态。p&gt;=1 画整圈(完成前一瞬仍可能被调用)。 */
    void setProgress(float p) {
        spinning = false;
        progress = Math.max(0f, Math.min(1f, p));
        invalidate();
    }

    /** 不确定态(旋转弧段): 排队中/总大小未知。 */
    void setSpinning() {
        if (!spinning) {
            spinning = true;
            if (attached) postInvalidateDelayed(FRAME_MS);
        }
        invalidate();
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        attached = true;
        if (spinning) postInvalidateDelayed(FRAME_MS);
    }

    @Override
    protected void onDetachedFromWindow() {
        attached = false; // 挂起的 postInvalidate 因 view 已 detach 不再触发 onDraw, 无需显式取消
        super.onDetachedFromWindow();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        float s = Math.min(getWidth(), getHeight());
        float inset = bgPaint.getStrokeWidth() / 2f;
        rect.set(inset, inset, s - inset, s - inset);
        canvas.drawArc(rect, 0, 360, false, bgPaint);
        if (spinning) {
            long t = SystemClock.elapsedRealtime() % SPIN_PERIOD_MS;
            float start = t / (float) SPIN_PERIOD_MS * 360f;
            canvas.drawArc(rect, start - 90f, 100f, false, arcPaint);
            if (attached && isShown()) postInvalidateDelayed(FRAME_MS);
        } else if (progress > 0f) {
            canvas.drawArc(rect, -90f, progress >= 1f ? 360f : progress * 360f,
                    false, arcPaint);
        }
    }
}
