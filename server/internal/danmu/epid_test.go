package danmu

import "testing"

// P1-1（2026-11-10）：服务端算 epid —— 与 web `episodeKey.ts` 的**同规黄金用例**。
// 若本组失败，说明三端（Go/TS/Java）口径发生漂移，必须先对齐再落库。

func TestEpidOf_五段格式(t *testing.T) {
	cases := []struct{ title, want string }{
		{"第3期中（上）", "s=-|v=-|p=中上|n=3|d=-"},
		{"20260803上", "s=-|v=-|p=上|n=-|d=20260803"},
		{"第1期特辑2上", "s=-|v=特辑#2|p=上|n=1|d=-"},
		{"夏日清凉特辑", "s=夏日清凉|v=特辑|p=-|n=-|d=-"},
		{"20260818中（下）", "s=-|v=-|p=中下|n=-|d=20260818"},
	}
	for _, c := range cases {
		if got := EpidOf(ParseEpisodeKey(c.title)); got != c.want {
			t.Errorf("EpidOf(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

func TestEpisodeTriggered_与TS同规(t *testing.T) {
	for _, title := range []string{"20260803上", "第3期中（上）", "特辑2", "夏日清凉特辑", "加更",
		"第3集", "第01集", "第12话"} { // 2026-11-11 放宽：**序号型也算触发**（片内唯一即够）
		if !EpisodeTriggered(ParseEpisodeKey(title)) {
			t.Errorf("%q 应触发", title)
		}
	}
	// 仍不触发：E 圆号（epid 串不含 e 段）与"无任何可区分信息"的串
	for _, title := range []string{"01", "E01.220705", "12", ""} {
		if EpisodeTriggered(ParseEpisodeKey(title)) {
			t.Errorf("%q 不应触发", title)
		}
	}
}

func TestBridgeNum_候选冲突则放弃(t *testing.T) {
	cur := ParseEpisodeKey("20260803上")
	// 单一候选 ⇒ 采纳
	if got := BridgeNum(cur, []EpisodeKey{ParseEpisodeKey("20260803第1期上")}); got != 1 {
		t.Errorf("单候选应采纳, got %d", got)
	}
	// 多候选期号不一致 ⇒ 放弃（宁缺勿错）
	two := []EpisodeKey{ParseEpisodeKey("20260803第1期上"), ParseEpisodeKey("20260803第2期上")}
	if got := BridgeNum(cur, two); got != 0 {
		t.Errorf("候选冲突应放弃, got %d", got)
	}
	// 日期不等 ⇒ 候选不算
	if got := BridgeNum(cur, []EpisodeKey{ParseEpisodeKey("20260810第1期上")}); got != 0 {
		t.Errorf("日期不等不应桥接, got %d", got)
	}
}

// EpidFor = 落库/匹配口径（D10 起锚点退役，降级链 = 自带 n → 双锚反查借 d → 双锚正查借 n → none），
// 并给出**来源**供 from_src 统计。
func TestEpidFor_来源标注与降级顺序(t *testing.T) {
	others := []string{"20260803第1期上", "20260810第2期上"}

	cases := []struct {
		name     string
		title    string
		titles   []string
		wantEpid string
		wantFrom string
	}{
		// 2026-11-11 放宽：**纯序号也算触发**（片内唯一即够）⇒ 产 `n=3`，来源=自带序号
		{"纯序号 ⇒ 自带序号身份", "第3集", others, "s=-|v=-|p=-|n=3|d=-", EpKeySourceSelf},
		{"自带期号 ⇒ self", "20260803第1期上", others,
			"s=-|v=-|p=上|n=1|d=20260803", EpKeySourceSelf},
		{"缺期号可桥接 ⇒ bridge", "20260803上", others,
			"s=-|v=-|p=上|n=1|d=20260803", EpKeySourceBridge},
		{"n-only 双锚反查借 d ⇒ bridge", "第5期",
			[]string{"第5期", "第5期20260601上"},
			"s=-|v=-|p=-|n=5|d=20260601", EpKeySourceBridge},
		{"n-only 反查不成 ⇒ 回落 self（d 留空）", "第5期",
			[]string{"第5期"},
			"s=-|v=-|p=-|n=5|d=-", EpKeySourceSelf},
		{"桥不上且无双锚行 ⇒ none（n 留空，D10 撤销锚点）", "20260803上",
			[]string{"20260803上", "20260810上", "20260817上"},
			"s=-|v=-|p=上|n=-|d=20260803", EpKeySourceNone},
		{"有日期但推不出 ⇒ none（n 留空）", "20260803上",
			[]string{"20260803上"},
			"s=-|v=-|p=上|n=-|d=20260803", EpKeySourceNone},
		{"触发但无日期 ⇒ none（身份来自栏目/变体/主次）", "特辑", others,
			"s=-|v=特辑|p=-|n=-|d=-", EpKeySourceNone},
	}
	for _, c := range cases {
		epid, from := EpidFor(c.title, c.titles)
		if epid != c.wantEpid || from != c.wantFrom {
			t.Errorf("%s: EpidFor(%q) = (%q, %q), want (%q, %q)",
				c.name, c.title, epid, from, c.wantEpid, c.wantFrom)
		}
	}
}

func TestEpidsForSourceRow_等长且逐集派生(t *testing.T) {
	titles := []string{"20260803第1期上", "20260803上", "第9集"}
	out := EpidsForSourceRow(titles)
	if len(out) != len(titles) {
		t.Fatalf("应返回等长切片, got %d want %d", len(out), len(titles))
	}
	if out[0][0] == "" || out[0][1] != EpKeySourceSelf {
		t.Errorf("自带期号应产身份: %v", out[0])
	}
	if out[1][0] == "" || out[1][1] != EpKeySourceBridge {
		t.Errorf("缺期号应桥接到同一身份: %v", out[1])
	}
	// 同一集（一个自带、一个桥接）应得到**同一个 epid**
	if out[0][0] != out[1][0] {
		t.Errorf("同集应同 epid: %q vs %q", out[0][0], out[1][0])
	}
	// 2026-11-11 放宽：**纯序号也算身份**（片内唯一即够）⇒ 「第9集」应产 `n=9`，来源=自带序号
	if out[2][0] != "s=-|v=-|p=-|n=9|d=-" || out[2][1] != EpKeySourceSelf {
		t.Errorf("纯序号「第9集」应产自带序号身份, got %v", out[2])
	}
}
