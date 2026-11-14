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

// TestIsSingleVersionName（R12）：单片/单版本形态词表的"整串可拼出"判定。
//   - 要认：正片 / HD / hd / **HD国语**（实测最多的一类，两项连写）/ 会员版 / 720P / 带空白脏值
//   - 不认：任何含序号的（OVA01 / EP09 / 第7.5集）、变体词（特辑）、日期（20240103）、噪声（None）
func TestIsSingleVersionName(t *testing.T) {
	yes := []string{"正片", "HD", "hd", "HD国语", "国语", "会员版", "720P", "4K", "OVA", "HD\t", " 正片 ", "完整版国语"}
	for _, s := range yes {
		if !IsSingleVersionName(s) {
			t.Errorf("IsSingleVersionName(%q) = false, want true", s)
		}
	}
	no := []string{"", "   ", "OVA01", "EP09", "第1集", "第7.5集", "特辑", "20240103", "第1-20集", "None", "正片2"}
	for _, s := range no {
		if IsSingleVersionName(s) {
			t.Errorf("IsSingleVersionName(%q) = true, want false", s)
		}
	}
}

// TestEpidsForSourceRow_SingleVersionN1（R12）：单片行补 `n=1`，但**必须**守住"本行只有 1 集"。
// 线上实测：整行全是单片类的源行里 123,547 行只有 1 集（补得安全 ✓），另 485 行同形态多条（补了就撞 ✗）。
func TestEpidsForSourceRow_SingleVersionN1(t *testing.T) {
	// ① 整行只有 1 集 + 单片形态 ⇒ n=1，来源记 form
	got := EpidsForSourceRow([]string{"正片"})
	if len(got) != 1 || got[0][0] != "s=-|v=-|p=-|n=1|d=-" || got[0][1] != EpKeySourceForm {
		t.Fatalf("单片单集应补 n=1/form，得 %v", got)
	}
	// ② 同形态多条 ⇒ 一条都不补（否则撞成同一身份 ✗）
	got = EpidsForSourceRow([]string{"正片", "HD"})
	for i, g := range got {
		if g[0] != "" || g[1] != EpKeySourceSkip {
			t.Errorf("整行多条同形态时第 %d 条不该补身份，得 %v", i, g)
		}
	}
	// ③ 混有真序号 ⇒ 单片那条不补（避免与「第1集」撞），真序号照常派生
	got = EpidsForSourceRow([]string{"正片", "第1集"})
	if got[0][0] != "" || got[0][1] != EpKeySourceSkip {
		t.Errorf("混序号时「正片」不该补 n=1（会撞第1集），得 %v", got[0])
	}
	if got[1][0] != "s=-|v=-|p=-|n=1|d=-" {
		t.Errorf("「第1集」应照常派生，得 %v", got[1])
	}
	// ④ 变体形态（「特辑」）不受本规则影响
	if got := EpidsForSourceRow([]string{"特辑"}); got[0][1] == EpKeySourceForm {
		t.Errorf("「特辑」不是单片形态，不该走 R12，得 %v", got[0])
	}
	// ⑤ 空集名不补
	if got := EpidsForSourceRow([]string{""}); got[0][1] == EpKeySourceForm {
		t.Errorf("空集名不该补，得 %v", got[0])
	}
}
