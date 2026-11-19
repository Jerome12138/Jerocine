package danmu

import "testing"

// 本批（2026-11-19）新增：M1 双锚 / L1-b plus / L1-a 篇 / 区间→n / L2 同日尾号。
// 每条都带反例，防"识别过宽"。

func TestM1_EAnchorDate_DoubleAnchor(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  EpisodeKey
	}{
		// 正例：整串 E{N}.{6/8 位日期}
		{"E414.251117 → n=414,d=20251117", "E414.251117", EpisodeKey{Num: 414, Date: "20251117"}},
		{"E02.220724 → n=2,d=20220724", "E02.220724", EpisodeKey{Num: 2, Date: "20220724"}},
		// 空格分隔（实测 11 行，ICU 下与 '.' 同类）
		{"E263 241002 → n=263,d=20241002", "E263 241002", EpisodeKey{Num: 263, Date: "20241002"}},
		// 8 位合法日期
		{"E12.20250731 → n=12,d=20250731", "E12.20250731", EpisodeKey{Num: 12, Date: "20250731"}},
		// 反例：8 位"日期+同日序号"（E04.22061601）不是 YYYYMMDD ⇒ 不设 Date
		{"E04.22061601 不认日期", "E04.22061601", EpisodeKey{ENum: 4}},
		// 反例：名前有其他字符 ⇒ 不整串锚定 ⇒ 不设 Date
		{"剧名 E414.251117 不锚定（但 E 圆号仍成立）", "某剧 E414.251117", EpisodeKey{ENum: 414}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseEpisodeKey(c.title)
			if got.Num != c.want.Num || got.Date != c.want.Date || got.ENum != c.want.ENum {
				t.Fatalf("ParseEpisodeKey(%q) = {Num:%d Date:%q ENum:%d}, want {Num:%d Date:%q ENum:%d}",
					c.title, got.Num, got.Date, got.ENum, c.want.Num, c.want.Date, c.want.ENum)
			}
		})
	}
}

func TestL1b_PlusVariant(t *testing.T) {
	// 正例：同含日期（6 位）⇒ 认 v=Plus
	if k := ParseEpisodeKey("大神260724Plus版"); k.Variant != "Plus" {
		t.Fatalf("6 位日期 + Plus 应认 v=Plus，实际 %q（全键 %+v）", k.Variant, k)
	}
	// 正例：同含"期"
	if k := ParseEpisodeKey("中国潮音2021.12.17-第6期plus"); k.Variant != "Plus" {
		t.Fatalf("期 + plus 应认 v=Plus，实际 %q", k.Variant)
	}
	// 反例：无日期、无期/集 ⇒ 品牌名不误判
	if k := ParseEpisodeKey("Plus版"); k.Variant != "" {
		t.Fatalf("无日期/期集语境不应认 v=Plus，实际 %q", k.Variant)
	}
}

func TestL1a_PianAsPart(t *testing.T) {
	// 正例：上篇/下篇 → part
	if k := ParseEpisodeKey("20240101上篇"); k.Part != "上" {
		t.Fatalf("上篇 应认 part=上，实际 %q（全键 %+v）", k.Part, k)
	}
	if k := ParseEpisodeKey("20240101下篇"); k.Part != "下" {
		t.Fatalf("下篇 应认 part=下，实际 %q", k.Part)
	}
	// 反例：粘字守卫仍生效（"上"后接非白名单汉字 ⇒ 不认）
	if k := ParseEpisodeKey("20240101上酸菜"); k.Part != "" {
		t.Fatalf("粘字应被守卫挡住，实际 part=%q", k.Part)
	}
}

func TestRange_FirstNum(t *testing.T) {
	// 正例：严格"第X-Y集"⇒ n=first（非空 epid）
	got := EpidsForSourceRow([]string{"第61-80集完结"})
	if len(got) != 1 || got[0][0] == "" {
		t.Fatalf("区间行应归一为 n=first，实际 %v", got)
	}
	k, _ := ParseEpidString(got[0][0])
	if k.Num != 61 {
		t.Fatalf("区间首号应为 61，实际 %d（epid=%q）", k.Num, got[0][0])
	}
	// 正例：第1~20集
	got2 := EpidsForSourceRow([]string{"第1~20集"})
	if len(got2) != 1 || got2[0][0] == "" {
		t.Fatalf("第1~20集应归一，实际 %v", got2)
	}
	// 反例：合集 ⇒ 仍 skip（空 epid）
	got3 := EpidsForSourceRow([]string{"第01-84合集"})
	if len(got3) != 1 || got3[0][0] != "" {
		t.Fatalf("合集不应归一（保持 skip），实际 %v", got3)
	}
	// 反例：start>end ⇒ 不归一
	got4 := EpidsForSourceRow([]string{"第80-61集"})
	if len(got4) != 1 || got4[0][0] != "" {
		t.Fatalf("start>end 不应归一，实际 %v", got4)
	}
}

func TestL2_SameDayTailDisambiguation(t *testing.T) {
	// 正例：同源同日两条、去尾号后同名 ⇒ 尾号写 p，两条 epid 必须不同
	got := EpidsForSourceRow([]string{"20250813旅行日记1", "20250813旅行日记2"})
	if len(got) != 2 {
		t.Fatalf("应有 2 行，实际 %d", len(got))
	}
	if got[0][0] == "" || got[1][0] == "" || got[0][0] == got[1][0] {
		t.Fatalf("同日尾号应消歧为不同 epid，实际 %q / %q", got[0][0], got[1][0])
	}
	// 反例：单条尾号不动（防 海贼王1000）
	one := EpidsForSourceRow([]string{"海贼王1000"})
	if len(one) != 1 {
		t.Fatalf("单条应 1 行，实际 %d", len(one))
	}
	if k, ok := ParseEpidString(one[0][0]); ok && k.Part != "" {
		t.Fatalf("单条尾号不应被当 part，实际 part=%q（epid=%q）", k.Part, one[0][0])
	}
	// 反例：前缀不同（非同一组）不应消歧
	two := EpidsForSourceRow([]string{"20250813旅行日记1", "20250814另一集1"})
	if len(two) == 2 && two[0][0] == two[1][0] {
		t.Fatalf("不同日期不应被同日消歧逻辑牵连")
	}
}
