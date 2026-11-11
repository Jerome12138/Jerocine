package danmu

import (
	"sort"
	"strconv"
	"strings"
)

// ===== 服务端算 epid（P1-1，2026-11-10）=====
//
// 背景：epid 原本只在客户端算（5 段串 + A2 期号）。P1-1 要把它**落库**（派生表），
// 因此服务端必须能**自己算**，且与 web 侧**严格同规**（同规基线：`web/src/utils/episodeKey.ts`）。
//
// 唯一的口径差异（有意为之，见 §4.6 端侧范围与「落库口径」）：
// web 的 `epidFor` **只做他源桥接**、不做锚点；而 `ekp` 上传的 `n` 用的是 `derivedNumFor`
// （**含锚点兜底**）。**落库取后者**（带 A2 期号、含锚点）—— 它是"匹配用身份"，
// 且与客户端 `epk` 隐含的身份一致。详见 EpidFor 的注释。

// EpisodeTriggered 综艺/衍生形态判定（**与 TS `episodeTriggered` 同规**）：
// 裸「第N集/期」与 E 圆号**不触发**（其身份就是集号本身，不需要 epid）。
// EpisodeTriggered 集名是否"可产身份"：**有任何可区分本集的信息**即可（2026-11-11 放宽，用户拍板）。
//
// 放宽点：**序号 `n`**。原判据只看 日期/主次/变体/栏目，**漏了序号** —— 而「第01集」这类**纯序号集名**
// 在**片内**是明确唯一的（同一部剧的第 3 集 ↔ 另一个源的第 3 集）⇒ 必须产 `n=3`（这正是连续剧/动漫
// 派生率为 0.05% / 1.1% 的根因）。
//
// 旧设计误加了"必须**全局**唯一"的前提 ✗：实际所有比较都在**片内** —— 续播是在同一部片的源集名列表里找、
// 切源对齐是同片两个源、弹幕池键本就带 `mid:`、对齐率指标 `GROUP BY (match_key, epid)` ⇒
// 不同片之间的 `n` 重复**永远不会被放在一起比较** ✓
//
// 仍**不触发**：E/EP 圆号（`EpidOf` 不含 `e` 段，会产出"触发但串全空"的病态；且不同源 E 编号常漂 ✗）
// 与真·裸集名（无任何可区分信息，如电影单集的片名/「正片」）。
func EpisodeTriggered(k EpisodeKey) bool {
	return k.Date != "" || k.Part != "" || k.Variant != "" || k.Series != "" || k.Num > 0
}

// EpidOf 5 段身份串 `s|v|p|n|d`（缺值填 `-`）；`v` 段含**变体序号**（`特辑#2`，防「特辑」与「特辑2」撞键）。
// **与 TS `epidOf` 同规**。
func EpidOf(k EpisodeKey) string {
	seg := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	v := k.Variant
	if v != "" && k.VariantNum > 0 {
		v += "#" + strconv.Itoa(k.VariantNum)
	}
	num := "-"
	if k.Num > 0 {
		num = strconv.Itoa(k.Num)
	}
	return "s=" + seg(k.Series) + "|v=" + seg(v) + "|p=" + seg(k.Part) + "|n=" + num + "|d=" + seg(k.Date)
}

// BridgeNum 他源桥接（**与 TS `bridgeNum` 同规**）：在 otherKeys 里找「**自带期号** + 日期相等 +
// 栏目/变体一致 + 主次相容」的集，读其期号；**多个候选期号不一致（错标/缺期漂移）⇒ 放弃（0）**。
// 0 = 桥接失败（调用方回退锚点/不借）。
func BridgeNum(cur EpisodeKey, otherKeys []EpisodeKey) int {
	seen := map[int]bool{}
	for _, k := range otherKeys {
		if k.Num == 0 || k.Date == "" || k.Date != cur.Date {
			continue
		}
		if k.Variant != cur.Variant || k.Series != cur.Series {
			continue
		}
		if !partCompatible(cur, k) {
			continue
		}
		seen[k.Num] = true
	}
	if len(seen) == 1 {
		for n := range seen {
			return n
		}
	}
	return 0
}

