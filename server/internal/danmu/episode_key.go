package danmu

import (
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

// EpisodeKey 从「集名/集标题」解析出的**同集判定键**（见同名文档 docs/排查-弹幕集名映射与综艺错位）。
type EpisodeKey struct {
	Date string // YYYYMMDD（19xx/20xx，无则空）
	Num  int    // 第N期 / 第N集 / 第N话 / 第N回 / 第N季 / 第N部（无则 0）
	ENum int    // E01 / EP01（无则 0）
	Part string // **主部分（复合单段）**：上 / 中 / 下；若紧跟（最多隔一个左括号）还有 上/中/下 ⇒ 复合为
	//                「中上」「中下」「上中」等（P0-1c 2026-11-10：由 part+sub 两字段合并为一个复合字段）。
	//                比较规则见 partCompatible —— **前缀相容**（短的是长的前缀 ⇒ 相容，`中` ⊂ `中上`/`中下`）。
	Variant string // 有意义的变体词集合，按字典序用 "," 连接（无则空）
	//                ⚠️ M1（2026-11-11 评审）：**不是 `|`** —— `|` 是 epid/epk 的段分隔符，
	//                多变体会把串写成 7 段 ⇒ 服务端拒收 + web parseEpid 恒 null + 落库坏串 ✗。
	// VariantNum **变体序号**（无则 0）：变体词后**紧跟**的数字 —— 「特辑2」⇒ (Variant=特辑, VariantNum=2)。
	// 必须与变体复合为身份：否则「特辑1..5」坍缩成同一个 Variant，L3a 组内不唯一 ⇒ 整组配不上
	// （桃花坞6 特辑 4~5 集丢弹幕的根因；P0-1a，2026-11-10 三端同规）。纯日期/5 位以上数字不取（见 reVariantNum）。
	VariantNum int
	Series     string // **栏目/系列名**（正片为空）：如「心动的周末」「心动的旅行」；
	//                需 ≥2 个中文字才算数（过滤「版」「片」这类残留）
}

func (k EpisodeKey) IsZero() bool {
	return k.Date == "" && k.Num == 0 && k.ENum == 0 && k.Part == "" &&
		k.Variant == "" && k.VariantNum == 0 && k.Series == ""
}

// ===== P0-1b（2026-11-10）：结构化身份字段契约 =====
//
// 客户端把「集名解析结果」序列化成传输串（HTTP 参数 `epk`）传下来：
//
//	v1|d=<date>|n=<num>|e=<eNum>|p=<part>|v=<variant>|vn=<variantNum>|s=<series>|o=<ordinal>|from=<src>
//
// 目的：① 服务端不再"重新猜"客户端的解析结果，**三端同规的漂移可被检出**（逐项交叉校验 + 打点）；
//      ② 为 P1-2（epid 升格为集唯一标识）铺路。
// 数值缺省写 0、字符串缺省写空；`o` 为可选序位兜底（本批仅透传，不参与判定）；
// `from` 为期号来源（P1-3：self|bridge|anchor|none）—— 不参与身份判定，但服务端用它判定
// "`n` 是否来自桥接"（见 `ParseEpKeySource` 与 `DanmuService.resolveAggregateEpisode` 的 numBridged）。
// L1（2026-11-11 评审）：P0-1c 起 `p` 是**复合单段**（如 `中上`），**没有独立的 `u=` 段**（旧注释残留已删）。
// 服务端仍收 `epTitle`：既作交叉校验基准，也作旧客户端回退与排查依据。

// EpKeyVersion 传输串版本前缀。格式变更时递增；服务端遇到不认识的版本一律按"未提供"处理（行为不变）。
const EpKeyVersion = "v1"

// Serialize 序列化为传输串（与 ParseEpisodeKeyString 互逆）；ordinal 为可选序位兜底，<=0 写 0。
//
// L1（2026-11-11 评审）：**与 TS `serializeEpisodeKey` 逐字对齐**（末尾补 `from=none`）——
// 原来 Go 侧少一段，同一份键两端序列化结果不同（Go 侧目前只用于测试/排查，但对齐可免后患）。
func (k EpisodeKey) Serialize(ordinal int) string {
	return strings.Join([]string{
		EpKeyVersion,
		"d=" + k.Date,
		"n=" + strconv.Itoa(k.Num),
		"e=" + strconv.Itoa(k.ENum),
		"p=" + k.Part,
		"v=" + k.Variant,
		"vn=" + strconv.Itoa(k.VariantNum),
		"s=" + k.Series,
		"o=" + strconv.Itoa(max(ordinal, 0)),
		"from=" + EpKeySourceNone,
	}, "|")
}

// ParseEpisodeKeyString 解析传输串。**版本不认识 / 结构不合法 ⇒ ok=false**（调用方按"未提供"处理，
// 行为与改动前完全一致）。未知字段名一律忽略（向前兼容）。
func ParseEpisodeKeyString(s string) (EpisodeKey, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return EpisodeKey{}, false
	}
	parts := strings.Split(s, "|")
	if len(parts) < 2 || parts[0] != EpKeyVersion {
		return EpisodeKey{}, false
	}
	var k EpisodeKey
	seen := false
	atoi := func(val string) (int, bool) {
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 || n > 9999 {
			return 0, false
		}
		return n, true
	}
	for _, kv := range parts[1:] {
		i := strings.IndexByte(kv, '=')
		if i <= 0 || len(kv)-i-1 > 64 { // 限长防滥用
			return EpisodeKey{}, false
		}
		key, val := kv[:i], kv[i+1:]
		switch key {
		case "d":
			k.Date = val
		case "n":
			n, ok := atoi(val)
			if !ok {
				return EpisodeKey{}, false
			}
			k.Num = n
		case "e":
			n, ok := atoi(val)
			if !ok {
				return EpisodeKey{}, false
			}
			k.ENum = n
		case "p":
			if !isPartMark(val) {
				return EpisodeKey{}, false
			}
			k.Part = val
		case "v":
			k.Variant = val
		case "vn":
			n, ok := atoi(val)
			if !ok {
				return EpisodeKey{}, false
			}
			k.VariantNum = n
		case "s":
			k.Series = val
		case "o":
			// 序位兜底：本批仅透传（解析校验后丢弃），不参与判定
			if _, ok := atoi(val); !ok {
				return EpisodeKey{}, false
			}
		default:
			// 未知字段：向前兼容，忽略
		}
		seen = true
	}
	if !seen {
		return EpisodeKey{}, false
	}
	return k, true
}

