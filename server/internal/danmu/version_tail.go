package danmu

import (
	"sort"
	"strings"
)

// ===== B 批（2026-11-17）：内容版本词「XX版」从栏目名剥离、并入变体 =====
//
// 依据（真库实测）：`s` 段以「版」结尾的取值 **125 个 / 3,069 行**；其中**内容版本类**（加长/尊享/大神/沉浸/精华/抢先…）
// 语义是"同一集的另一份内容" ⇒ 当作变体；而 `SameEpisode` 里 `s` 与 `v` **判等强度相同**（都必须完全相等）
// ⇒ **挪段是等价变换**（判等/触发不变，`v` 非空仍触发），收益是**前缀式/后缀式两种写法能收敛**
// （实测真正收敛面上限 = **33 mk**；写法一致的组是等价变换、写法不一致的 429 mk 属上游内容差异，收编救不了）。
//
// ⚠️ 三条实现纪律：
//  1. **从长到短**匹配：`init()` 按 rune 长度**降序**排序（防御性，不依赖上面列表的书写顺序）——
//     否则「尊享版」会抢在「超长尊享版」前面命中 ⇒ `超长尊享版` 被切成残片 `超长尊` ✗。
//  2. **保留「版」字**进变体（`v=加长版/尊享版/大神版/中国版`）：语义无歧义，且与 R17
//     （语言/画质「XX版」**删除**）清晰分开，避免"删"与"变体"两种处理在同一段里打架。
//  3. **排他清单（不在本表）**：语言/画质类（中字版/高清版/粤语版/普通话版… → R17 **删**）、
//     `整合版`（→ R19 打包词 P3-pack）；
//     `抢版/更版/谈版/享版/出版`（**源站原文残片**：实测 raw_name 与 s 值逐字一致，
//     如 `抢版`/`更版第24期`/`20260504谈版第1期`——**不是任何规则切出来的**）⇒ 不收：
//     它们与"抢先版/更新版"多属不同片/不同站，收编零收益，反有"把残片当身份"之嫌。
//  4. **`电影版/剧场版/访谈版/电视版` 已纳入**（2026-11-17 复用用户结论）：它们虽是"独立作品形态/栏目"，
//     但 `s`→`v` 是**等价变换**（判等/触发不变）⇒ 无功能影响，纳入可让"X电影版"这类**前缀式/后缀式**收敛，扩面零风险。
var versionTailWords = []string{
	// 6 字级（复合版本词，必须整体进 v）
	"期超长尊享版", "大放送加料版",
	// 5 字级
	"超长尊享版", "精编速看版", "春游精编版", "母带直出版", "下饭绝配版", "主题精编版", "加时营业版",
	"笑点浓缩版", "导演剪辑版", "初见面精编版", "练歌房加练版", "聚会上大神版", "聚会下大神版",
	"剧情加长版", "此生加长版", "初舞台精华版", "加码臻享版", "大结局沉浸版", "主题特映版", "夏日特别版",
	"官方制作版", "付费体验版", "入坑必刷版", "偷偷加料版", "加料放送版", "爆笑加料版", "轻体量版", "更尝鲜版",
	// 4 字级
	"尊享版", "加长版", "抢先版", "精编版", "加时版", "沉浸版", "加料版", "精华版", "展演版", "轻量版",
	"唱享版", "抢鲜版", "悠享版", "特别版", "衍生版", "升级版", "宿舍版", "独宠版", "烧脑版", "令和版",
	"收藏版", "推理版", "加练版", "正式版", "尝鲜版", "录制版", "体验版", "先行版", "重置版", "联动版",
	"侦藏版", "导剪版", "剪辑版", "精修版", "典藏版", "大神版", "独享版", "练歌房版", "大电影版", "电视台版",
	// 作品形态/栏目类（2026-11-17 复用用户结论纳入：等价变换、零功能影响，见文件头纪律 4）
	"电影版", "剧场版", "访谈版", "电视版",
	// 3 字级
	"中国版",
}

func init() {
	// 长度降序（同长度保持稳定）⇒ 「超长尊享版」必先于「尊享版」命中
	sort.SliceStable(versionTailWords, func(i, j int) bool {
		return len([]rune(versionTailWords[i])) > len([]rune(versionTailWords[j]))
	})
}

// splitVersionTail 把 `series` 尾部的**内容版本词**逐个剥出、并入 `variant`（多值 `,` 连并按字典序排序，
// 与既有 `reVariant` 的"词序无关"口径一致）。整值就是版本词 ⇒ `series` 归空（角色由 `v` 承接）。
//
// 幂等且不影响非版本词：未命中原样返回。
func splitVersionTail(series, variant string) (string, string) {
	if series == "" {
		return series, variant
	}
	hit := false
	for _, w := range versionTailWords {
		if strings.HasSuffix(series, w) {
			series = strings.TrimSuffix(series, w)
			variant = joinVariantParts(variant, w)
			hit = true
		}
	}
	_ = hit
	return series, variant
}

// joinVariantParts 把新词并入现有变体串：去重 + 字典序排序（与 `reVariant` 的 `sort.Strings` 同口径）。
func joinVariantParts(variant, w string) string {
	if w == "" {
		return variant
	}
	parts := make([]string, 0, 4)
	if variant != "" {
		parts = append(parts, strings.Split(variant, ",")...)
	}
	parts = append(parts, w)
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
