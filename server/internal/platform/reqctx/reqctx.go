// Package reqctx 把"客户端信息"(IP / User-Agent)沿 context 传给 service 层。
//
// 动机: 登录会话要记录来源 IP/UA(后台「在线会话」页展示与踢人), 但签发 token 的路径有
// 登录 / 注册即登录 / 激活即登录 / 设备码确认四条, 逐个加参数会污染一串签名。
// 由 Trace 中间件统一把信息塞进 request context, service 需要时按需读取, 读不到就是空值。
package reqctx

import "context"

type infoKey struct{}

// Info 客户端信息。
type Info struct {
	IP string
	UA string
}

// With 把客户端信息写入 context。
func With(ctx context.Context, ip, ua string) context.Context {
	return context.WithValue(ctx, infoKey{}, Info{IP: ip, UA: ua})
}

// From 读取客户端信息; 非 HTTP 请求(定时任务/测试)返回零值。
func From(ctx context.Context) Info {
	if v, ok := ctx.Value(infoKey{}).(Info); ok {
		return v
	}
	return Info{}
}
