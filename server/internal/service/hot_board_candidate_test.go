package service

import (
	"testing"
	"time"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// boardPidOf 8 个榜权重序对应的一级分类(电影 1 / 剧集 2 / 综艺 3 / 动漫 4)。
var boardPidOf = map[string]int64{
	"movie_hot_gaia": 1, "movie_showing": 1, "movie_weekly_best": 1,
	"tv_hot": 2, "tv_chinese_best_weekly": 2, "tv_global_best_weekly": 2,
	"tv_variety_show": 3, "tv_animation": 4,
}

// cbMember 构造一条热榜成员(影片字段平铺)。
func cbMember(board string, rank int, mid int64, pid int64, area string, year int, score float64, remarks string) repository.HotBoardMember {
	return repository.HotBoardMember{
		MovieSearch: entity.MovieSearch{Mid: mid, Pid: pid, Area: area, Year: year, DbScore: score, Remarks: remarks},
		SBoard:      board,
		SBRank:      rank,
	}
}

// fullBoardMembers 8 榜各 4 名、mid 全局唯一(boardIdx*100+rank), 默认地区美国(避开大陆保底)。
func fullBoardMembers() []repository.HotBoardMember {
	out := make([]repository.HotBoardMember, 0, 32)
	for i, board := range candidateWeightOrder {
		for r := 1; r <= 4; r++ {
			out = append(out, cbMember(board, r, int64(i*100+r), boardPidOf[board], "美国", 2026, 8.0, ""))
		}
	}
	return out
}

// findCandidate 在结果中按 mid 定位候选。
func findCandidate(t *testing.T, cands []repository.HotBoardCandidate, mid int64) repository.HotBoardCandidate {
	t.Helper()
	for _, c := range cands {
		if c.Mid == mid {
			return c
		}
	}
	t.Fatalf("候选池中未找到 mid=%d", mid)
	return repository.HotBoardCandidate{}
}

// TestBuildCandidatesFullPools 8 榜齐备: 恒 16 位、第 1 轮各榜 No.1、第 2 轮各榜 No.2、weight_pos 连续。
func TestBuildCandidatesFullPools(t *testing.T) {
	cands := BuildBoardCandidates(fullBoardMembers())
	if len(cands) != candidatePoolSize {
		t.Fatalf("候选池应恒为 %d 位, got %d", candidatePoolSize, len(cands))
	}
	for i, c := range cands {
		if c.WeightPos != i {
			t.Fatalf("weight_pos 应连续 0..15, 位置 %d 实际 %d", i, c.WeightPos)
		}
		wantBoard := candidateWeightOrder[i%len(candidateWeightOrder)]
		if c.Board != wantBoard {
			t.Fatalf("位置 %d 应来自 %s, got %s", i, wantBoard, c.Board)
		}
		wantRank := i/len(candidateWeightOrder) + 1 // 第 1 轮 rank1, 第 2 轮 rank2
		if c.Rank != wantRank {
			t.Fatalf("位置 %d(%s) 应取 rank%d, got rank%d", i, c.Board, wantRank, c.Rank)
		}
	}
}

// TestBuildCandidatesBackfillDedup 同片占据两个榜的 No.1 时, 第二个榜从 rank3/4 顺延回填,
// 且回填位整段压进垫底带(分数严格低于最低 No.2 理论值 -165), 不反超任何 No.1/No.2。
func TestBuildCandidatesBackfillDedup(t *testing.T) {
	members := fullBoardMembers()
	// 让 tv_hot(weightPos1) 的 No.1 与 gaia(weightPos0) 的 No.1 是同一部片 mid=1。
	for i := range members {
		if members[i].SBoard == "tv_hot" && members[i].SBRank == 1 {
			members[i].Mid = 1
			members[i].Pid = 1
		}
	}
	cands := BuildBoardCandidates(members)
	if len(cands) != candidatePoolSize {
		t.Fatalf("顺延回填后池仍应 %d 位, got %d", candidatePoolSize, len(cands))
	}
	// weightPos1(tv_hot 槽) 的 No.1 被占 → 顺延到 tv_hot rank3 (mid=103)。
	slot := cands[1]
	if slot.Board != "tv_hot" || slot.Rank != 3 {
		t.Fatalf("tv_hot 槽应顺延到 rank3, got %s rank%d", slot.Board, slot.Rank)
	}
	// 垫底带: 任何 rank≥3 回填位 < -165。
	if slot.Score >= -165 {
		t.Fatalf("回填位分数 %d 应严格低于 -165(垫底带), 不得反超 No.1/No.2", slot.Score)
	}
	// 同片 mid=1 在池中只出现一次(去重)。
	hits := 0
	for _, c := range cands {
		if c.Mid == 1 {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("同片多榜占位应去重, mid=1 出现 %d 次", hits)
	}
}

// TestCandidateScoreTCDrop 枪版: 候选分 −200, 多榜命中每多一榜回血 60(封顶 +120)。
func TestCandidateScoreTCDrop(t *testing.T) {
	now := time.Now()
	base := repository.HotBoardMember{
		MovieSearch: entity.MovieSearch{Mid: 1, Pid: 1, Area: "美国", Year: 2026, DbScore: 8.0},
		SBoard:      "movie_hot_gaia", SBRank: 1,
	}
	tc := base
	tc.Remarks = "HDTC"

	normal := candidateScore(base, 0, 1, now)
	// 单榜命中: 净 −200。
	if got := candidateScore(tc, 0, 1, now); got != normal-domain.HotBoardTCCandidateDrop {
		t.Fatalf("TC 单榜应 −200: normal=%d got=%d", normal, got)
	}
	// 2 榜: −200+60 = −140。
	if got := candidateScore(tc, 0, 2, now); got != normal-domain.HotBoardTCCandidateDrop+60 {
		t.Fatalf("TC 两榜应 −140: normal=%d got=%d", normal, got)
	}
	// 3 榜: −200+120 = −80。
	if got := candidateScore(tc, 0, 3, now); got != normal-domain.HotBoardTCCandidateDrop+120 {
		t.Fatalf("TC 三榜应 −80(回血封顶): normal=%d got=%d", normal, got)
	}
	// 4 榜以上回血不突破封顶。
	if got := candidateScore(tc, 0, 4, now); got != normal-domain.HotBoardTCCandidateDrop+120 {
		t.Fatalf("TC 四榜回血应封顶 +120: normal=%d got=%d", normal, got)
	}
}

// TestCandidateScoreBackfillBand 所有 rank≥3 回填位无论权重位/口碑/地区, 都落在垫底带。
func TestCandidateScoreBackfillBand(t *testing.T) {
	now := time.Now()
	best := repository.HotBoardMember{
		MovieSearch: entity.MovieSearch{Mid: 1, Pid: 1, Area: "中国大陆", Year: 2026, DbScore: 9.9},
		SBoard:      "movie_hot_gaia", SBRank: 3, // 即使是大陆 + 高口碑的回填位
	}
	worstNo2 := repository.HotBoardMember{
		MovieSearch: entity.MovieSearch{Mid: 2, Pid: 4, Area: "日本", Year: 2010, DbScore: 0},
		SBoard:      "tv_animation", SBRank: 2, // 即使是负地区 + 老片的 No.2
	}
	backfill := candidateScore(best, 0, 1, now)
	no2 := candidateScore(worstNo2, 7, 1, now)
	if backfill >= no2 {
		t.Fatalf("回填位 %d 必须低于最弱 No.2 %d, 不得反超", backfill, no2)
	}
}

// TestGuaranteeReplacesLowestPidSlot 池中无大陆片但深挖榜单有大陆片: 替换同 pid 最低分槽, 池恒 16。
func TestGuaranteeReplacesLowestPidSlot(t *testing.T) {
	members := fullBoardMembers()
	// gaia rank5 为大陆片(不进前 2/回填, 仅保底深挖可见)。
	mainland := cbMember("movie_hot_gaia", 5, 9001, 1, "中国大陆", 2026, 8.0, "")
	members = append(members, mainland)
	cands := BuildBoardCandidates(members)

	if len(cands) != candidatePoolSize {
		t.Fatalf("保底替换后池仍应守恒为 %d, got %d", candidatePoolSize, len(cands))
	}
	var guar *repository.HotBoardCandidate
	for i := range cands {
		if cands[i].Guaranteed {
			guar = &cands[i]
		}
	}
	if guar == nil {
		t.Fatalf("应出现一个大陆保底替换位")
	}
	if guar.Mid != 9001 || guar.Board != "movie_hot_gaia" || guar.Rank != 5 {
		t.Fatalf("保底位应是深挖到的 gaia rank5 大陆片, got %+v", guar)
	}
}

// TestGuaranteeNoopWhenMainlandPresent 池中已有大陆片时不替换。
func TestGuaranteeNoopWhenMainlandPresent(t *testing.T) {
	members := fullBoardMembers()
	// gaia No.1 直接就是大陆片。
	for i := range members {
		if members[i].SBoard == "movie_hot_gaia" && members[i].SBRank == 1 {
			members[i].Area = "中国大陆"
		}
	}
	cands := BuildBoardCandidates(members)
	for _, c := range cands {
		if c.Guaranteed {
			t.Fatalf("已有大陆片时不应产生保底替换位: %+v", c)
		}
	}
}

// TestGuaranteeNoopWhenNoMainlandSource 池中无大陆片、深挖榜单也无大陆片: 不替换、不报错。
func TestGuaranteeNoopWhenNoMainlandSource(t *testing.T) {
	cands := BuildBoardCandidates(fullBoardMembers())
	if len(cands) != candidatePoolSize {
		t.Fatalf("池应 %d 位, got %d", candidatePoolSize, len(cands))
	}
	for _, c := range cands {
		if c.Guaranteed {
			t.Fatalf("无大陆片来源时不应保底替换: %+v", c)
		}
	}
}

// TestGuaranteeNoopWhenNoPidSlot 某分类在池中无槽位时, 保底不新增、不越界、不 panic。
func TestGuaranteeNoopWhenNoPidSlot(t *testing.T) {
	// 电影系(pid1)三榜基本留空: 只给 gaia rank5 一部大陆片(深挖能找到), 但池里没有 pid1 槽。
	members := []repository.HotBoardMember{
		cbMember("movie_hot_gaia", 5, 9002, 1, "中国大陆", 2026, 8.0, ""),
	}
	for i, board := range candidateWeightOrder {
		if boardPidOf[board] == 1 {
			continue // 电影系榜留空(gaia 仅有上面 rank5)
		}
		for r := 1; r <= 4; r++ {
			members = append(members, cbMember(board, r, int64(i*100+r), boardPidOf[board], "美国", 2026, 8.0, ""))
		}
	}
	cands := BuildBoardCandidates(members)
	// 不应把大陆片 9002 凭空塞进池(名额守恒)。
	for _, c := range cands {
		if c.Mid == 9002 {
			t.Fatalf("池无 pid1 槽时不应保底塞入大陆片")
		}
		if c.Guaranteed {
			t.Fatalf("无同 pid 槽时不应产生保底位: %+v", c)
		}
	}
}

// TestBuildCandidatesEmpty 无成员时返回空切片(不 panic)。
func TestBuildCandidatesEmpty(t *testing.T) {
	if cands := BuildBoardCandidates(nil); len(cands) != 0 {
		t.Fatalf("空成员应派生空候选, got %d", len(cands))
	}
}
