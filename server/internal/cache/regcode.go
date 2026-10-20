package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

// 注册/邮件验证码协调态助手(DB1)。设计见 docs/design/账号注册功能设计.md §2.2/§5:
//   - 验证码只进 Redis 不落 MySQL: 短命一次性凭据, 淘汰后果只是"用户重点一次";
//   - 存 sha256(code+pepper) 不明文;
//   - 频率: 同邮箱 1 分钟 1 封 + 日 5 封; 全局日配额(默认 50)统一计数。
//
// coord client 为 nil(测试未接/未初始化)时一律"放行/不存在", 与 usertoken.go 同口径。

// regPepper 验证码哈希胡椒(与 Redis 内同存, 防库被拖后明文比对; 非长期凭据, 常量即可)。
const regPepper = "jerocine:regcode:v1"

// HashRegCode 验证码 → 存储用哈希(导出供测试断言"不明文落库")。
func HashRegCode(code string) string {
	sum := sha256.Sum256([]byte(code + regPepper))
	return hex.EncodeToString(sum[:])
}

// SaveRegCode 存验证码哈希, TTL(设计 5 分钟; 有意不叠 JitterTTL —— 这是凭据有效期不是缓存)。
func SaveRegCode(ctx context.Context, purpose, email, code string, ttl time.Duration) {
	if CoordClient() == nil {
		return
	}
	CoordClient().Set(ctx, KeyRegCode(purpose, email), HashRegCode(code), ttl)
}

// VerifyRegCode 校验验证码; 不存在(过期/未发)= ErrCodeExpired 语义由调用方判, 这里只回 bool。
func VerifyRegCode(ctx context.Context, purpose, email, code string) bool {
	if CoordClient() == nil {
		return true // 测试/未接 coord: 放行(与 IsUserTokenValid 同口径)
	}
	v, err := CoordClient().Get(ctx, KeyRegCode(purpose, email)).Result()
	if err != nil {
		return false
	}
	return v == HashRegCode(code)
}

// DelRegCode 消费掉验证码(注册/校验成功后一次性)。
func DelRegCode(ctx context.Context, purpose, email string) {
	if CoordClient() == nil {
		return
	}
	CoordClient().Del(ctx, KeyRegCode(purpose, email))
}

// AddRegTry 累计一次校验失败, 返回当前次数(供上层锁邮箱)。
func AddRegTry(ctx context.Context, email string) int {
	if CoordClient() == nil {
		return 0
	}
	key := KeyRegTry(email)
	n, err := CoordClient().Incr(ctx, key).Result()
	if err != nil {
		return 0
	}
	if n == 1 {
		CoordClient().Expire(ctx, key, 10*time.Minute) // 首次失败起 10 分钟锁窗
	}
	return int(n)
}

// DelRegTry 校验成功清失败计数。
func DelRegTry(ctx context.Context, email string) {
	if CoordClient() == nil {
		return
	}
	CoordClient().Del(ctx, KeyRegTry(email))
}

// AllowSendMinute 同邮箱 1 分钟 1 封: 窗口键存在即拒绝(SET NX)。
func AllowSendMinute(ctx context.Context, email string) bool {
	if CoordClient() == nil {
		return true
	}
	return CoordClient().SetNX(ctx, KeyRegSendMin(email), 1, time.Minute).Val()
}

// CountSendDay 同邮箱当日发送计数 +1, 返回当日累计(含本次)。首封起 TTL 24h。
func CountSendDay(ctx context.Context, email string) int {
	if CoordClient() == nil {
		return 0
	}
	key := KeyRegSendDay(email)
	n, err := CoordClient().Incr(ctx, key).Result()
	if err != nil {
		return 0
	}
	if n == 1 {
		CoordClient().Expire(ctx, key, 24*time.Hour)
	}
	return int(n)
}

// CountMailQuota 全局当日出站计数 +1, 返回当日累计(含本次); 首封起 TTL 24h。
func CountMailQuota(ctx context.Context, day string) int {
	if CoordClient() == nil {
		return 0
	}
	key := KeyMailQuota(day)
	n, err := CoordClient().Incr(ctx, key).Result()
	if err != nil {
		return 0
	}
	if n == 1 {
		CoordClient().Expire(ctx, key, 24*time.Hour)
	}
	return int(n)
}

// TodayDay 当日配额键片段(YYYYMMDD, 本地时区)。
func TodayDay(now time.Time) string { return now.Format("20060102") }

// AtoiSafe 解析计数(测试辅助); 非数字回 0。
func AtoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
