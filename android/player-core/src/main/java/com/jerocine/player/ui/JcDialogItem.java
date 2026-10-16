package com.jerocine.player.ui;

/**
 * 弹窗列表条目数据模型 — 合并"单选/单选带tag/普通列表"三种形态的关键:
 * <ul>
 *   <li>{@code text}: 主文字(必填);</li>
 *   <li>{@code tag}: 右侧灰色描述(如"已下载本集"), <b>null = 不显示、不占位</b>。</li>
 * </ul>
 * 是否显示左侧圆圈由 JcDialog.checkable() 控制, 与本模型无关。
 */
public final class JcDialogItem {

    public final CharSequence text;
    public final String tag;

    public JcDialogItem(CharSequence text) {
        this(text, null);
    }

    public JcDialogItem(CharSequence text, String tag) {
        this.text = text;
        this.tag = tag;
    }
}
