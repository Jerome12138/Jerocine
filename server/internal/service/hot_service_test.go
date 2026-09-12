package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"server/internal/domain/repository"
	"server/internal/douban"
)

// ---- 假仓储: 只实现 hotRepo 的 4 个方法 ----

type fakeHotRepo struct {
	byDbId  map[int64][]repository.HotCandidate // db_id → 本地候选(模拟重复 db_id 的分组)
	byKw    map[string][]repository.HotCandidate
	board   []repository.HotCandidate // 当前在榜快照
	applied []repository.HotRow
	err     error
}

func (f *fakeHotRepo) HotCandidatesByDbIds(_ context.Context, dbIds []int64) ([]repository.HotCandidate, error) {
	var out []repository.HotCandidate
	for _, id := range dbIds {
		out = append(out, f.byDbId[id]...)
	}
	return out, nil
}

func (f *fakeHotRepo) HotCandidatesByKeyword(_ context.Context, keyword string, _ int) ([]repository.HotCandidate, error) {
	return f.byKw[keyword], nil
}

func (f *fakeHotRepo) ListHotBoard(context.Context) ([]repository.HotCandidate, error) {
	return f.board, f.err
}

func (f *fakeHotRepo) ApplyHot(_ context.Context, rows []repository.HotRow) (int, error) {
	f.applied = append(f.applied, rows...)
	return len(rows), nil
}

// appliedMid 取写入行的 mid 集合。
func (f *fakeHotRepo) appliedByMid() map[int64]repository.HotRow {
	m := make(map[int64]repository.HotRow, len(f.applied))
	for _, r := range f.applied {
		m[r.Mid] = r
	}
	return m
}

// ---- 假豆瓣: 单集合, 每页 50 条, 共 350 条(> hotMinBoardItems 才过可信性判定) ----

const (
	hotTestItems     = 350
	hotTestSpecialID = 5000 // i=0: 只能靠片名匹配
	hotTestAmbigID   = 5001 // i=1: 同名两行 → 必须拒绝
	hotTestDupeID    = 5002 // i=2: 同一 db_id 两行(正片 + 解说)→ 取正片
	hotTestDbMatchID = 1003 // i=3: db_id 精确命中
)

func hotTestItem(i int) douban.Item {
	switch i {
	case 0:
		return douban.Item{Id: hotTestSpecialID, Title: "漫长的季节", Year: "2023",
			Actors: json.RawMessage(`["范伟","秦昊"]`), Directors: json.RawMessage(`["辛爽"]`),
			Rating: douban.Rating{Value: 9.4, Count: 800000}}
	case 1:
		return douban.Item{Id: hotTestAmbigID, Title: "同名片", Year: "2023",
			Actors: json.RawMessage(`["甲演员"]`)}
	case 2:
		return douban.Item{Id: hotTestDupeID, Title: "苍兰诀", Year: "2022"}
	default:
		return douban.Item{Id: int64(1000 + i), Title: "热片" + strconv.Itoa(i), Year: "2024"}
	}
}

func newHotTestClient(t *testing.T) *douban.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("start"))
		items := make([]douban.Item, 0, 50)
		for i := start; i < start+50 && i < hotTestItems; i++ {
			items = append(items, hotTestItem(i))
		}
		b, _ := json.Marshal(map[string]any{"subject_collection_items": items})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	c := douban.New()
	c.SetBaseURL(srv.URL)
	c.SetInterval(0) // 测试不等限速
	return c
}

// newHotTestRepo 造与假豆瓣对应的本地库: 一条只能靠片名匹配、一条同名两行(歧义)、
// 一条 db_id 重复(正片 + 解说)、一条 db_id 精确命中, 另有一条上一轮的掉榜片。
func newHotTestRepo(now time.Time) *fakeHotRepo {
	return &fakeHotRepo{
		byDbId: map[int64][]repository.HotCandidate{
			hotTestDbMatchID: {
				{Mid: 11, DbId: hotTestDbMatchID, Name: "热片3", Year: 2024, Remarks: "已完结", DbScore: 8.6},
			},
			hotTestDupeID: {
				{Mid: 29748, DbId: hotTestDupeID, Name: "苍兰诀2022", Year: 2022, Remarks: "已完结", Actor: "虞书欣,王鹤棣"},
				{Mid: 61203, DbId: hotTestDupeID, Name: "苍兰诀[电影解说]", Year: 2022, Remarks: "已完结", DbScore: 8.1},
			},
		},
		byKw: map[string][]repository.HotCandidate{
			// db_id 缺失(源站覆盖 59.2%)→ 靠片名 + 年份 + 编导三重校验回填
			"漫长的季节": {
				{Mid: 58589, DbId: 0, Name: "漫长的季节", Year: 2023, Remarks: "已完结",
					Actor: "范伟,秦昊,陈明昊", Director: "辛爽", DbScore: 0},
			},
			// 同名两行 → 歧义, 一律不猜
			"同名片": {
				{Mid: 71, DbId: 0, Name: "同名片", Year: 2023, Actor: "甲演员"},
				{Mid: 72, DbId: 0, Name: "同名片", Year: 2023, Actor: "甲演员"},
			},
		},
		board: []repository.HotCandidate{
			{Mid: 99, Name: "上轮在榜片", Year: 2019, Remarks: "已完结", HotRank: 5, HotRankAt: now.Unix() - 3600},
		},
	}
}

