package danmu

import (
	"testing"
)

// M3/M4 取证批测试（2026-11-17）—— 代表片逐集对比发现的两类解析错漏修复。
// 取证与普遍性实测：方案-代表片取证归因与优化方案-2026-11-17.md（§3 措施 / §7 普遍性）。

// M4：整串「第N」（缺单位）/「N[集话回期]」（缺第）⇒ Num（reDiNAnchor，锚定 ^$）。
func TestParseEpisodeKey_M4锚定缺第缺单位(t *testing.T) {
	cases := []struct {
		title string
		want  EpisodeKey
	}{
		// M4a：整串「第N」（缺单位）——取证片9（短剧 宗门里除了我都是卧底）
		{"第99", EpisodeKey{Num: 99}},
		{"第1", EpisodeKey{Num: 1}},
		{"第2024", EpisodeKey{Num: 2024}},
		// M4b：整串「N[集]」（缺第）——取证 src_lz 整源 '01集'..'NN集'
		{"103集", EpisodeKey{Num: 103}},
		{"01集", EpisodeKey{Num: 1}},
		{"99期", EpisodeKey{Num: 99}},
		// 反例（宁缺勿错）：复合形态不在内
		{"第99：狼来了", EpisodeKey{}},  // 有副标题 ⇒ 不吃（先试算后拍板的口径边界）；s= 也被裸第N剥离
		{"第99 上", EpisodeKey{Part: "上"}}, // 复合形态：Num=0（M4 只认整串）；Part 走 parsePart（尾部上仍合法）
		{"第7季", EpisodeKey{Series: "#S7"}}, // 季/部不给 Num（10-16 修正不变）；季标记入 s（方案A #S#B）
		{"第20240514", EpisodeKey{Date: "20240514"}}, // 8 位数字超出 \d{1,4} ⇒ Num=0；但 reDate8 抓到合法日期
		{"第99集", EpisodeKey{Num: 99}},             // 标准形态走 reDiN（不受 M4 影响）
	}
	for _, c := range cases {
		if got := ParseEpisodeKey(c.title); got != c.want {
			t.Errorf("ParseEpisodeKey(%q) = %+v, want %+v", c.title, got, c.want)
		}
	}
}

// M4 配套（seriesOf 侧）：裸「第N」从栏目名候选剥掉 ——「第99」与「第99集」的跨源对齐成立。
func TestSeriesOf_M4裸第N剥离(t *testing.T) {
	if got := ParseEpisodeKey("第99"); got.Series != "" || got.Num != 99 {
		t.Fatalf("「第99」应得 s=\"\" n=99（栏目候选剥掉裸第N）, got %+v", got)
	}
	// 跨源对齐成立：同一集的两种写法（缺单位 vs 缺第）判同集
	if !SameEpisode(ParseEpisodeKey("第99"), ParseEpisodeKey("第99集")) {
		t.Fatal("「第99」与「第99集」应判同集（M4 修复的取证场景）")
	}
	// 带单位的正常形态不受影响
	if got := ParseEpisodeKey("心动的周末第99期"); got.Series != "心动的周末" || got.Num != 99 {
		t.Fatalf("带单位形态不应受影响: %+v", got)
	}
}

// M3：parsePart 右邻防粘连 —— 主次字右邻是"其它汉字"（非 单位/主次/第/变体首字）⇒ 不作主次。
func TestParseEpisodeKey_M3右邻防粘连(t *testing.T) {
	cases := []struct {
		title string
		want  EpisodeKey
	}{
		// 取证片4（密室大逃脱第八季）：「全员空中飞人」的中被右邻"飞"粘连 ⇒ 不作主次
		{"第20260902期全员空中飞人", EpisodeKey{Date: "20260902", Series: "全员空中飞人"}},
		{"空中飞人第2期上", EpisodeKey{Num: 2, Part: "上", Series: "空中飞人"}}, // 复合词中段粘连 ⇒ 跳过后尾部的"上"仍命中
		// 右邻=变体词首字 ⇒ 主次成立（豁免④）
		{"第5期中纯享", EpisodeKey{Num: 5, Part: "中", Variant: "纯享"}},
		{"第20260803期上纯享", EpisodeKey{Date: "20260803", Part: "上", Variant: "纯享"}},
		{"第5期中加更", EpisodeKey{Num: 5, Part: "中", Variant: "加更"}},
		// 地名/普通复合词 ⇒ 不作主次
		{"上海之夜第3期", EpisodeKey{Num: 3, Series: "上海之夜"}},
		{"中秋特辑第1期", EpisodeKey{Num: 1, Series: "中秋", Variant: "特辑"}}, // 变体词从 s 剥离是既有行为（v=特辑 承接）
		{"中美第2期", EpisodeKey{Num: 2, Series: "中美"}},
		{"上线仪式第1期", EpisodeKey{Num: 1, Series: "上线仪式"}},
		// 既有口径保留（误伤反例）
		{"(副本存档中第1期)", EpisodeKey{Num: 1, Part: "中", Series: "副本存档"}}, // 中后为第（豁免③）
		{"20260831中上", EpisodeKey{Date: "20260831", Part: "中上"}},            // 右邻主次字（豁免②）
		{"第3期中（上）", EpisodeKey{Num: 3, Part: "中上"}},                        // 右邻括号（豁免外=合法）
		{"20260818中（下）", EpisodeKey{Date: "20260818", Part: "中下"}},
		{"心动的旅行上：草原团建", EpisodeKey{Part: "上", Series: "心动的旅行"}}, // 右邻冒号
		{"20260921上", EpisodeKey{Date: "20260921", Part: "上"}},           // 串尾
		{"第20260731期先导片上", EpisodeKey{Date: "20260731", Part: "上", Variant: "先导"}},
	}
	for _, c := range cases {
		if got := ParseEpisodeKey(c.title); got != c.want {
			t.Errorf("ParseEpisodeKey(%q) = %+v, want %+v", c.title, got, c.want)
		}
	}
}
