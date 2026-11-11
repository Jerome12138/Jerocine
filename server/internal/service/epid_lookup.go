package service

import "context"

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

// LookupEpids 按 `match_key` 批量取 epid（best-effort：出错只当"没有"）。
// 返回 `map[Id]map[epIndex]epid`，`Id = site_id + ":" + play_from`（与 `PlaySourceView.Id` 同口径）。
func LookupEpids(ctx context.Context, matchKeys []string) map[string]map[int]string {
	if epidLookupFn == nil || len(matchKeys) == 0 {
		return nil
	}
	m, err := epidLookupFn(ctx, matchKeys)
	if err != nil {
		return nil
	}
	return m
}
