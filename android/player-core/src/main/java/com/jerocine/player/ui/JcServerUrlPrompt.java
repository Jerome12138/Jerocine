package com.jerocine.player.ui;

import android.app.Activity;
import android.content.Context;
import android.text.InputType;
import android.view.ViewGroup;
import android.view.inputmethod.InputMethodManager;
import android.widget.EditText;
import android.widget.LinearLayout;

import com.jerocine.player.R;
import com.jerocine.player.data.ServerUrl;

/**
 * 服务器地址输入弹窗 —— 单包双壳共用的**唯一一份**（2026-11-05 组件化二期 A2 下沉）。
 *
 * <p>下沉前两壳各写一份（web 壳 {@code WebShellActivity.promptServerUrl} 手写
 * AlertDialog + 内联归一化；原生壳 {@code NativeShellActivity.onServerUrlEdit}
 * JcDialog.custom + 本地 withScheme），行为已漂移。统一规则：</p>
 * <ul>
 *   <li>归一化收口到 {@link ServerUrl#normalize}（去首尾空白+尾斜杠）+ 缺协议头补
 *       {@code http://}（见 {@link #normalizeCandidate}，纯函数有单测）；</li>
 *   <li>弹窗外框走 {@link JcDialog#custom}（铁律：禁手写 AlertDialog）；
 *       输入框样式取 core 可得令牌（jc_text/jc_text_tertiary），水平内边距用
 *       {@link JcDialog#contentPaddingX} 与标题左对齐。</li>
 * </ul>
 *
 * <p>壳特有逻辑留在 {@code onSave} 回调里：web 壳写 jerocine_prefs/server_url +
 * {@code ServiceLocator.setServer} + 重新 loadUrl；原生壳 {@code ServiceLocator.setServer}
 * + toast 反馈。软键盘拉起（原生壳既有写法）随弹窗下沉，两壳一致。</p>
 */
public final class JcServerUrlPrompt {

    /** 保存回调：url 已归一化（非空、带协议头、无尾斜杠）。 */
    public interface OnSave {
        void onSave(String url);
    }

    private JcServerUrlPrompt() {}

    /**
     * 弹出站点地址输入弹窗。
     *
     * @param activity 宿主
     * @param current  当前已存地址（可为空串 = 不预填）
     * @param onSave   保存回调（在「保存」确认后触发，url 已归一化且非空）
     */
    public static void show(Activity activity, String current, OnSave onSave) {
        EditText input = new EditText(activity);
        input.setText(current == null ? "" : current);
        input.setSelection(input.getText().length());
        input.setHint("https://example.com");
        input.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        input.setSingleLine(true);
        // 玻璃暗色弹窗里 EditText 用浅色文字/提示 —— 两壳原实现同款
        // (取色/取尺寸一律走 JcUi 唯一入口 —— A3 令牌入口铁律, R1 评审收编)
        input.setTextColor(JcUi.col(activity, R.color.jc_text));
        input.setHintTextColor(JcUi.col(activity, R.color.jc_text_tertiary));

        // 与 JcDialog 标题左对齐(内边距来自主题 dialogPreferredPadding, 不散写 24dp)
        int padX = JcDialog.contentPaddingX(activity);
        LinearLayout wrap = new LinearLayout(activity);
        wrap.setOrientation(LinearLayout.VERTICAL);
        wrap.setPadding(padX, JcUi.dp(activity, 8), padX, 0);
        wrap.addView(input, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, JcUi.dp(activity, 58)));

        JcDialog.custom(activity)
                .title("站点地址")
                .content(wrap)
                .confirmButton("保存", () -> {
                    String url = normalizeCandidate(input.getText().toString());
                    if (url.isEmpty()) {
                        GlassToast.show(activity, "地址不能为空");
                        return;
                    }
                    onSave.onSave(url);
                })
                .cancelButton("取消")
                .show();

        // 弹窗出来直接拉起软键盘(遥控器/触控都要能输入) —— 原生壳既有写法
        input.requestFocus();
        input.postDelayed(() -> {
            Object svc = activity.getSystemService(Context.INPUT_METHOD_SERVICE);
            if (svc instanceof InputMethodManager) {
                ((InputMethodManager) svc).showSoftInput(input, 0);
            }
        }, 200);
    }

    /**
     * 归一化用户输入（纯函数，单测 {@code JcServerUrlPromptTest} 锁定）：
     * {@link ServerUrl#normalize}（去首尾空白+尾斜杠）→ 缺协议头补 {@code http://}。
     * 空白输入返回空串。
     */
    static String normalizeCandidate(String raw) {
        String s = ServerUrl.normalize(raw);
        if (s.isEmpty()) {
            return s;
        }
        if (!s.startsWith("http://") && !s.startsWith("https://")) {
            s = "http://" + s;
        }
        return s;
    }
}
