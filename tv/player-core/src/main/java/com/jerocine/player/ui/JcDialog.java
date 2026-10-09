package com.jerocine.player.ui;

import android.app.AlertDialog;
import android.content.Context;
import android.util.TypedValue;
import android.view.LayoutInflater;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ListView;
import android.widget.ScrollView;
import android.widget.TextView;

import com.jerocine.player.R;

import java.util.ArrayList;
import java.util.List;

/**
 * 播放器通用弹窗组件 — 设计见 docs/播放器通用弹窗组件设计-JcDialog-2026-10-09.md。
 * 借鉴 Element Plus 的 props-slot-event 模式: <b>外框固定(标题/内容/按钮区)、内容插槽化、参数驱动</b>。
 *
 * <p>三个入口只是预设内容区插槽类型, 返回同一 builder, props 通用:
 * <ul>
 *   <li>{@link #list}: 列表型 — checkable 控制左侧圆圈(合并普通列表/单选),
 *       条目 tag 有无控制右侧灰标(合并单选/单选带tag);</li>
 *   <li>{@link #message}: 文本型(确认/信息), 按钮文案可传参(默认「确定」/「取消」);</li>
 *   <li>{@link #custom}: 自定义 View 插槽, 外框/标题/按钮仍由组件固定。</li>
 * </ul>
 *
 * <p>外框 jc_dialog_frame.xml 自绘(不使用 AlertDialog 自带 title/buttons),
 * AlertDialog 仅作窗口容器(沿用 JcPlayerDialog 玻璃底主题);
 * {@link #show()} 是统一出口: props 校验 + 80% 屏高上限(内容少时 wrap 不拉伸)。
 * 版式 token 只在 frame/row 布局与本类, 业务代码零字号散写。
 */
public final class JcDialog {

    /** 列表项点击回调; dialog 参数用于需要 stayOnItemClick 时手动 dismiss。 */
    public interface ItemClick {
        void onClick(AlertDialog dialog, int position);
    }

    private final Context ctx;
    private final int slot;

    private CharSequence title;
    private final List<JcDialogItem> items = new ArrayList<>();
    private boolean checkable = false;
    private int checkedIndex = -1;
    private CharSequence message;
    private View content;
    private ItemClick onItemClick;
    private boolean autoDismiss = true;
    private CharSequence positiveText;
    private Runnable positiveAction;
    private CharSequence negativeText;

    private JcDialog(Context ctx, int slot) {
        this.ctx = ctx;
        this.slot = slot;
    }

    // ---------------- 入口(预设插槽类型) ----------------

    public static JcDialog list(Context c) {
        return new JcDialog(c, JcDialogRules.SLOT_LIST);
    }

    public static JcDialog message(Context c) {
        return new JcDialog(c, JcDialogRules.SLOT_MESSAGE);
    }

    public static JcDialog custom(Context c) {
        return new JcDialog(c, JcDialogRules.SLOT_CUSTOM);
    }

    // ---------------- props ----------------

    public JcDialog title(CharSequence t) {
        this.title = t;
        return this;
    }

    /** 富条目列表(tag 可选)。 */
    public JcDialog items(List<JcDialogItem> list) {
        items.clear();
        items.addAll(list);
        return this;
    }

    /** 便捷重载: 纯文字条目(tag=null)。 */
    public JcDialog items(String[] arr) {
        items.clear();
        for (String s : arr) {
            items.add(new JcDialogItem(s));
        }
        return this;
    }

    /** 列表左侧圆圈开关(默认 false = 纯菜单列表)。 */
    public JcDialog checkable(boolean v) {
        this.checkable = v;
        return this;
    }

    /** 初始选中下标(-1 = 无选中; checkable=false 时忽略)。 */
    public JcDialog checkedIndex(int i) {
        this.checkedIndex = i;
        return this;
    }

    public JcDialog onItemClick(ItemClick l) {
        this.onItemClick = l;
        return this;
    }

    /** 点击列表项后是否自动关闭(默认 true)。 */
    public JcDialog stayOnItemClick() {
        this.autoDismiss = false;
        return this;
    }

    public JcDialog message(CharSequence m) {
        this.message = m;
        return this;
    }

    /** 自定义内容插槽(外框/标题/按钮仍由组件管)。 */
    public JcDialog content(View v) {
        this.content = v;
        return this;
    }

    /** 正向按钮, 文案默认「确定」, 仅关闭无动作。 */
    public JcDialog confirmButton() {
        return confirmButton(null, null);
    }

    /** 正向按钮, 自定义文案, 仅关闭无动作。 */
    public JcDialog confirmButton(CharSequence text) {
        return confirmButton(text, null);
    }

    /** 正向按钮: 自定义文案 + 动作(先执行动作再关闭)。 */
    public JcDialog confirmButton(CharSequence text, Runnable action) {
        this.positiveText = text != null ? text : "确定";
        this.positiveAction = action;
        return this;
    }

    /** 负向按钮, 文案默认「取消」, 仅关闭无动作。 */
    public JcDialog cancelButton() {
        return cancelButton(null);
    }

    public JcDialog cancelButton(CharSequence text) {
        this.negativeText = text != null ? text : "取消";
        return this;
    }

    // ---------------- 展示 ----------------

    /**
     * 标题/列表行/自定义内容共用的水平内边距 = 主题的 dialogPreferredPadding(默认 24dp)。
     * 自定义 View 的内容区想与标题左对齐时用这个, 别再散写 24dp(主题一改就漂移)。
     */
    public static int contentPaddingX(Context c) {
        TypedValue v = new TypedValue();
        if (c.getTheme().resolveAttribute(
                androidx.appcompat.R.attr.dialogPreferredPadding, v, true)) {
            if (v.resourceId != 0) {
                return c.getResources().getDimensionPixelSize(v.resourceId);
            }
            return TypedValue.complexToDimensionPixelSize(
                    v.data, c.getResources().getDisplayMetrics());
        }
        return (int) (24 * c.getResources().getDisplayMetrics().density);
    }