// isPartMark 合法的 part 值：空 / 上 / 中 / 下 / **复合**（如 中上、中下、上中 —— 主部分 + 紧跟的次部分）。
func isPartMark(s string) bool {
	rs := []rune(s)
	if len(rs) == 0 {
		return true
	}
	if len(rs) > 2 {
		return false
	}
	isP := func(r rune) bool { return r == '上' || r == '中' || r == '下' }
	// P2（2026-11-12）：日期尾随「当日集序」落 p 段 ⇒ 纯数字（1~2 位）也是合法 p 值。
	allP, allD := true, true
	for _, r := range rs {
		if !isP(r) {
			allP = false
		}
		if r < '0' || r > '9' {
			allD = false
		}
	}
	return allP || allD
}

// MergeEpidKey 交叉校验合并（P0-1b）：客户端结构化键为**实测值** ⇒ 默认优先采用，再与集名解析值逐项核对。
//   - date：双方都有且不等 ⇒ **以集名解析值为准**（硬证据），记冲突；
//   - num：同上；但 numBridged=true（该期号来自 A2 桥接、非集名自带）时**跳过比较** —— 否则会把桥接
//     结果打回原值，心动9/桃花坞6 的纯日期形态会再次全丢；
//   - 其余字段（eNum/part/variant/variantNum/series）服务端**无独立证据** ⇒ 只记不一致，**不覆盖**。
//     L1（2026-11-11 评审）：P0-1c 起主部分已合并（**没有独立的 `sub`**）⇒ 不再有 `sub` 项。
//
// 只记录不填补：若客户端某字段为空而集名解析非空，属解析漂移，记冲突以便发现（不做"猜着补"）。
func MergeEpidKey(client, fromTitle EpisodeKey, numBridged bool) (EpisodeKey, []string) {
	k := client
	var conflicts []string
	if k.Date != "" && fromTitle.Date != "" && k.Date != fromTitle.Date {
		conflicts = append(conflicts, "date")
		k.Date = fromTitle.Date
	}
	if !numBridged && k.Num > 0 && fromTitle.Num > 0 && k.Num != fromTitle.Num {
		conflicts = append(conflicts, "num")
		k.Num = fromTitle.Num
	}
	if k.ENum != fromTitle.ENum {
		conflicts = append(conflicts, "eNum")
	}
	if k.Part != fromTitle.Part {
		conflicts = append(conflicts, "part")
	}
	if k.Variant != fromTitle.Variant {
		conflicts = append(conflicts, "variant")
	}
	if k.VariantNum != fromTitle.VariantNum {
		conflicts = append(conflicts, "variantNum")
	}
	if k.Series != fromTitle.Series {
		conflicts = append(conflicts, "series")
	}
	return k, conflicts
}

// epKeyConflictTotal 客户端解析 ≠ 服务端解析的累计次数（P0-1b 打点）——
// 它是"三端同规"是否漂移的哨兵：正常应恒为 0；一旦上涨说明三端解析已不一致。
var epKeyConflictTotal int64

// EpKeySources 期号来源（P1-3, 2026-11-10）：`epk` 的 `from=` 段取值。**不参与身份判定**，
// 仅供服务端在 P1-1 落库/统计时记录「这个 `n` 是怎么来的」。
const (
	EpKeySourceSelf   = "self"   // 集名自带期号
	EpKeySourceBridge = "bridge" // 他源桥接（日期相等 + 栏目/变体一致 + 主次相容）
	EpKeySourceAnchor = "anchor" // 锚点推算（同栏目组内「上」的日期序）
	EpKeySourceNone   = "none"   // 无（未触发 / 期号缺失且推算不出）
)

// ParseEpKeySource 取 `epk` 里的 `from=` 段（P1-3）。缺省 / 超长 / 空串 ⇒ ""（调用方当"未提供"）。
// 只认四个已知值，其余一律返回 ""（避免把客户端异常值当来源统计）。
func ParseEpKeySource(epk string) string {
	s := strings.TrimSpace(epk)
	if s == "" {
		return ""
	}
	for _, kv := range strings.Split(s, "|") {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		if kv[:i] != "from" {
			continue
		}
		v := kv[i+1:]
		switch v {
		// ⚠️ 老规划 §A.1 改动点 2：`EpKeySourceSingle` 必须登记在此，否则端侧若带上 `from=single`
		// 会被静默当"未提供"（退回 `epNum > 0` 旧规则）。
		case EpKeySourceSelf, EpKeySourceBridge, EpKeySourceAnchor, EpKeySourceNone, EpKeySourceSingle:
			return v
		}
		return ""
	}
	return ""
}

// NoteEpKeyConflicts 记一次字段级冲突（落日志 + 计数）。
//
// L5（2026-11-11 评审）：**明细日志限频**（每 60s 最多一条）—— 三端解析漂移时日志量会 = 请求量 ✗；
// 计数（`EpKeyConflictTotal`）不受限频影响，仍逐次累加 ✓（用于监控"是否在漂移"）。
func NoteEpKeyConflicts(fields []string) {
	if len(fields) == 0 {
		return
	}
	atomic.AddInt64(&epKeyConflictTotal, 1)
	now := time.Now().UnixMilli()
	if last := lastEpKeyConflictLogMs.Load(); now-last >= 60_000 &&
		lastEpKeyConflictLogMs.CompareAndSwap(last, now) {
		log.Printf("[danmu] epkey conflict fields=%s（明细每 60s 限一条，计数仍逐次累加）",
			strings.Join(fields, ","))
	}
}

// lastEpKeyConflictLogMs 上次打明细日志的时间（L5 限频用）。
var lastEpKeyConflictLogMs atomic.Int64

// EpKeyConflictTotal 累计冲突次数（测试/运维观测用）。
func EpKeyConflictTotal() int64 { return atomic.LoadInt64(&epKeyConflictTotal) }

