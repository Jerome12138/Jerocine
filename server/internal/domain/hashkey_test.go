package domain

import "testing"

// NormalizeName: 去空白 / 去尾部别名 / 剔噪声词 / 去首尾标点 / 季后缀截断为"季"。
// 按片名采集用它作 wd, 与匹配口径一致。
func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"凡人修仙传":        "凡人修仙传",
		"凡人修仙传 ":       "凡人修仙传", // 去空格
		" 庆余年 ":        "庆余年",   // 去首尾空格
		"斗罗大陆第二季绝世唐门": "斗罗大陆第二季", // 季后缀截断
		"名侦探柯南～剧场版～":  "名侦探柯南",     // 去尾部别名
		"【某某】":         "某某",     // 去首尾全角括号
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q)=%q want %q", in, got, want)
		}
	}
}

// 噪声词剔除: 采集源把发行标记直接拼进片名时, 仍应与"干净片名"归一到同一个 key,
// 否则同一部片在各源之间匹配不上, 聚合不出多线路。
func TestNormalizeName_NoiseWords(t *testing.T) {
	cases := map[string]string{
		// 发布组前缀 + 分辨率 + 集数
		"[某某字幕组] 斗罗大陆 1080P 第01集": "斗罗大陆",
		"【某某字幕组】斗罗大陆 1080P":      "斗罗大陆",
		// 分辨率与画质标记连写(右边界不锚定, 故 "4KHDR" 也要能剔干净)
		"庆余年 第二季 4K HDR":         "庆余年第二季",
		"庆余年 第二季 1080Px265":      "庆余年第二季",
		// 集数标记
		"火影忍者 第720话 1080P":       "火影忍者",
		// 封装 + 体积
		"某某电影 MKV 2.5GB":          "某某电影",
		// 字幕 / 语言包装
		"某某电影 国语中字":              "某某电影",
		"某某电影 简繁字幕 BD":           "某某电影",
		// 更新进度
		"某某动漫 更新至120集":           "某某动漫",
		"某某剧 全集":                 "某某剧",
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q)=%q want %q", in, got, want)
		}
	}
}

// 季语义必须保留: "第N季"不属于噪声词 —— 若把它当噪声删掉,
// "某某第一季" 与 "某某第二季" 会被合并成同一部片。
func TestNormalizeName_SeasonKept(t *testing.T) {
	noise := []string{"1080P", "中字", "BD", "全集"}
	for _, n := range noise {
		if NormalizeName("某某第一季 "+n) != NormalizeName("某某第一季") {
			t.Errorf("噪声词 %q 不应影响分季: got %q", n, NormalizeName("某某第一季 "+n))
		}
	}
	if NormalizeName("某某第一季") == NormalizeName("某某第二季") {
		t.Fatal("不同季不应归一为同一 key")
	}
	if NormalizeName("某某第一季") == NormalizeName("某某") {
		t.Fatal("带季名不应与被剔掉季名等值(季是区分位)")
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
	// 噪声词差异同样应落到同一 key(跨源聚合的核心诉求)
	if GenerateHashKey("某某电影 1080P 中字") != GenerateHashKey("某某电影") {
		t.Fatal("噪声词差异应归一化为同一 key")
	}
}

// 非片名的数字键(如豆瓣 ID)必须原样保留, 不能被子串规则误剔。
func TestNormalizeName_NumericKeyPreserved(t *testing.T) {
	for _, id := range []string{"1", "42", "12345", "0"} {
		if got := NormalizeName(id); got != id {
			t.Errorf("数字键 %q 不应被改动, got %q", id, got)
		}
	}
}
