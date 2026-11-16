package entity

// MovieEpidOverride epid 源头校正**裁定台账**（epid 源头校正层 · 第②段，迁移 000049，2026-11-12）。
//
// 只放"已经判过"的结论（待定项留在离线清单，不入库）—— 口径见
// docs/epid-集身份/方案-epid源头校正层-2026-11-12.md §3.3 数据模型 / §4 状态机：
//   - 派生大表 movie_episode_epid **零新增列**：行内只放可重算状态，裁定放本台账 ⇒
//     增量 DELETE+INSERT 与 TRUNCATE 全量重建两条路径都一行不用改，校正永不丢（§8.3）；
//   - 应用前做**三元组校验** (src_row_id, key_raw_name, key_epid)：全对得上才套用，
//     对不上 ⇒ 标 stale 重回清单（**宁缺勿错**，§5 不变式）；
//   - uk_row_ep(src_row_id, ep_index)：同一源行同一集只允许一条裁定。
//
// ⚠️ 红线③：台账字段与 evidence **不进任何出参 DTO** —— 本结构仅供 repo/service 内部流转，
// 管理端出参必须在 dto 层显式映射。Evidence 已用 `json:"-"` 兜底防整包误序列化。
type MovieEpidOverride struct {
	Id            int64   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SrcRowId      int64   `gorm:"column:src_row_id" json:"srcRowId"`
	EpIndex       int     `gorm:"column:ep_index" json:"epIndex"`
	MatchKey      string  `gorm:"column:match_key" json:"matchKey"`
	Mid           int64   `gorm:"column:mid" json:"mid"`
	KeyRawName    string  `gorm:"column:key_raw_name" json:"keyRawName"`
	KeyEpid       string  `gorm:"column:key_epid" json:"keyEpid"`
	Verdict       string  `gorm:"column:verdict" json:"verdict"`
	NewEpid       string  `gorm:"column:new_epid" json:"newEpid"`
	Provenance    string  `gorm:"column:provenance" json:"provenance"`
	Confidence    string  `gorm:"column:confidence" json:"confidence"`
	Reason        string  `gorm:"column:reason" json:"reason"`
	Evidence      JSONMap `gorm:"column:evidence" json:"-"`
	Status        string  `gorm:"column:status" json:"status"`
	RuleVersionAt string  `gorm:"column:rule_version_at" json:"ruleVersionAt"`
	CreatedAt     int64   `gorm:"column:created_at" json:"createdAt"`
	VerifiedAt    int64   `gorm:"column:verified_at" json:"verifiedAt"`
	VerifiedBy    string  `gorm:"column:verified_by" json:"verifiedBy"`
}

func (MovieEpidOverride) TableName() string { return "movie_epid_override" }

// 台账状态机（方案 §4）：pending → active；旁支 stale（源/规则版本变，重回清单复核）/
// obsolete（规则已能算对，自动退休）/ revoked（人工撤销）/ orphan（源行被删，留审计不自动删）。
const (
	OverrideStatusPending  = "pending"
	OverrideStatusActive   = "active"
	OverrideStatusStale    = "stale"
	OverrideStatusObsolete = "obsolete"
	OverrideStatusRevoked  = "revoked"
	OverrideStatusOrphan   = "orphan"
)

// 台账列的合法枚举值（方案 §3.3）—— repo 的 import 校验与后续管理端入口共用同一口径。
const (
	OverrideVerdictCorrected = "corrected" // 要改（new_epid 为目标值）
	OverrideVerdictKept      = "kept"      // 判定不必改（"闭嘴"，值不变）
)

var overrideVerdicts = map[string]struct{}{
	OverrideVerdictCorrected: {},
	OverrideVerdictKept:      {},
}

var overrideProvenances = map[string]struct{}{
	"rule":     {}, // 规则层给出
	"external": {}, // 外部资料（TMDB/百科等）
	"agent":    {}, // agent 核对
	"human":    {}, // 人工直判
}

var overrideConfidences = map[string]struct{}{
	"inferred": {}, // 推断
	"verified": {}, // 已核实（外部链接/抓取佐证）
}

// ValidOverrideVerdict / ValidOverrideProvenance / ValidOverrideConfidence 枚举合法性
// （import 校验用；返回 false 的值一律拒收）。
func ValidOverrideVerdict(v string) bool    { _, ok := overrideVerdicts[v]; return ok }
func ValidOverrideProvenance(v string) bool { _, ok := overrideProvenances[v]; return ok }
func ValidOverrideConfidence(v string) bool { _, ok := overrideConfidences[v]; return ok }
