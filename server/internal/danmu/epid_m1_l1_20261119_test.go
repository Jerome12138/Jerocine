package danmu

import "testing"

// 本批（2026-11-19）新增：M1 双锚 / L1-b plus / L1-a 篇 / 区间→n / L2 同日尾号。
// 每条都带反例，防"识别过宽"。

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
	// A2（2026-11-19）：合集按噪声归一 —— `第01-84合集` → p=1-84, n=1（非空）
	got3 := EpidsForSourceRow([]string{"第01-84合集"})
	if len(got3) != 1 || got3[0][0] == "" {
		t.Fatalf("合集应归一为非空 epid，实际 %v", got3)
	}
	if k3, _ := ParseEpidString(got3[0][0]); k3.Num != 1 || k3.Variant != "1-84" {
		t.Fatalf("合集应 n=1 且 v=1-84，实际 %+v", k3)
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

// 裸 E/EP（无日期）保持"不产身份"的既有口径：EpidOf 无 e= 段 ⇒ 空 epid ⇒ skip。
// 本用例把该口径钉住，防止后续误改（2,897 行属此列）。
func TestM1_BareENumKeepsNoIdentity(t *testing.T) {
	for _, title := range []string{"E01", "EP01", "EP09", "E414.251117", "EP07.220705", "E01.220705"} {
		got := EpidsForSourceRow([]string{title})
		if got[0][0] != "" || got[0][1] != EpKeySourceSkip {
			t.Fatalf("%q 应保持不产身份（空 epid + skip），实际 epid=%q from=%s", title, got[0][0], got[0][1])
		}
	}
}
// EP + 日期 同规（实测全库仅 2 行）：EP 前缀也走 M1 双锚。

// S-去前缀（2026-11-19 用户口径）：s 以片名为前缀 ⇒ 去掉 s（含季号），带同源行内去重守卫。
func TestStripFilmNamePrefix(t *testing.T) {
	// 正例1：s 与片名一致 ⇒ 去掉
	{
		out := EpidsForSourceRow([]string{"第1期"})
		out[0][0] = "s=普通男女|v=-|p=-|n=1|d=-"
		if n := StripFilmNamePrefix(out, "普通男女"); n != 1 {
			t.Fatalf("s=片名 应去掉，实际改动 %d（%q）", n, out[0][0])
		}
		if out[0][0] != "s=-|v=-|p=-|n=1|d=-" {
			t.Fatalf("去掉后应为 s=-，实际 %q", out[0][0])
		}
	}
	// 正例2：s 带季号且以片名为前缀 ⇒ 整段去掉（含季号）
	{
		out := [][2]string{{"s=熊出没之探险日记#S3|v=-|p=-|n=5|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "熊出没之探险日记"); n != 1 {
			t.Fatalf("带季号前缀命中应去掉，实际 %d", n)
		}
		if out[0][0] != "s=-|v=-|p=-|n=5|d=-" {
			t.Fatalf("去掉后应为 s=-，实际 %q", out[0][0])
		}
	}
	// 正例3：空格/间隔号差异不影响命中
	{
		out := [][2]string{{"s=普通 男女|v=-|p=-|n=1|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "普通·男女"); n != 1 {
			t.Fatalf("规范化后应命中，实际 %d", n)
		}
	}
	// 反例1：s 不以片名为前缀（不同片）⇒ 保留（含季号）
	{
		out := [][2]string{{"s=另一部剧#S2|v=-|p=-|n=1|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "普通男女"); n != 0 {
			t.Fatalf("前缀不命中应保留，实际 %d", n)
		}
	}
	// 反例2：守卫——去掉后与同源行内其它行撞键 ⇒ 不改
	{
		out := [][2]string{
			{"s=普通男女|v=-|p=-|n=1|d=-", "self"},
			{"s=-|v=-|p=-|n=1|d=-", "self"},
		}
		if n := StripFilmNamePrefix(out, "普通男女"); n != 0 {
			t.Fatalf("撞键时应守卫回退，实际改动 %d", n)
		}
	}
	// 反例3：s 为空 ⇒ 不处理
	{
		out := [][2]string{{"s=-|v=-|p=-|n=1|d=-", "self"}}
		if n := StripFilmNamePrefix(out, "普通男女"); n != 0 {
			t.Fatalf("无 s 不应改动，实际 %d", n)
		}
	}
}