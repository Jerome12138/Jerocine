package com.jerocine.app;

import android.content.Context;
import android.view.Gravity;
import android.view.LayoutInflater;
import android.view.View;
import android.widget.ImageView;
import android.widget.TextView;
import android.widget.Toast;

import androidx.annotation.DrawableRes;

/**
 * 共享玻璃 Toast — 把系统默认灰底小条统一换成项目的暗色玻璃拟态风.
 *
 * 实现: new Toast(ctx) + setView(自定义玻璃布局 @layout/gf_toast), 复用 @drawable/gf_toast_bg
 * (与 PlayerActivity 内 center_toast 同款: 半透明深色面 + 1px 细描边 + 大圆角 + 白字),
 * 与全局玻璃令牌(colors.xml)一致, 不另造样式. 居中靠下显示, 支持可选左侧图标.
 *
 * MainActivity / PlayerActivity / JerocineBridge / UpdateChecker 的所有
 * Toast.makeText(...).show() 均改走此处, 文案不变.
 *
 * 注: setView/new Toast 在 API30+ 对"文本 toast"已弃用, 但对自定义 view toast 仍受支持且按预期工作;
 * minSdk=24, 行为一致.
 */
public final class GlassToast {

    private GlassToast() {}

    /** 短时玻璃 Toast (无图标). */
    public static void show(Context ctx, CharSequence msg) {
        show(ctx, msg, Toast.LENGTH_SHORT, 0);
    }

    /** 指定时长(Toast.LENGTH_SHORT / LENGTH_LONG)的玻璃 Toast (无图标). */
    public static void show(Context ctx, CharSequence msg, int duration) {
        show(ctx, msg, duration, 0);
    }

    /**
     * 带左侧图标的玻璃 Toast.
     * @param iconRes 图标资源 id, 传 0 表示无图标
     */
    @SuppressWarnings("deprecation") // 自定义 view toast: setView 在弃用后仍按预期工作
    public static void show(Context ctx, CharSequence msg, int duration, @DrawableRes int iconRes) {
        if (ctx == null || msg == null) return;
        View view = LayoutInflater.from(ctx).inflate(R.layout.gf_toast, null, false);
        TextView text = view.findViewById(R.id.gf_toast_text);
        text.setText(msg);
        ImageView icon = view.findViewById(R.id.gf_toast_icon);
        if (iconRes != 0) {
            icon.setImageResource(iconRes);
            icon.setVisibility(View.VISIBLE);
        } else {
            icon.setVisibility(View.GONE);
        }
        Toast toast = new Toast(ctx);
        toast.setDuration(duration == Toast.LENGTH_LONG ? Toast.LENGTH_LONG : Toast.LENGTH_SHORT);
        toast.setView(view);
        // 居中靠下: 比默认底部略上, 避开全面屏手势区, 又不挡正中内容.
        toast.setGravity(Gravity.CENTER_HORIZONTAL | Gravity.BOTTOM, 0,
                (int) (96 * ctx.getResources().getDisplayMetrics().density));
        toast.show();
    }
}
