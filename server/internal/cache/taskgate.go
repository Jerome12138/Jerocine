package cache

// 任务静默窗口（维护窗口闸门）· 2026-11-26
//
// 用途：部署 / 迁移 / 备份 / 重建派生表这类"需要一个安静窗口"的动作，由外部 agent 在
// 动作前调 POST /manage/task-center/pause 打开闸门，动作结束后调 resume。闸门管三件事：
//   · 窗口内**不再触发新任务**（调度器自动触发 + 手动"立即执行"一并拒绝）；
//   · 此刻正在跑的采集任务协作暂停（停在页边界，不中断事务、不浪费已完成的进度）；
//   · resume 只恢复"因闸门而暂停"的任务，**绝不误恢复用户手工暂停的**（见 KeyTaskGateJobs）。
//
// 为什么必须放 Redis（而不是进程内存）：
//  ① 多副本要一起闭嘴；
//  ② 容器重建后进程内存就没了，闸门必须还在 —— 否则 agent 的 resume 无所指；
//  ③ **必须带 TTL 自愈**：agent 崩了/忘了 resume 也不能把任务永久停住。

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// KeyTaskGate 闸门本体（String=打开者描述，TTL=窗口剩余时间）。
	KeyTaskGate = "v1:taskctrl:gate"
	// KeyTaskGateJobs 因闸门而暂停的采集 job 集合（resume 的恢复白名单）。
	KeyTaskGateJobs = "v1:taskctrl:gate:jobs"
	// gateJobsKeep 白名单兜底过期：闸门最长 6h，留 24h 足够 resume 取用。
	gateJobsKeep = 24 * time.Hour
)

// 闸门时长（分钟）：<=0 用默认；超上限截断（防"永久停摆"）。
const (
	GateTTLDefaultMin = 60
	GateTTLMaxMin     = 360
)

// GateTTL 归一化闸门时长。
func GateTTL(minutes int) time.Duration {
	if minutes <= 0 {
		minutes = GateTTLDefaultMin
	}
	if minutes > GateTTLMaxMin {
		minutes = GateTTLMaxMin
	}
	return time.Duration(minutes) * time.Minute
}

// TaskGateOpen 打开闸门（ttl 到点自动关闭）。
func TaskGateOpen(ctx context.Context, by string, ttl time.Duration) error {
	if coordRdb == nil {
		return ErrCoordUnavailable
	}
	if by == "" {
		by = "unknown"
	}
	if ttl <= 0 {
		ttl = GateTTL(0)
	}
	return coordRdb.Set(ctx, KeyTaskGate, by, ttl).Err()
}

// TaskGateInfo 读闸门：ok=false 表示未打开；until = 自动关闭时刻（TTL 归一化后的真实值）。
func TaskGateInfo(ctx context.Context) (by string, until time.Time, ok bool, err error) {
	if coordRdb == nil {
		return "", time.Time{}, false, ErrCoordUnavailable
	}
	by, err = coordRdb.Get(ctx, KeyTaskGate).Result()
	if errors.Is(err, redis.Nil) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, err
	}
	ttl, terr := coordRdb.TTL(ctx, KeyTaskGate).Result()
	if terr != nil || ttl < 0 {
		ttl = 0
	}
	return by, time.Now().Add(ttl), true, nil
}

// TaskGateBlockReason 维护窗口开着时返回可读原因（空串 = 未开）。
// 所有"触发新任务"的入口（调度器 tick、手动立即执行、单源启动采集）都应问它一句 ——
// 文案单点在这里，避免各入口各写一套、口径漂移。
func TaskGateBlockReason(ctx context.Context) string {
	by, until, ok, err := TaskGateInfo(ctx)
	if err != nil || !ok {
		return ""
	}
	return "维护窗口中（" + by + " 打开，" + until.Local().Format("2006-01-02 15:04:05") +
		" 自动关闭）：暂不触发新任务，如需执行请先 resume"
}

// TaskGateClose 关闭闸门（不动白名单 —— 白名单要等 resume 逐个恢复完再清）。
func TaskGateClose(ctx context.Context) error {
	if coordRdb == nil {
		return ErrCoordUnavailable
	}
	return coordRdb.Del(ctx, KeyTaskGate).Err()
}

// TaskGateMarkJob 记一个"因闸门而暂停"的采集 job（resume 只恢复这些）。
func TaskGateMarkJob(ctx context.Context, sourceId string) {
	if coordRdb == nil || sourceId == "" {
		return
	}
	coordRdb.SAdd(ctx, KeyTaskGateJobs, sourceId)
	coordRdb.Expire(ctx, KeyTaskGateJobs, gateJobsKeep)
}

// TaskGateJobs 列出"因闸门而暂停"的采集 job（已排序，便于响应稳定可断言）。
func TaskGateJobs(ctx context.Context) []string {
	if coordRdb == nil {
		return []string{}
	}
	ids, err := coordRdb.SMembers(ctx, KeyTaskGateJobs).Result()
	if err != nil || len(ids) == 0 {
		return []string{}
	}
	sort.Strings(ids)
	return ids
}

// TaskGateJobsClear 清空白名单（恢复流程结束、resume 成功收尾后调用）。
func TaskGateJobsClear(ctx context.Context) {
	if coordRdb == nil {
		return
	}
	coordRdb.Del(ctx, KeyTaskGateJobs)
}

// JobMarkInterrupted 服务启动时把**遗留的非终态采集任务**标记为中断（running/paused → error）。
//
// 为什么必须有：采集轮次是进程内 goroutine，进程一停它就没了，但 Redis 里的 job 进度 hash 还在。
// 不扫的话会留下"幽灵运行中/幽灵暂停中"——
//
//	· 页面永远显示某个源在跑（进度条卡住），管理员无法判断真假；
//	· 维护窗口 resume 时把 paused 翻回 running，却没人在跑 ⇒ 幽灵更顽固（直到 24h TTL）；
//	· 任务中心的"是否在跑"判断被带偏（停用时的 deferred 提示会错）。
//
// 标记成 error 是**如实**的：这一轮确实被中断了，下一轮会按窗口重新采（增量窗口滚动自愈）。
func JobMarkInterrupted(ctx context.Context) (int, error) {
	if coordRdb == nil {
		return 0, nil
	}
	list, err := JobList(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range list {
		if p.State != JobRunning && p.State != JobPaused {
			continue // 只收拾非终态；done/error/canceled 不动
		}
		JobFail(ctx, p.SourceId, "服务重启中断，可重新触发采集")
		n++
	}
	return n, nil
}
