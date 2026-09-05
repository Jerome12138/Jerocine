package domain

import "testing"

// NormalizeName: 去空格 / 去尾部别名 / 季后缀截断为"季"。按片名采集用它作 wd, 与匹配口径一致。
func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"凡人修仙传":        "凡人修仙传",
		"凡人修仙传 ":       "凡人修仙传",     // 去空格
		" 庆余年 ":        "庆余年",       // 去首尾空格
		"斗罗大陆第二季绝世唐门": "斗罗大陆第二季",   // 季后缀截断
		"名侦探柯南～剧场版～":   "名侦探柯南",     // 去尾部别名
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q)=%q want %q", in, got, want)
		}
	}
}

// GenerateHashKey 复用 NormalizeName: 归一化后相同的名字 → 同一 match_key。
func TestGenerateHashKeyUsesNormalize(t *testing.T) {
	if GenerateHashKey("凡人修仙传") != GenerateHashKey("凡人修仙传 ") {
		t.Fatal("仅空格差异应归一化为同一 key")
	}
	if GenerateHashKey("斗罗大陆第二季绝世唐门") != GenerateHashKey("斗罗大陆第二季") {
		t.Fatal("季后缀差异应归一化为同一 key")
	}
}
