package cache

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// 采集协调(db1): 采集 job 进度(Hash) + per-site 暂存(ZSet)。
// 替代旧代码进程内 JobRegistry 单例 + 全局单 key SearchInfoTemp, 支持多副本/重启续传。

// 采集任务状态
const (
	JobRunning  = "running"
	JobPaused   = "paused"
	JobCanceled = "canceled"
	JobDone     = "done"
	JobError    = "error"
)

const jobTTL = 24 * time.Hour // job hash 兜底过期, 避免残留

// JobProgress 采集进度快照。
type JobProgress struct {
	SourceId string `json:"sourceId"`
	Name     string `json:"name"`
	Total    int    `json:"total"`
	Done     int    `json:"done"`
	Failed   int    `json:"failed"`
	State    string `json:"state"`
}

// JobStart 初始化一个采集任务进度。
func JobStart(ctx context.Context, sourceId, name string, total int) error {
	if coordRdb == nil {
		return nil
	}
	key := KeyJob(sourceId)
	if err := coordRdb.HSet(ctx, key, map[string]any{
		"name": name, "total": total, "done": 0, "failed": 0, "state": JobRunning,
	}).Err(); err != nil {
		return err
	}
	return coordRdb.Expire(ctx, key, jobTTL).Err()
}

func JobSetTotal(ctx context.Context, sourceId string, total int) {
	if coordRdb != nil {
		coordRdb.HSet(ctx, KeyJob(sourceId), "total", total)
	}
}

func JobIncrDone(ctx context.Context, sourceId string, n int) {
	if coordRdb != nil {
		coordRdb.HIncrBy(ctx, KeyJob(sourceId), "done", int64(n))
	}
}

func JobIncrFailed(ctx context.Context, sourceId string, n int) {
	if coordRdb != nil {
		coordRdb.HIncrBy(ctx, KeyJob(sourceId), "failed", int64(n))
	}
}

// isTerminalState 任务是否已结束(可被 GC)。
func isTerminalState(state string) bool {
	return state == JobDone || state == JobError || state == JobCanceled
}

func JobSetState(ctx context.Context, sourceId, state string) {
	if coordRdb == nil {
		return
	}
	key := KeyJob(sourceId)
	coordRdb.HSet(ctx, key, "state", state)
	// 结束态打 endedAt 时间戳, 供 JobList 按 30min 窗口 GC; 运行中不记, 永不按时间丢弃
	if isTerminalState(state) {
		coordRdb.HSet(ctx, key, "endedAt", time.Now().Unix())
	}
}

// JobReadState 读当前状态(worker 每页前轮询, 实现暂停/取消)。
func JobReadState(ctx context.Context, sourceId string) string {
	if coordRdb == nil {
		return JobRunning
	}
	return coordRdb.HGet(ctx, KeyJob(sourceId), "state").Val()
}

// JobSnapshot 读单个任务进度。
func JobSnapshot(ctx context.Context, sourceId string) (JobProgress, error) {
	p := JobProgress{SourceId: sourceId, State: JobDone}
	if coordRdb == nil {
		return p, nil
	}
	m, err := coordRdb.HGetAll(ctx, KeyJob(sourceId)).Result()
	if err != nil || len(m) == 0 {
		return p, err
	}
	fillJob(&p, m)
	return p, nil
}

// jobKeepEnded 已结束任务在监控里保留的时间窗口, 超过则 GC。
const jobKeepEnded = 30 * time.Minute

// JobList 列出采集任务进度(SCAN v1:job:*)。
// 运行/暂停/排队中的任务永远返回(不按时间丢弃, 修"漏掉 >30min 未结束任务");
// 已结束(done/error/canceled)的仅在 endedAt 30min 窗口内返回, 超过则顺手删除。
func JobList(ctx context.Context) ([]JobProgress, error) {
	if coordRdb == nil {
		return nil, nil
	}
	cutoff := time.Now().Add(-jobKeepEnded).Unix()
	var cursor uint64
	var out []JobProgress
	for {
		keys, next, err := coordRdb.Scan(ctx, cursor, "v1:job:*", 100).Result()
		if err != nil {
			return out, err
		}
		for _, k := range keys {
			m, e := coordRdb.HGetAll(ctx, k).Result()
			if e != nil || len(m) == 0 {
				continue
			}
			p := JobProgress{SourceId: k[len("v1:job:"):]}
			fillJob(&p, m)
			if isTerminalState(p.State) {
				endedAt, _ := strconv.ParseInt(m["endedAt"], 10, 64)
				if endedAt > 0 && endedAt < cutoff {
					coordRdb.Del(ctx, k) // 结束超 30min, GC
					continue
				}
			}
			out = append(out, p)
		}
		if next == 0 {
			return out, nil
		}
		cursor = next
	}
}

func fillJob(p *JobProgress, m map[string]string) {
	p.Name = m["name"]
	p.State = m["state"]
	p.Total = atoi(m["total"])
	p.Done = atoi(m["done"])
	p.Failed = atoi(m["failed"])
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// ---- per-site 暂存 ZSet ----

// ScoredMember ZSet 成员(member=RawFilm JSON, score=mid)。
type ScoredMember struct {
	Score  float64
	Member string
}

// SpiderTempPush 把本批采集到的原始影片暂存到本站 ZSet(score=mid 去重)。
func SpiderTempPush(ctx context.Context, siteId string, items []ScoredMember) error {
	if coordRdb == nil || len(items) == 0 {
		return nil
	}
	zs := make([]redis.Z, 0, len(items))
	for _, it := range items {
		zs = append(zs, redis.Z{Score: it.Score, Member: it.Member})
	}
	if err := coordRdb.ZAdd(ctx, KeySpiderTemp(siteId), zs...).Err(); err != nil {
		return err
	}
	return coordRdb.Expire(ctx, KeySpiderTemp(siteId), 2*time.Hour).Err()
}

// SpiderTempPopMax 批量弹出本站暂存(同步落库用), 返回 member JSON 列表。
func SpiderTempPopMax(ctx context.Context, siteId string, count int64) ([]string, error) {
	if coordRdb == nil {
		return nil, nil
	}
	zs, err := coordRdb.ZPopMax(ctx, KeySpiderTemp(siteId), count).Result()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(zs))
	for _, z := range zs {
		if s, ok := z.Member.(string); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func SpiderTempClear(ctx context.Context, siteId string) {
	if coordRdb != nil {
		coordRdb.Del(ctx, KeySpiderTemp(siteId))
	}
}
