package handler

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"server/internal/dto"
	"server/internal/service"
)

// ManageEpidDeriveAll POST /manage/epid/derive-all —— 一键派生（H2 运维入口, 2026-11-11；2026-11-14 加 scope）。
//
// 参数（均可省略）：
//
//	mode=fill（默认，**补缺/更新**，不动已有派生）| rebuild（**先清空派生表**再全量重算，规则升级后用）
//	     | scope（**定向重算**：只重算"某个 `from_src` 档"的源行，不动其它行 —— 见下）
//	batch       每批源行数（默认 500，上限 5000）
//	maxBatches  批数上限（0=不限，默认 0；仅 fill/rebuild 用）
//	timeoutMin  总时长上限（分钟，默认 60，上限 240）
//
// **mode=scope**（2026-11-14，给"改解析规则 ⇒ 只重算受影响的行"用）：
//
//	rule=D7-single        按**规则编号**（推荐）：范围 = 登记表里的 `InFromSrc` + `FansOutMovie`；
//	                      改的是 `Scope=all` 的规则 ⇒ 直接 400（那种改法只能全量 rebuild）
//	fromSrc=skip,bridge   按**档**显式给（底层写法；应来自登记表，别手挑）
//	expandMovies=true     池扇出：∪ 这些行所在片里**池相关档**（bridge/anchor/none）的源行
//	                      （`rule=` 时由登记的 `FansOutMovie` 决定，无需手写）
//	tighten=true          用规则自己的选行谓词把档内不需要的排掉（只省时间；非池依赖规则才精确）
//
// 行为：**异步**启动（立即返回，不占 HTTP 连接）；与"采集收尾的自动触发"共用同一把锁（**互斥防重入**）。
// 进度看 `GET /manage/epid/report`（`mode` 显示 `scope:<rule|档>`）与后端日志（每批一行 `[epid]`）。
func (h *Handlers) ManageEpidDeriveAll(c *gin.Context) {
	mode := service.DeriveMode(c.DefaultQuery("mode", string(service.DeriveModeFill)))
	if mode != service.DeriveModeFill && mode != service.DeriveModeRebuild && mode != service.DeriveModeScope {
		dto.Error(c, http.StatusBadRequest, "mode must be fill|rebuild|scope")
		return
	}
	// 默认 500（2026-11-11 需求 2）：批小 ⇒ 单批更短、与采集的锁重叠窗口更小、对线上读的抖动更平滑；
	// 代价是总批数变多、总时长变长（可分次跑，续跑用 mode=fill）。
	batch := queryInt(c, "batch", 500)
	if batch <= 0 || batch > 5000 {
		batch = 500
	}
	maxBatches := queryInt(c, "maxBatches", 0)
	if maxBatches < 0 {
		maxBatches = 0
	}
	timeoutMin := queryInt(c, "timeoutMin", 60)
	if timeoutMin <= 0 || timeoutMin > 240 {
		timeoutMin = 60
	}
	timeout := time.Duration(timeoutMin) * time.Minute

	if mode == service.DeriveModeScope {
		req := service.ScopeRequest{
			Rule:         c.Query("rule"),
			FromSrc:      epidScopeCSV(c.Query("fromSrc")),
			ExpandMovies: c.Query("expandMovies") == "1" || c.Query("expandMovies") == "true",
			Tighten:      c.Query("tighten") == "1" || c.Query("tighten") == "true",
		}
		label, err := service.RunScopedDerive(req, batch, timeout)
		if err != nil {
			// 锁被占 ⇒ 409（与 fill/rebuild 同款语义）；其余是入参非法 ⇒ 400。
			if errors.Is(err, service.ErrDeriveBusy) {
				dto.Error(c, http.StatusConflict, err.Error())
				return
			}
			dto.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		dto.OK(c, gin.H{
			"started": true, "mode": "scope", "scope": label,
			"batch": batch, "timeoutMin": timeoutMin,
		})
		return
	}

	if !service.RunFullDerive(mode, batch, maxBatches, timeout) {
		dto.Error(c, http.StatusConflict, "已有派生任务在跑（或未装配）")
		return
	}
	dto.OK(c, gin.H{
		"started": true, "mode": string(mode), "batch": batch,
		"maxBatches": maxBatches, "timeoutMin": timeoutMin,
	})
}

// epidScopeCSV 解析 `mode=scope` 的 `fromSrc` 逗号列表（去空白、去空项）。
func epidScopeCSV(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0, 4)
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ManageEpidReport GET /manage/epid/report —— 派生**验收指标**（只读）。
//
// 返回：任务状态（在跑/模式/游标/阶段）+ 进度（派生行/已派生源行）+ 对齐率（§4.8 门槛 ≥85%）
// + 完整覆盖率（§6.1）+ 来源分布 + 分层有身份率 + 成熟度 + 空 epid 形态。
//
// 参数：`?full=1` 才计算覆盖率（要扫 `Σ JSON_LENGTH` 巨列 ≈13s）；
// **重建进行中**时只返回进度类指标（详见 `service.ReportEpid`）。
func (h *Handlers) ManageEpidReport(c *gin.Context) {
	rep, err := service.ReportEpid(c.Request.Context(), c.Query("full") == "1")
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, rep)
}