var (
	reDate8 = regexp.MustCompile(`(?:19|20)\d{6}`)
	// reDate6 6 位 YYMMDD（R11，2026-11-14 **服务端口径**）：线上实测"空 epid 里的 6 位纯数字"
	// 共 2,743 条，其中**月日合法 2,742（99.96%）**、年份前缀 ∈19–26 ⇒ 这是真实存在的日期写法
	// （周更综艺，如 240103 → 20240103，同片单线路 272 集、每周三）。补齐成 8 位后可与其它源的
	// 8 位日期**直接比对**（`d` 是原始事实 ⇒ 还能参与桥接）。
	// ⚠️ 这里只用 `\d{6}` 打底，**边界与合法性在 `date6` 里手工校验**（RE2 无 lookaround，
	// 沿用"消费后手工判边界"的既有手法），避免把 7/9 位数字串截出半截当日期 ✗。
	reDate6 = regexp.MustCompile(`\d{6}`)
	// reDiN 集序：`第<数>[期集话回]` → Num（**单位不含 季/部**，2026-11-16 修正）。
	// 为什么去掉 季/部：它们是**分季/分部标记，不是集序**；留在单位类里会让「第7季第5集」的
	// 第一个匹配落在"第7季" ⇒ 集号被**抢占**成 7（实测 `第N季` 73 行 / `第N部` 105 行的 n 段
	// 全部非空 = 被季/部号占据）。改后既消除这 178 行错值，又让「第X季第Y集」取到正确的 Y。
	// 季/部信息本身由 `seriesOf` 的 `reSeasonSpan` 一并从栏目名候选里剥掉（它不该进 s）。
	reDiN = regexp.MustCompile(`第\s*(\d{1,4})\s*[期集话回]`)
	// reDiNCn（2026-11-16）：**中文数字期号** —— `第一期/第十二期/第二十三集/第一百期`。
	// 为什么补：`reDiN` 只吃阿拉伯数字 ⇒「第二期」这类**落进 s 段**（实测 155 行 `s=第X期`），
	// 与其它源的「第2期」永不相等（伪分歧）。转换值由 `cnNumeral` 负责（支持 十/百/千 组合）。
	// 单位与 `reDiN` **保持一致**（期/集/话/回；**不含 季/部** —— 详见 reDiN 注释）。
	reDiNCn = regexp.MustCompile(`第\s*([一二三四五六七八九十百千零〇两]{1,6})\s*[期集话回]`)
	// reDiNAnchor（M4，2026-11-17 取证批）：**整串**「第N」（缺单位）/「N[期集话回]」（缺第）⇒ Num。
	// 取证（代表片逐集对比）：短剧「第99」（跨源邻槽=第98/100集 为证，243 行）与「103集」
	// （多站 2,264 行/57 mk，如 src_lz 整源 '01集'..'NN集'）。普遍性实测见
	// 方案-代表片取证归因与优化方案-2026-11-17.md §7（scripts/epid-class-prevalence.sql）。
	// ⚠️ 锚定 ^$：只吃**整串**形态；「第N：副标题」「第N 上」等复合形态不在内（先试算后拍板的口径就到这里）；
	// ⚠️ 单位不含 季/部（10-16 修正不变：「第7季」不以任何形态给 Num）。
	reDiNAnchor = regexp.MustCompile(`^(?:第\s*(\d{1,4})|(\d{1,4})\s*[期集话回])$`)
	// reDiNBare（M4 配套，seriesOf 侧）：裸「第N」（无单位）从栏目名候选剥掉 ——
	// 否则「第99」得 s=第99 与「第99集」的 s=- 永不相等（取证片9 跨源不一致实证）。
	reDiNBare = regexp.MustCompile(`第\s*\d{1,4}\s*$`)
	// reENum（R4，2026-11-15）：捕获组回填 —— 原式 `(?:^|[^A-Za-z])…(?:\D|$)` 的边界断言被正则
	// **消费掉**，seriesOf 的 `ReplaceAllString` 连相邻汉字一起删（`EP01白昼逃亡`→`昼逃亡`、
	// `白昼逆行EP10`→`白昼逆` 截断 ✗）。RE2 无 lookahead ⇒ 改捕获组：删除处以 `$1`/`$3` 回填邻居
	// 字符（取数用 m[2]）。仍保留消费式断言语义（邻居不参与 E 号本身）。
	reENum = regexp.MustCompile(`(?i)(^|[^A-Za-z])(?:E|EP)\s*0*(\d{1,3})(\D|$)`)
	// reVariant = **有意义的变体**：标识"同一集的另一份内容"，参与同集判定（两侧必须完全相等）。
	// R7（2026-11-15）：`未播`/`未公开` 从噪声表挪入 —— 它们是"同一集的另一份内容"（播出状态标签），
	// 当噪声删会把「第1期未播」与「第1期」拆成两个身份 ✗；而 P-4 实测 `直拍REACTION第10期` 与
	// `第10期` 是同一集 ⇒ `直拍` 留在噪声表不动。
	reVariant = regexp.MustCompile(`加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外|未播|未公开`)
	// reVariantNum 变体序号（P0-1a）：变体词后**紧跟**的数字（最多隔一个左括号/空白）。
	//   特辑2          → 2      （与「特辑」不同身份）
	//   特辑（2）      → 2
	//   加更版1        → 0      （隔着「版」⇒ 不算紧跟）
	//   特辑2026 / 特辑12345 → 0（纯日期/长数字不取）
	// ⚠️ RE2 **不支持 lookahead** ⇒ 用 `(?:[^0-9]|$)` **消费一个后继字符**做等价断言（三端同此写法，保证字面一致）。
	reVariantNum = regexp.MustCompile(`(?:加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外)[\s（(【\[《「『]*(\d{1,3})(?:[^0-9]|$)`)
	// reVariantSpan（R2，2026-11-15）：**变体跨度吸收前后修饰** —— seriesOf 删栏目候选时用它替代裸
	// reVariant：「舞台纯享版第6期」此前删掉纯享后残留「舞台…版」两段 ⇒ 最长连续汉字段 s=舞台 ✗；
	// 吸收后整段消失 ⇒ s=""，v 仍= 纯享（**不改 v 词表**，reVariant 本身不动）。前缀可重复（`收官特别`
	// = 两连缀）；后缀只有 版/典藏版。变体位置含 R7 新词（与 reVariant 词表一致）。
	reVariantSpan = regexp.MustCompile(`(?:剧情|舞台|收官|特别)*(?:加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外|未播|未公开)(?:典藏版|版)?`)
	// reVariantNoise = **无意义后缀**：只是同一集的版本标签，解析前一律剔除（与括号/标点同等）。
	// R7：未播/未公开 已挪进 reVariant（变体），此处不再删。
	reVariantNoise = regexp.MustCompile(`直拍|完整版|未删减`) // L1-c（2026-11-19）：会员移出噪声，改认 v=会员
	// reVariantPlus（L1-b，2026-11-19）：plus 版本标签（大小写不敏感）。
	reVariantPlus = regexp.MustCompile(`(?i)plus`)
	// reMemberVariant（L1-c，2026-11-19）：只认 会员版/会员加更/会员专享；裸"会员"不动。
	reMemberVariant = regexp.MustCompile(`会员\s*(?:版|加更|专享)`)
	// reQualityTail = **画质/字幕/语言修饰词（集名尾部）**：解析前剥离（R17，2026-11-15）。
	// 「HD中字」曾分裂同片多源 epid（中字行 s=中字|p=中 vs 国语/正片行 single n=1 ⇒ 39,227 部电影
	// 多源零共享，全库对齐率被压 ~11pt）。**限定尾部**（$）避免误删中缀真词（如「英语课」）；
	// 语言前缀（国语/粤语/英语…）与「中字」组合一并剥。s 站实测全谱见 server-state 2026-11-15。
	// R17 扩展（2026-11-17）：语言/画质类带「版」的写法。
	// 实测（真库）：英文版52/高清版34/中字版14/超清版14/英语版8/粤语版6/原声版2/日语版·法语版·川话版·美版·字幕版·清晰版 各1~2（≈130 行）——
	// 它们与「中字/国语/高清」同义，**必须当噪声删**（否则会与不带"版"的写法分裂：`中字` 被删 vs `中字版` 落 s ⇒ 新伪分歧）。
	// ⚠️ 只收「语言/画质」类；**内容版本类**（加长版/尊享版/大神版…）另行按变体处理，勿混入本表。
	reQualityTail = regexp.MustCompile(`(?:国语|粤语|英语|法语|日语|俄语|意大利语)?(?:中字|无字)$|(?:高清|标清|超清|国语版|黑白版|终极)$|(?:国语|粤语|英语|法语|日语|俄语|意大利语)$|(?:高清版|超清版|标清版|蓝光版|原声版|中字版|无字版|字幕版|清晰版|上色版|双音版|美版|粤语版|英语版|日语版|法语版|俄语版|意大利语版|普通话版|川话版|潮汕话版|湖南话版|闽南语版)$`)
	// reSeriesStop = 语言/画质/字幕/版本标签，不是栏目名（避免把「正片」「国语」「中字」当系列）。
	// 2026-11-15 补全（R17）：「HD中字」的「中字」此前被误当栏目（s=中字|p=中）⇒ 同片多源 epid 分裂
	//（中字行 vs 国语/正片行的 single n=1）⇒ 39,227 部电影多源零共享（全库对齐率被压 ~11pt）——
	// 实证与规模见 server-state 2026-11-15 电影根因条目。
	// 2026-11-17 扩展：语言/画质**带「版」**的写法也视为"不是栏目名"。
	// 为什么必须同时加这里（而不是只加 reQualityTail）：`reQualityTail` 是**串尾锚定**，
	// 遇到「HD（普通话版）」这种**括号包裹**的写法时 `$` 落在 `）` 上 ⇒ 不命中 ⇒
	// 括号转占位后「普通话版」会独占片段、被当成栏目名（单测实测抓到）。
	// ⚠️ 只收语言/画质类；内容版本类（加长版/尊享版/大神版…）不在此表。
	reSeriesStop = regexp.MustCompile(`^正片$|^国语$|^粤语$|^原声$|^普通话$|^双语$|^中字$|^国语中字$|^粤语中字$|^英语中字$|^法语中字$|^日语中字$|^俄语中字$|^国语无字$|^国语版$|^英语$|^日语$|^意大利语$|^高清$|^标清$|^超清$|^黑白版$|^终极$|^高清版$|^超清版$|^标清版$|^蓝光版$|^原声版$|^中字版$|^无字版$|^字幕版$|^清晰版$|^上色版$|^双音版$|^美版$|^粤语版$|^英语版$|^日语版$|^法语版$|^俄语版$|^意大利语版$|^普通话版$|^川话版$|^潮汕话版$|^湖南话版$|^闽南语版$`)
	// reBracketChar（R3，2026-11-15）：各类括号**字符** —— 只作分隔（占位、不删内容）。
	// 原式（连内容整段删）⇒ 括号里的栏目名丢失（`副本存档中`/`万事屋`/`居民采访`…）；
	// 改后内容交给护栏过滤：非汉字段（FieldsFunc）、reSeriesStop、2~8 字上限 ⇒
	// `【qq】`（非汉字）/`（会员版）`（会员是噪声⇒剩 1 字）仍被挡 ✓。
	reBracketChar = regexp.MustCompile(`【|】|（|）|\(|\)|《|》|「|」|『|』|\[|\]`)
	// reQuotes（R1，2026-11-15）：引号 = **透明字符** —— 无占位直接删、不丢内容：
	// `20261001“聚”乐部` 此前落入"非汉字分隔"把词切成 聚/乐部 两段（s=乐部 ✗），删后 聚乐部 连续 ✓。
	reQuotes = regexp.MustCompile(`[“”‘’]`)
	// reBareDateRow（R10，2026-11-15）：**裸日期行** —— 整串就是 8 位日期（可带「期」字）。
	reBareDateRow = regexp.MustCompile(`^\d{8}期?$`)
)

