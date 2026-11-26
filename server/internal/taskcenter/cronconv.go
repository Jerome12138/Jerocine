package taskcenter

// cron 表达式 → 友好文本周期 的转换（统一时间语义的迁移工具）
//
// 为什么需要它：采集定时任务历史用 cron 表达式（`0 0 4 * * 0`），内置任务用友好文本
// （`weekly:7 04:00`）。两套语义并存 ⇒ 配置入口两个、中心算不了下次时间。
// 本文件把存量表达式**幂等**转换过来；转不动的（用了步长/区间/列表等复杂语法）不猜，
// 交给调用方停用并标注，等人工在任务中心改周期。
//
// 支持（实测线上两条都在覆盖范围内）：
//
//	0 */20 * * * ?   → every:20m          （分钟步长 + 其余全通配）
//	0 0 4 * * 0      → weekly:7 04:00     （周几定点；cron 的 0/7 都是周日）
//	0 30 3 * * *     → daily:03:30        （日定点）
//	0 0 4 1 * *      → monthly:1 04:00    （每月某日定点）

import (
	"strconv"
	"strings"
)

// CronSpecToSchedule 把 cron 表达式转成友好文本；不支持时 ok=false。
func CronSpecToSchedule(spec string) (string, bool) {
	fields := strings.Fields(strings.ReplaceAll(strings.TrimSpace(spec), "?", "*"))
	switch len(fields) {
	case 6: // 带秒：只接受秒为 0 或 *（本系统约定无秒级任务）
		if fields[0] != "0" && fields[0] != "*" {
			return "", false
		}
		fields = fields[1:]
	case 5:
		// 标准 5 段
	default:
		return "", false
	}
	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]
	if month != "*" {
		return "", false
	}

	// every:Nm —— 分钟步长，其余全通配
	if strings.HasPrefix(minute, "*/") && hour == "*" && dom == "*" && dow == "*" {
		n, err := strconv.Atoi(strings.TrimPrefix(minute, "*/"))
		if err != nil || n <= 0 || n > 59 {
			return "", false
		}
		return "every:" + strconv.Itoa(n) + "m", true
	}

	mm, okM := fixedField(minute, 0, 59)
	hh, okH := fixedField(hour, 0, 23)
	if !okM || !okH {
		return "", false
	}
	hhmm := pad2(hh) + ":" + pad2(mm)

	switch {
	case dom == "*" && dow == "*":
		return "daily:" + hhmm, true
	case dom == "*" && dow != "*":
		d, ok := cronWeekday(dow)
		if !ok {
			return "", false
		}
		return "weekly:" + strconv.Itoa(d) + " " + hhmm, true
	case dom != "*" && dow == "*":
		d, err := strconv.Atoi(dom)
		if err != nil || d < 1 || d > 28 { // 友好文本只允许 1~28（避开月末不存在）
			return "", false
		}
		return "monthly:" + strconv.Itoa(d) + " " + hhmm, true
	}
	return "", false
}

// fixedField 解析"单一固定数值"的字段（拒绝 * / 步长 / 区间 / 列表）。
func fixedField(v string, min, max int) (int, bool) {
	if v == "*" || strings.ContainsAny(v, "*/,-") {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, false
	}
	return n, true
}

// cronWeekday cron 的周几（0/7=周日，1=周一…6=周六）→ 友好文本的 1~7（1=周一…7=周日）。
func cronWeekday(v string) (int, bool) {
	if strings.ContainsAny(v, "*/,-") {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 7 {
		return 0, false
	}
	if n == 0 || n == 7 {
		return 7, true // 周日
	}
	return n, true
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
