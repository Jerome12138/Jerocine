package cache

import (
	"context"
	"strconv"
	"time"
)

// 找回密码重置 token 协调态助手(DB1)。设计 docs/design/账号注册功能设计.md §5/§10.2:
//   - crypto/rand 32 字节, TTL 30 分钟, 最终一次性消费(改密成功后才 DEL);
//   - 有效期内的每次尝试只 peek 不消费 —— 与激活链接同范式(提交失败不烧链接);
//   - 重新申请 = 作废旧 token 再发新 token(反查键 KeyResetID 支撑)。
//
// coord client 为 nil 时一律"不存在"(fail-closed, 与激活链路口径一致)。

// pwdResetTTL 重置链接有效期 30 分钟(设计 §5: 比激活的 7 天短, 敏感操作从紧)。
const pwdResetTTL = 30 * time.Minute

// PwdResetTTL 导出 TTL(service 存 token / 测试断言用)。
func PwdResetTTL() time.Duration { return pwdResetTTL }

// SavePwdResetToken 存 token → userID, 同时写反查键 userID → token(重新申请用)。
func SavePwdResetToken(ctx context.Context, token string, userID int64, ttl time.Duration) {
	if CoordClient() == nil {
		return
	}
	CoordClient().Set(ctx, KeyReset(token), userID, ttl)
	CoordClient().Set(ctx, KeyResetID(userID), token, ttl)
}

// PeekPwdResetToken 只读校验 token → userID; 不存在/过期 → (0, false)。不消费。
func PeekPwdResetToken(ctx context.Context, token string) (int64, bool) {
	if CoordClient() == nil {
		return 0, false
	}
	v, err := CoordClient().Get(ctx, KeyReset(token)).Result()
	if err != nil {
		return 0, false
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// DelPwdResetToken 一次性消费(改密+踢下线成功后调用): 删正向键。
// 反查键(userID→token)留着无害: 重新申请时 Rotate 顶多删一个已不存在的正向键, TTL 兜底清。
func DelPwdResetToken(ctx context.Context, token string) {
	if CoordClient() == nil {
		return
	}
	CoordClient().Del(ctx, KeyReset(token))
}

// RotatePwdResetToken 作废该用户当前重置 token(重新申请前调用), 返回旧 token(没有则为空)。
func RotatePwdResetToken(ctx context.Context, userID int64) string {
	if CoordClient() == nil {
		return ""
	}
	old, err := CoordClient().GetDel(ctx, KeyResetID(userID)).Result()
	if err != nil {
		return ""
	}
	CoordClient().Del(ctx, KeyReset(old))
	return old
}
