package service

import "testing"

// TestScopeRequest_Resolve `mode=scope` 的范围解析（**纯函数**，不碰 DB）：
// 规则 ⇒ 从登记表推 `InFromSrc` + `FansOutMovie`；`Scope=all` 必须被拒（那种改法只能全量 rebuild）。
func TestScopeRequest_Resolve(t *testing.T) {
	type want struct {
		fromSrc []string
		expand  bool
		tighten bool
		wantErr bool
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	cases := []struct {
		name string
		req  ScopeRequest
		w    want
	}{
		{
			name: "D7(R12) ⇒ skip 档、不扩片、收紧",
			req:  ScopeRequest{Rule: "D7-single"},
			w:    want{fromSrc: []string{"skip"}, expand: false, tighten: true},
		},
		{
			name: "D8(R13) ⇒ skip 档、不扩片、收紧",
			req:  ScopeRequest{Rule: "D8-bare-num"},
			w:    want{fromSrc: []string{"skip"}, expand: false, tighten: true},
		},
		{
			name: "池一族(D5) ⇒ 池相关档、扩片、不收紧（判定需片级池）",
			req:  ScopeRequest{Rule: "D5-anchor"},
			w:    want{fromSrc: []string{"bridge", "anchor", "none"}, expand: true, tighten: false},
		},
		{
			name: "登记的 FansOutMovie 覆盖手写的 expandMovies",
			req:  ScopeRequest{Rule: "D7-single", ExpandMovies: true},
			w:    want{fromSrc: []string{"skip"}, expand: false, tighten: true},
		},
		{name: "D1 是门（Scope=all）⇒ 拒绝", req: ScopeRequest{Rule: "D1-skip"}, w: want{wantErr: true}},
		{name: "D2 是期号提取（Scope=all）⇒ 拒绝", req: ScopeRequest{Rule: "D2-self"}, w: want{wantErr: true}},
		{name: "D3 是日期提取（Scope=all）⇒ 拒绝", req: ScopeRequest{Rule: "D3-none"}, w: want{wantErr: true}},
		{name: "未知规则 ⇒ 拒绝", req: ScopeRequest{Rule: "D99-x"}, w: want{wantErr: true}},
		{name: "什么都没给 ⇒ 拒绝", req: ScopeRequest{}, w: want{wantErr: true}},
		{
			name: "显式档（底层写法）：不自动扩片、不自动收紧",
			req:  ScopeRequest{FromSrc: []string{"skip"}},
			w:    want{fromSrc: []string{"skip"}, expand: false, tighten: false},
		},
		{
			name: "显式档 + 手写扩片/收紧",
			req:  ScopeRequest{FromSrc: []string{"skip", "bridge"}, ExpandMovies: true, Tighten: true},
			w:    want{fromSrc: []string{"skip", "bridge"}, expand: true, tighten: true},
		},
		{name: "未知档 ⇒ 拒绝", req: ScopeRequest{FromSrc: []string{"bogus"}}, w: want{wantErr: true}},
	}
	for _, tc := range cases {
		fromSrc, expand, tighten, err := tc.req.Resolve()
		if tc.w.wantErr {
			if err == nil {
				t.Errorf("%s: 期望报错，却通过了", tc.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: 意外报错 %v", tc.name, err)
			continue
		}
		if !eq(fromSrc, tc.w.fromSrc) || expand != tc.w.expand || tighten != tc.w.tighten {
			t.Errorf("%s: got (fromSrc=%v expand=%v tighten=%v), want (fromSrc=%v expand=%v tighten=%v)",
				tc.name, fromSrc, expand, tighten, tc.w.fromSrc, tc.w.expand, tc.w.tighten)
		}
	}
}

// TestScopeRequest_ScopeLabel 范围标签（写进进度卡片的 `mode`）。
func TestScopeRequest_ScopeLabel(t *testing.T) {
	if got := (ScopeRequest{Rule: "D7-single"}).ScopeLabel(); got != "scope:D7-single" {
		t.Errorf("got %q", got)
	}
	if got := (ScopeRequest{FromSrc: []string{"skip", "bridge"}}).ScopeLabel(); got != "scope:skip,bridge" {
		t.Errorf("got %q", got)
	}
}