// dateSeqTail 返回 8 位日期后**紧跟**的「当日集序」（1~2 位数字）；数字串 ≥3 位（不透明长
// 编号，如源站内部 ID）或不存在 ⇒ ""。
// P2（2026-11-12 评审）：202401011/202401012 = 同一期的两集，序号位此前被静默丢弃 ⇒ 两集
// epid 逐字相同（①强等跨源错配、弹幕池合并）。RE2 不支持 lookahead ⇒ 用索引扫描 + 终点
// 字符判断（与 reVariantNum 的消费式断言同一约束）。
func dateSeqTail(s string, dateEnd int) string {
	j := dateEnd
	for j < len(s) && j-dateEnd < 2 && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == dateEnd {
		return ""
	}
	if j < len(s) && s[j] >= '0' && s[j] <= '9' {
		return "" // 数字串 ≥3 位: 不透明长编号, 不拆序号
	}
	return s[dateEnd:j]
}

// reDashDate 破折/点分日期(2026-10-10期 / 2022.05.01期)—— 弹幕库侧综艺标题的主流日期形态。
// 解析前统一归一为 8 位, 使三格式(8位/破折/点分)落到同一个键(2026-11-07 v2, 实测最大单项收益)。
var reDashDate = regexp.MustCompile(`((?:19|20)\d{2})[-.](\d{2})[-.](\d{2})`)

