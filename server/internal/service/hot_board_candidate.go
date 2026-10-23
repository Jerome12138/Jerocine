package service

import (
	"sort"
	"strings"
	"time"

	"server/internal/domain"
	"server/internal/domain/repository"
)

// 轮播候选池(见 docs/热榜页面与轮播候选方案-2026-10-23 §2)。
//
// 候选池恒 16 位: 8 个榜各取前 2, 按"权重序"轮转混排; 同片多榜占位时由该榜第 3/4 名顺延回填。
const (
	candidatePoolSize       = 16 // 候选位总数
	candidateTopN           = 2  // 每个榜的起点取前 2
	candidateBackfillMaxRank = 4 // 去重顺延最多补到该榜第 4 名
	candidateGuaranteeRank  = 10 // 大陆保底深挖到 rank 1..10

	// candidateBackfillBase 顺延回填位(rank≥3)的垫底带基准。
	// 必须让回填位严格低于任何 No.1/No.2: 最低 No.2(动漫 wp7、负地区、无年份口碑)理论分 -165,
	// 回填位带上权重(跨度≤7)与微调 1/4(跨度≤21)后整体落在 -203..-170, 恒 < -165, 不反超。
	candidateBackfillBase = -190
)

// candidateWeightOrder 候选轮播的榜单权重序(与 douban.Collections 清单序不同, 方案 §2.1 用户定稿)。
// 电影系主榜压前, 口碑榜居中, 综艺/动漫垫后。
var candidateWeightOrder = []string{
	"movie_hot_gaia",             // 0 热门电影
	"tv_hot",                     // 1 热门剧集
	"movie_showing",              // 2 正在上映
	"movie_weekly_best",          // 3 一周口碑榜
	"tv_chinese_best_weekly",     // 4 华语口碑剧集榜
	"tv_global_best_weekly",      // 5 全球口碑剧集榜
	"tv_variety_show",            // 6 热门综艺
	"tv_animation",               // 7 热门动漫
}

// candidateGuaranteeBoards 大陆保底的"补位候选榜": 按 pid 分组, 按权重序深挖。
var candidateGuaranteeBoards = map[int64][]string{
	1: {"movie_hot_gaia", "movie_showing", "movie_weekly_best"}, // 电影
	2: {"tv_hot", "tv_chinese_best_weekly", "tv_global_best_weekly"}, // 剧集
}

// candidateSlot 池内一个槽位(带派生分)。
type candidateSlot struct {
	repository.HotBoardMember
	weightPos  int
	score      int
	guaranteed bool
}

// BuildBoardCandidates 从热榜快照成员(已 JOIN 影片、剔除软删/隐藏)派生 16 位轮播候选。
// 纯函数, 不访问 DB/时间注入外的副作用; 输出按 weight_pos 升序(即轮转混排顺序)。
func BuildBoardCandidates(members []repository.HotBoardMember) []repository.HotBoardCandidate {
	now := time.Now()

	// 按榜分组、组内 rank 升序; 同时统计每部片命中的榜数(TC 多榜回血用)。
	byBoard := make(map[string][]repository.HotBoardMember)
	boardCount := make(map[int64]int)
	for _, m := range members {
		if m.SBoard == "" || m.SBRank <= 0 || m.Mid <= 0 {
			continue
		}
		byBoard[m.SBoard] = append(byBoard[m.SBoard], m)
		boardCount[m.Mid]++ // 同片在同榜只占一行(快照 uq_board_rank 保证), 直接计榜数
	}
	for b := range byBoard {
		sort.SliceStable(byBoard[b], func(i, j int) bool {
			return byBoard[b][i].SBRank < byBoard[b][j].SBRank
		})
	}
	wpOf := func(board string) int {
		for i, b := range candidateWeightOrder {
			if b == board {
				return i
			}
		}
		return len(candidateWeightOrder)
	}

	// 轮转: 第 1 轮各榜 No.1, 第 2 轮各榜 No.2; 槽位内重复时由同榜 rank3/4 顺延。
	pool := make([]candidateSlot, 0, candidatePoolSize)
	used := make(map[int64]bool)
	for round := 1; round <= candidateTopN; round++ {
		for _, board := range candidateWeightOrder {
			if len(pool) >= candidatePoolSize {
				break
			}
			group := byBoard[board]
			// 第 1 槽优先 rank1, 第 2 槽优先 rank2; 占用后从 rank3/4 顺延(顺位互不抢占)。
			pref := []int{round, candidateTopN + 1, candidateBackfillMaxRank}
			chosen := pickForSlot(group, pref, used)
			if chosen == nil {
				continue
			}
			used[chosen.Mid] = true
			pool = append(pool, candidateSlot{
				HotBoardMember: *chosen,
				weightPos:      len(pool),
				score: candidateScore(*chosen, wpOf(board), boardCount[chosen.Mid], now),
			})
		}
	}

	pool = applyGuarantee(pool, byBoard, wpOf, boardCount, now)

	out := make([]repository.HotBoardCandidate, 0, len(pool))
	for _, s := range pool {
		out = append(out, repository.HotBoardCandidate{
			WeightPos:  s.weightPos,
			Board:      s.SBoard,
			Rank:       s.SBRank,
			Mid:        s.Mid,
			Score:      s.score,
			Guaranteed: s.guaranteed,
		})
	}
	return out
}