// ManageEpidAlignmentTolerant GET /manage/epid/alignment/tolerant —— 容忍口径（M4，2026-11-16）按需计算。
//
// 为什么单独一个接口、而不并进 `/epid/report`：容忍判定要多取"每源身份序列"再在 Go 里判
// （骨架取最长源 + 仅插删 + 主源必备相容），比严格口径慢，且它是**体检指标**（不随进度变化）——
// 用户 2026-11-16 定："算法时间长，离线算即可" ⇒ 不进 600s 快照、只按需触发。
//
// 口径提示：`totalMks` 只统计"≥2 个源"的片（**单源不背书**），故与严格口径的片级**不可直接相减**；
// 同一分母下的严格读数在同响应的 `strictAligned` 里（便于直接对比"容忍能多收编多少片"）。
func (h *Handlers) ManageEpidAlignmentTolerant(c *gin.Context) {
	rep, err := service.ReportAlignmentTolerant(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, rep)
}

// ManageEpidAlignmentDate GET /manage/epid/alignment/date —— 综艺「日期对齐」口径（S，2026-11-17）按需计算。
//
// 与容忍口径（M4）同构：只换"身份取什么"——M4 吃 epid 值序列、按位置对齐；
// 本口径吃 `(日期, part)` 复合键、按值对齐 ⇒ 跨源位序错位不再产出伪分歧。
//
// 范围：只统计「≥2 个纯日期型源（该源全部行 s/v/n 空、d 非空）且通过守卫」的片。
// 守卫：每源 `(日期, part)` 复合键唯一 —— **同日多行但 part 不同是合法的**（一天两集）；
// 只有同一 (日期, part) 重复才整组放弃，放弃数在 `skippedMks` 里。
//
// ⚠️ 按需现算（不进 600s 快照、不进请求路径）；`totalMks` 与严格口径的片级**不可直接相减**，
// 同分母对照读 `strictAligned`。**读侧（弹幕聚合/挑集）不消费本口径。**
func (h *Handlers) ManageEpidAlignmentDate(c *gin.Context) {
	rep, err := service.ReportAlignmentDate(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, rep)
}

// ManageEpidAlignmentFunnel GET /manage/epid/alignment/funnel —— 对齐率分层漏斗（L0~L5，2026-11-17）按需计算。
//
// 总则：docs/epid-集身份/方案-对齐率分层判定总则-2026-11-17.md
// 只做判定、不改 epid；每层只吃上一层未判齐的剩余，分母恒为全部非空槽位：
//
//	L0 单源槽（默认对齐） → L1 槽位全等 → L2 值序列容忍（仅插删）
//	→ L3 单日期锚（s/v 空 ∧ n 空）→ L4 单期号锚（s/v 空 ∧ d 空）
//	→ L5 多身份行（本版单列不判）→ 剩余（真未对齐，进归因）
//
// ⚠️ 按需现算（不进 600s 快照、不进请求路径）；各层计数可单独回放。
func (h *Handlers) ManageEpidAlignmentFunnel(c *gin.Context) {
	rep, err := service.ReportAlignmentFunnel(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, rep)
}