// AnchorNumFor 锚点期号（**与 TS `anchorNumFor` 同规**）：期号 = **同栏目组**内「正片『上』」的**日期集合**中
// 不晚于本集日期者个数；组内「上」的日期 < 2 个 ⇒ 返回 0（不猜）；本集无日期或有变体 ⇒ 0。
// 入参 `allTitles` 允许是"本片**所有源**的所有集名"（内部按日期去重，同一期多源只算一次）。
func AnchorNumFor(title string, allTitles []string) int {
	k := ParseEpisodeKey(title)
	if k.Date == "" || k.Variant != "" {
		return 0
	}
	set := map[string]bool{}
	for _, t := range allTitles {
		o := ParseEpisodeKey(t)
		if o.Series != k.Series || o.Variant != "" {
			continue
		}
		if !strings.HasPrefix(o.Part, "上") {
			continue
		}
		if o.Date != "" {
			set[o.Date] = true
		}
	}
	if len(set) < 2 {
		return 0
	}
	dates := make([]string, 0, len(set))
	for d := range set {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	n := 0
	for _, d := range dates {
		if d <= k.Date {
			n++
		}
	}
	return n
}

// DerivedNumFor A2 现算期号（**与 TS `derivedNumFor` 同规**）：
// 自带期号直接用 → 他源桥接 → **锚点兜底**；都不行返回 0。
func DerivedNumFor(title string, allTitles []string) int {
	k := ParseEpisodeKey(title)
	if !EpisodeTriggered(k) {
		return 0
	}
	if k.Num > 0 {
		return k.Num
	}
	if k.Date == "" {
		return 0
	}
	keys := make([]EpisodeKey, 0, len(allTitles))
	for _, t := range allTitles {
		keys = append(keys, ParseEpisodeKey(t))
	}
	if n := BridgeNum(k, keys); n > 0 {
		return n
	}
	return AnchorNumFor(title, allTitles)
}

// EpidFor **落库/匹配口径**的 epid + 期号来源（供 `movie_episode_epid.from_src` 统计）。
//
// 口径 = 「web `epidFor` 的桥接 + `derivedNumFor` 的锚点兜底」——即**带 A2 期号**的身份：
//  1. 未触发（裸「第N集」）⇒ ("", none)：**不产身份**（L6）
//  2. 自带期号 ⇒ (epid, self)
//  3. 缺期号但有日期 ⇒ 先他源桥接（bridge），再锚点兜底（anchor）
//  4. 触发但无日期（身份来自 栏目/变体/主次）⇒ (epid, none)
//
// ⚠️ 与客户端 `danmakuEpid`（= `epidFor`，**不含锚点**）**不同**——见方案 §4.6 的"两处 epid 变体"待办：
// 落库取本函数（含锚点、更精确）；客户端内存池键取 `epidFor`（无锚点）。统一留待 P1-2。
func EpidFor(title string, allTitles []string) (epid string, fromSrc string) {
	k := ParseEpisodeKey(title)
	if !EpisodeTriggered(k) {
		return "", EpKeySourceNone
	}
	if k.Num > 0 {
		return EpidOf(k), EpKeySourceSelf
	}
	if k.Date == "" {
		return EpidOf(k), EpKeySourceNone
	}
	keys := make([]EpisodeKey, 0, len(allTitles))
	for _, t := range allTitles {
		keys = append(keys, ParseEpisodeKey(t))
	}
	if n := BridgeNum(k, keys); n > 0 {
		k.Num = n
		return EpidOf(k), EpKeySourceBridge
	}
	if n := AnchorNumFor(title, allTitles); n > 0 {
		k.Num = n
		return EpidOf(k), EpKeySourceAnchor
	}
	return EpidOf(k), EpKeySourceNone
}

// EpidsForSourceRow 把一个源行的**整段集名**派生成 epid（P1-1 的核心入口）。
// 入参 `titles` 通常取"本片所有源的所有集名"（桥接/锚点需要他源），
// 返回与 `titles` **等长**的 (epid, fromSrc) 切片（未触发/推不出 ⇒ 空串 + none）。
func EpidsForSourceRow(titles []string) [][2]string {
	return EpidsForSourceRowWithPool(titles, titles)
}

// EpidsForSourceRowWithPool 同上，但**显式给出借用/锚点池**（M2, 2026-11-11 评审）：
// 池应是"**本片所有源**的所有集名"（web 的 `allEpisodeTitles` 口径）——
// 只放本源会让 ①**桥接恒不可能命中**（池里没有"他源"）②**锚点在本源缺期时推错** ✗。
// `pool` 为空 ⇒ 退化为 `titles`（单源片的正确行为）。
func EpidsForSourceRowWithPool(titles, pool []string) [][2]string {
	if len(pool) == 0 {
		pool = titles
	}
	out := make([][2]string, 0, len(titles))
	for _, t := range titles {
		epid, from := EpidFor(t, pool)
		out = append(out, [2]string{epid, from})
	}
	return out
}
