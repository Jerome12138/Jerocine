package danmu

import "testing"

// TestParseEpisodeKey_SixDigitDate（R11，2026-11-14 服务端口径）：
// 6 位 YYMMDD **补齐为 8 位**（便于与其它源的 8 位日期直接比对），且边界/合法性必须把关：
//   - 前后不接数字（7/9 位数字串不得截出半截当日期 ✗）
//   - 月 01–12、日按当月天数（含闰年）
//   - 世纪：yy >= 70 ⇒ 19xx，否则 20xx
//   - **8 位优先**（`20240103` 不能被当成本身的 6 位片段）
func TestParseEpisodeKey_SixDigitDate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"240103", "20240103"},
		{"240110", "20240110"},
		{"250409", "20250409"},
		{"990101", "19990101"},   // yy >= 70 ⇒ 19xx
		{"240132", ""},           // 1 月 32 日 ⇒ 非法
		{"241303", ""},           // 月 13 ⇒ 非法
		{"230229", ""},           // 2023 非闰年 ⇒ 2/29 非法
		{"240229", "20240229"},   // 2024 闰年 ⇒ 合法
		{"20240103", "20240103"}, // 8 位优先
		{"2401031", ""},          // 7 位 ⇒ 边界不符
		{"123456", ""},           // 月 34 非法
		{"", ""},
	}
	for _, c := range cases {
		if got := ParseEpisodeKey(c.in).Date; got != c.want {
			t.Errorf("ParseEpisodeKey(%q).Date = %q, want %q", c.in, got, c.want)
		}
	}
	// 6 位日期要能**触发**并进身份（d 是原始事实 ⇒ 还能参与桥接）
	k := ParseEpisodeKey("240103")
	if !EpisodeTriggered(k) {
		t.Error("6 位日期应触发（d 段有值）")
	}
	if got := EpidOf(k); got != "s=-|v=-|p=-|n=-|d=20240103" {
		t.Errorf("EpidOf(240103) = %q, want s=-|v=-|p=-|n=-|d=20240103", got)
	}
}

// TestHasASCIIDigit（R12 的排除判据）：判据是"集名不含 ASCII 数字" —— 一次排除所有带序号/区间/日期的写法，
// 从而既不会与真序号撞（`第1集`），也不会把待定形态（`OVA01`/`EP09`/`第7.5集`）夹带进来。
func TestHasASCIIDigit(t *testing.T) {
	noDigit := []string{"正片", "HD", "HD国语", "HD中字", "None", "OVA篇", "正片·", "HD??", "某某电影", "国语"}
	for _, s := range noDigit {
		if hasASCIIDigit(s) {
			t.Errorf("hasASCIIDigit(%q) = true, want false", s)
		}
	}
	hasDigit := []string{"第1集", "第1-20集", "OVA01", "EP09", "第7.5集", "240103", "1024", "第一季2"}
	for _, s := range hasDigit {
		if !hasASCIIDigit(s) {
			t.Errorf("hasASCIIDigit(%q) = false, want true", s)
		}
	}
}

// TestEpidsForSourceRow_SingleEpisodeN1（R12 放宽版，2026-11-14 用户拍板）：
// **整行只有 1 集 ∧ 集名不含 ASCII 数字** ⇒ `n=1`（来源 `form`）。
// 线上依据：含 `HD国语` 的源行 4,101/4,557（90%）只有 1 集 ⇒ `HD国语` 是那一集的名字（HD+国语 两个标签）；
// 另 428 条是 `HD国语 | 第01集 | …` 的"整片位 + 逐集"写法 ⇒ 必须由"只有 1 集"挡掉 ✓。
func TestEpidsForSourceRow_SingleEpisodeN1(t *testing.T) {
	// ① 单集 + 无数字 + 常规派生推不出 ⇒ n=1（含实测中最多与易漏的写法）
	for _, name := range []string{"正片", "HD", "None"} {
		got := EpidsForSourceRow([]string{name})
		if len(got) != 1 || got[0][0] != "s=-|v=-|p=-|n=1|d=-" || got[0][1] != EpKeySourceForm {
			t.Errorf("%q 单集应补 n=1/form，得 %v", name, got)
		}
	}
	// ①b ⚠️ 两类"看着像漏网、其实已有身份"的写法（本轮实测打脸假设）——**不该**再补 n=1：
	//   - `HD中字` ⇒ 既有怪癖：`中字` 被当作「栏目 + 主次」⇒ 得 `s=中字|p=中|n=-`（跨源一致 ⇒ 无害；
	//     要治得走 R 系列的"标点/噪声词"那条，不在 R12 范围）
	//   - `某某电影` ⇒ 整名进 `s` 段 ⇒ 已有身份（对单片而言这个身份其实是对的 ✓）
	for _, name := range []string{"HD中字", "某某电影"} {
		got := EpidsForSourceRow([]string{name})
		if got[0][1] == EpKeySourceForm || got[0][0] == "" {
			t.Errorf("%q 应已有身份（不该走 R12、也不该为空），得 %v", name, got)
		}
	}
	// ② 单集但含数字（区间 / 形态+序号 / 小数 / 裸数字）⇒ 不补（留给 R13 或待定，宁缺勿错）
	for _, name := range []string{"第1-20集", "OVA01", "EP09", "第7.5集", "1"} {
		if got := EpidsForSourceRow([]string{name}); got[0][1] == EpKeySourceForm {
			t.Errorf("%q 含数字不该走 R12，得 %v", name, got)
		}
	}
	// ③ 整行多集 ⇒ 一条都不补
	got := EpidsForSourceRow([]string{"正片", "HD"})
	for i, g := range got {
		if g[1] == EpKeySourceForm {
			t.Errorf("多集行第 %d 条不该走 R12，得 %v", i, g)
		}
	}
	// ④ 混有真序号 ⇒ 无数字那条也不补（避免与「第1集」撞），真序号照常派生
	got = EpidsForSourceRow([]string{"正片", "第1集"})
	if got[0][1] == EpKeySourceForm {
		t.Errorf("混序号时「正片」不该走 R12（会撞第1集），得 %v", got[0])
	}
	if got[1][0] != "s=-|v=-|p=-|n=1|d=-" {
		t.Errorf("「第1集」应照常派生，得 %v", got[1])
	}
	// ⑤ 空集名不补
	if got := EpidsForSourceRow([]string{""}); got[0][1] == EpKeySourceForm {
		t.Errorf("空集名不该补，得 %v", got[0])
	}
	// ⑥ 变体形态（「特辑」）本就触发，不走 R12
	if got := EpidsForSourceRow([]string{"特辑"}); got[0][1] == EpKeySourceForm {
		t.Errorf("「特辑」不该走 R12，得 %v", got[0])
	}
}