// ===== epid 源头校正层 · T4 管理端入口（2026-11-12）=====
//
// 裁定台账 movie_epid_override（迁移 000049）的管理面（全部挂 mg 组：Bearer + RequireAdmin +
// NoStore ⇒ 未登录 401，见 router.go）：
//
//	POST /manage/epid/overrides/import     导入候选行 → pending（T2 UpsertPending；不受生效开关影响）
//	POST /manage/epid/overrides/apply      B 快速应用（可传 targets，缺省=全部 active；受 D3 开关把守）
//	POST /manage/epid/overrides/recompute  C 定向重算（受 D3 开关把守；只重算 targets 里的 src_row_id）
//	GET  /manage/epid/overrides            列表 + 三维统计 + 开关状态（不受生效开关影响）
//	PUT  /manage/epid/overrides/enabled    D3 生效开关（默认关；重启回默认关 = fail-safe）
//
// 红线③：台账行出参一律 service.OverrideView 白名单映射（evidence / rule_version_at / 审计列不外泄）；
// apply/recompute/import 响应均为纯计数结构（不携带任何台账字段）。

// ManageEpidOverrideImport POST /manage/epid/overrides/import —— 导入候选行 → pending。
//
// 请求体 = JSON 数组，逐项字段（T5 生成候选文件严格按此格式；详见 service.OverrideImportInput）：
//
//	[{"srcRowId":int64, "epIndex":int, "keyRawName":string, "keyEpid":string,
//	  "verdict":"corrected"|"kept", "newEpid":string(corrected 必填),
//	  "provenance":"rule"|"external"|"agent"|"human", "confidence":"inferred"|"verified",
//	  "reason":string(可选), "evidence":object(可选), "ruleVersionAt":string(可选)}]
//
// status/id/matchKey/mid/createdAt 由 repo 强制/回填（请求体传了无效）；逐行校验/批内去重/
// 唯一键冲突/key_epid 对账在 repo UpsertPending。响应 = 纯计数：
//
//	{"inserted":int, "deduped":int, "rejected":[{"srcRowId":int64,"epIndex":int,"reason":string}]}
//
// 空数组 / 部分拒收都是 200（业务结果在字段里，不承载在 HTTP 状态码）。
func (h *Handlers) ManageEpidOverrideImport(c *gin.Context) {
	var items []service.OverrideImportInput
	if err := c.ShouldBindJSON(&items); err != nil {
		dto.Error(c, http.StatusBadRequest, "body 必须是候选行 JSON 数组（service.OverrideImportInput）: "+err.Error())
		return
	}
	res, err := service.ImportOverrides(c.Request.Context(), items)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	rejected := make([]gin.H, 0, len(res.Rejected)) // 保证序列化成 [] 而非 null（口径同 report 的 null 兜底）
	for _, r := range res.Rejected {
		rejected = append(rejected, gin.H{"srcRowId": r.SrcRowId, "epIndex": r.EpIndex, "reason": r.Reason})
	}
	dto.OK(c, gin.H{"inserted": res.Inserted, "deduped": res.Deduped, "rejected": rejected})
}

// ManageEpidOverrideApply POST /manage/epid/overrides/apply —— B 快速应用（方案 §8.2）。
//
// 请求体（可省略）：{"targets":[{"srcRowId":int64,"epIndex":int}]}；省略/空 ⇒ 应用**全部 active**
// （§5「清空重建收尾」/ T4「一键应用」共用语义）。响应直接展平 service.OverrideApplyResult：
//
//	{"considered":int,"applied":int,"inEffect":int,"kept":int,"stale":int,"missing":int}
//
// D3 生效开关（默认关）关着 ⇒ 403 拒绝（service 层同样把守，fail-closed 双保险）。
func (h *Handlers) ManageEpidOverrideApply(c *gin.Context) {
	if !service.EpidOverrideEnabled() {
		dto.Error(c, http.StatusForbidden, "epid 源头校正生效开关未开启（D3 默认关；先录入核对，PUT /manage/epid/overrides/enabled 打开后再应用）")
		return
	}
	var req struct {
		Targets []service.OverrideTarget `json:"targets"`
	}
	// body 可整体省略（缺省=全部 active）：EOF（空 body）不是错误
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		dto.Error(c, http.StatusBadRequest, "targets 解析失败（应为 {\"targets\":[{\"srcRowId\":int64,\"epIndex\":int}]} 或省略）: "+err.Error())
		return
	}
	res, err := service.ApplyOverrides(c.Request.Context(), req.Targets)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, res)
}

