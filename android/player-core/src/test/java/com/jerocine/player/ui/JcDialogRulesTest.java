package com.jerocine.player.ui;

import org.junit.Test;

import static org.junit.Assert.assertThrows;

/** JcDialog props 校验纯逻辑: title 必填 / 三插槽内容必需 / checkedIndex 越界。 */
public class JcDialogRulesTest {

    private static final CharSequence T = "标题";

    @Test
    public void valid_props_pass() {
        // 合法组合直接调用 — 抛异常即测试失败
        JcDialogRules.validate(JcDialogRules.SLOT_LIST, T, true, 5, false, false, true, 2);
        // 无选中(-1)合法; checkable=false 时 checkedIndex 不校验
        JcDialogRules.validate(JcDialogRules.SLOT_LIST, T, true, 5, false, false, true, -1);
        JcDialogRules.validate(JcDialogRules.SLOT_LIST, T, true, 1, false, false, false, 999);
        JcDialogRules.validate(JcDialogRules.SLOT_MESSAGE, T, false, 0, true, false, false, -1);
        JcDialogRules.validate(JcDialogRules.SLOT_CUSTOM, T, false, 0, false, true, false, -1);
    }

    @Test
    public void title_required() {
        assertThrows(IllegalStateException.class, () -> JcDialogRules.validate(
                JcDialogRules.SLOT_LIST, null, true, 5, false, false, false, -1));
        assertThrows(IllegalStateException.class, () -> JcDialogRules.validate(
                JcDialogRules.SLOT_LIST, "", true, 5, false, false, false, -1));
    }

    @Test
    public void list_requires_items() {
        assertThrows(IllegalStateException.class, () -> JcDialogRules.validate(
                JcDialogRules.SLOT_LIST, T, false, 0, false, false, false, -1));
    }

    @Test
    public void checkedIndex_out_of_range_when_checkable() {
        assertThrows(IllegalStateException.class, () -> JcDialogRules.validate(
                JcDialogRules.SLOT_LIST, T, true, 3, false, false, true, 3));
    }

    @Test
    public void message_and_custom_require_content() {
        assertThrows(IllegalStateException.class, () -> JcDialogRules.validate(
                JcDialogRules.SLOT_MESSAGE, T, false, 0, false, false, false, -1));
        assertThrows(IllegalStateException.class, () -> JcDialogRules.validate(
                JcDialogRules.SLOT_CUSTOM, T, false, 0, false, false, false, -1));
    }

    @Test
    public void unknown_slot_rejected() {
        assertThrows(IllegalStateException.class, () -> JcDialogRules.validate(
                99, T, true, 5, false, false, false, -1));
    }
}