func newHotTestService(t *testing.T, repo *fakeHotRepo) *HotService {
	t.Helper()
	svc := NewHotService(repo, newHotTestClient(t))
	svc.cols = []douban.Collection{{Name: "movie_hot_gaia", Label: "热门电影", Depth: 308}}
	return svc
}

// TestHotService_Refresh_EndToEnd 全链路: 抓榜 → 三重门匹配 → 写榜位/兜底分 → 掉榜回落回填。
func TestHotService_Refresh_EndToEnd(t *testing.T) {
	setupCache(t)
	now := time.Now()
	repo := newHotTestRepo(now)
	svc := newHotTestService(t, repo)

	rep, err := svc.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh err: %v (report=%+v)", err, rep)
	}
	if rep.Fetched != hotTestItems || rep.Pages != 7 || rep.Empty != 0 || rep.Skipped != 0 {
		t.Fatalf("抓取统计不对: %+v", rep)
	}
	// 匹配: 1003(db_id) + 5002(db_id 重复取正片) + 5000(片名兜底) = 3; 5001 歧义必须被拒
	if rep.Matched != 3 {
		t.Fatalf("匹配数不对: %+v", rep)
	}
	if rep.FellOff != 1 || rep.Applied != 4 {
		t.Fatalf("掉榜/写入数不对: %+v", rep)
	}
	if rep.DbIdFilled != 1 || rep.ScoreFilled != 1 {
		t.Fatalf("回填数不对: %+v", rep)
	}

	got := repo.appliedByMid()
	if len(got) != 4 {
		t.Fatalf("写入行数=%d, want 4: %+v", len(got), got)
	}
	// 歧义条目绝不能出现在结果里
	for _, mid := range []int64{71, 72} {
		if _, ok := got[mid]; ok {
			t.Fatalf("同名歧义(2 行)不该被匹配: %+v", got[mid])
		}
	}

	// db_id 精确命中: 位次 4(第 4 条) → 100000 - 3×100 = 99700
	// + tiebreak(2024 新片 18/3=6 + 豆瓣 8.6→9×4=36) = 99742
	if r := got[11]; r.HotRank != 4 || r.HotScore != 99742 || r.HotBoard != "movie_hot_gaia" {
		t.Fatalf("db_id 命中行不对: %+v", r)
	}
	// 重复 db_id: 取正片(29748), 不是解说行(61203)
	if r := got[29748]; r.HotRank != 3 {
		t.Fatalf("重复 db_id 应取正片: %+v", r)
	}
	if _, ok := got[61203]; ok {
		t.Fatalf("解说行不该上榜: %+v", got[61203])
	}
	// 片名兜底: 回填 db_id(源站缺失) + 回填豆瓣分 9.4; 位次 1 → 100000 + tiebreak(2023→17/3=5 + 9×4=36)
	fill := got[58589]
	if fill.DbId != hotTestSpecialID || fill.DbScore != 9.4 {
		t.Fatalf("片名兜底行未回填 db_id/db_score: %+v", fill)
	}
	if fill.HotRank != 1 || fill.HotScore != 100041 {
		t.Fatalf("片名兜底行分数不对: %+v", fill)
	}
	// 掉榜片: 榜位/榜单来源归零 + 分数回落兜底(2019 老片、已完结、无评分 → 年份新鲜度 20-7=13 → 130)
	if r := got[99]; r.HotRank != 0 || r.HotRankAt != 0 || r.HotBoard != "" || r.HotScore != 130 {
		t.Fatalf("掉榜片应回落兜底分并清空榜单来源: %+v", r)
	}
}

