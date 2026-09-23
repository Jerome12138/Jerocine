package com.jerocine.player;

import android.content.Context;
import android.content.SharedPreferences;

/**
 * 中转开关 — "媒体分片是否经服务器转发"的用户开关(默认关).
 *
 * 为什么单独成类: 该状态既跨集(开关态)又要持久化, 而 session 不碰 Context/SharedPreferences.
 * 与其它 helper 一样只依赖 {@link PlayerSession} + Host, 互不引用.
 *
 * 与"自愈"的关系(两者都写 forceRelayIdx, 但语义不同, 别合并):
 *   - 本开关 = 用户意图, 持久化, 对所有集生效(proxyMedia=1);
 *   - 直连分片失败后的自动中转(见 PlayerActivity.onPlayerError) = 单集自愈, 不落盘、不动开关,
 *     只把那一集改成中转, 保证"能播"优先.
 * 所以状态点反映的是**用户开关**, 不会因为某一集自愈而跳动.
 */
public class PlayerNetworkModeHelper {

    private static final String PREFS_NAME = "jerocine";
    private static final String PREF_NETWORK_RELAY = "network_relay_enabled";

    private final Context context;
    private final PlayerSession session;

    public PlayerNetworkModeHelper(Context context, PlayerSession session) {
        this.context = context;
        this.session = session;
    }

    private SharedPreferences prefs() {
        return context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE);
    }

    /** 读取上次的开关态(默认关 = 分片直连, 省带宽). 起播前调用一次. */
    void applyPersisted() {
        session.relayOn = prefs().getBoolean(PREF_NETWORK_RELAY, false);
    }

    /** 底栏「中转」按钮点击: 翻转 + 落盘 + 用新线路重建当前集(保留进度) + 说清用途. */
    void toggle() {
        if (!session.relayOn) {
            // 开之前先确认这条路走得通(要有代理地址, 且当前是能中转的直连清单)
            int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
            if (session.proxyBase == null || session.proxyBase.isEmpty()) {
                session.host().showCenterToast("中转不可用: 未配置代理地址", 1800);
                return;
            }
            if (!session.canSwitchNetworkMode(idx)) {
                session.host().showCenterToast(
                        session.adFilterOn
                                ? "当前片源走服务端清单, 无需中转"
                                : "请先开启「过滤」, 中转依赖代理链路", 1800);
                return;
            }
        }
        session.relayOn = !session.relayOn;
        prefs().edit().putBoolean(PREF_NETWORK_RELAY, session.relayOn).apply();
        session.forceRelayIdx.clear();
        session.forceRawIdx.clear();
        session.reloadCurrentSourceKeepPosition();
        session.host().renderNetworkMode(session.relayOn);
        session.host().showCenterToast(
                session.relayOn
                        ? "中转已开启 · 分片经服务器转发, 可绕开直连受限的片源(更耗带宽)"
                        : "中转已关闭 · 分片设备直连, 更快更省流量", 2600);
    }
}
