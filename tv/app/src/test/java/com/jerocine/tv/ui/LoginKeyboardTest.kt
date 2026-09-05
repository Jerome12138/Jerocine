package com.jerocine.tv.ui

import org.junit.Assert.assertEquals
import org.junit.Test

class LoginKeyboardTest {
    @Test
    fun loginKeyboardHasStableControlKeys() {
        assertEquals(listOf("退格", "清空", "空格", "登录"), loginKeyboardKeys().takeLast(4))
    }

    @Test
    fun applyLoginKeyboardKeyAppendsCharacters() {
        assertEquals("AB", applyLoginKeyboardKey("A", "B"))
    }

    @Test
    fun applyLoginKeyboardKeyHandlesBackspaceAndClear() {
        assertEquals("AB", applyLoginKeyboardKey("ABC", "BACKSPACE"))
        assertEquals("", applyLoginKeyboardKey("ABC", "CLEAR"))
    }
}
