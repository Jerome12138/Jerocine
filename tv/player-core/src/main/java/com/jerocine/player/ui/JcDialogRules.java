package com.jerocine.player.ui;

/**
 * JcDialog props 校验(纯逻辑, JVM 可测):
 * builder 防呆挡不住所有组合(反射/误用/将来新增入口), show() 前最后闸一道。
 * 校验规则:
 * <ul>
 *   <li>title 必填;</li>
 *   <li>LIST 插槽必须有 items, 且 checkable 时 checkedIndex 不得越界(-1 = 无选中, 合法);</li>
 *   <li>MESSAGE 插槽必须有 message;</li>
 *   <li>CUSTOM 插槽必须有 content。</li>
 * </ul>
 */
final class JcDialogRules {

    static final int SLOT_LIST = 0;
    static final int SLOT_MESSAGE = 1;
    static final int SLOT_CUSTOM = 2;

    private JcDialogRules() {
    }

    static void validate(int slot, CharSequence title, boolean hasItems, int itemCount,
                         boolean hasMessage, boolean hasContent,
                         boolean checkable, int checkedIndex) {
        if (title == null || title.length() == 0) {
            throw new IllegalStateException("JcDialog: title 必填");
        }
        switch (slot) {
            case SLOT_LIST:
                if (!hasItems) {
                    throw new IllegalStateException("JcDialog: list 弹窗必须传入 items");
                }
                if (checkable && checkedIndex >= itemCount) {
                    throw new IllegalStateException("JcDialog: checkedIndex(" + checkedIndex
                            + ") 越界, items.size=" + itemCount + "(-1 表示无选中)");
                }
                break;
            case SLOT_MESSAGE:
                if (!hasMessage) {
                    throw new IllegalStateException("JcDialog: message 弹窗必须传入 message");
                }
                break;
            case SLOT_CUSTOM:
                if (!hasContent) {
                    throw new IllegalStateException("JcDialog: custom 弹窗必须传入 content view");
                }
                break;
            default:
                throw new IllegalStateException("JcDialog: 未知插槽类型 " + slot);
        }
    }
}
