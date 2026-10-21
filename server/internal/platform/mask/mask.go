// Package mask 凭据展示脱敏。
//
// 管理端需要"看得见配了什么、看不见具体值", 所有凭据类字段(TMDB Key、SMTP 授权码 …)
// 统一走这里, 不再各写一份实现 —— 否则同一后台里会出现几种不同的掩码样式
// (如 a****@qq.com 与 0c06…676b), 既难认也容易某个新字段忘了脱敏。
//
// 口径(唯一):
//   - 空串 → 空串。调用方靠"空串 = 未配置"区分状态, 不要替换成占位符;
//   - 字符数 < 12 → 整串打码 ••••（首尾各露 4 位会超过一半, 失去脱敏意义）;
//   - 其余 → 首 4 + "…" + 末 4。
//
// 明文只应存在于服务端内部: 任何管理端 DTO 只回传本函数的返回值。
package mask

const (
	// Ellipsis 中间省略号(U+2026)。
	Ellipsis = "…"
	// Redacted 值过短时的整串打码。
	Redacted = "••••"
)

// Middle 返回 s 的中间脱敏形式(首 4 + … + 末 4)。按 rune 切, 多字节凭据不会截断半个字符。
func Middle(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) < 12 {
		return Redacted
	}
	return string(r[:4]) + Ellipsis + string(r[len(r)-4:])
}
