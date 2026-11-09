package bloburl

import "testing"

func TestResolveToKey_PathBase(t *testing.T) {
	SetBase("/filestore/")
	defer SetBase("")

	cases := []struct{ in, want string }{
		{"backdrop/1.webp", "/filestore/backdrop/1.webp"},
		{"gallery/1699999999.jpg", "/filestore/gallery/1699999999.jpg"},
		{"", ""},
		{"-", "-"}, // backdrop miss 哨兵: 无斜杠, 原样
		{"http://img.site/a.jpg", "http://img.site/a.jpg"},
		{"https://cdn.x/a.webp", "https://cdn.x/a.webp"},
		{"/api/upload/legacy.jpg", "/api/upload/legacy.jpg"}, // 已根相对, 原样放行
	}
	for _, c := range cases {
		if got := Resolve(c.in); got != c.want {
			t.Errorf("Resolve(%q)=%q, want %q", c.in, got, c.want)
		}
	}

	keys := []struct{ in, want string }{
		{"/filestore/backdrop/1.webp", "backdrop/1.webp"},
		{"/api/upload/backdrop/1.webp", "backdrop/1.webp"}, // 旧前缀也剥
		{"backdrop/1.webp", "backdrop/1.webp"},             // 已是裸 key
		{"http://img.site/a.jpg", "http://img.site/a.jpg"}, // 外链原样
		{"", ""},
		{"-", "-"},
	}
	for _, c := range keys {
		if got := ToKey(c.in); got != c.want {
			t.Errorf("ToKey(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolve_DefaultBase(t *testing.T) {
	SetBase("")
	defer SetBase("")
	// 基址未配置时不做任何拼接
	if got := Resolve("backdrop/1.webp"); got != "backdrop/1.webp" {
		t.Errorf("base 空时 Resolve 应原样, got %q", got)
	}
}

func TestResolveToKey_AbsoluteBase(t *testing.T) {
	SetBase("https://img.example.com/")
	defer SetBase("")

	if got := Resolve("backdrop/1.webp"); got != "https://img.example.com/backdrop/1.webp" {
		t.Errorf("绝对域基址 Resolve=%q", got)
	}
	if got := ToKey("https://img.example.com/backdrop/1.webp"); got != "backdrop/1.webp" {
		t.Errorf("绝对域基址 ToKey=%q", got)
	}
}