// reDiNWide 期号跨度: 第 + 0~8位数字或中文数字 + 单位字 —— 只用于栏目名清理。
// 比 reDiN(数字限 1~4 位)宽: 「第20260731期」这类日期形态 reDiN 吃不掉,
// 残留的「第/期」单字会污染栏目名(第期先导片 ≠ 先导片, 实测 lz↔uk/md/360/ikun 全 0% 的根因)。
// 2026-11-16：并入**中文数字**（否则「第二期」会被 seriesOf 当成栏目名 ⇒ `s=第二期`）。
var reDiNWide = regexp.MustCompile(`第\s*[\d一二三四五六七八九十百千零〇两]{0,8}\s*[期集话回季部]`)

// reSeasonSpan 季号跨度: 第 + 阿拉伯/中文数字 + 季(王牌对王牌第七季 → 第七季 整段删除,
// 「季」是分季区分位不是栏目名成分; 栏目名在「季」边界截断, 保持连续)。
var reSeasonSpan = regexp.MustCompile(`第\s*[\d一二三四五六七八九十零两]+\s*季`)

// ParseEpisodeKey 解析集名（形态见包注释与文档）。
func ParseEpisodeKey(title string) EpisodeKey {
	var k EpisodeKey
	s := strings.TrimSpace(title)
	if s == "" {
		return k
	}
	// 引号透明（R1，2026-11-15）：先于一切剥离 —— 「“”‘’」视作无字符直接删（不丢内容、不留占位）。
	s = reQuotes.ReplaceAllString(s, "")
	// 画质/字幕/语言修饰词（R17）：先于一切提取剥离——「中字」不剥会被 part 正则吃成 p=中、
	// 被 series 循环吞成 s=中字 ⇒ 同片多源 epid 分裂（生产实证 39,227 部电影零共享）。
	// R8（2026-11-15）：删除带 \x00 占位 —— 无占位会让相邻汉字拼接出假片段（与 seriesOf 口径统一）。
	s = reQualityTail.ReplaceAllString(s, "\x00")
	// 日期格式归一(先于一切提取): 破折/点分 → 8 位
	s = reDashDate.ReplaceAllString(s, "$1$2$3")
	s = reVariantNoise.ReplaceAllString(s, "\x00") // R8：带占位（同上）
	// L1-c（2026-11-19）：会员版/会员加更/会员专享 → v=会员（裸"会员"不动）
	memberVariant := ""
	if m := reMemberVariant.FindString(s); m != "" {
		memberVariant = "会员"
		s = reMemberVariant.ReplaceAllString(s, "\x00")
	}
	var dateTail string
	if loc := reDate8.FindStringIndex(s); loc != nil {
		// R6（2026-11-15）：8 位日期合法性 —— 原式只校验 (19|20) 前缀，「202300707下」被切成
		// d=20230070（月 00/日 70 的伪日期）✗。不合法 ⇒ d 不采用、也不作为 dateSeqTail 基准。
		// 月 1~12、日 ≤ 当月天数（含闰年，与 R11 date6 同规 —— 比规格的「日 1~31」更严一档）。
		if d := s[loc[0]:loc[1]]; validDate8(d) {
			k.Date = d
			dateTail = dateSeqTail(s, loc[1])
		}
	}
	if m := reDiN.FindStringSubmatch(s); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			k.Num = v
		}
	}
	// 中文数字期号（2026-11-16）：阿拉伯数字没取到时再试；`cnNumeral` 不认的（含非数字字符）一律不设
	// ⇒ 与"宁缺勿错"一致（如「第X期」的 X 是别的字 ⇒ 保持不产期号）。
	if k.Num == 0 {
		if m := reDiNCn.FindStringSubmatch(s); m != nil {
			if v, ok := cnNumeral(m[1]); ok && v > 0 {
				k.Num = v
			}
		}
	}
	// M4（2026-11-17 取证批）：整串「第N」（缺单位）/「N[集话回期]」（缺第）⇒ Num —— 见 reDiNAnchor 注释。
	// 放在 中文数字 之后：两者都是 k.Num==0 的兜底形态，互不重叠（阿拉伯 vs 中文数字）。
	if k.Num == 0 {
		if m := reDiNAnchor.FindStringSubmatch(s); m != nil {
			v := m[1]
			if v == "" {
				v = m[2]
			}
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				k.Num = n
			}
		}
	}
	if m := reENum.FindStringSubmatch(s); m != nil {
		// R4：捕获组回填后取数组移（m[1]=左邻居、m[2]=E 号数字、m[3]=右邻居）
		if v, err := strconv.Atoi(m[2]); err == nil {
			k.ENum = v
		}
	}
	// R11（2026-11-14 **服务端口径**）：6 位 YYMMDD 兼容 —— 补齐为 8 位（便于与其它源的 8 位日期直接比对）。
	//   - **8 位优先**（上面已处理）⇒ 这里只在常规日期没取到时尝试；
	//   - ⚠️ **E 圆号类（`E01.220705`/`EP09`）不适用**：口径上 E 圆号整条**不产身份**
	//     （不同源 E 编号常漂 ⇒ 黄金用例 `E01.220705` 明确"不应触发"）⇒ 6 位日期不得把它"救活" ✗。
	//     8 位日期 + E 圆号的既有行为**不动**（那是另一件事）。
	if k.Date == "" && k.ENum == 0 {
		if d, _, end, ok := date6(s); ok {
			k.Date = d
			dateTail = dateSeqTail(s, end)
		}
	}
	k.Part = parsePart(s)
	// P2（2026-11-12 评审）：日期尾随「当日集序」进 p 段 —— 202401011/202401012 = 同一期按
	// 日期再分的两集，序号位此前被静默丢弃 ⇒ 两集 epid 逐字相同（①强等跨源错配、池键合并）。
	// 落 p 的行为：①强等区分两集；②档 p 双有须「相等」（数字序不做前缀相容，见 partCompatible）
	// ⇒ 正确不匹配；单侧没写序号走通配（宁弱勿错）。parsePart 已命中 上/中/下 时**不叠加** ——
	// 复合值（如「上1」）过不了 epk 的 isPartMark 校验。
	if dateTail != "" && k.Part == "" {
		k.Part = dateTail
	}
	// L1-b/L1-c（2026-11-19）：plus 需有日期/期/集语境；会员已在上面识别
	extraVariants := make([]string, 0, 2)
	if memberVariant != "" {
		extraVariants = append(extraVariants, memberVariant)
	}
	if reVariantPlus.MatchString(s) && (k.Date != "" || strings.ContainsRune(s, '期') || strings.ContainsRune(s, '集')) {
		extraVariants = append(extraVariants, "Plus")
	}
	ms := reVariant.FindAllString(s, -1)
	ms = append(ms, extraVariants...)
	if len(ms) > 0 {
		sort.Strings(ms)
		k.Variant = strings.Join(ms, ",")
	}
	if m := reVariantNum.FindStringSubmatch(s); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			k.VariantNum = v
		}
	}
	k.Series = seriesOf(s)
	// B 批（2026-11-17）：把栏目名尾部的**内容版本词**剥出来、并入变体（`version_tail.go`）。
	// 等价变换（`SameEpisode` 里 s 与 v 同为"必须完全相等"）⇒ 判等/触发不变；收益是前缀式/后缀式写法收敛。
	// ⚠️ 必须在**追加季/部标记之前**做（否则 `#S7` 会挡住 HasSuffix）。
	k.Series, k.Variant = splitVersionTail(k.Series, k.Variant)
	// 季/部标记并入 Series（2026-11-16，方案 A）：`王牌对王牌第七季` ⇒ `s=王牌对王牌#S7`。
	// 为什么：季/部是**分季/分部标记** —— 既不该当集序（`reDiN` 已排除 季/部），
	// 若完全不进身份，"同片多季"的 S1E1 与 S2E1 会得到**同一身份**（实测当前 0 碰撞，但机制存在：
	// 多季同片实测 4 部 / 多部同片 34 部）。**归一化**（阿拉伯/中文数字统一成 `#S<n>`）⇒
	// 各源写法差异（`第七季`/`第7季`/`7`）**不会**制造新的伪分歧。
	// ⚠️ 本改动属**预处理族**（Scope=all）⇒ 必须全量 rebuild 才作用于存量。
	k.Series += seasonMarkOf(s)
	return k
}

