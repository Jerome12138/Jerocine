package cache

import (
	"context"
	"strconv"
	"time"
)

// 申请注册激活 token 协调态助手(DB1)。设计见 docs/design/账号注册功能设计.md §5:
//   - crypto/rand 32 字节, TTL 7 天, 最终一次性消费(事务成功后才 DEL);
//   - 有效期内的每次尝试只 peek 不消费 —— 用户名窗口期被占时用户改名重试, 链接仍有效(§9.3);
//   - 重发激活邮件 = 作废旧 token 再发新 token(反查键 KeyActivateID 支撑).
//
// coord client 为 nil(测试未接/未初始化)时一律"不存在"(激活链路 fail-closed,
// 与验证码"放行"不同: 无凭据比放过更安全, 且 service 测试都接 miniredis)。

// activateTTL 激活链接有效期 7 天(设计 §5)。
const activateTTL = 7 * 24 * time.Hour

// ActivateTTL 导出 TTL(service 存 token / 测试断言用)。
func ActivateTTL() time.Duration { return activateTTL }

// SaveActivateToken 存 token → applyID, 同时写反查键 applyID → token(重发用)。
func SaveActivateToken(ctx context.Context, token string, applyID int64, ttl time.Duration) {
	if CoordClient() == nil {
		return
	}
	CoordClient().Set(ctx, KeyActivate(token), applyID, ttl)
	CoordClient().Set(ctx, KeyActivateID(applyID), token, ttl)
}

// PeekActivateToken 只读校验 token → applyID; 不存在/过期 → (0, false)。不消费。
func PeekActivateToken(ctx context.Context, token string) (int64, bool) {
	if CoordClient() == nil {
		return 0, false
	}
	v, err := CoordClient().Get(ctx, KeyActivate(token)).Result()
	if err != nil {
		return 0, false
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// DelActivateToken 一次性消费(建号事务提交后调用): 删正向键。
// 反查键(applyID→token)留着无害: 重发时 RotateActivateToken 顶多删一个已不存在的正向键, TTL 兜底清。
func DelActivateToken(ctx context.Context, token string) {
	if CoordClient() == nil {
		return
	}
	CoordClient().Del(ctx, KeyActivate(token))
}

// RotateActivateToken 作废该申请当前激活 token(补发/重发前调用), 返回旧 token(没有则为空)。
func RotateActivateToken(ctx context.Context, applyID int64) string {
	if CoordClient() == nil {
		return ""
	}
	old, err := CoordClient().GetDel(ctx, KeyActivateID(applyID)).Result()
	if err != nil {
		return ""
	}
	CoordClient().Del(ctx, KeyActivate(old))
	return old
}
