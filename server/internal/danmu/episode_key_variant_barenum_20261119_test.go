package danmu

import "testing"

// 本批（2026-11-19 交接版方案 ①②③，docs/epid-集身份/方案/方案-变体归一与去前缀与去诊断档-2026-11-19）：
// ① 变体复合词整段识别+归一（§1，含 §1.3 三条守卫反例）；
// ② 裸数字紧跟日期则不剥（§2，创造101 类）；
// ③ StripFilmNamePrefix 放宽 A/B/C（§3）—— 反例（另一部剧#S2/普通男女）按评审要求保留。

// §1.4 变体复合词整段识别 + 归一（正例 9 条 + 守卫反例 3 条）。
func TestVariantCompound_Canon(t *testing.T) {
	cases := []struct {
		title string
		wantV string
		wantS string
	}{
		{"超前营业", "超前", ""},
		{"超长花絮", "花絮", ""},
		{"回顾特辑", "特辑", ""},
		{"竖屏舞台纯享（版）", "纯享", ""},
		{"剧目纯享版", "纯享", ""},
		{"特别加更", "加更", ""},
		{"抢鲜看", "超前", ""},
		{"福利篇", "福利篇", ""},
		{"母带", "母带", ""},
	}
	for _, c := range cases {
		k := ParseEpisodeKey(c.title)
		if k.Variant != c.wantV || k.Series != c.wantS {
			t.Errorf("%q ⇒ 期望 v=%q s=%q，实际 v=%q s=%q（全键 %+v）", c.title, c.wantV, c.wantS, k.Variant, k.Series, k)
		}
	}
	// 守卫1 反例：营业中（无"超前"）⇒ v 为空、营业不被变体跨度吞掉。
	// 「中」被 parsePart 取走（p=中）属既有行为，改动前即 s=营业 p=中（与本批无关）。
	if k := ParseEpisodeKey("营业中"); k.Variant != "" || k.Series != "营业" || k.Part != "中" {
		t.Errorf("营业中 反例：期望 v=\"\" s=营业 p=中，实际 %+v", k)
	}
	// 守卫1 正面：营业只在紧邻"超前"之后才被吃 ⇒ 超前营业整段消失
	if k := ParseEpisodeKey("20260805超前营业"); k.Variant != "超前" || k.Series != "" {
		t.Errorf("20260805超前营业：期望 v=超前 s=\"\"，实际 %+v", k)
	}
	// 守卫3 反例：特别企划（栏目名）⇒ 「特别」只作加更/篇/特辑修饰，不误吃
	if k := ParseEpisodeKey("特别企划"); k.Series != "特别企划" || k.Variant != "" {
		t.Errorf("特别企划 反例：期望 s=特别企划 v=\"\"，实际 %+v", k)
	}
	// 守卫2 反例：片名含"母带" ⇒ s 保留、不归 v
	if k := ParseEpisodeKey("母带修复计划"); k.Series != "母带修复计划" || k.Variant != "" {
		t.Errorf("母带修复计划 反例：期望 s 保留 v=\"\"，实际 %+v", k)
	}
	// 守卫2 正面：第N期后缀形态的 母带（s 恰为"母带"）⇒ 归 v
	if k := ParseEpisodeKey("第3期母带"); k.Variant != "母带" || k.Series != "" {
		t.Errorf("第3期母带：期望 v=母带 s=\"\"，实际 %+v", k)
	}
	// 复合 + 既有词共存：归一去重，超前营业不该产出 v=超前,超前
	if k := ParseEpisodeKey("20260805超前营业彩蛋"); k.Variant != "彩蛋,超前" || k.Series != "" {
		t.Errorf("超前营业+彩蛋：期望 v=彩蛋,超前 s=\"\"，实际 %+v", k)
	}
	// 复合短语 + 变体序号规则不受影响：特辑2 ⇒ v=特辑#2（§1.3 守卫第 4 行）
	if k := ParseEpisodeKey("回顾特辑2"); k.Variant != "特辑" || k.VariantNum != 2 || k.Series != "" {
		t.Errorf("回顾特辑2：期望 v=特辑 vn=2 s=\"\"，实际 %+v", k)
	}
	// 守卫4 对新词闭环（评审补全，2026-11-19）：词后紧跟数字 ⇒ VariantNum，
	// 否则「福利篇2」vn=0 与「福利篇」同身份 ⇒ L3a/L1 判齐合并弹幕池。
	if k := ParseEpisodeKey("福利篇2"); k.Variant != "福利篇" || k.VariantNum != 2 || k.Series != "" {
		t.Errorf("福利篇2：期望 v=福利篇 vn=2 s=\"\"，实际 %+v", k)
	}
	if k := ParseEpisodeKey("母带2"); k.Variant != "母带" || k.VariantNum != 2 || k.Series != "" {
		t.Errorf("母带2：期望 v=母带 vn=2 s=\"\"，实际 %+v", k)
	}
	// 抢鲜看 归一为 超前 ⇒ 「抢鲜看2」与「超前2」键一致（epid 同串 s=-|v=超前#2）
	a, b := ParseEpisodeKey("抢鲜看2"), ParseEpisodeKey("超前2")
	if a.Variant != "超前" || a.VariantNum != 2 || a.Series != "" || a.Variant != b.Variant || a.VariantNum != b.VariantNum {
		t.Errorf("抢鲜看2 应与 超前2 同键（v=超前 vn=2），实际 抢鲜看2=%+v 超前2=%+v", a, b)
	}
	// 序号区分身份：福利篇2 与 福利篇 必须不同
	if ka, kb := ParseEpisodeKey("福利篇2"), ParseEpisodeKey("福利篇"); ka.VariantNum == kb.VariantNum {
		t.Errorf("福利篇2(vn=%d) 与 福利篇(vn=%d) 不应同身份", ka.VariantNum, kb.VariantNum)
	}
	// 新词 + 纯日期：福利篇20180623 ⇒ vn=0（reVariantNum 的 [^0-9]|$ 消费断言不吃日期，
	// 与 特辑2026→0 同规，评审复核 2026-11-20）
	if k := ParseEpisodeKey("福利篇20180623"); k.Variant != "福利篇" || k.VariantNum != 0 || k.Date != "20180623" || k.Series != "" {
		t.Errorf("福利篇20180623：期望 v=福利篇 vn=0 d=20180623 s=\"\"，实际 %+v", k)
	}
}

