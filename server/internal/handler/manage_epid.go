package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"server/internal/dto"
	"server/internal/service"
)

// ManageEpidDeriveAll POST /manage/epid/derive-all —— 一键全量派生（H2 运维入口, 2026-11-11）。
//
// 参数（均可省略）：
//
//	mode=fill（默认，**补缺/更新**，不动已有派生）| rebuild（**先清空派生表**再全量重算，规则升级后用）
//	batch       每批源行数（默认 2000，上限 5000）
//	maxBatches  批数上限（0=不限，默认 0）
//	timeoutMin  总时长上限（分钟，默认 60，上限 240）
//
// 行为：**异步**启动（立即返回，不占 HTTP 连接）；与"采集收尾的自动触发"共用同一把锁（**互斥防重入**）。
// 进度看 `GET /manage/epid/report`（游标/阶段/指标）与后端日志（每批一行 `[epid]` 汇总）。
func (h *Handlers) ManageEpidDeriveAll(c *gin.Context) {
	mode := service.DeriveMode(c.DefaultQuery("mode", string(service.DeriveModeFill)))
	if mode != service.DeriveModeFill && mode != service.DeriveModeRebuild {
		dto.Error(c, http.StatusBadRequest, "mode must be fill|rebuild")
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
	if !service.RunFullDerive(mode, batch, maxBatches, time.Duration(timeoutMin)*time.Minute) {
		dto.Error(c, http.StatusConflict, "已有派生任务在跑（或未装配）")
		return
	}
	dto.OK(c, gin.H{
		"started": true, "mode": string(mode), "batch": batch,
		"maxBatches": maxBatches, "timeoutMin": timeoutMin,
	})
}

// ManageEpidReport GET /manage/epid/report —— 派生**验收指标**（只读）。
//
// 返回：任务状态（在跑/模式/游标/阶段）+ 对齐率（§4.8 门槛 85%）+ 完整覆盖率（§6.1 口径）
// + 来源分布（self/bridge/anchor/none）+ 派生行数/空 epid 行数。
func (h *Handlers) ManageEpidReport(c *gin.Context) {
	rep, err := service.ReportEpid(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, rep)
}
