package com.jerocine.player.data;

import androidx.annotation.Nullable;

/**
 * 网络可达性判定的**纯决策函数**(组件化二期 A5 下沉, 单包双壳共用一份)。
 *
 * <p>只做"探测结论 → 是否有网"的纯映射, 不碰 ConnectivityManager —— 服务的获取与
 * try/catch 留在各壳(那部分不纯), 纯部分才可被 JVM 单测锁定方向语义(NetLinkTest)。</p>
 *
 * <p><b>fail-safe 方向参数为什么必须存在</b>: 探测可能给出**确定**结论(拿到 ActiveNetworkInfo
 * 且 isConnected 可读), 也可能**判定不了**(系统服务缺失 / 探测抛异常)。判定不了时按哪个方向
 * 兜底是各调用方的安全属性, 不能被"统一"掉:</p>
 * <ul>
 *   <li>{@link #FAIL_OPEN}: 按"有网"兜底 —— WebShellActivity.isOnline(loadServer 前置判定),
 *       错报"无网"会把有网用户误拦在离线兜底页;</li>
 *   <li>{@link #FAIL_CLOSED}: 按"无网"兜底 —— 保守方向, 供降级计数这类"宁可不计数也不把
 *       链路抖动误计为 WebView 能力不足"的场景(误计会把弱网用户**永久**踢进原生壳)。</li>
 * </ul>
 *
 * <p>方向语义由 :app 的 NetLinkTest 双向锁定; 两壳调用点的现状行为(含各自注释口径)在
 * 下沉时逐分支保留 —— 见两处调用点注释。</p>
 */
public final class NetLink {

    /** 方向: 判定不了 → 按"有网"兜底(fail-open, WebShellActivity.isOnline 现行语义)。 */
    public static final boolean FAIL_OPEN = true;
    /** 方向: 判定不了 → 按"无网"兜底(fail-closed, 保守方向; 语义已由 NetLinkTest 锁定)。 */
    public static final boolean FAIL_CLOSED = false;

    private NetLink() {
    }

    /**
     * 一次网络探测的结论 → 是否有网。
     *
     * @param determined 探测结论: {@code TRUE}=有网(ActiveNetworkInfo 存在且已连接);
     *                   {@code FALSE}=无网(**两壳既有口径一致**: 拿得到服务但拿不到
     *                   ActiveNetworkInfo, 与"拿得到但未连接"同样按无网处理);
     *                   {@code null}=判定不了(拿不到 ConnectivityManager / 探测抛异常)。
     * @param failOpen   方向参数: {@link #FAIL_OPEN} / {@link #FAIL_CLOSED}。
     *                   只作用于 {@code null}(判定不了)分支 —— 确定结论与方向无关。
     */
    public static boolean isOnline(@Nullable Boolean determined, boolean failOpen) {
        if (determined == null) {
            return failOpen;
        }
        return determined;
    }
}