// memberAtRank 取某榜中指定 rank 的成员, 不存在返回 nil。
func memberAtRank(group []repository.HotBoardMember, rank int) *repository.HotBoardMember {
	for i := range group {
		if group[i].SBRank == rank {
			return &group[i]
		}
		if group[i].SBRank > rank {
			return nil
		}
	}
	return nil
}

// pickForSlot 按偏好 rank 列表挑一个未被占用的成员。
func pickForSlot(group []repository.HotBoardMember, pref []int, used map[int64]bool) *repository.HotBoardMember {
	for _, rank := range pref {
		if rank > candidateBackfillMaxRank {
			continue
		}
		m := memberAtRank(group, rank)
		if m != nil && !used[m.Mid] {
			return m
		}
	}
	return nil
}

// candidateScore 候选分(方案 §2.2 公式)。
//
//	score = (9−权重位)×30 − (rank−1)×200 + 口碑×3 + 地区分 + 年份分 [+ 枪版降级/多榜回血]
//
// rank≥3 的顺延回填位整段压进垫底带(见 candidateBackfillBase), 不反超任何 No.1/No.2。
func candidateScore(m repository.HotBoardMember, weightPos, boardCount int, now time.Time) int {
	praise := domain.HotPraiseScore(m.DbScore)
	area := domain.HotAreaScore(m.Area)
	year := domain.HotYearFresh(m.Year, now)

	var s int
	if m.SBRank >= candidateTopN+1 {
		// 回填位: 垫底带, 同档内用权重位与微调(1/4 折算)小幅定序。
		micro := praise*3 + area + year
		s = candidateBackfillBase - (m.SBRank-candidateTopN-1)*200 -
			(len(candidateWeightOrder)-1-weightPos) + micro/4
	} else {
		s = (9-weightPos)*30 - (m.SBRank-1)*200 + praise*3 + area + year
	}

	if domain.IsLowQualityTelesync(m.Remarks) {
		s -= domain.HotBoardTCCandidateDrop
		s += domain.HotBoardTCMultiBoardBonusScore(boardCount)
	}
	return s
}

// applyGuarantee 中国大陆保底: 池中缺该 pid 大陆片时, 从对应榜单深挖大陆片替换池中同 pid 最低分槽。
func applyGuarantee(
	pool []candidateSlot,
	byBoard map[string][]repository.HotBoardMember,
	wpOf func(string) int,
	boardCount map[int64]int,
	now time.Time,
) []candidateSlot {
	for pid, boards := range candidateGuaranteeBoards {
		hasMainland := false
		for _, s := range pool {
			if s.Pid == pid && isMainland(s.Area) {
				hasMainland = true
				break
			}
		}
		if hasMainland {
			continue
		}
		// 池中同 pid 最低分槽位(名额守恒: 只替换同分类槽, 不挤占其他分类名额)。
		idx := -1
		for i := range pool {
			if pool[i].Pid != pid {
				continue
			}
			if idx == -1 || pool[i].score < pool[idx].score {
				idx = i
			}
		}
		if idx == -1 {
			continue // 池里没有该分类槽, 不新增(守恒)
		}
		// 深挖: 按权重序、rank 1..10 找第一部不在池中的大陆片。
		occupied := func(mid int64) bool {
			for _, s := range pool {
				if s.Mid == mid {
					return true
				}
			}
			return false
		}
		var found *repository.HotBoardMember
		var foundBoard string
	scan:
		for _, b := range boards {
			for rank := 1; rank <= candidateGuaranteeRank; rank++ {
				m := memberAtRank(byBoard[b], rank)
				if m != nil && isMainland(m.Area) && !occupied(m.Mid) {
					found, foundBoard = m, b
					break scan
				}
			}
		}
		if found == nil {
			continue
		}
		// 替换项沿用被替换槽的分数(稳定混排顺序), 只更新成员与来源标记并打保底标。
		pool[idx].HotBoardMember = *found
		pool[idx].SBoard = foundBoard
		pool[idx].SBRank = found.SBRank
		pool[idx].score = candidateScore(*found, wpOf(foundBoard), boardCount[found.Mid], now)
		pool[idx].guaranteed = true
	}
	return pool
}

// isMainland 判定地区是否为中国大陆(不含中国香港/中国台湾)。
func isMainland(area string) bool {
	return strings.Contains(area, "大陆")
}