// ManageEpidOverrideRecompute POST /manage/epid/overrides/recompute —— C 定向重算（方案 §8.2）。
//
// 请求体：{"targets":[{"srcRowId":int64,"epIndex":int}]}（按 src_row_id 去重后走完整派生链路，
// force=true 绕过抖动守卫，只 ReplaceForSourceRows 受影响源行，不跑全量）。响应展平 DeriveStats：
//
//	{"scanned","rebuilt","rows","skipped","skipWhy","emptyEpid","lastId",
//	 "overrideApplied","overrideKept","overrideStale"}
//
// D3 生效开关关着 ⇒ 403：C 会整源行重写派生行，开关关着 ⇒ A 点缺席 ⇒ 现有校正在重算的行上
// 会被规则值冲掉 —— 必须先打开开关。
func (h *Handlers) ManageEpidOverrideRecompute(c *gin.Context) {
	if !service.EpidOverrideEnabled() {
		dto.Error(c, http.StatusForbidden, "epid 源头校正生效开关未开启（D3 默认关）：定向重算会整源行重写，须先打开开关让 A 点套用裁定")
		return
	}
	var req struct {
		Targets []service.OverrideTarget `json:"targets"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		dto.Error(c, http.StatusBadRequest, "targets 解析失败（应为 {\"targets\":[{\"srcRowId\":int64,\"epIndex\":int}]}）: "+err.Error())
		return
	}
	st, err := service.RecomputeForOverrides(c.Request.Context(), req.Targets)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, st)
}

// ManageEpidOverrideList GET /manage/epid/overrides —— 台账列表 + 三维统计 + 开关状态（只读）。
//
// 响应：{"enabled":bool, "list":[service.OverrideView...], "stats":{"byVerdict"/"byProvenance"/
// "byStatus":[{"name","n"}]}}。`?status=` 可选过滤（内存过滤：台账小表整读，12 条量级）。
// 不受 D3 生效开关影响（查看先行：录入 → 核对 → 打开开关 → apply）。
func (h *Handlers) ManageEpidOverrideList(c *gin.Context) {
	rows, stats, err := service.ListOverrides(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	status := c.Query("status")
	list := make([]service.OverrideView, 0, len(rows))
	for _, r := range rows {
		if status != "" && r.Status != status {
			continue
		}
		list = append(list, r)
	}
	dto.OK(c, gin.H{"enabled": service.EpidOverrideEnabled(), "list": list, "stats": stats})
}

// ManageEpidOverrideSetEnabled PUT /manage/epid/overrides/enabled —— D3 生效开关（默认关）。
//
// 请求体：{"enabled":true|false}。打开后 A 点（派生写库前）即会套用 active 裁定、
// apply/recompute 接口放行；import 与 GET 不受影响。
// ⚠️ 内存态（对齐仓内运行时开关模式，deriveHotPhase 同款）：重启回默认关 = fail-safe ——
// 已落库的校正值不动、A 停止套用不会写错值；重新打开后 B 一键重套即可。
func (h *Handlers) ManageEpidOverrideSetEnabled(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"` // 指针：区分"没传"与 false（没传 ⇒ 400）
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		dto.Error(c, http.StatusBadRequest, `body 必须是 {"enabled":true|false}`)
		return
	}
	service.SetEpidOverrideEnabled(*req.Enabled)
	log.Printf("[epid] 生效开关（D3）已由管理端置为 %v", *req.Enabled)
	dto.OK(c, gin.H{"enabled": *req.Enabled})
}
