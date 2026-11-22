package danmu

import (
	"testing"
)

// R1~R10 黄金用例（2026-11-15）—— 规格：Harness docs/epid-集身份/方案-集名解析口径修正-2026-11-13.md
// §一（规则表）+ §五（黄金用例清单）。全部取自现网真实集名样本。
//
// ⚠️ 两处与规格字面的**实测修正**（见该文档「附三」）：
//  1. R3 黄金用例 `20260528(副本存档中第1期)`：尾部「中」会被 seriesOf 的 part 尾截吃掉
//     （`心动的旅行中` → `心动的旅行` 是既有拍板行为，二者形状无法区分）⇒ 实得 `s=副本存档`；
//  2. R5 黄金用例 `01`/`04`：由 R13（D8-bare-num，行内 distinct ≥2 守卫）实现 ——
//     ParseEpisodeKey 级不认孤立裸数字（单条可能是线路号，宁缺勿错）。
func TestParseEpisodeKey_R1R10Golden(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  EpisodeKey
	}{
		// ---- R1 引号透明：不切段、不丢内容 ----
		{"R1 引号不切段", `20261001“聚”乐部`, EpisodeKey{Date: "20261001", Series: "聚乐部"}},
		{"R1 引号+期号", `滚烫“聚”乐部第5期`, EpisodeKey{Num: 5, Series: "滚烫聚乐部"}},
		// ---- R2 变体跨度吸收前后修饰（v 词表不动）----
		{"R2 舞台纯享版", "20261017舞台纯享版第6期", EpisodeKey{Date: "20261017", Num: 6, Variant: "纯享"}},
		{"R2 舞台纯享典藏版", "20261105舞台纯享典藏版", EpisodeKey{Date: "20261105", Variant: "纯享"}},
		{"R2 剧情纯享", "20260527第1期剧情纯享", EpisodeKey{Date: "20260527", Num: 1, Variant: "纯享"}},
		{"R2 收官特别加更版（双前缀）", "20261106收官特别加更版", EpisodeKey{Date: "20261106", Variant: "加更"}},
		// ---- R3 括号只作分隔（内容保留，护栏过滤）----
		{"R3 副本存档（尾部中被 part 尾截吃掉：实测修正①）", "20260528(副本存档中第1期)",
			EpisodeKey{Date: "20260528", Num: 1, Part: "中", Series: "副本存档"}},
		{"R3 万事屋加更", "20260529(万事屋加更第1期)", EpisodeKey{Date: "20260529", Num: 1, Variant: "加更", Series: "万事屋"}},
		{"R3 居民采访", "(居民采访第1期)", EpisodeKey{Num: 1, Series: "居民采访"}},
		{"R3 副本解锁中", "(副本解锁中第2期)", EpisodeKey{Num: 2, Part: "中", Series: "副本解锁"}},
		{"R3 护栏：非汉字段不许回退", "【qq】第1期上：又争又抢", EpisodeKey{Num: 1, Part: "上"}},
		{"R3 护栏：噪声残留 1 字", "（会员版）", EpisodeKey{Variant: "会员"}},
		// ---- R4 E 号删除不截断邻居 ----
		{"R4 EP 前缀", "EP01白昼逃亡", EpisodeKey{ENum: 1, Series: "白昼逃亡"}},
		{"R4 EP 后缀", "白昼逆行EP10", EpisodeKey{ENum: 10, Series: "白昼逆行"}},
		// ---- R6 日期合法性：伪日期不采用 ----
		{"R6 月00日70", "202300707下", EpisodeKey{Part: "下"}},
		{"R6 2月30日", "20240230上", EpisodeKey{Part: "上"}},
		{"R6 闰年 2/29 合法", "20240229上", EpisodeKey{Date: "20240229", Part: "上"}},
		{"R6 合法日期不受影响", "20240101", EpisodeKey{Date: "20240101"}},
		// ---- R7 未播/未公开进变体 ----
		{"R7 未播", "第1期未播", EpisodeKey{Num: 1, Variant: "未播"}},
		{"R7 未公开", "第2期未公开", EpisodeKey{Num: 2, Variant: "未公开"}},
		{"R7 直拍仍是噪声（P-4：与第10期同集）", "直拍REACTION第10期", EpisodeKey{Num: 10}},
		// ---- R8 噪声删除带占位：相邻汉字不拼接出假片段 ----
		{"R8 假片段防护", "你好完整版世界", EpisodeKey{Series: "你好"}},
		// ---- S1 防回归（保持现状：不跨「：」拼栏目）----
		{"S1 保持现状", "20260830大结局上：时空逃脱篇1", EpisodeKey{Date: "20260830", Part: "上", Series: "大结局"}},
	}
	for _, c := range cases {
		if got := ParseEpisodeKey(c.title); got != c.want {
			t.Errorf("%s: ParseEpisodeKey(%q) = %+v, want %+v", c.name, c.title, got, c.want)
		}
	}
}

