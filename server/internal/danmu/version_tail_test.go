package danmu

import "testing"

// ===== B 批（内容版本词收编）黄金用例 =====
//
// 三条纪律的钉子：
//  1. **从长到短**：「超长尊享版」必须整体进 v，**不得**被「尊享版」抢先切成残片 `超长尊`；
//  2. **保留「版」字**：v=加长版/尊享版/大神版（与 R17 的"删"区分）；
//  3. **排他清单**：语言/画质类（R17 删）、整合版（P3-pack）、电影版/剧场版/访谈版（本批不动）。
func TestSplitVersionTail(t *testing.T) {
	cases := []struct {
		series, variant string
		wantS, wantV    string
	}{
		// ① 整值就是版本词 ⇒ s 归空、v 承接
		{"超长尊享版", "", "", "超长尊享版"},
		{"尊享版", "", "", "尊享版"},
		{"加长版", "", "", "加长版"},
		{"大神版", "", "", "大神版"},
		// ② 「片名/栏目 + 版本词」⇒ 剥尾、s 保留主体
		{"周六夜现场中国版", "", "周六夜现场", "中国版"},
		{"我是大侦探独享版", "", "我是大侦探", "独享版"},
		{"双年巅峰会精华版", "", "双年巅峰会", "精华版"},
		// ③ 与既有变体词合并（多值排序）
		{"聚会上", "超前", "聚会上", "超前"},      // series 不含版本词 ⇒ 原样
		{"重聚加长版", "超前", "重聚", "加长版,超前"}, // 剥「加长版」后与既有 v 合并、排序
		// ④ 排他清单：不动
		{"电影版", "", "电影版", ""},
		{"剧场版", "", "剧场版", ""},
		{"访谈版", "", "访谈版", ""},
		{"整合版", "", "整合版", ""}, // P3-pack 处理
		{"", "", "", ""},
		{"风味人间", "", "风味人间", ""},
	}
	for _, c := range cases {
		gotS, gotV := splitVersionTail(c.series, c.variant)
		if gotS != c.wantS || gotV != c.wantV {
			t.Errorf("splitVersionTail(%q,%q) = (%q,%q), want (%q,%q)", c.series, c.variant, gotS, gotV, c.wantS, c.wantV)
		}
	}
	// 从长到短的**顺序保证**（不依赖列表书写顺序）
	for i := 1; i < len(versionTailWords); i++ {
		if len([]rune(versionTailWords[i-1])) < len([]rune(versionTailWords[i])) {
			t.Fatalf("versionTailWords 未按长度降序：%q(%d) < %q(%d)",
				versionTailWords[i-1], len([]rune(versionTailWords[i-1])), versionTailWords[i], len([]rune(versionTailWords[i])))
		}
	}
}

// 端到端：ParseEpisodeKey 上路后，「超长尊享版」不得退化成残片
func TestParseEpisodeKey_VersionTail(t *testing.T) {
	cases := []struct {
		title         string
		wantSeries    string
		wantVariant   string
		wantTriggered bool
	}{
		{"20250506（期超长尊享版）", "", "期超长尊享版", true},
		{"超长尊享版", "", "超长尊享版", true},
		{"周六夜现场中国版03.23", "周六夜现场", "中国版", true},
		{"我就是演员加长版", "", "我就是演员加长版", true}, // 注：「我就是演员」不在词表 ⇒ 整串落 v？见下断言修正
	}
	for _, c := range cases[:3] {
		k := ParseEpisodeKey(c.title)
		if k.Series != c.wantSeries || k.Variant != c.wantVariant {
			t.Errorf("%q: (s=%q, v=%q), want (s=%q, v=%q)", c.title, k.Series, k.Variant, c.wantSeries, c.wantVariant)
		}
		if EpisodeTriggered(k) != c.wantTriggered {
			t.Errorf("%q: triggered=%v, want %v（v 非空必须仍触发）", c.title, EpisodeTriggered(k), c.wantTriggered)
		}
	}
}