var (
	// reSeasonMark / rePartMark：`第X季` / `第X部`（阿拉伯或中文数字）——提归一化标记用。
	reSeasonMark = regexp.MustCompile(`第\s*([\d一二三四五六七八九十百千零〇两]{1,6})\s*季`)
	rePartMark   = regexp.MustCompile(`第\s*([\d一二三四五六七八九十百千零〇两]{1,6})\s*部`)
)

// seasonMarkOf 归一化季/部标记：`#S7`（季）/ `#B3`（部），两者都有则按「季→部」拼接；无 ⇒ ""。
func seasonMarkOf(s string) string {
	out := ""
	if m := reSeasonMark.FindStringSubmatch(s); m != nil {
		if v, ok := numberValue(m[1]); ok && v > 0 {
			out += "#S" + strconv.Itoa(v)
		}
	}
	if m := rePartMark.FindStringSubmatch(s); m != nil {
		if v, ok := numberValue(m[1]); ok && v > 0 {
			out += "#B" + strconv.Itoa(v)
		}
	}
	return out
}

// numberValue 阿拉伯数字（Atoi）或中文数字（`cnNumeral`）→ 值。
func numberValue(s string) (int, bool) {
	if v, err := strconv.Atoi(s); err == nil {
		return v, true
	}
	return cnNumeral(s)
}

// parseEpisodeKey = ParseEpisodeKey 的**计数缝**（性能防呆 2026-11-15）：解析热路径
// （decideTitleInCtx / ruleCtx.poolKeysOf / AnchorNumFor 包装 / DerivedNumFor）统一走它，
// 让 `TestDeriveRow_ParsesPoolOnce` 能断言"池只解析一次"；运行时零成本（普通函数变量）。
var parseEpisodeKey = ParseEpisodeKey

// parsePart 解析**主部分（复合）**：第3期中（上）→ `中上`；20260831中上 → `中上`；20260803上 → `上`；正片 → `""`。
// 次部分只在**紧邻主部分**（最多隔一个左括号）时才算，避免把摘要正文当成分支标记。
// P0-1c（2026-11-10）：由「part + sub 两字段」合并为**单个复合值** —— 少一个字段、少一处三端同步面，
// 且让 epid 串自足（`p=中上`），派生表/SQL 可直接按串匹配（见 partCompatible 的前缀相容规则）。
func parsePart(s string) string {
	rs := []rune(s)
	n := len(rs)
	isP := func(r rune) bool { return r == '上' || r == '中' || r == '下' }
	// M3（2026-11-17 取证批）：候选**右邻防粘连** —— 主次字的右邻若是"其它汉字"（非 单位字/主次字/
	// 期号起始「第」/变体词首字）⇒ 该字是复合词成分而非主次标记（全员空**中**飞人/上**海**/中**国**）
	// ⇒ 跳过，继续找下一个候选（「…空中飞人第2期上」的尾部上仍能命中）。无合格候选 ⇒ ""。
	for i := 0; i < n; i++ {
		if !isP(rs[i]) {
			continue
		}
		if i+1 < n && partFollowerSuspicious(rs[i+1]) {
			continue
		}
		pi := i
		out := rs[pi : pi+1]
		j := pi + 1
		if j < n && isLeftBracket(rs[j]) {
			j++
		}
		if j < n && isP(rs[j]) {
			out = append(out, rs[j])
		}
		return string(out)
	}
	return ""
}

// partFollowerSuspicious 主次字右邻的"粘连"判定（M3）：右邻是汉字且不属于下列豁免 ⇒ 粘连（复合词成分）。
// 豁免：① 单位字 期集话回季部（「第3期中」尾部、「XX中集」）② 主次字 上中下（「20260831中上」复合）
// ③ 「第」（期号起始 —— 既有口径「(副本存档中第1期)」中后为第 不受影响）④ 变体词首字
// 加陪纯先超彩特花预番未（「第5期中纯享/中加更」——右邻是变体 ⇒ 主次成立；R7 词表同步）。
func partFollowerSuspicious(r rune) bool {
	if !unicode.Is(unicode.Han, r) {
		return false
	}
	switch r {
	case '期', '集', '话', '回', '季', '部', '篇', '上', '中', '下', '第',
		'加', '陪', '纯', '先', '超', '彩', '特', '花', '预', '番', '未':
		return false
	}
	return true
}