// TestBetterBoard 多榜命中同一部片时的代表榜选择:
// 位次小者优先; 同位次取更深的榜(大榜 No.2 比 10 条小榜 No.2 含金量高);
// 再同按 Collections 清单顺序(稳定可复现, 不依赖 map 迭代序)。
func TestBetterBoard(t *testing.T) {
	gaia := douban.Ranked{Collection: "movie_hot_gaia", Label: "热门电影", Rank: 5, Depth: 308}
	weekly := douban.Ranked{Collection: "movie_weekly_best", Label: "一周口碑榜", Rank: 2, Depth: 10}
	showing := douban.Ranked{Collection: "movie_showing", Label: "正在上映", Rank: 2, Depth: 50}

	if got := betterBoard(gaia, weekly); got.Collection != "movie_weekly_best" {
		t.Fatalf("位次小者优先: %+v", got)
	}
	// 同为 No.2: 深度 50 的"正在上映"胜过深度 10 的"一周口碑榜"
	if got := betterBoard(weekly, showing); got.Collection != "movie_showing" {
		t.Fatalf("同位次取更深榜: %+v", got)
	}
	// 同 rank 同 depth(不同集合): 按 Collections 清单顺序
	a := douban.Ranked{Collection: "movie_weekly_best", Rank: 2, Depth: 10}
	b := douban.Ranked{Collection: "tv_global_best_weekly", Rank: 2, Depth: 10}
	if got := betterBoard(b, a); got.Collection != "movie_weekly_best" {
		t.Fatalf("同位次同深度按清单顺序: %+v", got)
	}
}

// TestHotService_Refresh_RefusesPartialBoard 抓取不可信时**整轮不落库** ——
// 全量替换模型下, 半份榜单会被误读成"这些片掉榜了"。
func TestHotService_Refresh_RefusesPartialBoard(t *testing.T) {
	setupCache(t)
	// 只有一个集合、首页空返回(限流特征)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"subject_collection_items":[]}`))
	}))
	defer srv.Close()
	client := douban.New()
	client.SetBaseURL(srv.URL)
	client.SetInterval(0)

	now := time.Now()
	repo := newHotTestRepo(now)
	svc := NewHotService(repo, client)
	svc.cols = []douban.Collection{{Name: "movie_hot_gaia", Label: "热门电影", Depth: 308}}

	rep, err := svc.Refresh(context.Background())
	if err == nil {
		t.Fatalf("空榜必须报错, got %+v", rep)
	}
	if rep.Empty != 1 {
		t.Fatalf("应记 Empty=1: %+v", rep)
	}
	// 快照还新鲜(1 小时前) → 一行都不许写
	if len(repo.applied) != 0 {
		t.Fatalf("不可信的一轮不该落库: %+v", repo.applied)
	}
}

// TestHotService_Refresh_RefusesShrunkenBoard 抓取"结构合法但语义漂移"(对端换榜 / 库里 db_id 失效)时,
// 匹配数会塌向个位数 —— 全量替换下这等于把整个榜清空。新榜比上轮缩水一半以上必须整轮不落库。
func TestHotService_Refresh_RefusesShrunkenBoard(t *testing.T) {
	setupCache(t)
	now := time.Now()
	repo := newHotTestRepo(now)
	// 上一轮有成规模的榜(100 部), 本轮只匹配到 3 部 → 判异常
	board := make([]repository.HotCandidate, 0, 100)
	for i := 0; i < 100; i++ {
		board = append(board, repository.HotCandidate{
			Mid: int64(200 + i), Name: "上轮在榜片", Year: 2024, Remarks: "已完结",
			HotRank: i + 1, HotRankAt: now.Unix() - 3600,
		})
	}
	repo.board = board
	svc := newHotTestService(t, repo)

	rep, err := svc.Refresh(context.Background())
	if err == nil {
		t.Fatalf("榜缩水 97%% 必须报错, got %+v", rep)
	}
	if rep.Matched != 3 {
		t.Fatalf("应记 matched=3: %+v", rep)
	}
	// 快照还新鲜(1 小时前) → 一部都不许动(既不清榜, 也不回落)
	if len(repo.applied) != 0 {
		t.Fatalf("异常萎缩的一轮不该落库: %+v", repo.applied)
	}
}