    private static final int NO_BUTTON_BOTTOM_PAD_DP = 16;

    /** 统一出口: props 校验 → 装配外框 → show → 80% 屏高上限。返回弹窗句柄。 */
    public AlertDialog show() {
        JcDialogRules.validate(slot, title, !items.isEmpty(), items.size(),
                message != null, content != null, checkable, checkedIndex);

        AlertDialog dialog = new AlertDialog.Builder(ctx, R.style.JcPlayerDialog).create();
        View frame = LayoutInflater.from(ctx).inflate(R.layout.jc_dialog_frame, null);

        TextView titleView = frame.findViewById(R.id.jc_dialog_title);
        titleView.setText(title);

        ViewGroup box = frame.findViewById(R.id.jc_dialog_content);
        switch (slot) {
            case JcDialogRules.SLOT_LIST:
                box.addView(buildListView(dialog));
                break;
            case JcDialogRules.SLOT_MESSAGE:
                box.addView(wrapScroll(buildMessageView()));
                break;
            default: // SLOT_CUSTOM
                box.addView(wrapScroll(content));
                break;
        }

        boolean hasButtons = setupButtons(frame, dialog);
        // 无按钮时按钮区 GONE, 内容区直接贴底 → 补 16dp 底距(2026-10-09 用户拍板)
        if (!hasButtons) {
            box.setPadding(0, 0, 0, (int) (NO_BUTTON_BOTTOM_PAD_DP
                    * ctx.getResources().getDisplayMetrics().density));
        }

        dialog.setView(frame);
        dialog.show();
        capHeight(dialog);
        return dialog;
    }

    private View buildListView(AlertDialog dialog) {
        ListView lv = new ListView(ctx);
        lv.setDivider(null);
        lv.setAdapter(new JcDialogRowAdapter(ctx, items, checkable, checkedIndex));
        lv.setOnItemClickListener((p, v, pos, id) -> {
            if (onItemClick != null) onItemClick.onClick(dialog, pos);
            if (autoDismiss) dialog.dismiss();
        });
        // D-pad 首焦点: 有选中项落选中项, 否则首项
        final int focus = checkable ? Math.max(0, checkedIndex) : 0;
        lv.post(() -> {
            lv.setSelection(focus);
            lv.requestFocus();
        });
        return lv;
    }

    private TextView buildMessageView() {
        TextView tv = new TextView(ctx);
        tv.setText(message);
        // token: 正文 15sp jc_text(与列表主文字同一档)
        tv.setTextSize(TypedValue.COMPLEX_UNIT_SP, 15);
        tv.setTextColor(ctx.getResources().getColor(R.color.jc_text));
        int padX = contentPaddingX(ctx);
        tv.setPadding(padX, (int) (padX * 0.25f), padX, (int) (padX * 0.75f));
        return tv;
    }

    /** 内容超高时可滚动(诊断信息长文本/自定义面板)。 */
    private View wrapScroll(View body) {
        if (body.getParent() != null) {
            throw new IllegalStateException("JcDialog: content view 已有 parent, 请先从旧容器移除");
        }
        ScrollView sc = new ScrollView(ctx);
        sc.addView(body, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        return sc;
    }

    /** 装配按钮区; 无任何按钮时整块 GONE。返回是否配置了按钮。 */
    private boolean setupButtons(View frame, AlertDialog dialog) {
        LinearLayout btnRow = frame.findViewById(R.id.jc_dialog_buttons);
        Button pos = frame.findViewById(R.id.jc_dialog_positive);
        Button neg = frame.findViewById(R.id.jc_dialog_negative);
        if (positiveText == null && negativeText == null) {
            btnRow.setVisibility(View.GONE);
            return false;
        }
        if (positiveText != null) {
            pos.setText(positiveText);
            pos.setOnClickListener(v -> {
                if (positiveAction != null) positiveAction.run();
                dialog.dismiss();
            });
        } else {
            pos.setVisibility(View.GONE);
        }
        if (negativeText != null) {
            neg.setText(negativeText);
            neg.setOnClickListener(v -> dialog.dismiss());
        } else {
            neg.setVisibility(View.GONE);
        }
        return true;
    }

    /**
     * 高度上限 80% 屏(选集/换源等大列表弹窗), 内容少时保持 wrap_content 不拉伸。
     * 不能用 UNSPECIFIED 量 decor: ListView wrap-content 只实测前几个子项、其余按均值
     * 估算 → 30 集的选集弹窗实际远超 80% 却被判成"没超"。
     * 改用 AT_MOST(maxH) 上限测量, 量到上限即钉死。
     */
    private void capHeight(AlertDialog dialog) {
        android.view.Window win = dialog.getWindow();
        if (win == null) return;
        View decor = win.getDecorView();
        android.util.DisplayMetrics dm = ctx.getResources().getDisplayMetrics();
        int maxH = (int) (dm.heightPixels * 0.8f);
        decor.measure(
                View.MeasureSpec.makeMeasureSpec(dm.widthPixels, View.MeasureSpec.AT_MOST),
                View.MeasureSpec.makeMeasureSpec(maxH, View.MeasureSpec.AT_MOST));
        if (decor.getMeasuredHeight() >= maxH) {
            android.view.WindowManager.LayoutParams lp = win.getAttributes();
            lp.height = maxH;
            win.setAttributes(lp);
        }
    }
}
