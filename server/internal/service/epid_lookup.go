package service

import (
	"context"
	"log"
)

// P1-1b（2026-11-10）：出参带 epid 的**查询注入点**。
//
// 取向：`FilmService` 的构造签名被多处测试使用 ⇒ **不加依赖参数**；改为注入一个查询函数（main 里一行装配）。
// 未装配 / 查询失败 / 无 key ⇒ 返回 nil（= 无 epid）⇒ 出参不带 `epid` 字段 ⇒ 端侧回退实时解析
// ⇒ **行为与改动前完全一致**（安全降级，符合"过渡期"口径 §4.6）。
var epidLookupFn func(ctx context.Context, matchKeys []string) (map[string]map[int]string, error)

// SetEpidLookup 装配查询函数（生产传 `repomysql.LookupEpidsByMatchKeys`）。
func SetEpidLookup(fn func(ctx context.Context, matchKeys []string) (map[string]map[int]string, error)) {
	epidLookupFn = fn
}

// lookupEpidsBatched 按**小批**取 epid（H1 粒度修正, 2026-11-11 生产实测）：
//
// 一次把几千个 match_key 交给派生侧那条 `SELECT match_key, episodes_json ... WHERE match_key IN (...)`
// ⇒ 会拉出**上万行 × 1GB 级巨列**（生产上单条 SQL 数分钟不返回 ⇒ 批超时、该轮派生白跑 ✗）。
// 故与"派生任务的借用池"共用同一份限制：**每批 200 个 key**（超时的代价被压到一批以内）。
func lookupEpidsBatched(ctx context.Context, keys []string) map[string]map[int]string {
	if len(keys) == 0 {
		return nil
	}
	const batch = 200
	out := make(map[string]map[int]string, len(keys))
	for i := 0; i < len(keys); i += batch {
		j := min(i+batch, len(keys))
		for k, v := range LookupEpids(ctx, keys[i:j]) {
			out[k] = v
		}
	}
	return out
}

// LookupEpids 按 `match_key` 批量取 epid（best-effort：出错只当"没有"）。
// 返回 `map[Id]map[epIndex]epid`，`Id = site_id + ":" + play_from`（与 `PlaySourceView.Id` 同口径）。
func LookupEpids(ctx context.Context, matchKeys []string) map[string]map[int]string {
	if epidLookupFn == nil || len(matchKeys) == 0 {
		return nil
	}
	m, err := epidLookupFn(ctx, matchKeys)
	if err != nil {
		// R6（2026-11-11 评审）：**不要静默吞错** —— 否则出参没有 epid 时无法区分
		// "未派生 / 未装配 / 查询挂了" ✗（排障无着手点）。这里只记账，不影响详情/播放返回。
		log.Printf("[epid] 出参 epid 查询失败（本响应不带 epid，端侧回退实时解析）: %v", err)
		return nil
	}
	return m
}
