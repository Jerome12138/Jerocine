package danmu

import "testing"

// 本组用例把 2026-11-17 的实测事实钉成行为（证据：docs/epid-集身份/索引-观察项总账-2026-11-17.md §4.1）。
// 口径**暂未接线**，用例只保证判据本身不跑偏。

func TestDateAligned_SubsetBothWays(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"完全相同", []string{"20240101", "20240108"}, []string{"20240101", "20240108"}, true},
		{"一侧尾部多出（源不全）", []string{"20240101", "20240108"}, []string{"20240101", "20240108", "20240115"}, true},
		{"一侧头部多出（位序错位）", []string{"20240108", "20240115"}, []string{"20240101", "20240108", "20240115"}, true},
		{"一侧中间缺失", []string{"20240101", "20240115"}, []string{"20240101", "20240108", "20240115"}, true},
		{"交错缺集（替换语义，必须判不齐）", []string{"20240101", "20240115"}, []string{"20240108", "20240115"}, false},
		{"日期完全不相干", []string{"20240101"}, []string{"20240201"}, false},
		{"一侧空", []string{}, []string{"20240101"}, false},
		{"一侧含无日期项", []string{"20240101", "-"}, []string{"20240101"}, false},
		{"一侧含空串", []string{"20240101", ""}, []string{"20240101"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DateAligned(tc.a, tc.b); got != tc.want {
				t.Fatalf("DateAligned(%v,%v)=%v want %v", tc.a, tc.b, got, tc.want)
			}
			if got := DateAligned(tc.b, tc.a); got != tc.want {
				t.Fatalf("对称性失败 DateAligned(%v,%v)=%v want %v", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

// 守卫三条：每源同日 ≤1 行 / ≥2 个有日期的源 / 空源不计。
func TestDateAlignUsable_Guards(t *testing.T) {
	cases := []struct {
		name    string
		sources [][]string
		want    bool
	}{
		{
			name:    "两源且各自唯一 => 可用",
			sources: [][]string{{"20240101", "20240108"}, {"20240101", "20240108"}},
			want:    true,
		},
		{
			name:    "某源同日 ≥2 行（实测歧义组 9.1%）=> 整组放弃",
			sources: [][]string{{"20240101", "20240101"}, {"20240101"}},
			want:    false,
		},
		{
			name:    "只有 1 个源（实测 42.2% 单源组）=> 不判",
			sources: [][]string{{"20240101"}},
			want:    false,
		},
		{
			name:    "一个源 + 一个无日期源 => 有效源只有 1 => 不判",
			sources: [][]string{{"20240101"}, {"-", "-"}},
			want:    false,
		},
		{
			name:    "两源都可判（第三源无日期不否决）",
			sources: [][]string{{"20240101"}, {"20240101"}, {"", ""}},
			want:    true,
		},
		{
			name:    "空组",
			sources: [][]string{},
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DateAlignUsable(tc.sources); got != tc.want {
				t.Fatalf("DateAlignUsable(%v)=%v want %v", tc.sources, got, tc.want)
			}
		})
	}
}

// 多源要**两两**相容，不能只跟最长源比。
func TestDateAlignVerdict_PairwiseRequired(t *testing.T) {
	cases := []struct {
		name    string
		sources [][]string
		want    bool
	}{
		{
			name:    "三源同一排期（一源缺两集）",
			sources: [][]string{{"20240101", "20240108"}, {"20240101", "20240108", "20240115"}, {"20240101", "20240108"}},
			want:    true,
		},
		{
			name:    "三源里两源日期不相交 => 不齐",
			sources: [][]string{{"20240101"}, {"20240101", "20240108"}, {"20240201"}},
			want:    false,
		},
		{
			name:    "守卫不通过（同源同日两行）=> 即使日期相容也不判齐",
			sources: [][]string{{"20240101", "20240101"}, {"20240101"}},
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DateAlignVerdict(tc.sources); got != tc.want {
				t.Fatalf("DateAlignVerdict(%v)=%v want %v", tc.sources, got, tc.want)
			}
		})
	}
}

// ===== 2026-11-17 二次拍板：同日多行**允许**，只要 part 互不相同且与它源对齐 =====
// 守卫从「每源同日 ≤1 行」放宽为「每源 (日期,part) 复合键唯一」。

func it(date, part string) DateAlignItem { return DateAlignItem{Date: date, Part: part} }

func TestDateAlignItems_PartDistinguishedSameDate(t *testing.T) {
	// 一天两集：两源都用 上/中 区分，且 date 相同
	pair := [][]DateAlignItem{
		{it("20240101", "上"), it("20240101", "中"), it("20240108", "")},
		{it("20240101", "上"), it("20240101", "中"), it("20240108", "")},
	}
	if !DateAlignVerdictItems(pair) {
		t.Fatal("同日多行但 part 区分且两源一致 => 应判齐（放宽后）")
	}

	// 同一形态在旧口径（纯日期）下必须仍然判不齐 —— 防回退到"不看 part"
	old := [][]string{
		{"20240101", "20240101", "20240108"},
		{"20240101", "20240101", "20240108"},
	}
	if DateAlignVerdict(old) {
		t.Fatal("纯日期口径下同日两行 = 歧义，必须判不齐（旧口径保持不变）")
	}

	cases := []struct {
		name string
		src  [][]DateAlignItem
		want bool
	}{
		{
			name: "一源缺上集（子集）=> 齐",
			src: [][]DateAlignItem{
				{it("20240101", "上"), it("20240101", "中")},
				{it("20240101", "中")},
			},
			want: true,
		},
		{
			name: "part 对不上（上 vs 下）=> 不齐",
			src: [][]DateAlignItem{
				{it("20240101", "上"), it("20240101", "中")},
				{it("20240101", "下")},
			},
			want: false,
		},
		{
			name: "一源带 part、另一源同日无 part => 不齐（不做通配）",
			src: [][]DateAlignItem{
				{it("20240101", "上"), it("20240101", "中")},
				{it("20240101", "")},
			},
			want: false,
		},
		{
			name: "完全相同的 part 重复两行（同键）=> 整组放弃",
			src: [][]DateAlignItem{
				{it("20240101", "上"), it("20240101", "上")},
				{it("20240101", "上")},
			},
			want: false,
		},
		{
			name: "同日三集 上中下 两源一致 => 齐",
			src: [][]DateAlignItem{
				{it("20240101", "上"), it("20240101", "中"), it("20240101", "下")},
				{it("20240101", "上"), it("20240101", "中"), it("20240101", "下")},
			},
			want: true,
		},
		{
			name: "数字 part（当日集序 1/2）两源一致 => 齐",
			src: [][]DateAlignItem{
				{it("20240101", "1"), it("20240101", "2")},
				{it("20240101", "1"), it("20240101", "2")},
			},
			want: true,
		},
		{
			name: "第三源缺 part 而前两源带 part => 不齐",
			src: [][]DateAlignItem{
				{it("20240101", "上")},
				{it("20240101", "上")},
				{it("20240101", "")},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DateAlignVerdictItems(tc.src); got != tc.want {
				t.Fatalf("DateAlignVerdictItems(%v)=%v want %v", tc.src, got, tc.want)
			}
		})
	}
}

// 复合键必须能区分同日不同 part，且 Key() 稳定。
func TestDateAlignItemKey(t *testing.T) {
	if k := (DateAlignItem{Date: "20240101"}).Key(); k != "20240101" {
		t.Fatalf("无 part 的键 = %q", k)
	}
	if k := (DateAlignItem{Date: "20240101", Part: "中"}).Key(); k != "20240101#中" {
		t.Fatalf("带 part 的键 = %q", k)
	}
	if (DateAlignItem{Date: "20240101", Part: "上"}).Key() == (DateAlignItem{Date: "20240101", Part: "中"}).Key() {
		t.Fatal("同日不同 part 必须是不同键")
	}
}

// ===== 2026-11-17 审查加固：① 日期必须纯数字（保证复合键单射）② 方向按集合大小 =====

func TestDateAlign_NonDigitDateRejected(t *testing.T) {
	// 脏日期（含 `#`）若被当作合法，会与另一组 (date,part) 撞键 ⇒ 假"齐"（危险方向）⇒ 必须判不齐
	dirty := [][]DateAlignItem{
		{it("2024#0101", ""), it("20240102", "")},
		{it("2024", "0101"), it("20240102", "")}, // 键同为 "2024#0101"
	}
	if DateAlignVerdictItems(dirty) {
		t.Fatal("脏日期不得被当作合法日期（否则复合键碰撞会虚增对齐率）")
	}
	if DatePartAligned(dirty[0], dirty[1]) {
		t.Fatal("含非数字日期项的源必须直接判不相容")
	}
	// 纯数字日期仍须正常工作（含带 part 的同日多行）
	if !DatePartAligned(
		[]DateAlignItem{it("20240101", "")},
		[]DateAlignItem{it("20240101", ""), it("20240108", "")}) {
		t.Fatal("纯数字日期应正常判定")
	}
	if DatePartAligned(
		[]DateAlignItem{it("20240101", "上")},
		[]DateAlignItem{it("20240101", "中")}) {
		t.Fatal("同日不同 part 且无交集 => 不齐")
	}
}

func TestDatePartAligned_DirectionUsesSetSize(t *testing.T) {
	// ca 列表更长但集合更小（含重复键），cb 集合更大：真值应为"相容"（ca ⊆ cb），
	// 按原始长度选方向会误判成不齐。
	ca := []DateAlignItem{it("20240101", ""), it("20240101", ""), it("20240101", "")}
	cb := []DateAlignItem{it("20240101", ""), it("20240108", "")}
	if !DatePartAligned(ca, cb) {
		t.Fatal("方向须按去重集合大小选择：ca 集合 {20240101} ⊆ cb 集合 => 相容")
	}
	// 反向也必须对称
	if !DatePartAligned(cb, ca) {
		t.Fatal("对称性：DatePartAligned 必须双向一致")
	}
	// 真正的交错缺集仍然不齐
	cc := []DateAlignItem{it("20240101", ""), it("20240115", "")}
	cd := []DateAlignItem{it("20240108", ""), it("20240115", "")}
	if DatePartAligned(cc, cd) {
		t.Fatal("交错缺集必须判不齐（不放过替换语义）")
	}
}