// TestHotService_Refresh_ClearsStaleBoardOnceExpired 快照超 72h 未刷新才主动回落兜底分。
func TestHotService_Refresh_ClearsStaleBoardOnceExpired(t *testing.T) {
	setupCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 对端整体不可用
	}))
	defer srv.Close()
	client := douban.New()
	client.SetBaseURL(srv.URL)
	client.SetInterval(0)

	now := time.Now()
	repo := newHotTestRepo(now)
	repo.board = []repository.HotCandidate{
		{Mid: 99, Name: "四天前的在榜片", Year: 2024, Remarks: "已完结", HotRank: 5, HotRankAt: now.Add(-4 * 24 * time.Hour).Unix()},
	}
	svc := NewHotService(repo, client)
	svc.cols = []douban.Collection{{Name: "movie_hot_gaia", Label: "热门电影", Depth: 308}}

	rep, err := svc.Refresh(context.Background())
	if err == nil {
		t.Fatal("抓取失败必须报错")
	}
	if rep.Cleared != 1 {
		t.Fatalf("超期快照应被回落: %+v", rep)
	}
	// 回落的是**兜底分**(2024 新片 + 无评分 = 18 分 → 存储单位 180), 不是简单清 0
	got := repo.appliedByMid()
	r, ok := got[99]
	if !ok || r.HotRank != 0 || r.HotRankAt != 0 || r.HotScore != 180 {
		t.Fatalf("超期在榜片应回落到兜底分: %+v", r)
	}
}

func TestPickNameMatch(t *testing.T) {
	item := douban.Item{Id: 1, Title: "漫长的季节", Year: "2023",
		Actors: json.RawMessage(`["范伟"]`), Directors: json.RawMessage(`["辛爽"]`)}
	ok := repository.HotCandidate{Mid: 1, Name: "漫长的季节", Year: 2023, Actor: "范伟,秦昊", Director: "辛爽"}

	cases := []struct {
		name string
		item douban.Item
		list []repository.HotCandidate
		want bool
	}{
		{"唯一命中 + 年份一致 + 主演交集", item, []repository.HotCandidate{ok}, true},
		{"年份差 1 可接受", item, []repository.HotCandidate{{Mid: 1, Name: "漫长的季节", Year: 2022, Actor: "范伟"}}, true},
		{"年份差 2 拒绝", item, []repository.HotCandidate{{Mid: 1, Name: "漫长的季节", Year: 2021, Actor: "范伟"}}, false},
		{"同名两行(歧义)拒绝", item, []repository.HotCandidate{ok, {Mid: 2, Name: "漫长的季节", Year: 2023, Actor: "范伟"}}, false},
		{"季后缀归一化后同名 → 命中", item, []repository.HotCandidate{{Mid: 1, Name: "漫长的季节 第一季", Year: 2023, Actor: "范伟"}}, true},
		{"片名归一化后仍不同 → 拒绝", item, []repository.HotCandidate{{Mid: 1, Name: "漫长四季", Year: 2023, Actor: "范伟"}}, false},
		{"编导无交集拒绝(同名不同片)", item, []repository.HotCandidate{{Mid: 1, Name: "漫长的季节", Year: 2023, Actor: "张三"}}, false},
		{"本地无编导信息拒绝", item, []repository.HotCandidate{{Mid: 1, Name: "漫长的季节", Year: 2023}}, false},
		{"豆瓣无人员信息拒绝", douban.Item{Title: "漫长的季节", Year: "2023"}, []repository.HotCandidate{ok}, false},
		{"年份未知拒绝", douban.Item{Title: "漫长的季节", Actors: json.RawMessage(`["范伟"]`)},
			[]repository.HotCandidate{ok}, false},
		{"空候选拒绝", item, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, hit := pickNameMatch(c.item, c.list)
			if hit != c.want {
				t.Fatalf("hit=%v, want %v (got %+v)", hit, c.want, got)
			}
		})
	}
}

