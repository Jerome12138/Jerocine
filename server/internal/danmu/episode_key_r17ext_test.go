package danmu

import "testing"

// ===== R17 扩展（2026-11-17）：语言/画质类带「版」的写法当**噪声删** =====
//
// 依据（真库实测）：英文版52/高清版34/中字版14/超清版14/英语版8/粤语版6/原声版2/日语版·法语版·川话版·美版·字幕版·清晰版各1~2（≈130 行）。
// 它们与「中字/国语/高清」**同义** ⇒ 必须与不带"版"的写法**归一到同一身份**（否则 `中字` 被删、`中字版` 落 s ⇒ 新伪分歧）。
// ⚠️ 只收语言/画质类；内容版本类（加长版/尊享版/大神版…）**不在此表**（另行按变体处理）。
func TestParseEpisodeKey_QualityTailWithBan(t *testing.T) {
	cases := []struct {
		title string
		wantS string
	}{
		{"高清版", ""}, // 被 R17 删 ⇒ 不落 s
		{"超清版", ""},
		{"中字版", ""},
		{"粤语版", ""},
		{"英语版", ""},
		{"原声版", ""},
		{"普通话版", ""},
		{"HD中字版", ""}, // ASCII 前缀 + 标签 ⇒ 仍不落 s
		{"HD（普通话版）", ""},
		// 既有口径不受影响
		{"国语版", ""},
		{"HD中字", ""},
		// ⚠️ 反例：**内容版本类**不属本表 ⇒ 保持落 s（B 批另行处理）；`整合版` 归 P3-pack
		{"电影版", "电影版"},
		{"整合版", "整合版"},
		// B 批（2026-11-17）后：「中国版」被收编为**变体** ⇒ s 只剩主体（不再是 R17 的范围）
		{"周六夜现场中国版", "周六夜现场"},
	}
	for _, c := range cases {
		if got := ParseEpisodeKey(c.title).Series; got != c.wantS {
			t.Errorf("ParseEpisodeKey(%q).Series = %q, want %q", c.title, got, c.wantS)
		}
	}
}

// ===== C（2026-11-17）：「整合版」并入 R19 打包词（P3-pack）=====
//
// 依据：实测 172 行**全部来自"该源行只有 1 集"**、全属连续剧 ⇒ 与 全集/合集 同族。
func TestRulePackWord_整合版(t *testing.T) {
	// 单集源行 ⇒ n=1（single），与其它源的「第01集」对齐
	got := EpidsForSourceRowWithPool([]string{"整合版"}, []string{"整合版"})
	if len(got) != 1 || got[0] != [2]string{"s=-|v=-|p=-|n=1|d=-", EpKeySourceSingle} {
		t.Errorf("单集源行的「整合版」应给 n=1/single，得 %v", got)
	}
	// 多集源行 ⇒ 不命中 ⇒ 保留系列身份（行为不变）
	titles := []string{"整合版", "第01集", "第02集"}
	got2 := EpidsForSourceRowWithPool(titles, titles)
	if len(got2) != 3 || got2[0] != [2]string{"s=整合版|v=-|p=-|n=-|d=-", EpKeySourceNone} {
		t.Errorf("多集源行的「整合版」应保留 s，得 %v", got2)
	}
	// RuleSelectsRow 精确性（scope 用）
	if !RuleSelectsRow(RulePrePack, []string{"整合版"}) {
		t.Error("单集源行的整合版应命中 P3-pack")
	}
	if RuleSelectsRow(RulePrePack, titles) {
		t.Error("多集源行不该命中 P3-pack")
	}
}
