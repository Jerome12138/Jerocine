package danmu

import "testing"

// ParseEpidString：epid 串（5 段格式）→ EpisodeKey（"弹幕挑集改用 epid"的前提）。
func TestParseEpidString(t *testing.T) {
	cases := []struct {
		in   string
		want EpisodeKey
		ok   bool
	}{
		{"s=-|v=-|p=-|n=1|d=-", EpisodeKey{Num: 1}, true},
		{"s=王牌对王牌|v=-|p=-|n=5|d=-", EpisodeKey{Series: "王牌对王牌", Num: 5}, true},
		{"s=-|v=特辑#2|p=中|n=12|d=20260803", EpisodeKey{Variant: "特辑", VariantNum: 2, Part: "中", Num: 12, Date: "20260803"}, true},
		{"s=特别企划|v=-|p=上|n=-|d=20260803", EpisodeKey{Series: "特别企划", Part: "上", Date: "20260803"}, true},
		{"s=-|v=-|p=-|n=-|d=20260803", EpisodeKey{Date: "20260803"}, true},
		// 负例：全空 / 空串 / 无可辨段 ⇒ 不构成身份
		{"s=-|v=-|p=-|n=-|d=-", EpisodeKey{}, false},
		{"", EpisodeKey{}, false},
		{"胡写", EpisodeKey{}, false},
		// 未知段忽略、坏数字不致命
		{"x=1|n=abc|d=20260803", EpisodeKey{Date: "20260803"}, true},
	}
	for _, tc := range cases {
		got, ok := ParseEpidString(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseEpidString(%q) = (%+v, %v), want (%+v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	// 与 EpidOf 互逆（同一身份串往返后仍等值）—— 这是"服务端能用 epid 当挑集特征"的根据
	for _, ep := range []string{
		"s=-|v=-|p=-|n=1|d=-",
		"s=王牌对王牌|v=特辑#2|p=中上|n=12|d=20260803",
		"s=大人进度|v=-|p=-|n=22|d=-",
	} {
		k, ok := ParseEpidString(ep)
		if !ok {
			t.Fatalf("%q 应可解析", ep)
		}
		if got := EpidOf(k); got != ep {
			t.Errorf("往返不等：%q ⇒ %+v ⇒ %q", ep, k, got)
		}
	}
}