// R8 的对照（seriesOf 层）：「你好完整版世界」若噪声删除无占位 ⇒ 相邻汉字拼出假栏目「你好世界」；
// 带占位后两段各成片段（2 字都过 2~8 闸，等长取最先 ⇒ 你好）。
func TestSeriesOf_R8噪声占位不拼假片段(t *testing.T) {
	if got := seriesOf("你好完整版世界"); got == "你好世界" {
		t.Fatalf("噪声删除无占位 ⇒ 相邻汉字拼出假片段: %q", got)
	}
	if got := seriesOf("你好完整版世界"); got != "你好" {
		t.Fatalf("带占位后应为最先的 2 字片段, got %q", got)
	}
}

// R5（= R13 落地面，D8-bare-num）：纯数字集名在**行内成串**时按集序产 n（来源 self）。
// 孤立单条不认（可能是线路号）—— 守卫是行内 distinct 裸数字 ≥2（不能用行数：双写恒成立 ✗）。
func TestR5_纯数字集名经R13产序号(t *testing.T) {
	got := EpidsForSourceRowWithPool([]string{"01", "02", "03", "04"}, nil)
	want := []string{"s=-|v=-|p=-|n=1|d=-", "s=-|v=-|p=-|n=2|d=-", "s=-|v=-|p=-|n=3|d=-", "s=-|v=-|p=-|n=4|d=-"}
	for i, g := range got {
		if g[0] != want[i] || g[1] != EpKeySourceSelf {
			t.Fatalf("titles[%d] = %v, want epid=%q from=self", i, g, want[i])
		}
	}
	// 孤立单条 ⇒ 维持 skip（宁缺勿错）
	if got := EpidsForSourceRowWithPool([]string{"01"}, nil); got[0][0] != "" || got[0][1] != EpKeySourceSkip {
		t.Fatalf("孤立裸数字应 skip, got %v", got[0])
	}
}

// R9/R10 的 D10 终局（2026-11-22）：**锚点层整体退役** —— 原来的"裸日期行收窄"不再需要：
// 任何行都不再按「上」日期计数推 n。池里只有 d+p 行（无双锚行）⇒ 一律不推 n（身份按日期，宁缺勿错）。
func TestR9R10_锚点退役后不推n(t *testing.T) {
	// 池：只有 d+p / v 行，**没有任何双锚行**（Num>0 ∧ Date≠""）⇒ 两张映射为空 ⇒ 不借
	pool := []string{"20260101上", "20260108上", "20260115上", "20260105未播", "20260106下"}
	// 裸日期行 / 带 p 行：即便池里「上」充足也不再推 n（D10 撤销锚点推算）
	for _, title := range []string{"20260105", "20260105期", " 20260106 ", "20260110上"} {
		epid, from, id := EpidForRule(title, pool)
		if from != EpKeySourceNone || id != RuleNonePoolMiss {
			t.Fatalf("R9/R10: EpidForRule(%q) 收 from=%s id=%s, want none(D6)", title, from, id)
		}
		if want := EpidOf(ParseEpisodeKey(title)); epid != want {
			t.Fatalf("R9/R10: EpidForRule(%q) = %q, want 原键（不带 n）%q", title, epid, want)
		}
	}
	// 有 v ⇒ 同样只落 d（v 是身份的一部分，n 留空）
	epid, from, id := EpidForRule("20260105未播", pool)
	if from != EpKeySourceNone || id != RuleNonePoolMiss ||
		epid != "s=-|v=未播|p=-|n=-|d=20260105" {
		t.Fatalf("有 v 行应收 none(D6) 且 n=-, got from=%s id=%s epid=%q", from, id, epid)
	}
}
