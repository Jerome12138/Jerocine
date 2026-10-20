package mailer

import "fmt"

// 出站邮件模板(中文简版, 代码内置不发配置 —— 过度设计)。
// 每封含: 主题 + 正文(关键信息前置, 行动指引收尾)。

// CodeSubject 验证码邮件主题(按用途区分, 便于用户邮箱里检索)。
func CodeSubject(purpose string) string {
	switch purpose {
	case "reset":
		return "Jerocine影视 — 找回密码验证码"
	default:
		return "Jerocine影视 — 注册验证码"
	}
}

// CodeBody 验证码正文。ttlMin 分钟有效。
func CodeBody(purpose, code string, ttlMin int) string {
	verb := "注册"
	if purpose == "reset" {
		verb = "找回密码"
	}
	return fmt.Sprintf(`您正在%s Jerocine影视 账号。

验证码: %s

验证码 %d 分钟内有效, 请尽快完成验证。
如果这不是您本人的操作, 请忽略本邮件, 账号不会受到任何影响。`, verb, code, ttlMin)
}

// ActivateSubject 激活邮件主题(申请注册审批通过)。
const ActivateSubject = "Jerocine影视 — 注册申请已通过, 请激活账号"

// ActivateBody 激活正文: 一次性链接(7 天有效) + 丢失邮件的兜底指引(进度查询重发)。
func ActivateBody(activateURL string) string {
	return fmt.Sprintf(`您的 Jerocine影视 注册申请已通过审核, 请点击以下链接设置密码并激活账号:

%s

链接 7 天内有效, 且只能使用一次。
如果链接打不开, 可复制到浏览器地址栏访问; 若邮件丢失或链接过期, 可在登录页通过「查询注册申请进度」用邮箱重新获取。`, activateURL)
}

// RejectSubject 驳回通知主题。
const RejectSubject = "Jerocine影视 — 注册申请未通过"

// RejectBody 驳回正文: 必含理由 + 重申入口。
func RejectBody(reason string) string {
	return fmt.Sprintf(`很抱歉, 您的 Jerocine影视 注册申请未通过审核。

驳回理由: %s

您可以修正后重新提交申请, 或在登录页通过「查询注册申请进度」查看详情。`, reason)
}

// ResetSubject 重置密码链接邮件主题(P3)。
const ResetSubject = "Jerocine影视 — 重置密码"

// ResetBody 重置密码链接正文: 一次性链接(30 分钟有效) + 安全提示(非本人操作可忽略,
// 改密成功后全部设备将退出登录)。
func ResetBody(resetURL string) string {
	return fmt.Sprintf(`您正在重置 Jerocine影视 账号密码。

请点击以下链接设置新密码:

%s

链接 30 分钟内有效, 且只能使用一次。
如果这不是您本人的操作, 请忽略本邮件, 密码不会改变。`, resetURL)
}

// TestSubject 管理后台测试邮件主题。
const TestSubject = "Jerocine影视 — 邮件通道测试"

// TestBody 测试邮件正文。
func TestBody() string {
	return `这是一封来自 Jerocine影视 管理后台的测试邮件。

如果您收到了这封邮件, 说明当前 SMTP 邮件通道配置正确, 可以正常发送验证码、激活链接等邮件。`
}