// §2.3 裸数字紧跟日期则不剥（正例 3 + 反例 2 + epid 端到端）。
func TestBareNumBeforeDate_KeptInSeries(t *testing.T) {
	// 正例：裸数字（无 第/集/期/话/回 标记）紧跟日期 ⇒ 数字属片名，保留在 s
	if k := ParseEpisodeKey("创造1012018.06.23"); k.Series != "创造101" || k.Date != "20180623" {
		t.Errorf("创造1012018.06.23 ⇒ 期望 s=创造101 d=20180623，实际 %+v", k)
	}
	if k := ParseEpisodeKey("创造1012018-06-23"); k.Series != "创造101" || k.Date != "20180623" {
		t.Errorf("创造1012018-06-23 ⇒ 期望 s=创造101 d=20180623，实际 %+v", k)
	}
	if k := ParseEpisodeKey("第101次求婚2018.06.23"); k.Series != "第101次求婚" || k.Date != "20180623" || k.Num != 0 {
		t.Errorf("第101次求婚2018.06.23 ⇒ 期望 s=第101次求婚 d=20180623 n=0，实际 %+v", k)
	}
	// 反例1：第5集20250101 —— 「5」有 第N集 标记 ⇒ 仍走原路（n=5、s 空、d 正常）
	if k := ParseEpisodeKey("第5集20250101"); k.Num != 5 || k.Date != "20250101" || k.Series != "" {
		t.Errorf("第5集20250101 反例：期望 n=5 d=20250101 s=\"\"，实际 %+v", k)
	}
	// 反例2：奔跑吧20180623 —— 无裸数字（数字本身是日期）⇒ 不受影响
	if k := ParseEpisodeKey("奔跑吧20180623"); k.Series != "奔跑吧" || k.Date != "20180623" || k.Num != 0 {
		t.Errorf("奔跑吧20180623 反例：期望 s=奔跑吧 d=20180623 n=0，实际 %+v", k)
	}
	// epid 端到端：s 不再被削短 ⇒ 与其它源（s=创造101 / s=-）可对上
	got := EpidsForSourceRow([]string{"创造1012018.06.23"})
	if len(got) != 1 || got[0][0] != "s=创造101|v=-|p=-|n=-|d=20180623" {
		t.Errorf("epid 端到端：期望 s=创造101|v=-|p=-|n=-|d=20180623，实际 %v", got)
	}
}

// §3.3 StripFilmNamePrefix 放宽 A/B/C（在既有 TestStripFilmNamePrefix 5 正 3 反基础上追加；
// 评审注记：C 情形跨片误伤理论面用户知情接受，跨片反例务必保留）。
func TestStripFilmNamePrefix_ABC(t *testing.T) {
	// A 相等：s=创造101 / 片名 创造101
	{
		out := [][2]string{{"s=创造101|v=-|p=-|n=1|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "创造101"); n != 1 || out[0][0] != "s=-|v=-|p=-|n=1|d=-" {
			t.Fatalf("A 相等应清空 s，实际 n=%d out=%v", n, out)
		}
	}
	// C 片名去尾数字后相等：s=创造 / 片名 创造101（s 被削短的兜底）
	{
		out := [][2]string{{"s=创造|v=-|p=-|n=1|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "创造101"); n != 1 || out[0][0] != "s=-|v=-|p=-|n=1|d=-" {
			t.Fatalf("C 应清空 s，实际 n=%d out=%v", n, out)
		}
	}
	// B 反向：片名是 s 的前缀 —— s=创造101 / 片名 创造
	{
		out := [][2]string{{"s=创造101|v=-|p=-|n=1|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "创造"); n != 1 || out[0][0] != "s=-|v=-|p=-|n=1|d=-" {
			t.Fatalf("B 反向应清空 s，实际 n=%d out=%v", n, out)
		}
	}
	// 反例（评审要求保留）：另一部剧#S2 / 普通男女 ⇒ 保留不动
	{
		out := [][2]string{{"s=另一部剧#S2|v=-|p=-|n=1|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "普通男女"); n != 0 || out[0][0] != "s=另一部剧#S2|v=-|p=-|n=1|d=-" {
			t.Fatalf("跨片反例应保留，实际 n=%d out=%v", n, out)
		}
	}
	// 撞键守卫（C 情形同样回退）：去掉后与同源行内其它行撞键 ⇒ 不改
	{
		out := [][2]string{
			{"s=创造|v=-|p=-|n=1|d=-", "self"},
			{"s=-|v=-|p=-|n=1|d=-", "self"},
		}
		if n := StripFilmNamePrefix(out, "创造101"); n != 0 {
			t.Fatalf("撞键应守卫回退，实际改动 %d", n)
		}
	}
}
