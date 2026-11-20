package danmu

import (
	"regexp"
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
//
// ⚠️ **性能（2026-11-16）**：本函数是**线性扫池**（片内 O(E×P)）。**生产派生路径已改走
// `poolIndex.bridgeNum`**（`pool_index.go`：同 (Date, Series, Variant) 桶）；本函数**保留作
// 兼容/对照**——它是等价性单测的基准（`TestPoolIndex_EquivalentToLinear`），TS 侧 `bridgeNum` 仍按本算法。
// 改桥接口径时**两处都要改**，否则对照测试会红。
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
// AnchorNumFor 锚点期号（**与 TS `anchorNumFor` 同规**）：期号 = **同栏目组**内「正片『上』」的**日期集合**中
// 不晚于本集日期者个数；组内「上」的日期 < 2 个 ⇒ 返回 0（不猜）；本集无日期或有变体 ⇒ 0。
// 入参 `allTitles` 允许是"本片**所有源**的所有集名"（内部按日期去重，同一期多源只算一次）。
//
// 兼容包装（性能修复 2026-11-15）：内部解析 title 与 allTitles 各一次后转调键版 `AnchorNumForKey`
// —— 解析顺序与原实现逐字相同 ⇒ 行为等价；池已解析的调用方请直用键版（省 E×P 重解析）。
func AnchorNumFor(title string, allTitles []string) int {
	k := parseEpisodeKey(title)
	keys := make([]EpisodeKey, 0, len(allTitles))
	for _, t := range allTitles {
		keys = append(keys, parseEpisodeKey(t))
	}
	return AnchorNumForKey(title, k, keys)
}

// AnchorNumForKey 锚点期号的**已解析键**版（与 AnchorNumFor 逐字节等价）：
// `k` 必须是 `ParseEpisodeKey(title)` 的结果（title 原文仅供 R10 裸日期守卫匹配），
// `poolKeys` 是池标题的解析结果（顺序 = 池标题顺序；去重语义在函数内不变）。
//
// ⚠️ **性能（2026-11-16）**：本函数**每次调用都重建**「Series → 「上」日期集合」（O(P) + 分配）
// ⇒ 片内 O(E×P)。**生产派生路径已改走 `poolIndex.anchorNum`**（建一次 + 二分）；
// 本函数**保留作兼容/对照**（等价性单测基准），TS 侧 `anchorNumFor` 仍按本算法。改口径两处都要改。
func AnchorNumForKey(title string, k EpisodeKey, poolKeys []EpisodeKey) int {
	if k.Date == "" || k.Variant != "" {
		return 0
	}
	// R9/R10（2026-11-15）：裸日期行收窄 —— 每天更新、多数行没有上/下的节目「上」稀疏，
	// anchor 会把多条不同日期的行压成同一个 n（实测 21,768 行 / 370 mk / 42% 真踩：
	// 0520/0521/0526/0527 同为 n=1）。宁缺勿错 ⇒ 裸日期行（`^\d{8}期?$`）且无 p ⇒ 不猜（n=-，靠 d 区分）。
	// 有 p（如 20230616上）⇒ 仍走 anchor（「上」计数正是 anchor 的本职）；有 v ⇒ 上面已挡（原本就不走）。
	if k.Part == "" && reBareDateRow.MatchString(strings.TrimSpace(title)) {
		return 0
	}
	set := map[string]bool{}
	for _, o := range poolKeys {
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
// （池只解析一次；原实现经 AnchorNumFor 会把池解析两遍。）
func DerivedNumFor(title string, allTitles []string) int {
	k := parseEpisodeKey(title)
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
		keys = append(keys, parseEpisodeKey(t))
	}
	if n := BridgeNum(k, keys); n > 0 {
		return n
	}
	return AnchorNumForKey(title, k, keys)
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
// EpKeySourceSkip **未触发** —— 与 `none`（触发了但推不出期号）**区分开**：
// 指标层据此把"空 epid"拆成 ①未触发 ②触发推不出 ③未派生 三档（2026-11-11 审查补齐）。
// 旧数据里这两种情况都记成 `none` ⇒ rebuild 后才有 `skip` ✓（不破坏兼容：`from=` 只是标注）。
const EpKeySourceSkip = "skip"

// EpKeySourceSingle **单集作品级身份**（老规划 §A.1 定名；2026-11-14 落地）：整行只有 1 集、
// 且集名**不含任何序号信息**（不含 ASCII 数字）⇒ 按 `n=1` 给身份。与 self/bridge/anchor/none 并列，
// 指标 `byFromSrc` 自动分档（便于回看这条规则贡献了多少）。
// ⚠️ 该值须同时登记进 `ParseEpKeySource` 的已知值白名单（老规划 §A.1 改动点 2），否则端侧若带上它
// 会被静默当"未提供"⇒ 退回旧规则。详见 `EpidsForSourceRowWithPool`。
const EpKeySourceSingle = "single"

func EpidFor(title string, allTitles []string) (epid string, fromSrc string) {
	epid, fromSrc, _ = EpidForRule(title, allTitles)
	return epid, fromSrc
}

// EpidForRule 同 `EpidFor`，但**额外返回命中的决策规则编号**（`RuleID`，登记表见 `epid_rules.go`）。
//
// 分派本身是**有序规则表**（`titleRules` 的 D1~D6，顺序即优先级），本函数只是它的薄封装 ⇒
// "改规则 ⇒ 重算范围"可以直接查那条规则的 `Scope`/`InFromSrc`（不再靠人脑推影响面）。
// 将来若把"命中规则"落库到 `movie_episode_epid`，存的就是这个 `id`。
func EpidForRule(title string, allTitles []string) (epid string, fromSrc string, id RuleID) {
	epid, id = decideTitle(title, allTitles)
	return epid, RuleMetaOf(id).OutFromSrc, id
}

// EpidsForSourceRow 把一个源行的**整段集名**派生成 epid（P1-1 的核心入口）。
// 入参 `titles` 通常取"本片所有源的所有集名"（桥接/锚点需要他源），
// 返回与 `titles` **等长**的 (epid, fromSrc) 切片（未触发/推不出 ⇒ 空串 + none）。
// hasASCIIDigit 名称里是否含 ASCII 数字（R12 的排除判据）。
//
// 判据演进（2026-11-14）：原用"单片/单版本形态白名单"，线上实测发现漏掉常见写法
// （`HD中字`、`None`、以片名命名的单集行…）；用户拍板「**只要该行只有 1 集，就直接按单片单集处理**」⇒
// 改成"**集名不含 ASCII 数字**"：整行只有 1 集时 `n=1` 就是这一行的事实，而"含数字"恰好一次排除掉
// 所有带序号/区间/日期的写法（`第1-20集`/`OVA01`/`EP09`/`第7.5集`/`240103`/裸数字…）⇒
// 既简单又守得住"宁缺勿错"（不误撞真序号，也不把待定形态 R14/R15/R16 夹带进来）。
func hasASCIIDigit(name string) bool {
	for i := 0; i < len(name); i++ {
		if isDigitByte(name[i]) {
			return true
		}
	}
	return false
}

// bareNum 裸数字集名（R13，2026-11-14 服务端口径）：去空白后**整串**是 1–4 位十进制数字 ⇒ 返回其值。
//
// 上界取 4 位：6 位是 YYMMDD（R11 已处理）、5 位以上含义不明 ⇒ 宁缺勿错；前导零按十进制（`001` ⇒ 1 ✓）。
// 是否采纳还要看调用方的**行内 distinct ≥2** 守卫（单条裸数字可能是线路/版本号，成串才是集序 ✓）。
func bareNum(name string) (int, bool) {
	s := strings.Join(strings.Fields(name), "")
	if len(s) == 0 || len(s) > 4 {
		return 0, false
	}
	v := 0
	for i := 0; i < len(s); i++ {
		if !isDigitByte(s[i]) {
			return 0, false
		}
		v = v*10 + int(s[i]-'0')
	}
	if v <= 0 {
		return 0, false
	}
	return v, true
}

// cnNumeral 中文数字 → 值（2026-11-16）：支持 十/百/千 组合（`十一`=11、`二十`=20、`二十三`=23、
// `一百二十三`=123、`两百`=200、`十`=10）。**含任何非中文数字字符 ⇒ 拒绝**（返回 false）——
// 与"宁缺勿错"一致：`第X期` 里的 X 若含别的字就不产期号。
// 上界不额外设（实际集名不会到 千位以上）；`零/〇` 单独出现 ⇒ 返回 0 ⇒ 调用方按"不产期号"处理。
func cnNumeral(s string) (int, bool) {
	digit := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
		'五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	unit := map[rune]int{'十': 10, '百': 100, '千': 1000}
	total, cur, seen := 0, 0, false
	for _, r := range s {
		if d, ok := digit[r]; ok {
			cur = d
			seen = true
			continue
		}
		if u, ok := unit[r]; ok {
			if cur == 0 {
				cur = 1 // `十一` 的 `十` 前面没有数字 ⇒ 当 1
			}
			total += cur * u
			cur = 0
			seen = true
			continue
		}
		return 0, false // 含非中文数字字符 ⇒ 拒绝
	}
	if !seen {
		return 0, false
	}
	return total + cur, true
}

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
	// 逐集分派（D1~D6）+ 行级补充（D7=R12 / D8=R13）都在 `epid_rules.go` 的**有序规则表**里；
	// 本函数只负责"算一次行级上下文 → 逐集取结果"。两条行级规则的判据与守卫见其 `Desc`：
	//   D7（R12）**整行只有 1 集** ∧ 集名不含 ASCII 数字 ⇒ `n=1`（只跑 `skip` 档）——
	//     实测"含 HD国语 的源行"4,101/4,557（90%）只有 1 集 ⇒ `HD国语` 是**那一集的名字**；另 428 条
	//     `HD国语 | 第01集 | …`（整片位 + 逐集）由"只有 1 集"这条守卫挡掉 ✓
	//   D8（R13）行内 **distinct** 裸数字名 ≥ 2 ⇒ `n=该数字`（`1`/`02`/`001` ⇒ 1/2/1；来源仍记 `self`）
	//     ⚠️ **不能用"行内 ≥2 条"**：双写会让同一集名在同一行出现两次 ⇒ 该条件恒成立 ✗（实测踩过）
	ctx := newRuleCtx(titles, pool)
	out := make([][2]string, 0, len(titles))
	for _, t := range titles {
		epid, from, _ := decideRow(t, ctx)
		out = append(out, [2]string{epid, from})
	}
	// L2（2026-11-19）：同源同日消歧——同日期 ≥2 条、去尾号后文本一致 ⇒ 尾号写 p（compact epid 无 vn 段）
	disambiguateSameDayTails(titles, out)
	return out
}

// reTailNum L2 用：名字末尾的 1~3 位序号。
var reTailNum = regexp.MustCompile(`^(.*?)(\d{1,3})$`)

// disambiguateSameDayTails（L2，2026-11-19）：
// 同源、同一日期出现 ≥2 条，且"去掉尾部序号后的文本完全一致"时，把尾部序号写进 p 段。
// 守卫：只在该日期组内生效；单条尾号一律不动（防「海贼王1000」）；p 已有值（上/下）或带 vn 的不动。
func disambiguateSameDayTails(titles []string, out [][2]string) {
	if len(out) < 2 {
		return
	}
	byDate := make(map[string][]int, len(out)/2)
	keys := make([]EpisodeKey, len(out))
	for i := range out {
		k, ok := ParseEpidString(out[i][0])
		if !ok || k.Date == "" {
			continue
		}
		keys[i] = k
		byDate[k.Date] = append(byDate[k.Date], i)
	}
	for _, idxs := range byDate {
		if len(idxs) < 2 {
			continue
		}
		base := ""
		nums := make(map[int]bool, len(idxs))
		ok := true
		for _, i := range idxs {
			k := keys[i]
			if k.Part != "" || k.VariantNum != 0 {
				ok = false
				break
			}
			m := reTailNum.FindStringSubmatch(strings.TrimSpace(titles[i]))
			if m == nil || strings.TrimSpace(m[1]) == "" {
				ok = false
				break
			}
			n, err := strconv.Atoi(m[2])
			if err != nil || n <= 0 || n > 2000 || nums[n] {
				ok = false
				break
			}
			if base == "" {
				base = m[1]
			} else if m[1] != base {
				ok = false
				break
			}
			nums[n] = true
			keys[i].Part = m[2]
		}
		if !ok {
			continue
		}
		for _, i := range idxs {
			out[i][0] = EpidOf(keys[i])
		}
	}
}

// normNameForPrefix 规范化片名（去半角/全角空格与间隔号），用于"s 前缀命中"比较。
func normNameForPrefix(s string) string {
	return strings.NewReplacer(" ", "", "\u3000", "", "·", "").Replace(s)
}

// StripFilmNamePrefix（2026-11-19 用户口径；③ 同日放宽为三条命中情形）：某行 s 与「片名」满足
// A/B/C 任一 ⇒ 去掉该 s（含季号 `#S<n>`/`#B<n>`）：
//   - **A 相等**：norm(s) == norm(片名)；
//   - **B 互为前缀**：norm(s) 以片名为前缀，**或片名以 norm(s) 为前缀**（后者用于 s 被削短的场景，
//     与 ②「裸数字紧跟日期不剥」配套 —— 修前 s=创造 vs 片名 创造101）；
//   - **C 片名去尾数字后相等**：norm(s) == strings.TrimRight(norm(片名), "0123456789")（s=创造 vs 创造101）。
//
// 评审注记（用户知情接受）：C 的跨片误伤理论面已知（他片去尾数字后撞名）；反例用例
// （另一部剧#S2 / 普通男女）钉住常见形态。
//   - 只处理 s 非空的行；filmName 为空则不做任何事；
//   - 守卫：去掉后若在同一源行内导致 epid 重复 ⇒ 该行不改（宁可不改）；
//   - 返回实际改动的行数。
func StripFilmNamePrefix(out [][2]string, filmName string) int {
	fn := normNameForPrefix(filmName)
	if fn == "" || len(out) == 0 {
		return 0
	}
	fnTailless := strings.TrimRight(fn, "0123456789")
	type chg struct {
		idx     int
		newEpid string
	}
	var changes []chg
	for i := range out {
		k, ok := ParseEpidString(out[i][0])
		if !ok || k.Series == "" {
			continue
		}
		ns := normNameForPrefix(k.Series)
		if ns == "" {
			// 规范化后为空（纯空格/间隔号）⇒ B 反向（片名以 "" 为前缀恒真）会误清任何行，保守跳过
			continue
		}
		hit := ns == fn || // A 相等
			strings.HasPrefix(ns, fn) || strings.HasPrefix(fn, ns) || // B 互为前缀
			ns == fnTailless // C 片名去尾数字后相等
		if !hit {
			continue
		}
		k2 := k
		k2.Series = ""
		changes = append(changes, chg{i, EpidOf(k2)})
	}
	if len(changes) == 0 {
		return 0
	}
	seen := make(map[string]int, len(out))
	for i := range out {
		seen[out[i][0]]++
	}
	for _, c := range changes {
		seen[c.newEpid]++
	}
	n := 0
	for _, c := range changes {
		if seen[c.newEpid] > 1 { // 去重守卫：会与同源行内其它行撞键 ⇒ 不改
			continue
		}
		out[c.idx][0] = c.newEpid
		n++
	}
	return n
}