// TestHotService_MatchSkipsCommentaryOnlyDbIdGroup 线上实测(2026-09-12 首轮刷新):
// 源站把正片的 db_id 填到解说行上、正片行缺 db_id → "全解说组"经 db_id 精确命中,
// 「杀死比尔：血色全传[电影解说]」直接站上热度榜首。全解说组必须降级走片名兜底:
// 库里有正片就挂正片(顺带回填 db_id), 没有就丢弃; 混合组(正片+解说)行为不变取正片。
func TestHotService_MatchSkipsCommentaryOnlyDbIdGroup(t *testing.T) {
	repo := &fakeHotRepo{
		byDbId: map[int64][]repository.HotCandidate{
			5002: { // 混合组: 取正片
				{Mid: 29748, DbId: 5002, Name: "苍兰诀", Year: 2022},
				{Mid: 61203, DbId: 5002, Name: "苍兰诀[电影解说]", Year: 2022},
			},
			6001: { // 全解说组: 降级片名兜底
				{Mid: 61204, DbId: 6001, Name: "杀死比尔[电影解说]", Year: 2026, Remarks: "已完结"},
			},
		},
		byKw: map[string][]repository.HotCandidate{
			"杀死比尔": {{Mid: 30001, DbId: 0, Name: "杀死比尔", Year: 2026, Actor: "甲", Director: "乙"}},
		},
	}
	svc := NewHotService(repo, nil) // match 不依赖 client

	ranked := []douban.Ranked{
		{Item: douban.Item{Id: 5002, Title: "苍兰诀", Year: "2022"}, Rank: 1, Depth: 10},
		{Item: douban.Item{Id: 6001, Title: "杀死比尔", Year: "2026",
			Actors: json.RawMessage(`["甲"]`), Directors: json.RawMessage(`["乙"]`)}, Rank: 2, Depth: 10},
	}
	matches, err := svc.match(context.Background(), ranked)
	if err != nil {
		t.Fatalf("match err: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("匹配数=%d, want 2: %+v", len(matches), matches)
	}
	byItem := make(map[int64]int64, len(matches)) // item.id → 命中的 mid
	for _, m := range matches {
		byItem[m.Item.Id] = m.Mid
	}
	if byItem[5002] != 29748 {
		t.Fatalf("混合组应取正片: %+v", byItem)
	}
	if byItem[6001] != 30001 {
		t.Fatalf("全解说组应降级挂正片 30001(而非解说行 61204): %+v", byItem)
	}
}

// TestPickByDbId 重复 db_id(库里 16,755 组)取"本体": 片名与豆瓣标题一致 > 非解说 > mid 最小。
func TestPickByDbId(t *testing.T) {
	// 实测样本: 「苍兰诀」正片(29748) db_score=0, 解说行(61203) 8.1 —— 必须挂正片
	group := []repository.HotCandidate{
		{Mid: 29748, Name: "苍兰诀2022"},
		{Mid: 61203, Name: "苍兰诀[电影解说]"},
	}
	got, ok := pickByDbId(douban.Item{Title: "苍兰诀"}, group)
	if !ok || got.Mid != 29748 {
		t.Fatalf("应取正片 29748: %+v", got)
	}
	// 片名与豆瓣标题完全一致者优先
	group2 := []repository.HotCandidate{
		{Mid: 1, Name: "别名甲"},
		{Mid: 2, Name: "漫长的季节"},
	}
	if got, _ := pickByDbId(douban.Item{Title: "漫长的季节"}, group2); got.Mid != 2 {
		t.Fatalf("应取片名一致者 2: %+v", got)
	}
	// 全是解说 → 至少取一个(比丢掉整条好), 取 mid 最小
	group3 := []repository.HotCandidate{
		{Mid: 9, Name: "甲[电影解说]"},
		{Mid: 8, Name: "甲[电影解说]"},
	}
	if got, _ := pickByDbId(douban.Item{Title: "甲"}, group3); got.Mid != 9 {
		t.Fatalf("同档应取首个(mid 升序): %+v", got)
	}
	if _, ok := pickByDbId(douban.Item{Title: "甲"}, nil); ok {
		t.Fatal("空分组不该命中")
	}
}

func TestHotKeywordTruncates(t *testing.T) {
	if got := hotKeyword("  短标题  "); got != "短标题" {
		t.Fatalf("hotKeyword=%q", got)
	}
	long := "一二三四五六七八九十十一十二十三"
	if got := hotKeyword(long); len([]rune(got)) != hotTitleMaxRunes {
		t.Fatalf("超长标题应截断到 %d 字: %q", hotTitleMaxRunes, got)
	}
}

// TestHotService_Due 每日刷新时刻判定: 04:00 前不跑, 退避期内不跑。
func TestHotService_Due(t *testing.T) {
	svc := NewHotService(nil, nil)
	day := time.Date(2026, 9, 12, 0, 0, 0, 0, time.Local)
	if svc.due(day.Add(3 * time.Hour)) {
		t.Fatal("04:00 前不该跑")
	}
	if !svc.due(day.Add(4 * time.Hour)) {
		t.Fatal("04:00 之后该跑")
	}
	svc.backoff(day.Add(4 * time.Hour))
	if svc.due(day.Add(4*time.Hour + 30*time.Minute)) {
		t.Fatal("退避期内不该跑")
	}
	if !svc.due(day.Add(5*time.Hour + 1*time.Minute)) {
		t.Fatal("退避到期后该跑")
	}
}