func isLeftBracket(r rune) bool {
	switch r {
	case '（', '(', '【', '[', '《', '「', '『':
		return true
	}
	return false
}

// seriesOf 提取**栏目/系列名**（正片返回 ""）—— 2026-11-07 v2: **连续片段提取，连续性由构造保证**。
//
//	【qq】 第1期上：又争又抢…            → ""（非汉字段/残渣不足 2 字，护栏挡）
//	心动的周末第1期上：高甜游戏…          → 心动的周末
//	20261012心动的周末上                → 心动的周末
//	20261001“聚”乐部                    → 聚乐部（R1：引号透明，不把词切段）
//	20261017舞台纯享版第6期             → ""（R2：变体跨度吸收 舞台/版，v=纯享）
//	20260528(副本存档中第1期)           → 副本存档（R3：括号内容保留；尾部「中」被 part 截断吃掉）
//	20260529(万事屋加更第1期)           → 万事屋（R3+R2）
//	EP01白昼逃亡                        → 白昼逃亡（R4：E 号删除不截断邻居）
//	檀健次带北江小分队回好六了！ 2025-01-04期 → ""（13 字描述 >8，拒识）
//	王牌对王牌第七季2022.05.01期-会员版   → 王牌对王牌（「季」边界截断）
//	20260803上 / 第3期中（上）           → ""
//
// 做法（用户拍板：栏目名必须连续，绝不跨段拼字）：截断到第一个「：」→ 引号透明删除（R1）→ 逐跨度删除
// （括号字符（R3：只占位不删内容）/日期/第N期宽口径/E 号（R4：回填邻居）/变体跨度（R2：吸收修饰）/噪声，
// 删除处以 \x00 占位为界）→ 取**最长纯汉字连续段** →
// 只做两类**边界截断**：尾部 上/中/下（part 标记总在尾部）、首个「季」处（季是分季区分位）
// → 要求 2~8 个中文字（≤8: 真栏目标签实测全部 ≤8；≥9 全是库侧自由描述/片名+残留）。
// 旧实现"只留中文+逐字剔除"会跨段拼出假栏目名（残渣巧合匹配），已废。
// isDigitByte 单字节数字判断（`date6` / `stripDate6` 判边界用；只处理 ASCII 数字）。
func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// daysInMonth 当月天数（含闰年）。
func daysInMonth(year, month int) int {
	if month < 1 || month > 12 {
		return 0
	}
	if month == 2 && (year%4 == 0 && year%100 != 0 || year%400 == 0) {
		return 29
	}
	days := [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	return days[month]
}

// validDate8 8 位日期合法性（R6，2026-11-15）：月 1~12、日不超当月天数（含闰年，与 R11 date6 同规）。
// 「202300707下」型 7 位数字此前被切成 d=20230070（月 00/日 70）——伪日期进身份。
//
// ⚠️ 长度守卫（2026-11-17 审查发现）：原实现**直接切片** `d[0:4]/d[4:6]/d[6:8]` ⇒ 传短串（如 `-`、7 位数字）
// 会 **panic: slice bounds out of range** 打挂调用方 —— 此前安全只因唯一调用方传的是正则切出的 8 位切片。
// 日期对齐口径（`align_date.go`）在**判定边界**要吃"`d` 段原值"，必须有此守卫 ⇒ 补 `len(d) != 8` 提前返回。
// 对既有调用方行为不变（它们传的必是 8 位）；非数字字符仍由 Atoi 失败 ⇒ mm=0 ⇒ 返回 false。
func validDate8(d string) bool {
	if len(d) != 8 {
		return false
	}
	yy, _ := strconv.Atoi(d[0:4])
	mm, _ := strconv.Atoi(d[4:6])
	dd, _ := strconv.Atoi(d[6:8])
	return mm >= 1 && mm <= 12 && dd >= 1 && dd <= daysInMonth(yy, mm)
}

// date6 在 s 里找**符合 YYMMDD 语义**的 6 位日期，返回 8 位规范形与匹配区间（R11，2026-11-14）。
//
// 边界（缺一不可）：① 前后都不接数字（否则是更长数字串的一部分 ✗，如 7/9 位）；
// ② 月 01–12；③ 日不超当月天数（含闰年）；④ 世纪：yy >= 70 ⇒ 19xx，否则 20xx。
func date6(s string) (date string, start, end int, ok bool) {
	for _, loc := range reDate6.FindAllStringIndex(s, -1) {
		if loc[0] > 0 && isDigitByte(s[loc[0]-1]) {
			continue
		}
		if loc[1] < len(s) && isDigitByte(s[loc[1]]) {
			continue
		}
		v := s[loc[0]:loc[1]]
		yy, _ := strconv.Atoi(v[0:2])
		mm, _ := strconv.Atoi(v[2:4])
		dd, _ := strconv.Atoi(v[4:6])
		year := 2000 + yy
		century := "20"
		if yy >= 70 {
			year, century = 1900+yy, "19"
		}
		if dd < 1 || dd > daysInMonth(year, mm) {
			continue
		}
		return century + v, loc[0], loc[1], true
	}
	return "", 0, 0, false
}

// stripDate6 把 s 里**符合语义**的 6 位日期替换为占位符（`seriesOf` 提栏目名用；占位符非数字 ⇒ 不会自我循环）。
func stripDate6(s, sep string) string {
	for {
		_, start, end, ok := date6(s)
		if !ok {
			return s
		}
		s = s[:start] + sep + s[end:]
	}
}

func seriesOf(s string) string {
	if i := strings.IndexAny(s, "：:"); i >= 0 {
		s = s[:i]
	}
	// 引号透明（R1）：ParseEpisodeKey 入口已删，直调 seriesOf 的路径（测试/排查）仍要剥 —— 透明字符不留占位。
	s = reQuotes.ReplaceAllString(s, "")
	// 画质/字幕/语言修饰词（R17）：先剥——「中字」不剥会被 series 循环吞成 s=中字（同片多源 epid 分裂）。
	s = reQualityTail.ReplaceAllString(s, "\x00")
	// 跨度删除以 \x00 占位: 残片之间永不拼接, 片段即原文中的连续子串
	const sep = "\x00"
	// R3（2026-11-15）：括号一律只作分隔 —— 逐字符占位、内容保留，交给护栏过滤
	//（非汉字段 / reSeriesStop / 2~8 字上限）。原式连内容整段删 ⇒ 括号里的栏目名丢失 ✗。
	s = reBracketChar.ReplaceAllString(s, sep)
	// ⚠️ 顺序要紧：先删「第N期/集/话…」再删日期。反过来的话日期会先被替换成占位符，
	// reDiNWide 的 `\s*\d{0,8}\s*` 跨不过占位符 ⇒ 「第20261005期心动的旅行上」这类日期样式期号
	// 会残留一个「期」，栏目名变「期心动的旅行」，与库侧对不上（与 TS 侧同一 bug，2026-11-09 三端同修）
	s = reDiNWide.ReplaceAllString(s, sep)
	// M4 配套（2026-11-17 取证批）：裸「第N」（无单位）从栏目名候选剥掉 ——「第99」得 s=第99 与
	// 「第99集」的 s=- 永不相等（跨源不一致实证；同 Series 会污染同集判定）。
	// 放 reDiNWide 之后：带单位的「第N期」已被宽口径吃掉，这里只兜缺单位的尾部形态。
	s = reDiNBare.ReplaceAllString(s, sep)
	s = reDate8.ReplaceAllString(s, sep)
	s = stripDate6(s, sep) // R11：6 位 YYMMDD 同样剥离（否则 240103 会漏进栏目名 ✗）
	s = reSeasonSpan.ReplaceAllString(s, sep)
	// R4：E 圆号以捕获组回填邻居（$1/$3），只删 E 号本身 —— 不再截断相邻汉字（EP01白昼逃亡 ⇒ s=白昼逃亡）
	s = reENum.ReplaceAllString(s, "$1"+sep+"$3")
	// R2：变体跨度吸收前后修饰（剧情/舞台/收官/特别 ×变体×版/典藏版）—— 替代裸 reVariant 删除
	s = reVariantSpan.ReplaceAllString(s, sep)
	s = reVariantNoise.ReplaceAllString(s, sep)
	best := ""
	for _, frag := range strings.FieldsFunc(s, func(r rune) bool { return r == 0 || !unicode.Is(unicode.Han, r) }) {
		rs := []rune(frag)
		// 尾部 part 标记截断（上/中/下总在栏目名之后）
		// B（2026-11-19）：先剥「X篇/X部/X集」（X ∈ 上/中/下）—— 它们已被 parsePart 取作 p，不应污染 s
		// 实测：s=上篇 33 / 下篇 33 / 中篇 2 / 上集 110 / 下集 106（全库约 1,680 行）
		if n := len(rs); n >= 2 {
			last := rs[n-1]
			if (last == '篇' || last == '部' || last == '集') && (rs[n-2] == '上' || rs[n-2] == '中' || rs[n-2] == '下') {
				rs = rs[:n-2]
			}
		}
		for len(rs) > 0 && (rs[len(rs)-1] == '上' || rs[len(rs)-1] == '中' || rs[len(rs)-1] == '下') {
			rs = rs[:len(rs)-1]
		}
		// 「季」边界截断
		for i, r := range rs {
			if r == '季' {
				rs = rs[:i]
				break
			}
		}
		if len(rs) < 2 || len(rs) > 8 {
			continue
		}
		f := string(rs)
		if reSeriesStop.MatchString(f) {
			continue
		}
		if len(rs) > len([]rune(best)) {
			best = f
		}
	}
	return best
}

// partCompatible 主部分是否指向同一支 —— **前缀相容**（P0-1c 2026-11-10，替代旧「part 严格 + sub 通配」两段式）：
//   - 一方为空 ⇒ 通配（App 常只有「中」而库里是「中（上）/（下）」）；
//   - 短的是长的前缀 ⇒ 相容：`中` ⊂ `中上`/`中下`（保留上面的宽容）；
//   - 互不为前缀 ⇒ 不相容：`中上` ≠ `中下`（**区分仍在**，不会把上下半集混为一集）。
//
// 与旧实现（part 相等 + sub 双方非空才比较）**逐例等价**；收益是「一条规则同时表达宽容与区分」，
// 且不再需要"两处必须一起用"的隐式契约（旧实现里 epid 串丢 sub，①档必须额外叠加 SameEpisode）。
func partCompatible(a, b EpisodeKey) bool {
	if a.Part == "" || b.Part == "" {
		return true
	}
	// P2（2026-11-12）：数字序位（日期尾随「当日集序」落 p 段）**不做前缀相容** —— 前缀规则是
	// 为「中」⊂「中上」设计的，"1" 与 "12" 是不同集。任一侧为纯数字 ⇒ 要求严格相等。
	if isAllDigitPart(a.Part) || isAllDigitPart(b.Part) {
		return a.Part == b.Part
	}
	return strings.HasPrefix(a.Part, b.Part) || strings.HasPrefix(b.Part, a.Part)
}

// isAllDigitPart p 段是否为纯数字（P2 的「当日集序」形态，见 dateSeqTail）。
func isAllDigitPart(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// SameEpisode 两个键是否指向同一集。顺序即优先级：
//  1. **变体（含变体序号）与栏目名必须完全相等**（含"都为空"；一方有一方没有 → 不同集）；
//  2. 双方都有日期 → 相等 → 同一集（再校验主/次部分）；**不等 → 不同集（硬证据，即使期号相等）**；
//  3. 双方都有期号/集号且相等 → 同一集（同样校验主/次部分）；
//  4. 双方都有 E 圆号且相等 → 同一集；
//  5. 其余 → 不同集。
//
// 2026-11-06 修订：① 规则 2/3 补主/次部分校验；② 新增栏目名（Series）防衍生栏目串集。
// 2026-11-07 修订（SameEpisode′ 落地）：**日期双有不等 → 否决**——旧逻辑允许"期号相等而日期不同"
// 判同（现实里那是重播/错标），宁缺勿错。
// 2026-11-10 修订（P0-1a）：**变体序号纳入身份** —— 「特辑2」≠「特辑」；两者皆无日期/期号时
// 仍判不同（属"无特征键"，交由 L3a 组内唯一兜底）。
func SameEpisode(a, b EpisodeKey) bool {
	if a.Variant != b.Variant || a.VariantNum != b.VariantNum || a.Series != b.Series {
		return false
	}
	if a.Date != "" && b.Date != "" {
		if a.Date != b.Date {
			return false
		}
		return partCompatible(a, b)
	}
	if a.Num > 0 && b.Num > 0 && a.Num == b.Num {
		return partCompatible(a, b)
	}
	if a.ENum > 0 && b.ENum > 0 && a.ENum == b.ENum {
		return true
	}
	return false
}
