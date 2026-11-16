package danmu

import (
	"strconv"
	"strings"
)

// ===== epid 串 → EpisodeKey 解析（2026-11-16，"弹幕挑集改用 epid"配套）=====
//
// 背景：弹幕聚合要"在**弹幕库**的分集列表里挑对那一集"，原来靠端侧上传的
// `epk`（端侧解析结果）/`epTitle`（集名）/`epNum`（端侧现算期号）三件套。
// 方案 α（`方案-端侧epid判等简化`）：端侧改传**该集的服务端 `epid`**（`episodes[].epid`），
// 服务端直接用它当挑集特征（`epid` 的 s/v/p/n/d **信息更全**），`epTitle` 作**回退**。
//
// ⚠️ `epid` 与 `epk` 是**两种格式**，不能混用解析器：
//
//	epid：`s=<栏目>|v=<变体[#序号]>|p=<主次>|n=<期号>|d=<日期>`（缺值 `-`）
//	epk ：`v1|d=..|n=..|e=..|p=..|v=..|vn=..|s=..|o=..|from=..`（`ParseEpisodeKeyString`）
//
// ParseEpidString 宽松解析：段序固定（s/v/p/n/d），缺段/未知段忽略；`-` 视为空；
// `v` 段里的 `#<数字>` 拆成 VariantNum；全部段皆空 ⇒ 返回 false（"不构成身份"，与 F6 同规）。
func ParseEpidString(epid string) (EpisodeKey, bool) {
	var k EpisodeKey
	s := strings.TrimSpace(epid)
	if s == "" {
		return k, false
	}
	for _, seg := range strings.Split(s, "|") {
		i := strings.Index(seg, "=")
		if i <= 0 {
			continue
		}
		name, val := seg[:i], seg[i+1:]
		if val == "-" || val == "" {
			continue
		}
		switch name {
		case "s":
			k.Series = val
		case "v":
			// `特辑#2` ⇒ (Variant=特辑, VariantNum=2)；`#` 后非数字/≤0 ⇒ 整段当变体词
			if j := strings.Index(val, "#"); j > 0 {
				if n, err := strconv.Atoi(val[j+1:]); err == nil && n > 0 {
					k.Variant, k.VariantNum = val[:j], n
					continue
				}
			}
			k.Variant = val
		case "p":
			k.Part = val
		case "n":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				k.Num = n
			}
		case "d":
			k.Date = val
		}
	}
	if k.IsZero() {
		return k, false // 五段全空 ⇒ 不构成身份（宁缺勿错）
	}
	return k, true
}
