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