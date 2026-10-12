package com.jerocine.app;

import android.app.Activity;
import android.content.Intent;
import android.os.Bundle;

import com.jerocine.tv.NativeShellActivity;
// DeviceProbe 2026-10-12 起与 EntryRouterActivity 同处宿主 :app（com.jerocine.app），无需 import。

/**
 * 单包合并后的唯一 launcher Activity。
 *
 * 职责: 启动时根据"用户选择 + 设备能力探针"决定走哪个壳, 然后立即转发并 finish 自己。
 * - 用户在设置里选了 web/native → 尊重用户选择
 * - auto(默认) → DeviceProbe 判定: SDK>=24 + WebView>=87 + 内存>=2GB → WebShell, 否则 NativeShell
 *
 * 设计为无 UI 的透明转发页: theme 用 AppTheme.NoActionBarLaunch(与 WebShell 同主题),
 * 用户看到的第一帧就是目标壳的启动画面, 感知不到中间有个路由器。
 */
public class EntryRouterActivity extends Activity {

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        DeviceProbe.ShellTarget target = DeviceProbe.resolve(this);

        Intent intent;
        if (target == DeviceProbe.ShellTarget.WEB) {
            intent = new Intent(this, WebShellActivity.class);
        } else {
            intent = new Intent(this, NativeShellActivity.class);
        }
        // 保留可能的 extras(如外部 deep link 传入的参数)
        if (getIntent().getExtras() != null) {
            intent.putExtras(getIntent().getExtras());
        }
        startActivity(intent);
        finish();
        // 取消转场动画, 让路由器页→壳页的切换无感知
        overridePendingTransition(0, 0);
    }
}
